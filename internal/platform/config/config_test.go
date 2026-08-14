package config_test

import (
	"strings"
	"testing"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/config"
)

// envMap builds a getenv func.func(string) string backed by m, returning ""
// for any key not present -- mirroring os.Getenv's own zero-value behavior
// for unset variables.
func envMap(m map[string]string) func(string) string {
	return func(key string) string {
		return m[key]
	}
}

// validBaseEnv returns a map already populated with every required variable
// (development-safe values), so individual test cases only need to
// override/remove exactly what they want to exercise.
func validBaseEnv() map[string]string {
	return map[string]string{
		"DATABASE_URL": "postgres://user:supersecret@localhost:5432/mesaposta?sslmode=disable",
		"JWT_SECRET":   "this-is-a-perfectly-fine-32-byte-plus-secret-value",
	}
}

func TestLoadFrom_MissingRequiredVars_ListsAllAtOnce(t *testing.T) {
	_, err := config.LoadFrom(envMap(map[string]string{}))
	if err == nil {
		t.Fatal("LoadFrom: want error when all required vars are missing, got nil")
	}

	msg := err.Error()
	for _, want := range []string{"DATABASE_URL", "JWT_SECRET"} {
		if !strings.Contains(msg, want) {
			t.Errorf("LoadFrom error = %q, want it to mention missing var %q", msg, want)
		}
	}
}

func TestLoadFrom_MissingOnlyOneRequiredVar(t *testing.T) {
	env := validBaseEnv()
	delete(env, "JWT_SECRET")

	_, err := config.LoadFrom(envMap(env))
	if err == nil {
		t.Fatal("LoadFrom: want error when JWT_SECRET is missing, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "JWT_SECRET") {
		t.Errorf("LoadFrom error = %q, want it to mention JWT_SECRET", msg)
	}
	if strings.Contains(msg, "DATABASE_URL") {
		t.Errorf("LoadFrom error = %q, want it to NOT mention DATABASE_URL (it was present)", msg)
	}
}

func TestLoadFrom_JWTSecretTooShort(t *testing.T) {
	env := validBaseEnv()
	env["JWT_SECRET"] = "too-short"

	_, err := config.LoadFrom(envMap(env))
	if err == nil {
		t.Fatal("LoadFrom: want error for a JWT_SECRET shorter than 32 bytes, got nil")
	}
	if !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Errorf("LoadFrom error = %q, want it to mention JWT_SECRET", err.Error())
	}
}

func TestLoadFrom_JWTSecretExactly32Bytes_IsAccepted(t *testing.T) {
	env := validBaseEnv()
	env["JWT_SECRET"] = strings.Repeat("a", 32)

	if _, err := config.LoadFrom(envMap(env)); err != nil {
		t.Fatalf("LoadFrom: want no error for an exactly-32-byte JWT_SECRET, got %v", err)
	}
}

func TestLoadFrom_ProductionWithExampleSecret_IsRejected(t *testing.T) {
	env := validBaseEnv()
	env["APP_ENV"] = "production"
	env["JWT_SECRET"] = config.ExampleJWTSecret

	_, err := config.LoadFrom(envMap(env))
	if err == nil {
		t.Fatal("LoadFrom: want error when APP_ENV=production and JWT_SECRET matches the .env.example placeholder, got nil")
	}
}

func TestLoadFrom_DevelopmentWithExampleSecret_IsAllowed(t *testing.T) {
	env := validBaseEnv()
	env["APP_ENV"] = "development"
	env["JWT_SECRET"] = config.ExampleJWTSecret

	if _, err := config.LoadFrom(envMap(env)); err != nil {
		t.Fatalf("LoadFrom: want no error for the example secret outside production, got %v", err)
	}
}

