package jobs

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"review-workflow/internal/application"
)

type Worker struct {
	service      *application.Service
	pollInterval time.Duration
	batchSize    int
	logger       *slog.Logger
	tracer       trace.Tracer
}

func NewWorker(service *application.Service, pollInterval time.Duration, batchSize int, logger *slog.Logger) *Worker {
	if batchSize <= 0 {
		batchSize = 25
	}
	return &Worker{
		service:      service,
		pollInterval: pollInterval,
		batchSize:    batchSize,
		logger:       logger,
		tracer:       otel.Tracer("review-workflow/jobs"),
	}
}

func (w *Worker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	w.run(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.run(ctx)
		}
	}
}

func (w *Worker) run(ctx context.Context) {
	ctx, span := w.tracer.Start(ctx, "worker.run", trace.WithAttributes(
		attribute.Int("worker.batch_size", w.batchSize),
		attribute.String("worker.component", "scheduler"),
	))
	defer span.End()

	if err := w.service.ProcessDueReminders(ctx, w.batchSize); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		w.logger.Error("worker reminder pass failed", "error", err)
	}
	if err := w.service.ProcessDueExecutions(ctx, w.batchSize); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		w.logger.Error("worker execution pass failed", "error", err)
	}
}
