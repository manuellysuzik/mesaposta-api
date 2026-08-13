package httpx

import (
	"net/http"
	"sync"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

const (
	// DefaultCleanupInterval is how often NewRateLimiter's background
	// goroutine sweeps for idle buckets when RateLimitOption
	// WithCleanupInterval is not supplied.
	DefaultCleanupInterval = time.Minute

	// DefaultIdleTTL is how long a bucket may sit unused before the
	// cleanup sweep evicts it, when RateLimitOption WithIdleTTL is not
	// supplied.
	DefaultIdleTTL = 5 * time.Minute
)

// tokenBucket is a single key's token-bucket state.
type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
	lastUsed   time.Time
}

// RateLimitOption configures a *RateLimiter built by NewRateLimiter.
type RateLimitOption func(*RateLimiter)

// WithCleanupInterval overrides how often the idle-bucket cleanup sweep
// runs.
func WithCleanupInterval(d time.Duration) RateLimitOption {
	return func(rl *RateLimiter) {
		if d > 0 {
			rl.cleanupInterval = d
		}
	}
}

// WithIdleTTL overrides how long a bucket may sit unused before the
// cleanup sweep evicts it.
func WithIdleTTL(d time.Duration) RateLimitOption {
	return func(rl *RateLimiter) {
		if d > 0 {
			rl.idleTTL = d
		}
	}
}

// RateLimiter is an in-memory, thread-safe token-bucket rate limiter keyed
// by an arbitrary string (see RateLimit). Every distinct key gets its own
// bucket, refilled continuously at refillPerSecond up to a maximum of
// burst tokens; each allowed request consumes exactly one token.
//
// Because keys are caller-supplied (see RateLimit's keyFunc), and a
// malicious or buggy caller could otherwise drive unbounded memory growth
// by generating an unbounded number of distinct keys, RateLimiter caps the
// total number of buckets it will ever hold at once (maxBuckets, passed to
// NewRateLimiter). Once that cap is reached, a request for a KEY NOT
// ALREADY TRACKED is denied outright rather than creating a new bucket --
// see allow's doc comment for why this fail-closed choice was made over
// the alternative (routing overflow keys through one shared bucket).
//
// The zero value is not usable; construct one via NewRateLimiter, and call
// Close when done to stop its background cleanup goroutine.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket

	refillPerSecond float64
	burst           float64
	maxBuckets      int

	cleanupInterval time.Duration
	idleTTL         time.Duration

	closeOnce sync.Once
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// NewRateLimiter builds a RateLimiter. maxBuckets caps the number of
// distinct keys tracked simultaneously (protecting against unbounded
// memory growth from arbitrary caller-supplied keys); refillPerSecond is
// how many tokens each bucket regains per second; burst is each bucket's
// maximum token capacity (and therefore the largest instantaneous burst of
// requests a single key may make before being throttled). Non-positive
// values for any of the three are replaced with a small, safe default
// rather than producing a limiter that either does nothing or blocks
// everything.
//
// NewRateLimiter starts a background goroutine that periodically evicts
// idle buckets (see WithCleanupInterval/WithIdleTTL); callers MUST call
// Close when the RateLimiter is no longer needed, to stop that goroutine.
func NewRateLimiter(maxBuckets int, refillPerSecond float64, burst int, opts ...RateLimitOption) *RateLimiter {
	if maxBuckets <= 0 {
		maxBuckets = 10_000
	}
	if refillPerSecond <= 0 {
		refillPerSecond = 1
	}
	if burst <= 0 {
		burst = 1
	}

	rl := &RateLimiter{
		buckets:         make(map[string]*tokenBucket),
		refillPerSecond: refillPerSecond,
		burst:           float64(burst),
		maxBuckets:      maxBuckets,
		cleanupInterval: DefaultCleanupInterval,
		idleTTL:         DefaultIdleTTL,
		stopCh:          make(chan struct{}),
		doneCh:          make(chan struct{}),
	}
	for _, opt := range opts {
		opt(rl)
	}

	go rl.cleanupLoop()

	return rl
}

// cleanupLoop runs until Close is called, periodically evicting buckets
// that have sat idle longer than rl.idleTTL.
func (rl *RateLimiter) cleanupLoop() {
	defer close(rl.doneCh)

	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rl.stopCh:
			return
		case now := <-ticker.C:
			rl.evictIdle(now)
		}
	}
}

// evictIdle removes every bucket whose lastUsed timestamp is older than
// rl.idleTTL relative to now.
func (rl *RateLimiter) evictIdle(now time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for key, b := range rl.buckets {
		if now.Sub(b.lastUsed) > rl.idleTTL {
			delete(rl.buckets, key)
		}
	}
}

// Close stops the background cleanup goroutine and blocks until it has
// fully exited (so a test or a graceful-shutdown path can rely on no
// goroutine leaking past Close's return). Safe to call more than once:
// only the first call has any effect, and every call still waits for the
// goroutine to have exited.
func (rl *RateLimiter) Close() {
	rl.closeOnce.Do(func() {
		close(rl.stopCh)
	})
	<-rl.doneCh
}

// allow reports whether a request for key is allowed at time now,
// consuming one token from key's bucket if so.
//
// If key has no existing bucket AND the map is already at maxBuckets
// capacity, allow denies the request WITHOUT creating a bucket -- it does
// not fall back to a shared "overflow" bucket. A shared bucket was
// considered and rejected: it would let a single high-cardinality burst of
// brand-new keys (e.g. an attacker cycling through spoofed identifiers)
// exhaust one shared quota that every other legitimate, equally-new key
// also depends on, effectively defeating the per-key isolation this
// limiter exists to provide. Failing closed for the specific new key that
// would have overflowed the cap is a strictly smaller blast radius.
func (rl *RateLimiter) allow(key string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		if len(rl.buckets) >= rl.maxBuckets {
			return false
		}
		b = &tokenBucket{tokens: rl.burst, lastRefill: now, lastUsed: now}
		rl.buckets[key] = b
	}

	if elapsed := now.Sub(b.lastRefill).Seconds(); elapsed > 0 {
		b.tokens += elapsed * rl.refillPerSecond
		if b.tokens > rl.burst {
			b.tokens = rl.burst
		}
		b.lastRefill = now
	}
	b.lastUsed = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// RateLimit builds a middleware enforcing rl's token-bucket limit, keyed
// per-request by keyFunc -- e.g. client IP, tenant id, or some combination
// of both, entirely at the caller's discretion; this package makes no
// assumption about what a "key" represents. A request whose key's bucket
// has no tokens left is rejected with errs.TooManyRequests (HTTP 429, via
// RespondError) and never reaches next.
func (rl *RateLimiter) RateLimit(keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			if !rl.allow(key, time.Now()) {
				RespondError(w, r, errs.New(errs.TooManyRequests, "rate_limited", "too many requests", nil))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
