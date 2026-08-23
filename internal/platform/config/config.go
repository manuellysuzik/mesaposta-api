// Package config loads mesaposta-api's runtime configuration from
// environment variables. It has zero third-party dependencies (stdlib
// only): Load reads from os.Getenv directly, while LoadFrom accepts any
// func(string) string lookup so callers (in particular this package's own
// tests) can exercise the validation logic against a synthetic environment
// without mutating real process state.
//
// Validation is deliberately exhaustive rather than fail-fast: every
// problem found (missing required variables, malformed values, an insecure
// default secret left in production, ...) is collected and reported
// together in a single error, so an operator fixing a broken deployment
// does not have to redeploy once per missing variable.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/errs"
)

// ExampleJWTSecret is the placeholder value shipped in .env.example. It is
// deliberately well-formed (>= 32 bytes) so that, on its own, it passes the
// length check -- the only thing that makes it invalid is being used
// unchanged in a production environment (see LoadFrom's production check).
const ExampleJWTSecret = "replace-with-a-random-secret-of-at-least-32-bytes"

// minJWTSecretBytes is the minimum acceptable length, in bytes, for
// JWT_SECRET.
const minJWTSecretBytes = 32

// Defaults for optional configuration variables.
const (
	defaultPort              = "8080"
	defaultAppEnv            = "development"
	defaultDBMaxConns        = "10"
	defaultDBMinConns        = "0"
	defaultDBMaxConnLifetime = "1h"
	defaultDBMaxConnIdleTime = "30m"
	defaultDBConnectTimeout  = "5s"
	defaultOTELServiceName   = "mesaposta-api"
	// defaultOTELExporterOTLPEndpoint is deliberately empty: see
	// internal/platform/observability's package doc comment for why an
	// empty endpoint -- meaning observability is OFF by default -- is the
	// only safe default (in particular so `go test ./...` never depends on
	// a collector being reachable).
	defaultOTELExporterOTLPEndpoint = ""
	// defaultCORSAllowedOrigins matches the frontend's local dev server
	// (Next.js default port).
	defaultCORSAllowedOrigins = "http://localhost:3000"
)

// Config is mesaposta-api's fully validated runtime configuration.
type Config struct {
	// AppEnv identifies the deployment environment (e.g. "development",
	// "staging", "production"). Defaults to "development".
	AppEnv string
	// Port is the TCP port the HTTP server listens on. Defaults to "8080".
	Port string
	// DatabaseURL is the Postgres connection string (DSN). Required --
	// there is no safe default. Treated as a secret: never logged in full,
	// see Config.String.
	DatabaseURL string
	// JWTSecret signs and verifies access tokens (see
	// internal/platform/auth/token). Required, must be at least 32 bytes,
	// and must not equal ExampleJWTSecret when AppEnv is "production".
	// Treated as a secret: never logged in full, see Config.String.
	JWTSecret string

	// DBMaxConns is the maximum number of connections the Postgres pool
	// may open. Defaults to 10.
	DBMaxConns int32
	// DBMinConns is the minimum number of idle connections the Postgres
	// pool keeps warm. Defaults to 0.
	DBMinConns int32
	// DBMaxConnLifetime is the maximum lifetime of a single pooled
	// connection before it is recycled. Defaults to 1h.
	DBMaxConnLifetime time.Duration
	// DBMaxConnIdleTime is the maximum time a pooled connection may sit
	// idle before it is closed. Defaults to 30m.
	DBMaxConnIdleTime time.Duration
	// DBConnectTimeout bounds how long establishing a new Postgres
	// connection may take. Defaults to 5s.
	DBConnectTimeout time.Duration

	// OTELServiceName identifies this process in exported traces/metrics
	// (see internal/platform/observability.Config.ServiceName). Defaults
	// to "mesaposta-api".
	OTELServiceName string
	// OTELExporterOTLPEndpoint is the OTLP/HTTP collector this process
	// exports traces to (e.g. "http://jaeger:4318"). Defaults to "" --
	// empty means observability is disabled entirely, see
	// internal/platform/observability's package doc comment for why that
	// is the only safe default.
	OTELExporterOTLPEndpoint string

	// CORSAllowedOrigins lists the origins allowed to make cross-origin
	// requests (e.g. the frontend's dev server). Defaults to
	// ["http://localhost:3000"].
	CORSAllowedOrigins []string
}

// redacted is what Config.String prints in place of any secret value.
const redacted = "***REDACTED***"

// String renders cfg for logs. DatabaseURL and JWTSecret are ALWAYS
// redacted, regardless of their content -- this is the one property this
// method exists to guarantee, and it must never regress even if Config
// grows more secret-shaped fields later.
func (c Config) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Config{AppEnv:%s Port:%s DatabaseURL:%s JWTSecret:%s "+
		"DBMaxConns:%d DBMinConns:%d DBMaxConnLifetime:%s DBMaxConnIdleTime:%s DBConnectTimeout:%s "+
		"OTELServiceName:%s OTELExporterOTLPEndpoint:%s CORSAllowedOrigins:%s}",
		c.AppEnv, c.Port, redacted, redacted,
		c.DBMaxConns, c.DBMinConns, c.DBMaxConnLifetime, c.DBMaxConnIdleTime, c.DBConnectTimeout,
		c.OTELServiceName, c.OTELExporterOTLPEndpoint, c.CORSAllowedOrigins)
	return b.String()
}

