package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/exaring/otelpgx"

	"github.com/manuellysuzik/mesaposta-api/internal/platform/app"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/config"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/db"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/httpx"
	"github.com/manuellysuzik/mesaposta-api/internal/platform/observability"
)

// observabilityShutdownTimeout bounds how long run waits for the
// TracerProvider/MeterProvider to flush pending spans/metrics and close
// their OTLP connection during shutdown, mirroring defaultShutdownTimeout's
// role for the HTTP server itself below.
const observabilityShutdownTimeout = 5 * time.Second

// defaultShutdownTimeout bounds how long serve waits for in-flight
// requests to finish once a shutdown signal (SIGINT/SIGTERM) arrives,
// before forcibly closing whatever connections are still open.
const defaultShutdownTimeout = 10 * time.Second

// migrateOnBootEnvVar and migrateDatabaseURLEnvVar are read directly via
// getenv (not through internal/platform/config, which has no opinion on
// deploy-time concerns) because they control a deploy-time decision --
// whether THIS process should run pending schema migrations on startup,
// and if so, with which DSN -- not application runtime configuration.
//
// migrateOnBootEnvVar defaults to disabled: see README.md, "Migrations",
// for why running migrations as a separate deploy step (never automatically
// on every replica's boot) is the recommended production path. When
// enabled, migrateDatabaseURLEnvVar SHOULD point at an administrative DSN
// distinct from DATABASE_URL -- internal/platform/db.Migrate's own doc
// comment explains why the restricted, RLS-bound application DSN is not
// appropriate for running migrations -- falling back to DATABASE_URL only
// as a local/dev convenience when unset.
const (
	migrateOnBootEnvVar      = "MIGRATE_ON_BOOT"
	migrateDatabaseURLEnvVar = "MIGRATE_DATABASE_URL"
)

