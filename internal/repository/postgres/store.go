package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"review-workflow/internal/application"
	"review-workflow/internal/domain"
)

type Store struct {
	pool   *pgxpool.Pool
	tracer trace.Tracer
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool:   pool,
		tracer: otel.Tracer("review-workflow/repository/postgres"),
	}
}

func (s *Store) CreateRequest(ctx context.Context, req domain.Request) error {
	ctx, span := s.startSpan(ctx, "postgres.create_request", "insert", "requests",
		attribute.String("workflow.request_id", req.ID),
	)
	defer span.End()

	_, err := s.pool.Exec(ctx, `
		INSERT INTO requests (
			id, type, target_resource, justification, requester_id, assigned_reviewer_id, status,
			version, reminder_count, execution_attempts, last_execution_error, created_at, updated_at,
			submitted_at, approved_at, reviewed_at, executed_at, last_reminded_at, next_execution_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13,
			$14, $15, $16, $17, $18, $19
		)
	`,
		req.ID, req.Type, req.TargetResource, req.Justification, req.RequesterID, req.AssignedReviewerID, req.Status,
		req.Version, req.ReminderCount, req.ExecutionAttempts, req.LastExecutionError, req.CreatedAt, req.UpdatedAt,
		req.SubmittedAt, req.ApprovedAt, req.ReviewedAt, req.ExecutedAt, req.LastRemindedAt, req.NextExecutionAt,
	)
	recordSpanErr(span, err)
	return err
}

func (s *Store) GetRequest(ctx context.Context, id string) (domain.Request, error) {
	ctx, span := s.startSpan(ctx, "postgres.get_request", "select", "requests",
		attribute.String("workflow.request_id", id),
	)
	defer span.End()

	row := s.pool.QueryRow(ctx, `
		SELECT id, type, target_resource, justification, requester_id, assigned_reviewer_id, status,
			version, reminder_count, execution_attempts, last_execution_error, created_at, updated_at,
			submitted_at, approved_at, reviewed_at, executed_at, last_reminded_at, next_execution_at
		FROM requests
		WHERE id = $1
	`, id)

	req, err := scanRequest(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			recordSpanErr(span, application.ErrNotFound)
			return domain.Request{}, application.ErrNotFound
		}
		recordSpanErr(span, err)
		return domain.Request{}, err
	}
	return req, nil
}

func (s *Store) ListRequests(ctx context.Context, filter domain.RequestFilter) ([]domain.Request, error) {
	ctx, span := s.startSpan(ctx, "postgres.list_requests", "select", "requests")
	defer span.End()

	query := `
		SELECT id, type, target_resource, justification, requester_id, assigned_reviewer_id, status,
			version, reminder_count, execution_attempts, last_execution_error, created_at, updated_at,
			submitted_at, approved_at, reviewed_at, executed_at, last_reminded_at, next_execution_at
		FROM requests
		WHERE ($1 = '' OR requester_id = $1)
		  AND ($2 = '' OR assigned_reviewer_id = $2)
		  AND ($3 = '' OR status = $3)
		ORDER BY created_at ASC
	`
	rows, err := s.pool.Query(ctx, query, filter.RequesterID, filter.ReviewerID, filter.Status)
	if err != nil {
		recordSpanErr(span, err)
		return nil, err
	}
	defer rows.Close()

	var requests []domain.Request
	for rows.Next() {
		req, err := scanRequest(rows)
		if err != nil {
			recordSpanErr(span, err)
			return nil, err
		}
		requests = append(requests, req)
	}
	err = rows.Err()
	recordSpanErr(span, err)
	return requests, err
}

func (s *Store) UpdateRequest(ctx context.Context, req domain.Request, expectedVersion int) error {
	ctx, span := s.startSpan(ctx, "postgres.update_request", "update", "requests",
		attribute.String("workflow.request_id", req.ID),
	)
	defer span.End()

	tag, err := s.pool.Exec(ctx, `
		UPDATE requests
		SET type = $2,
			target_resource = $3,
			justification = $4,
			requester_id = $5,
			assigned_reviewer_id = $6,
			status = $7,
			version = $8,
			reminder_count = $9,
			execution_attempts = $10,
			last_execution_error = $11,
			updated_at = $12,
			submitted_at = $13,
			approved_at = $14,
			reviewed_at = $15,
			executed_at = $16,
			last_reminded_at = $17,
			next_execution_at = $18
		WHERE id = $1 AND version = $19
	`,
		req.ID, req.Type, req.TargetResource, req.Justification, req.RequesterID, req.AssignedReviewerID, req.Status,
		req.Version, req.ReminderCount, req.ExecutionAttempts, req.LastExecutionError, req.UpdatedAt, req.SubmittedAt,
		req.ApprovedAt, req.ReviewedAt, req.ExecutedAt, req.LastRemindedAt, req.NextExecutionAt, expectedVersion,
	)
	if err != nil {
		recordSpanErr(span, err)
		return err
	}
	if tag.RowsAffected() == 0 {
		recordSpanErr(span, application.ErrConflict)
		return application.ErrConflict
	}
	return nil
}