func TestLoadFrom_Defaults(t *testing.T) {
	cfg, err := config.LoadFrom(envMap(validBaseEnv()))
	if err != nil {
		t.Fatalf("LoadFrom: unexpected error: %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("cfg.Port = %q, want default %q", cfg.Port, "8080")
	}
	if cfg.AppEnv != "development" {
		t.Errorf("cfg.AppEnv = %q, want default %q", cfg.AppEnv, "development")
	}
	if cfg.DBMaxConns != 10 {
		t.Errorf("cfg.DBMaxConns = %d, want default 10", cfg.DBMaxConns)
	}
	if cfg.DBMinConns != 0 {
		t.Errorf("cfg.DBMinConns = %d, want default 0", cfg.DBMinConns)
	}
	if cfg.DBMaxConnLifetime.String() != "1h0m0s" {
		t.Errorf("cfg.DBMaxConnLifetime = %v, want 1h0m0s", cfg.DBMaxConnLifetime)
	}
	if cfg.DBMaxConnIdleTime.String() != "30m0s" {
		t.Errorf("cfg.DBMaxConnIdleTime = %v, want 30m0s", cfg.DBMaxConnIdleTime)
	}
	if cfg.DBConnectTimeout.String() != "5s" {
		t.Errorf("cfg.DBConnectTimeout = %v, want 5s", cfg.DBConnectTimeout)
	}
	if cfg.OTELServiceName != "mesaposta-api" {
		t.Errorf("cfg.OTELServiceName = %q, want default %q", cfg.OTELServiceName, "mesaposta-api")
	}
	if cfg.OTELExporterOTLPEndpoint != "" {
		t.Errorf("cfg.OTELExporterOTLPEndpoint = %q, want default empty string (observability off)", cfg.OTELExporterOTLPEndpoint)
	}
}

func TestLoadFrom_ExplicitOverrides(t *testing.T) {
	env := validBaseEnv()
	env["PORT"] = "9090"
	env["APP_ENV"] = "staging"
	env["DB_MAX_CONNS"] = "25"
	env["DB_MIN_CONNS"] = "2"
	env["DB_MAX_CONN_LIFETIME"] = "2h"
	env["DB_MAX_CONN_IDLE_TIME"] = "15m"
	env["DB_CONNECT_TIMEOUT"] = "10s"
	env["OTEL_SERVICE_NAME"] = "mesaposta-api-staging"
	env["OTEL_EXPORTER_OTLP_ENDPOINT"] = "http://jaeger:4318"

	cfg, err := config.LoadFrom(envMap(env))
	if err != nil {
		t.Fatalf("LoadFrom: unexpected error: %v", err)
	}

	if cfg.Port != "9090" {
		t.Errorf("cfg.Port = %q, want %q", cfg.Port, "9090")
	}
	if cfg.AppEnv != "staging" {
		t.Errorf("cfg.AppEnv = %q, want %q", cfg.AppEnv, "staging")
	}
	if cfg.DBMaxConns != 25 {
		t.Errorf("cfg.DBMaxConns = %d, want 25", cfg.DBMaxConns)
	}
	if cfg.DBMinConns != 2 {
		t.Errorf("cfg.DBMinConns = %d, want 2", cfg.DBMinConns)
	}
	if cfg.DBMaxConnLifetime.String() != "2h0m0s" {
		t.Errorf("cfg.DBMaxConnLifetime = %v, want 2h0m0s", cfg.DBMaxConnLifetime)
	}
	if cfg.DBMaxConnIdleTime.String() != "15m0s" {
		t.Errorf("cfg.DBMaxConnIdleTime = %v, want 15m0s", cfg.DBMaxConnIdleTime)
	}
	if cfg.DBConnectTimeout.String() != "10s" {
		t.Errorf("cfg.DBConnectTimeout = %v, want 10s", cfg.DBConnectTimeout)
	}
	if cfg.OTELServiceName != "mesaposta-api-staging" {
		t.Errorf("cfg.OTELServiceName = %q, want %q", cfg.OTELServiceName, "mesaposta-api-staging")
	}
	if cfg.OTELExporterOTLPEndpoint != "http://jaeger:4318" {
		t.Errorf("cfg.OTELExporterOTLPEndpoint = %q, want %q", cfg.OTELExporterOTLPEndpoint, "http://jaeger:4318")
	}
}

func TestLoadFrom_InvalidNumericOverride(t *testing.T) {
	env := validBaseEnv()
	env["DB_MAX_CONNS"] = "not-a-number"

	_, err := config.LoadFrom(envMap(env))
	if err == nil {
		t.Fatal("LoadFrom: want error for a non-numeric DB_MAX_CONNS, got nil")
	}
	if !strings.Contains(err.Error(), "DB_MAX_CONNS") {
		t.Errorf("LoadFrom error = %q, want it to mention DB_MAX_CONNS", err.Error())
	}
}

func TestLoadFrom_InvalidDurationOverride(t *testing.T) {
	env := validBaseEnv()
	env["DB_CONNECT_TIMEOUT"] = "not-a-duration"

	_, err := config.LoadFrom(envMap(env))
	if err == nil {
		t.Fatal("LoadFrom: want error for a non-duration DB_CONNECT_TIMEOUT, got nil")
	}
	if !strings.Contains(err.Error(), "DB_CONNECT_TIMEOUT") {
		t.Errorf("LoadFrom error = %q, want it to mention DB_CONNECT_TIMEOUT", err.Error())
	}
}

func TestLoadFrom_MultipleInvalidValues_AreAllReported(t *testing.T) {
	env := map[string]string{
		"JWT_SECRET":   "too-short",
		"DB_MAX_CONNS": "bogus",
	}

	_, err := config.LoadFrom(envMap(env))
	if err == nil {
		t.Fatal("LoadFrom: want error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"DATABASE_URL", "JWT_SECRET", "DB_MAX_CONNS"} {
		if !strings.Contains(msg, want) {
			t.Errorf("LoadFrom error = %q, want it to mention %q", msg, want)
		}
	}
}

func TestConfig_String_RedactsSecrets(t *testing.T) {
	env := validBaseEnv()
	cfg, err := config.LoadFrom(envMap(env))
	if err != nil {
		t.Fatalf("LoadFrom: unexpected error: %v", err)
	}

	rendered := cfg.String()

	if strings.Contains(rendered, "supersecret") {
		t.Errorf("Config.String() = %q, must not leak the DATABASE_URL password", rendered)
	}
	if strings.Contains(rendered, env["DATABASE_URL"]) {
		t.Errorf("Config.String() = %q, must not leak the raw DATABASE_URL", rendered)
	}
	if strings.Contains(rendered, env["JWT_SECRET"]) {
		t.Errorf("Config.String() = %q, must not leak the raw JWT_SECRET", rendered)
	}
	// Sanity check: non-secret fields are still visible for operators
	// reading logs.
	if !strings.Contains(rendered, "development") {
		t.Errorf("Config.String() = %q, want it to still show non-secret fields like AppEnv", rendered)
	}
}

func TestLoad_ReadsFromOSEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pw@localhost:5432/mesaposta")
	t.Setenv("JWT_SECRET", strings.Repeat("b", 40))

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("cfg.Port = %q, want default %q", cfg.Port, "8080")
	}
}
