package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_Allow_ExhaustsBucketThenDenies(t *testing.T) {
	rl := NewRateLimiter(10, 1, 2)
	defer rl.Close()

	now := time.Now()

	if !rl.allow("key-a", now) {
		t.Fatal("1st request: want allowed")
	}
	if !rl.allow("key-a", now) {
		t.Fatal("2nd request: want allowed (burst = 2)")
	}
	if rl.allow("key-a", now) {
		t.Fatal("3rd request (same instant): want denied, bucket should be empty")
	}
}

func TestRateLimiter_Allow_RefillsAfterInterval(t *testing.T) {
	rl := NewRateLimiter(10, 1 /* token/sec */, 1 /* burst */)
	defer rl.Close()

	now := time.Now()
	if !rl.allow("key-a", now) {
		t.Fatal("1st request: want allowed")
	}
	if rl.allow("key-a", now) {
		t.Fatal("2nd request (same instant): want denied")
	}

	// 1 second later, at 1 token/sec, exactly 1 new token should be
	// available.
	later := now.Add(1 * time.Second)
	if !rl.allow("key-a", later) {
		t.Fatal("request 1s later: want allowed after refill")
	}
	if rl.allow("key-a", later) {
		t.Fatal("second request at the same later instant: want denied (only 1 token refilled)")
	}
}

func TestRateLimiter_Allow_TokensNeverExceedBurst(t *testing.T) {
	rl := NewRateLimiter(10, 100 /* token/sec, fast refill */, 3 /* burst */)
	defer rl.Close()

	now := time.Now()
	// Prime the bucket, then let a very long time pass -- tokens must cap
	// at burst (3), not grow unbounded.
	if !rl.allow("key-a", now) {
		t.Fatal("1st request: want allowed")
	}
	muchLater := now.Add(1 * time.Hour)

	allowedCount := 0
	for i := 0; i < 10; i++ {
		if rl.allow("key-a", muchLater) {
			allowedCount++
		}
	}
	if allowedCount != 3 {
		t.Fatalf("allowed %d of 10 requests after a long idle period, want exactly 3 (burst cap)", allowedCount)
	}
}

func TestRateLimiter_Allow_DistinctKeysDoNotInterfere(t *testing.T) {
	rl := NewRateLimiter(10, 1, 1)
	defer rl.Close()

	now := time.Now()
	if !rl.allow("key-a", now) {
		t.Fatal("key-a 1st request: want allowed")
	}
	if rl.allow("key-a", now) {
		t.Fatal("key-a 2nd request: want denied (exhausted)")
	}
	if !rl.allow("key-b", now) {
		t.Fatal("key-b 1st request: want allowed -- key-a's exhaustion must not affect key-b")
	}
}

func TestRateLimiter_Allow_MapCardinalityCap_RejectsBrandNewKeysWhenFull(t *testing.T) {
	rl := NewRateLimiter(1 /* maxBuckets */, 1, 5)
	defer rl.Close()

	now := time.Now()
	if !rl.allow("key-a", now) {
		t.Fatal("key-a 1st request: want allowed (first key, map has room)")
	}
	// key-a already has a bucket, so it keeps working even though the map
	// is now "full" relative to maxBuckets=1.
	if !rl.allow("key-a", now) {
		t.Fatal("key-a 2nd request: want allowed -- an already-tracked key must not be punished by the cap")
	}

	// key-b has never been seen, and the map is already at capacity: it
	// must be denied outright, without creating a new bucket for it (see
	// allow's doc comment on the fail-closed choice over a shared
	// overflow bucket).
	if rl.allow("key-b", now) {
		t.Fatal("key-b 1st request: want denied -- map is at maxBuckets capacity")
	}

	rl.mu.Lock()
	bucketCount := len(rl.buckets)
	rl.mu.Unlock()
	if bucketCount != 1 {
		t.Fatalf("bucket count = %d after a denied brand-new key, want 1 (no bucket created for key-b)", bucketCount)
	}
}

func TestRateLimiter_Close_StopsCleanupGoroutine_WithoutBlockingForever(t *testing.T) {
	rl := NewRateLimiter(10, 1, 1, WithCleanupInterval(time.Millisecond))

	done := make(chan struct{})
	go func() {
		rl.Close()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return within 2s -- cleanup goroutine appears to be leaking/blocked")
	}
}

func TestRateLimiter_Close_IsIdempotent(t *testing.T) {
	rl := NewRateLimiter(10, 1, 1)
	rl.Close()
	rl.Close() // must not panic or block
}

func TestRateLimiter_RateLimit_MiddlewareDeniesExhaustedKeyWith429(t *testing.T) {
	rl := NewRateLimiter(10, 0.0001 /* effectively no refill within the test */, 1)
	defer rl.Close()

	calls := 0
	handler := rl.RateLimit(func(r *http.Request) string { return "fixed-key" })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(http.StatusOK)
		}),
	)

	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec1.Code != http.StatusOK {
		t.Fatalf("1st request status = %d, want 200", rec1.Code)
	}

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("2nd request status = %d, want 429, body=%s", rec2.Code, rec2.Body.String())
	}

	if calls != 1 {
		t.Fatalf("downstream handler called %d times, want exactly 1 (2nd request must be rejected before reaching it)", calls)
	}
}

func TestRateLimiter_RateLimit_KeyFuncDeterminesIsolation(t *testing.T) {
	rl := NewRateLimiter(10, 0.0001, 1)
	defer rl.Close()

	keyFunc := func(r *http.Request) string { return r.Header.Get("X-Client-Key") }
	handler := rl.RateLimit(keyFunc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	reqA := httptest.NewRequest(http.MethodGet, "/", nil)
	reqA.Header.Set("X-Client-Key", "client-a")
	recA := httptest.NewRecorder()
	handler.ServeHTTP(recA, reqA)
	if recA.Code != http.StatusOK {
		t.Fatalf("client-a 1st request status = %d, want 200", recA.Code)
	}

	reqB := httptest.NewRequest(http.MethodGet, "/", nil)
	reqB.Header.Set("X-Client-Key", "client-b")
	recB := httptest.NewRecorder()
	handler.ServeHTTP(recB, reqB)
	if recB.Code != http.StatusOK {
		t.Fatalf("client-b 1st request status = %d, want 200 -- distinct key must not be affected by client-a", recB.Code)
	}
}

func TestNewRateLimiter_NonPositiveOptions_FallBackToSafeDefaults(t *testing.T) {
	rl := NewRateLimiter(0, 0, 0)
	defer rl.Close()

	if rl.maxBuckets <= 0 {
		t.Errorf("maxBuckets = %d, want a positive default", rl.maxBuckets)
	}
	if rl.refillPerSecond <= 0 {
		t.Errorf("refillPerSecond = %v, want a positive default", rl.refillPerSecond)
	}
	if rl.burst <= 0 {
		t.Errorf("burst = %v, want a positive default", rl.burst)
	}
}