func (s *Store) AppendAudit(ctx context.Context, entry domain.AuditEntry) error {
	ctx, span := s.startSpan(ctx, "postgres.append_audit", "insert", "request_audit_logs",
		attribute.String("workflow.request_id", entry.RequestID),
	)
	defer span.End()

	_, err := s.pool.Exec(ctx, `
		INSERT INTO request_audit_logs (id, request_id, actor_id, actor_role, action, description, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, entry.ID, entry.RequestID, entry.ActorID, entry.ActorRole, entry.Action, entry.Description, entry.CreatedAt)
	recordSpanErr(span, err)
	return err
}

func (s *Store) ListAudit(ctx context.Context, requestID string) ([]domain.AuditEntry, error) {
	ctx, span := s.startSpan(ctx, "postgres.list_audit", "select", "request_audit_logs",
		attribute.String("workflow.request_id", requestID),
	)
	defer span.End()

	rows, err := s.pool.Query(ctx, `
		SELECT id, request_id, actor_id, actor_role, action, description, created_at
		FROM request_audit_logs
		WHERE request_id = $1
		ORDER BY created_at ASC
	`, requestID)
	if err != nil {
		recordSpanErr(span, err)
		return nil, err
	}
	defer rows.Close()

	var entries []domain.AuditEntry
	for rows.Next() {
		var entry domain.AuditEntry
		if err := rows.Scan(&entry.ID, &entry.RequestID, &entry.ActorID, &entry.ActorRole, &entry.Action, &entry.Description, &entry.CreatedAt); err != nil {
			recordSpanErr(span, err)
			return nil, err
		}
		entries = append(entries, entry)
	}
	err = rows.Err()
	recordSpanErr(span, err)
	return entries, err
}

func (s *Store) CreateReview(ctx context.Context, review domain.ReviewAction) error {
	ctx, span := s.startSpan(ctx, "postgres.create_review", "insert", "request_reviews",
		attribute.String("workflow.request_id", review.RequestID),
	)
	defer span.End()

	_, err := s.pool.Exec(ctx, `
		INSERT INTO request_reviews (id, request_id, decision, comment, actor_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, review.ID, review.RequestID, review.Decision, review.Comment, review.ActorID, review.CreatedAt)
	recordSpanErr(span, err)
	return err
}

func (s *Store) GetIdempotency(ctx context.Context, scope, key string) (domain.IdempotencyRecord, error) {
	ctx, span := s.startSpan(ctx, "postgres.get_idempotency", "select", "idempotency_keys")
	defer span.End()

	row := s.pool.QueryRow(ctx, `
		SELECT scope, key, fingerprint, request_id, created_at
		FROM idempotency_keys
		WHERE scope = $1 AND key = $2
	`, scope, key)
	var record domain.IdempotencyRecord
	if err := row.Scan(&record.Scope, &record.Key, &record.Fingerprint, &record.RequestID, &record.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			recordSpanErr(span, application.ErrNotFound)
			return domain.IdempotencyRecord{}, application.ErrNotFound
		}
		recordSpanErr(span, err)
		return domain.IdempotencyRecord{}, err
	}
	return record, nil
}

func (s *Store) CreateIdempotency(ctx context.Context, record domain.IdempotencyRecord) error {
	ctx, span := s.startSpan(ctx, "postgres.create_idempotency", "insert", "idempotency_keys",
		attribute.String("workflow.request_id", record.RequestID),
	)
	defer span.End()

	tag, err := s.pool.Exec(ctx, `
		INSERT INTO idempotency_keys (scope, key, fingerprint, request_id, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT DO NOTHING
	`, record.Scope, record.Key, record.Fingerprint, record.RequestID, record.CreatedAt)
	if err != nil {
		recordSpanErr(span, err)
		return err
	}
	if tag.RowsAffected() == 0 {
		recordSpanErr(span, application.ErrConflict)
		return application.ErrConflict
	}
	return nil
}

