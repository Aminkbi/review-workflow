package memory

import (
	"context"
	"testing"
	"time"

	"review-workflow/internal/domain"
)

func TestUpdateRequestOptimisticLocking(t *testing.T) {
	store := NewStore()
	req := domain.Request{
		ID:          "req_1",
		Type:        "system_access",
		Status:      domain.StatusDraft,
		RequesterID: "alice",
		Version:     1,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.CreateRequest(context.Background(), req); err != nil {
		t.Fatalf("CreateRequest() error = %v", err)
	}

	req.Version = 2
	if err := store.UpdateRequest(context.Background(), req, 99); err == nil {
		t.Fatal("UpdateRequest() error = nil, want conflict")
	}
}
