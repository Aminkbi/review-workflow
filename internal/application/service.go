package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"review-workflow/internal/domain"
)

type Service struct {
	store                Store
	executor             Executor
	defaultReviewerID    string
	reminderAfter        time.Duration
	executionRetryBase   time.Duration
	executionMaxAttempts int
	now                  func() time.Time
}

func NewService(store Store, executor Executor, defaultReviewerID string, reminderAfter, executionRetryBase time.Duration, executionMaxAttempts int) *Service {
	return &Service{
		store:                store,
		executor:             executor,
		defaultReviewerID:    strings.TrimSpace(defaultReviewerID),
		reminderAfter:        reminderAfter,
		executionRetryBase:   executionRetryBase,
		executionMaxAttempts: executionMaxAttempts,
		now:                  time.Now,
	}
}

type CreateDraftInput struct {
	Type           string `json:"type"`
	TargetResource string `json:"target_resource"`
	Justification  string `json:"justification"`
}

type ReviewInput struct {
	Comment string `json:"comment"`
}

type ExecutionResultInput struct {
	Success           bool   `json:"success"`
	Error             string `json:"error,omitempty"`
	ExternalReference string `json:"external_reference,omitempty"`
}

func (s *Service) CreateDraft(ctx context.Context, actor domain.Actor, input CreateDraftInput) (domain.Request, error) {
	if err := domain.ValidateActor(actor); err != nil {
		return domain.Request{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if strings.TrimSpace(input.Type) == "" || strings.TrimSpace(input.TargetResource) == "" || strings.TrimSpace(input.Justification) == "" {
		return domain.Request{}, fmt.Errorf("%w: type, target_resource, and justification are required", ErrInvalidInput)
	}

	now := s.now().UTC()
	req := domain.Request{
		ID:             domain.NewID("req"),
		Type:           strings.TrimSpace(input.Type),
		TargetResource: strings.TrimSpace(input.TargetResource),
		Justification:  strings.TrimSpace(input.Justification),
		RequesterID:    actor.ID,
		Status:         domain.StatusDraft,
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.store.CreateRequest(ctx, req); err != nil {
		return domain.Request{}, err
	}
	if err := s.store.AppendAudit(ctx, s.auditFor(req, actor, "draft_created", "draft request created")); err != nil {
		return domain.Request{}, err
	}
	return req, nil
}

func (s *Service) GetRequest(ctx context.Context, actor domain.Actor, requestID string) (domain.Request, error) {
	req, err := s.store.GetRequest(ctx, requestID)
	if err != nil {
		return domain.Request{}, err
	}
	if !canView(actor, req) {
		return domain.Request{}, ErrForbidden
	}
	return req, nil
}

func (s *Service) ListRequests(ctx context.Context, actor domain.Actor, filter domain.RequestFilter) ([]domain.Request, error) {
	if err := domain.ValidateActor(actor); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	effectiveFilter := filter
	switch actor.Role {
	case domain.RoleEmployee:
		effectiveFilter.RequesterID = actor.ID
		effectiveFilter.ReviewerID = ""
	case domain.RoleReviewer:
		if effectiveFilter.RequesterID == "" {
			effectiveFilter.ReviewerID = actor.ID
		}
	case domain.RoleAdmin:
	default:
		return nil, ErrForbidden
	}
	return s.store.ListRequests(ctx, effectiveFilter)
}

func (s *Service) SubmitRequest(ctx context.Context, actor domain.Actor, requestID, idempotencyKey string) (domain.Request, error) {
	req, err := s.requireOwnedOrAdmin(ctx, actor, requestID)
	if err != nil {
		return domain.Request{}, err
	}
	scope := "submit:" + requestID
	if err := s.ensureIdempotency(ctx, scope, idempotencyKey, requestID, actor.ID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return s.store.GetRequest(ctx, requestID)
		}
		if !errors.Is(err, ErrConflict) {
			return domain.Request{}, err
		}
		record, getErr := s.store.GetIdempotency(ctx, scope, idempotencyKey)
		if getErr != nil {
			return domain.Request{}, err
		}
		if record.Fingerprint != actor.ID {
			return domain.Request{}, ErrIdempotencyKeyUsed
		}
		return s.store.GetRequest(ctx, requestID)
	}
	if req.Status != domain.StatusDraft {
		return domain.Request{}, fmt.Errorf("%w: only draft requests can be submitted", ErrConflict)
	}

	now := s.now().UTC()
	expectedVersion := req.Version
	req.Status = domain.StatusPendingReview
	req.AssignedReviewerID = s.defaultReviewerID
	req.SubmittedAt = &now
	req.UpdatedAt = now
	req.Version++

	if err := s.store.UpdateRequest(ctx, req, expectedVersion); err != nil {
		return domain.Request{}, err
	}
	if err := s.store.AppendAudit(ctx, s.auditFor(req, actor, "submitted", "request submitted for review")); err != nil {
		return domain.Request{}, err
	}
	if err := s.store.AppendAudit(ctx, s.auditFor(req, systemActor(), "review_assigned", "request assigned to reviewer")); err != nil {
		return domain.Request{}, err
	}
	return req, nil
}

func (s *Service) ApproveRequest(ctx context.Context, actor domain.Actor, requestID, idempotencyKey string, input ReviewInput) (domain.Request, error) {
	return s.reviewRequest(ctx, actor, requestID, idempotencyKey, "approved", input.Comment)
}

func (s *Service) RejectRequest(ctx context.Context, actor domain.Actor, requestID, idempotencyKey string, input ReviewInput) (domain.Request, error) {
	return s.reviewRequest(ctx, actor, requestID, idempotencyKey, "rejected", input.Comment)
}

func (s *Service) reviewRequest(ctx context.Context, actor domain.Actor, requestID, idempotencyKey, decision, comment string) (domain.Request, error) {
	req, err := s.store.GetRequest(ctx, requestID)
	if err != nil {
		return domain.Request{}, err
	}
	if req.Status == domain.StatusApproved && decision == "approved" {
		if err := s.ensureIdempotency(ctx, decision+":"+requestID, idempotencyKey, requestID, fingerprint(decision, comment)); err == nil || errors.Is(err, ErrNotFound) {
			return req, nil
		}
		return domain.Request{}, err
	}
	if req.Status == domain.StatusRejected && decision == "rejected" {
		if err := s.ensureIdempotency(ctx, decision+":"+requestID, idempotencyKey, requestID, fingerprint(decision, comment)); err == nil || errors.Is(err, ErrNotFound) {
			return req, nil
		}
		return domain.Request{}, err
	}
	if req.Status != domain.StatusPendingReview {
		return domain.Request{}, fmt.Errorf("%w: request is not awaiting review", ErrConflict)
	}
	if !canReview(actor, req) {
		return domain.Request{}, ErrForbidden
	}
	if actor.ID == req.RequesterID {
		return domain.Request{}, ErrForbidden
	}

	scope := decision + ":" + requestID
	if err := s.ensureIdempotency(ctx, scope, idempotencyKey, requestID, fingerprint(decision, comment)); err != nil {
		if errors.Is(err, ErrNotFound) {
			return s.store.GetRequest(ctx, requestID)
		}
		return domain.Request{}, err
	}

	now := s.now().UTC()
	expectedVersion := req.Version
	req.ReviewedAt = &now
	req.UpdatedAt = now
	req.Version++
	if decision == "approved" {
		req.Status = domain.StatusApproved
		req.ApprovedAt = &now
		nextExecutionAt := now
		req.NextExecutionAt = &nextExecutionAt
		req.LastExecutionError = ""
	} else {
		req.Status = domain.StatusRejected
		req.NextExecutionAt = nil
	}

	if err := s.store.UpdateRequest(ctx, req, expectedVersion); err != nil {
		return domain.Request{}, err
	}
	if err := s.store.CreateReview(ctx, domain.ReviewAction{
		ID:        domain.NewID("rev"),
		RequestID: requestID,
		Decision:  decision,
		Comment:   strings.TrimSpace(comment),
		ActorID:   actor.ID,
		CreatedAt: now,
	}); err != nil {
		return domain.Request{}, err
	}
	if err := s.store.AppendAudit(ctx, s.auditFor(req, actor, decision, decisionDescription(decision, comment))); err != nil {
		return domain.Request{}, err
	}
	return req, nil
}

func (s *Service) RecordExecutionResult(ctx context.Context, actor domain.Actor, requestID string, input ExecutionResultInput) (domain.Request, error) {
	if actor.Role != domain.RoleAdmin && actor.Role != domain.RoleReviewer {
		return domain.Request{}, ErrForbidden
	}
	req, err := s.store.GetRequest(ctx, requestID)
	if err != nil {
		return domain.Request{}, err
	}
	return s.applyExecutionResult(ctx, req, actor, input.Success, input.Error)
}

func (s *Service) ListAudit(ctx context.Context, actor domain.Actor, requestID string) ([]domain.AuditEntry, error) {
	req, err := s.GetRequest(ctx, actor, requestID)
	if err != nil {
		return nil, err
	}
	return s.store.ListAudit(ctx, req.ID)
}

func (s *Service) ProcessDueReminders(ctx context.Context, limit int) error {
	now := s.now().UTC()
	requests, err := s.store.ListRequestsNeedingReminder(ctx, now.Add(-s.reminderAfter), limit)
	if err != nil {
		return err
	}
	for _, req := range requests {
		expectedVersion := req.Version
		req.Version++
		req.ReminderCount++
		req.UpdatedAt = now
		req.LastRemindedAt = &now
		if err := s.store.UpdateRequest(ctx, req, expectedVersion); err != nil {
			continue
		}
		_ = s.store.AppendAudit(ctx, s.auditFor(req, systemActor(), "reminder_sent", "review reminder generated"))
		_ = s.store.AppendJobRun(ctx, domain.JobRun{
			ID:         domain.NewID("job"),
			RequestID:  req.ID,
			JobType:    "reminder",
			Status:     "succeeded",
			Attempt:    req.ReminderCount,
			Message:    "reminder recorded",
			ExecutedAt: now,
		})
	}
	return nil
}

func (s *Service) ProcessDueExecutions(ctx context.Context, limit int) error {
	now := s.now().UTC()
	requests, err := s.store.ListRequestsReadyForExecution(ctx, now, limit)
	if err != nil {
		return err
	}
	for _, req := range requests {
		run := domain.JobRun{
			ID:         domain.NewID("job"),
			RequestID:  req.ID,
			JobType:    "execution",
			Status:     "succeeded",
			Attempt:    req.ExecutionAttempts + 1,
			ExecutedAt: now,
		}
		execErr := s.executor.Execute(ctx, req)
		if execErr != nil {
			run.Status = "failed"
			run.Message = execErr.Error()
			_, _ = s.applyExecutionResult(ctx, req, systemActor(), false, execErr.Error())
		} else {
			run.Message = "execution completed"
			_, _ = s.applyExecutionResult(ctx, req, systemActor(), true, "")
		}
		_ = s.store.AppendJobRun(ctx, run)
	}
	return nil
}

func (s *Service) applyExecutionResult(ctx context.Context, req domain.Request, actor domain.Actor, success bool, failureMessage string) (domain.Request, error) {
	if req.Status != domain.StatusApproved && req.Status != domain.StatusExecutionFailed {
		return domain.Request{}, fmt.Errorf("%w: request is not ready for execution updates", ErrConflict)
	}

	now := s.now().UTC()
	expectedVersion := req.Version
	req.UpdatedAt = now
	req.Version++
	req.ExecutionAttempts++

	if success {
		req.Status = domain.StatusExecuted
		req.ExecutedAt = &now
		req.NextExecutionAt = nil
		req.LastExecutionError = ""
	} else {
		req.Status = domain.StatusExecutionFailed
		req.LastExecutionError = strings.TrimSpace(failureMessage)
		if req.ExecutionAttempts >= s.executionMaxAttempts {
			req.NextExecutionAt = nil
		} else {
			next := now.Add(time.Duration(req.ExecutionAttempts) * s.executionRetryBase)
			req.NextExecutionAt = &next
		}
	}

	if err := s.store.UpdateRequest(ctx, req, expectedVersion); err != nil {
		return domain.Request{}, err
	}
	action := "execution_succeeded"
	description := "request executed successfully"
	if !success {
		action = "execution_failed"
		description = "execution failed: " + strings.TrimSpace(failureMessage)
	}
	if err := s.store.AppendAudit(ctx, s.auditFor(req, actor, action, description)); err != nil {
		return domain.Request{}, err
	}
	return req, nil
}

func (s *Service) requireOwnedOrAdmin(ctx context.Context, actor domain.Actor, requestID string) (domain.Request, error) {
	req, err := s.store.GetRequest(ctx, requestID)
	if err != nil {
		return domain.Request{}, err
	}
	if actor.Role == domain.RoleAdmin || req.RequesterID == actor.ID {
		return req, nil
	}
	return domain.Request{}, ErrForbidden
}

func (s *Service) ensureIdempotency(ctx context.Context, scope, key, requestID, payload string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("%w: idempotency key is required", ErrInvalidInput)
	}
	record, err := s.store.GetIdempotency(ctx, scope, key)
	if err == nil {
		if record.Fingerprint != payload {
			return ErrIdempotencyKeyUsed
		}
		return ErrNotFound
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	return s.store.CreateIdempotency(ctx, domain.IdempotencyRecord{
		Key:         key,
		Scope:       scope,
		Fingerprint: payload,
		RequestID:   requestID,
		CreatedAt:   s.now().UTC(),
	})
}

func (s *Service) auditFor(req domain.Request, actor domain.Actor, action, description string) domain.AuditEntry {
	return domain.AuditEntry{
		ID:          domain.NewID("audit"),
		RequestID:   req.ID,
		ActorID:     actor.ID,
		ActorRole:   actor.Role,
		Action:      action,
		Description: description,
		CreatedAt:   s.now().UTC(),
	}
}

func canView(actor domain.Actor, req domain.Request) bool {
	if actor.Role == domain.RoleAdmin {
		return true
	}
	if req.RequesterID == actor.ID {
		return true
	}
	return req.AssignedReviewerID == actor.ID && actor.Role == domain.RoleReviewer
}

func canReview(actor domain.Actor, req domain.Request) bool {
	if actor.Role == domain.RoleAdmin {
		return true
	}
	return actor.Role == domain.RoleReviewer && req.AssignedReviewerID == actor.ID
}

func fingerprint(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(hash[:])
}

func decisionDescription(decision, comment string) string {
	if strings.TrimSpace(comment) == "" {
		return decision
	}
	return fmt.Sprintf("%s: %s", decision, strings.TrimSpace(comment))
}

func systemActor() domain.Actor {
	return domain.Actor{ID: "system", Role: domain.RoleAdmin}
}