// run wires and starts the whole mesaposta-api HTTP service, blocking until
// ctx is cancelled (in production, via the SIGINT/SIGTERM
// signal.NotifyContext below arranges to deliver into it) or an
// unrecoverable startup/serve error occurs. It returns the process exit
// code main() should use -- 0 for a clean shutdown, non-zero for any
// failure -- and writes any failure as a single, human-readable line to
// stderr. It never panics on a bad configuration or a failed dependency:
// a misconfigured deployment gets a clear message on stderr and a non-zero
// exit code, never a Go runtime stack trace.
func run(ctx context.Context, getenv func(string) string, stderr, stdout io.Writer) int {
	cfg, err := config.LoadFrom(getenv)
	if err != nil {
		fmt.Fprintln(stderr, "mesaposta-api: invalid configuration:", err)
		return 1
	}

	logger := slog.New(slog.NewJSONHandler(stdout, nil))

	// Refused before opening any database connection, deliberately ahead of
	// db.Open below: MIGRATE_ON_BOOT is only safe against a database whose
	// mesaposta_app role password has already been rotated away from
	// 0001_baseline.sql's versioned dev-only placeholder (see that
	// migration's own doc comment, and docs/adr/0004). This process cannot
	// verify that rotation happened, so in production it refuses to run
	// migrations automatically at all -- see README.md, "Migrations", for
	// the documented separate-deploy-step path this forces instead.
	if cfg.AppEnv == "production" && strings.EqualFold(getenv(migrateOnBootEnvVar), "true") {
		fmt.Fprintf(stderr, "mesaposta-api: %s=true is refused when APP_ENV=production; run migrations as a separate deploy step instead (see README.md, \"Migrations\")\n", migrateOnBootEnvVar)
		return 1
	}

	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// obsHandle.Shutdown is deferred here, BEFORE pool.Close below, so it
	// runs LAST (defers unwind LIFO): the TracerProvider/MeterProvider stay
	// alive through the pool's own teardown, then get their own bounded
	// window to flush whatever spans/metrics that teardown produced. When
	// observability is disabled (cfg.OTELExporterOTLPEndpoint == ""),
	// obsHandle.Shutdown is a no-op -- see observability.Setup's doc
	// comment.
	obsHandle, err := observability.Setup(signalCtx, observability.Config{
		ServiceName:  cfg.OTELServiceName,
		OTLPEndpoint: cfg.OTELExporterOTLPEndpoint,
	})
	if err != nil {
		fmt.Fprintln(stderr, "mesaposta-api: setting up observability:", err)
		return 1
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), observabilityShutdownTimeout)
		defer cancel()
		if err := obsHandle.Shutdown(shutdownCtx); err != nil {
			logger.Error("observability shutdown did not complete cleanly", "err", err)
		}
	}()

	pool, err := db.Open(signalCtx, cfg.DatabaseURL,
		db.WithMaxConns(cfg.DBMaxConns),
		db.WithMinConns(cfg.DBMinConns),
		db.WithMaxConnLifetime(cfg.DBMaxConnLifetime),
		db.WithMaxConnIdleTime(cfg.DBMaxConnIdleTime),
		db.WithConnectTimeout(cfg.DBConnectTimeout),
		db.WithQueryTracer(otelpgx.NewTracer()),
	)
	if err != nil {
		fmt.Fprintln(stderr, "mesaposta-api: opening database pool:", err)
		return 1
	}
	defer pool.Close()

	// Reports connection-pool saturation (used/idle/max) alongside the
	// http.server.request.duration histogram otelhttp already records --
	// see observability.RegisterPoolGauges' own doc comment for why pool
	// exhaustion is a common, otherwise-invisible root cause of HTTP p99
	// tail latency. Safe regardless of whether observability is enabled.
	if err := observability.RegisterPoolGauges(func() observability.PoolStats {
		stat := pool.Stat()
		return observability.PoolStats{
			UsedConns: int64(stat.AcquiredConns()),
			IdleConns: int64(stat.IdleConns()),
			MaxConns:  int64(stat.MaxConns()),
		}
	}); err != nil {
		fmt.Fprintln(stderr, "mesaposta-api: registering db pool gauges:", err)
		return 1
	}

	if strings.EqualFold(getenv(migrateOnBootEnvVar), "true") {
		migrateURL := getenv(migrateDatabaseURLEnvVar)
		if migrateURL == "" {
			migrateURL = cfg.DatabaseURL
		}
		if err := db.Migrate(signalCtx, migrateURL); err != nil {
			fmt.Fprintln(stderr, "mesaposta-api: running migrations on boot:", err)
			return 1
		}
		logger.Info("migrations applied on boot")
	}

	verifier, err := app.BuildVerifier(*cfg)
	if err != nil {
		fmt.Fprintln(stderr, "mesaposta-api: building token verifier:", err)
		return 1
	}

	issuer, err := app.BuildIssuer(*cfg)
	if err != nil {
		fmt.Fprintln(stderr, "mesaposta-api: building token issuer:", err)
		return 1
	}

	handler := app.NewApp(*cfg, pool, logger, verifier, issuer, obsHandle.MetricsHandler)
	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:    ":" + cfg.Port,
		Handler: handler,
	})

	logger.Info("starting server", "addr", srv.Addr, "app_env", cfg.AppEnv)
	return serve(signalCtx, srv, nil, logger, defaultShutdownTimeout)
}

// serve runs srv until ctx is cancelled, then performs a graceful
// shutdown bounded by shutdownTimeout: srv.Shutdown stops accepting new
// connections and waits for in-flight handlers to return on their own,
// forcibly closing anything still open once shutdownTimeout elapses.
//
// ln lets tests bind a real, ephemeral TCP port deterministically (via
// srv.Serve(ln)) instead of relying on srv.Addr through
// srv.ListenAndServe(); production code (run, above) always passes nil.
func serve(ctx context.Context, srv *http.Server, ln net.Listener, logger *slog.Logger, shutdownTimeout time.Duration) int {
	serveErrCh := make(chan error, 1)
	go func() {
		var err error
		if ln != nil {
			err = srv.Serve(ln)
		} else {
			err = srv.ListenAndServe()
		}
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveErrCh <- err
	}()

	select {
	case err := <-serveErrCh:
		if err != nil {
			logger.Error("server stopped unexpectedly", "err", err)
			return 1
		}
		return 0

	case <-ctx.Done():
		logger.Info("shutdown signal received, draining in-flight requests")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown did not complete within the timeout", "err", err)
			return 1
		}

		if err := <-serveErrCh; err != nil {
			logger.Error("server stopped with error after shutdown", "err", err)
			return 1
		}
		logger.Info("shutdown complete")
		return 0
	}
}
