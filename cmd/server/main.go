package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"review-workflow/internal/application"
	"review-workflow/internal/config"
	"review-workflow/internal/jobs"
	"review-workflow/internal/platform/observability"
	"review-workflow/internal/platform/sqlutil"
	"review-workflow/internal/repository/postgres"
	httptransport "review-workflow/internal/transport/http"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := observability.Setup(ctx, cfg.OTel, logger)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			logger.Error("otel shutdown failed", "error", err)
		}
	}()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatal(err)
	}
	if err := sqlutil.RunMigrations(ctx, pool); err != nil {
		log.Fatal(err)
	}

	store := postgres.NewStore(pool)
	service := application.NewService(
		store,
		application.SimulatedExecutor{},
		cfg.DefaultReviewerID,
		cfg.ReminderAfter,
		cfg.ExecutionRetryBase,
		cfg.ExecutionMaxAttempts,
	)
	worker := jobs.NewWorker(service, cfg.WorkerPollInterval, cfg.WorkerBatchSize, logger)
	go worker.Start(ctx)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httptransport.NewServer(service, pool.Ping, cfg.OTel.ServiceName).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("server listening", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown failed", "error", err)
	}
}