func (s *Store) ListRequestsNeedingReminder(ctx context.Context, before time.Time, limit int) ([]domain.Request, error) {
	ctx, span := s.startSpan(ctx, "postgres.list_requests_needing_reminder", "select", "requests")
	defer span.End()

	if limit <= 0 {
		limit = 25
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, type, target_resource, justification, requester_id, assigned_reviewer_id, status,
			version, reminder_count, execution_attempts, last_execution_error, created_at, updated_at,
			submitted_at, approved_at, reviewed_at, executed_at, last_reminded_at, next_execution_at
		FROM requests
		WHERE status = $1
		  AND submitted_at IS NOT NULL
		  AND submitted_at <= $2
		  AND (last_reminded_at IS NULL OR last_reminded_at <= $2)
		ORDER BY submitted_at ASC
		LIMIT $3
	`, domain.StatusPendingReview, before, limit)
	if err != nil {
		recordSpanErr(span, err)
		return nil, err
	}
	defer rows.Close()
	requests, err := collectRequests(rows)
	recordSpanErr(span, err)
	return requests, err
}

func (s *Store) ListRequestsReadyForExecution(ctx context.Context, before time.Time, limit int) ([]domain.Request, error) {
	ctx, span := s.startSpan(ctx, "postgres.list_requests_ready_for_execution", "select", "requests")
	defer span.End()

	if limit <= 0 {
		limit = 25
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, type, target_resource, justification, requester_id, assigned_reviewer_id, status,
			version, reminder_count, execution_attempts, last_execution_error, created_at, updated_at,
			submitted_at, approved_at, reviewed_at, executed_at, last_reminded_at, next_execution_at
		FROM requests
		WHERE status IN ($1, $2)
		  AND next_execution_at IS NOT NULL
		  AND next_execution_at <= $3
		ORDER BY next_execution_at ASC
		LIMIT $4
	`, domain.StatusApproved, domain.StatusExecutionFailed, before, limit)
	if err != nil {
		recordSpanErr(span, err)
		return nil, err
	}
	defer rows.Close()
	requests, err := collectRequests(rows)
	recordSpanErr(span, err)
	return requests, err
}

func (s *Store) AppendJobRun(ctx context.Context, run domain.JobRun) error {
	ctx, span := s.startSpan(ctx, "postgres.append_job_run", "insert", "job_runs",
		attribute.String("workflow.request_id", run.RequestID),
		attribute.String("workflow.job_type", run.JobType),
	)
	defer span.End()

	_, err := s.pool.Exec(ctx, `
		INSERT INTO job_runs (id, request_id, job_type, status, attempt, message, executed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, run.ID, run.RequestID, run.JobType, run.Status, run.Attempt, run.Message, run.ExecutedAt)
	recordSpanErr(span, err)
	return err
}

func (s *Store) startSpan(ctx context.Context, name, operation, table string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", operation),
	}
	if table != "" {
		baseAttrs = append(baseAttrs, attribute.String("db.sql.table", table))
	}
	baseAttrs = append(baseAttrs, attrs...)
	return s.tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(baseAttrs...))
}

func recordSpanErr(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

type requestScanner interface {
	Scan(dest ...any) error
}

func scanRequest(row requestScanner) (domain.Request, error) {
	var req domain.Request
	var assignedReviewerID string
	var lastExecutionError string
	var submittedAt, approvedAt, reviewedAt, executedAt, lastRemindedAt, nextExecutionAt *time.Time

	err := row.Scan(
		&req.ID, &req.Type, &req.TargetResource, &req.Justification, &req.RequesterID, &assignedReviewerID, &req.Status,
		&req.Version, &req.ReminderCount, &req.ExecutionAttempts, &lastExecutionError, &req.CreatedAt, &req.UpdatedAt,
		&submittedAt, &approvedAt, &reviewedAt, &executedAt, &lastRemindedAt, &nextExecutionAt,
	)
	if err != nil {
		return domain.Request{}, err
	}

	req.AssignedReviewerID = strings.TrimSpace(assignedReviewerID)
	req.LastExecutionError = strings.TrimSpace(lastExecutionError)
	req.SubmittedAt = submittedAt
	req.ApprovedAt = approvedAt
	req.ReviewedAt = reviewedAt
	req.ExecutedAt = executedAt
	req.LastRemindedAt = lastRemindedAt
	req.NextExecutionAt = nextExecutionAt
	return req, nil
}

func collectRequests(rows pgx.Rows) ([]domain.Request, error) {
	var requests []domain.Request
	for rows.Next() {
		req, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, req)
	}
	return requests, rows.Err()
}
