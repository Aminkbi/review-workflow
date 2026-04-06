package application

import (
	"context"
	"time"

	"review-workflow/internal/domain"
)

type Store interface {
	CreateRequest(ctx context.Context, req domain.Request) error
	GetRequest(ctx context.Context, id string) (domain.Request, error)
	ListRequests(ctx context.Context, filter domain.RequestFilter) ([]domain.Request, error)
	UpdateRequest(ctx context.Context, req domain.Request, expectedVersion int) error
	AppendAudit(ctx context.Context, entry domain.AuditEntry) error
	ListAudit(ctx context.Context, requestID string) ([]domain.AuditEntry, error)
	CreateReview(ctx context.Context, review domain.ReviewAction) error
	GetIdempotency(ctx context.Context, scope, key string) (domain.IdempotencyRecord, error)
	CreateIdempotency(ctx context.Context, record domain.IdempotencyRecord) error
	ListRequestsNeedingReminder(ctx context.Context, before time.Time, limit int) ([]domain.Request, error)
	ListRequestsReadyForExecution(ctx context.Context, before time.Time, limit int) ([]domain.Request, error)
	AppendJobRun(ctx context.Context, run domain.JobRun) error
}

type Executor interface {
	Execute(ctx context.Context, req domain.Request) error
}