// getenvFunc is the lookup signature LoadFrom accepts: it must mirror
// os.Getenv's own contract (empty string for an unset variable, no error
// return).
type getenvFunc func(key string) string

// Load reads configuration from the real process environment (os.Getenv).
func Load() (*Config, error) {
	return LoadFrom(os.Getenv)
}

// validator accumulates every configuration problem found across a single
// LoadFrom call, instead of returning on the first one, so a caller sees
// every missing/invalid variable in one pass.
type validator struct {
	problems []string
}

func (v *validator) addf(format string, args ...any) {
	v.problems = append(v.problems, fmt.Sprintf(format, args...))
}

func (v *validator) err() error {
	if len(v.problems) == 0 {
		return nil
	}
	msg := "invalid configuration: " + strings.Join(v.problems, "; ")
	return errs.New(errs.Invalid, "invalid_config", msg, nil)
}

// requireString reads key via getenv, recording a problem via v if it is
// unset/empty.
func requireString(getenv getenvFunc, v *validator, key string) string {
	val := getenv(key)
	if val == "" {
		v.addf("missing required environment variable %s", key)
	}
	return val
}

// stringWithDefault reads key via getenv, falling back to def when unset.
func stringWithDefault(getenv getenvFunc, key, def string) string {
	if val := getenv(key); val != "" {
		return val
	}
	return def
}

// int32WithDefault reads key via getenv as a base-10 int32, falling back to
// def when unset, and recording a problem via v if the value is present
// but not a valid integer.
func int32WithDefault(getenv getenvFunc, v *validator, key, def string) int32 {
	raw := stringWithDefault(getenv, key, def)
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		v.addf("%s must be a valid integer, got %q", key, raw)
		return 0
	}
	return int32(n)
}

// durationWithDefault reads key via getenv as a time.Duration, falling back
// to def when unset, and recording a problem via v if the value is present
// but not a valid duration.
func durationWithDefault(getenv getenvFunc, v *validator, key, def string) time.Duration {
	raw := stringWithDefault(getenv, key, def)
	d, err := time.ParseDuration(raw)
	if err != nil {
		v.addf("%s must be a valid duration (e.g. \"30s\", \"5m\"), got %q", key, raw)
		return 0
	}
	return d
}

// sliceWithDefault reads key via getenv as a comma-separated list, falling
// back to def when unset. Entries are trimmed of surrounding whitespace and
// empty entries are dropped (so a trailing comma or accidental double comma
// does not produce a blank origin).
func sliceWithDefault(getenv getenvFunc, key, def string) []string {
	raw := stringWithDefault(getenv, key, def)
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// LoadFrom builds a Config by reading every variable via getenv, validating
// as it goes, and returns every problem found (if any) as a single error.
func LoadFrom(getenv getenvFunc) (*Config, error) {
	v := &validator{}

	cfg := &Config{
		DatabaseURL: requireString(getenv, v, "DATABASE_URL"),
		JWTSecret:   requireString(getenv, v, "JWT_SECRET"),
		AppEnv:      stringWithDefault(getenv, "APP_ENV", defaultAppEnv),
		Port:        stringWithDefault(getenv, "PORT", defaultPort),

		DBMaxConns:        int32WithDefault(getenv, v, "DB_MAX_CONNS", defaultDBMaxConns),
		DBMinConns:        int32WithDefault(getenv, v, "DB_MIN_CONNS", defaultDBMinConns),
		DBMaxConnLifetime: durationWithDefault(getenv, v, "DB_MAX_CONN_LIFETIME", defaultDBMaxConnLifetime),
		DBMaxConnIdleTime: durationWithDefault(getenv, v, "DB_MAX_CONN_IDLE_TIME", defaultDBMaxConnIdleTime),
		DBConnectTimeout:  durationWithDefault(getenv, v, "DB_CONNECT_TIMEOUT", defaultDBConnectTimeout),

		OTELServiceName:          stringWithDefault(getenv, "OTEL_SERVICE_NAME", defaultOTELServiceName),
		OTELExporterOTLPEndpoint: stringWithDefault(getenv, "OTEL_EXPORTER_OTLP_ENDPOINT", defaultOTELExporterOTLPEndpoint),

		CORSAllowedOrigins: sliceWithDefault(getenv, "CORS_ALLOWED_ORIGINS", defaultCORSAllowedOrigins),
	}

	if cfg.JWTSecret != "" && len(cfg.JWTSecret) < minJWTSecretBytes {
		v.addf("JWT_SECRET must be at least %d bytes, got %d", minJWTSecretBytes, len(cfg.JWTSecret))
	}
	if cfg.AppEnv == "production" && cfg.JWTSecret == ExampleJWTSecret {
		v.addf("JWT_SECRET must not be the .env.example placeholder value when APP_ENV=production")
	}

	if err := v.err(); err != nil {
		return nil, err
	}
	return cfg, nil
}
