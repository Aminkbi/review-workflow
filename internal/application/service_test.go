package application_test

import (
	"context"
	"testing"
	"time"

	"review-workflow/internal/application"
	"review-workflow/internal/domain"
	"review-workflow/internal/repository/memory"
)

func TestWorkflowHappyPath(t *testing.T) {
	store := memory.NewStore()
	service := application.NewService(store, application.SimulatedExecutor{}, "reviewer-1", time.Minute, time.Second, 3)

	employee := domain.Actor{ID: "alice", Role: domain.RoleEmployee}
	reviewer := domain.Actor{ID: "reviewer-1", Role: domain.RoleReviewer}

	req, err := service.CreateDraft(context.Background(), employee, application.CreateDraftInput{
		Type:           "system_access",
		TargetResource: "crm",
		Justification:  "need support access",
	})
	if err != nil {
		t.Fatalf("CreateDraft() error = %v", err)
	}

	req, err = service.SubmitRequest(context.Background(), employee, req.ID, "submit-1")
	if err != nil {
		t.Fatalf("SubmitRequest() error = %v", err)
	}
	if req.Status != domain.StatusPendingReview {
		t.Fatalf("SubmitRequest() status = %s, want %s", req.Status, domain.StatusPendingReview)
	}

	req, err = service.ApproveRequest(context.Background(), reviewer, req.ID, "approve-1", application.ReviewInput{Comment: "looks good"})
	if err != nil {
		t.Fatalf("ApproveRequest() error = %v", err)
	}
	if req.Status != domain.StatusApproved {
		t.Fatalf("ApproveRequest() status = %s, want %s", req.Status, domain.StatusApproved)
	}

	if err := service.ProcessDueExecutions(context.Background(), 10); err != nil {
		t.Fatalf("ProcessDueExecutions() error = %v", err)
	}

	req, err = service.GetRequest(context.Background(), employee, req.ID)
	if err != nil {
		t.Fatalf("GetRequest() error = %v", err)
	}
	if req.Status != domain.StatusExecuted {
		t.Fatalf("final status = %s, want %s", req.Status, domain.StatusExecuted)
	}

	audit, err := service.ListAudit(context.Background(), employee, req.ID)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(audit) < 5 {
		t.Fatalf("audit entries = %d, want at least 5", len(audit))
	}
}

func TestSubmitIdempotencyReplayReturnsCurrentState(t *testing.T) {
	store := memory.NewStore()
	service := application.NewService(store, application.SimulatedExecutor{}, "reviewer-1", time.Minute, time.Second, 3)
	employee := domain.Actor{ID: "alice", Role: domain.RoleEmployee}

	req, err := service.CreateDraft(context.Background(), employee, application.CreateDraftInput{
		Type:           "role_change",
		TargetResource: "billing-admin",
		Justification:  "temporary escalation",
	})
	if err != nil {
		t.Fatalf("CreateDraft() error = %v", err)
	}

	first, err := service.SubmitRequest(context.Background(), employee, req.ID, "idem-submit")
	if err != nil {
		t.Fatalf("first SubmitRequest() error = %v", err)
	}
	second, err := service.SubmitRequest(context.Background(), employee, req.ID, "idem-submit")
	if err != nil {
		t.Fatalf("second SubmitRequest() error = %v", err)
	}
	if first.ID != second.ID || second.Status != domain.StatusPendingReview {
		t.Fatalf("second submit returned %+v, want same request in pending_review", second)
	}
}

func TestReviewerCannotSelfApprove(t *testing.T) {
	store := memory.NewStore()
	service := application.NewService(store, application.SimulatedExecutor{}, "reviewer-1", time.Minute, time.Second, 3)
	requester := domain.Actor{ID: "reviewer-1", Role: domain.RoleEmployee}
	reviewer := domain.Actor{ID: "reviewer-1", Role: domain.RoleReviewer}

	req, err := service.CreateDraft(context.Background(), requester, application.CreateDraftInput{
		Type:           "system_access",
		TargetResource: "finance",
		Justification:  "support rotation",
	})
	if err != nil {
		t.Fatalf("CreateDraft() error = %v", err)
	}
	if _, err := service.SubmitRequest(context.Background(), requester, req.ID, "submit-self"); err != nil {
		t.Fatalf("SubmitRequest() error = %v", err)
	}
	if _, err := service.ApproveRequest(context.Background(), reviewer, req.ID, "approve-self", application.ReviewInput{}); err == nil {
		t.Fatal("ApproveRequest() error = nil, want forbidden")
	}
}

func TestExecutionFailureSchedulesRetry(t *testing.T) {
	store := memory.NewStore()
	service := application.NewService(store, application.SimulatedExecutor{}, "reviewer-1", time.Minute, 2*time.Minute, 3)
	start := time.Now().UTC()

	employee := domain.Actor{ID: "alice", Role: domain.RoleEmployee}
	reviewer := domain.Actor{ID: "reviewer-1", Role: domain.RoleReviewer}

	req, err := service.CreateDraft(context.Background(), employee, application.CreateDraftInput{
		Type:           "system_access",
		TargetResource: "fail-erp",
		Justification:  "test retry",
	})
	if err != nil {
		t.Fatalf("CreateDraft() error = %v", err)
	}
	if _, err := service.SubmitRequest(context.Background(), employee, req.ID, "submit-2"); err != nil {
		t.Fatalf("SubmitRequest() error = %v", err)
	}
	if _, err := service.ApproveRequest(context.Background(), reviewer, req.ID, "approve-2", application.ReviewInput{}); err != nil {
		t.Fatalf("ApproveRequest() error = %v", err)
	}
	if err := service.ProcessDueExecutions(context.Background(), 10); err != nil {
		t.Fatalf("ProcessDueExecutions() error = %v", err)
	}

	req, err = store.GetRequest(context.Background(), req.ID)
	if err != nil {
		t.Fatalf("GetRequest() error = %v", err)
	}
	if req.Status != domain.StatusExecutionFailed {
		t.Fatalf("status = %s, want %s", req.Status, domain.StatusExecutionFailed)
	}
	if req.NextExecutionAt == nil {
		t.Fatal("NextExecutionAt = nil, want retry timestamp")
	}
	min := start.Add(2 * time.Minute).Add(-5 * time.Second)
	max := time.Now().UTC().Add(2 * time.Minute).Add(5 * time.Second)
	if req.NextExecutionAt.Before(min) || req.NextExecutionAt.After(max) {
		t.Fatalf("NextExecutionAt = %v, want between %v and %v", req.NextExecutionAt, min, max)
	}
}
