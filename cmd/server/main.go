// Command server is the entrypoint for the device telemetry router service.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"device-telemetry-router/internal/auth"
	"device-telemetry-router/internal/config"
	"device-telemetry-router/internal/httpapi"
	"device-telemetry-router/internal/metrics"
	"device-telemetry-router/internal/migrate"
	"device-telemetry-router/internal/router"
	"device-telemetry-router/internal/service"
	"device-telemetry-router/internal/store"
	"device-telemetry-router/internal/worker"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return err
	}
	if err := migrate.Apply(db); err != nil {
		return err
	}

	st := store.New(db)
	rtr := router.New()
	svc := service.New(st, st, rtr, cfg.DedupWindow)

	// Prometheus metrics registry instrumenting the HTTP and worker layers.
	reg := metrics.NewRegistry()
	httpInFlight := reg.NewGauge("telemetry_http_in_flight", "In-flight HTTP requests.")
	httpDuration := reg.NewDefaultHistogram("telemetry_http_duration_seconds", "HTTP request latency.")
	dispatchTotal := reg.NewCounter("telemetry_dispatch_total", "Total topic dispatch attempts.", "outcome")

	// API key authentication (optional; disabled when API_KEYS is empty).
	var authenticator *auth.Auth
	if cfg.APIKeys != "" {
		keys, perr := auth.ParseKeyPairs(cfg.APIKeys)
		if perr != nil {
			return perr
		}
		authenticator = auth.New(keys, st)
	}

	srv := httpapi.NewServerWith(svc, httpapi.ServerDeps{
		Metrics: reg.Handler(),
		Auth:    authenticator,
	})

	// Wrap the root handler with metrics instrumentation.
	rootHandler := instrumentHandler(httpInFlight, httpDuration, srv.Handler())

	// Background delivery retry worker.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var w *worker.Worker
	if cfg.Worker.Enabled {
		sink := worker.LogSink{}
		instrumentedSink := &countingSink{next: sink, counter: dispatchTotal}
		w = worker.New(st, instrumentedSink, worker.Options{
			Interval:   cfg.Worker.Interval,
			BatchSize:  cfg.Worker.BatchSize,
			MaxAttempt: cfg.Worker.MaxAttempt,
			BaseDelay:  cfg.Worker.BaseDelay,
		})
		go w.Run(ctx)
	}

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           rootHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("listening on :%s", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		cancel()
		return err
	case sig := <-stop:
		log.Printf("received %s, shutting down", sig)
	}
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	return httpServer.Shutdown(shutdownCtx)
}

// countingSink wraps a Sink and records dispatch outcomes into a counter.
type countingSink struct {
	next    worker.Sink
	counter *metrics.Counter
}

func (c *countingSink) Dispatch(ctx context.Context, topic string, payload []byte) error {
	err := c.next.Dispatch(ctx, topic, payload)
	if err != nil {
		return err
	}
	c.counter.Inc()
	return nil
}

// instrumentHandler returns a handler wrapped with request metrics.
func instrumentHandler(inFlight *metrics.Gauge, dur *metrics.Histogram, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		inFlight.Inc()
		defer func() {
			inFlight.Add(-1)
			dur.Observe(time.Since(start).Seconds())
		}()
		next.ServeHTTP(w, r)
	})
}
