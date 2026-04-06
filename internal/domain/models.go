package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Role string

const (
	RoleEmployee Role = "employee"
	RoleReviewer Role = "reviewer"
	RoleAdmin    Role = "admin"
)

type Status string

const (
	StatusDraft           Status = "draft"
	StatusSubmitted       Status = "submitted"
	StatusPendingReview   Status = "pending_review"
	StatusApproved        Status = "approved"
	StatusRejected        Status = "rejected"
	StatusExecuted        Status = "executed"
	StatusExecutionFailed Status = "execution_failed"
)

type Actor struct {
	ID   string `json:"id"`
	Role Role   `json:"role"`
}

type Request struct {
	ID                 string     `json:"id"`
	Type               string     `json:"type"`
	TargetResource     string     `json:"target_resource"`
	Justification      string     `json:"justification"`
	RequesterID        string     `json:"requester_id"`
	AssignedReviewerID string     `json:"assigned_reviewer_id"`
	Status             Status     `json:"status"`
	Version            int        `json:"version"`
	ReminderCount      int        `json:"reminder_count"`
	ExecutionAttempts  int        `json:"execution_attempts"`
	LastExecutionError string     `json:"last_execution_error,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	SubmittedAt        *time.Time `json:"submitted_at,omitempty"`
	ApprovedAt         *time.Time `json:"approved_at,omitempty"`
	ReviewedAt         *time.Time `json:"reviewed_at,omitempty"`
	ExecutedAt         *time.Time `json:"executed_at,omitempty"`
	LastRemindedAt     *time.Time `json:"last_reminded_at,omitempty"`
	NextExecutionAt    *time.Time `json:"next_execution_at,omitempty"`
}

type AuditEntry struct {
	ID          string    `json:"id"`
	RequestID   string    `json:"request_id"`
	ActorID     string    `json:"actor_id"`
	ActorRole   Role      `json:"actor_role"`
	Action      string    `json:"action"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type ReviewAction struct {
	ID        string    `json:"id"`
	RequestID string    `json:"request_id"`
	Decision  string    `json:"decision"`
	Comment   string    `json:"comment"`
	ActorID   string    `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}

type JobRun struct {
	ID         string    `json:"id"`
	RequestID  string    `json:"request_id"`
	JobType    string    `json:"job_type"`
	Status     string    `json:"status"`
	Attempt    int       `json:"attempt"`
	Message    string    `json:"message"`
	ExecutedAt time.Time `json:"executed_at"`
}

type IdempotencyRecord struct {
	Key         string    `json:"key"`
	Scope       string    `json:"scope"`
	Fingerprint string    `json:"fingerprint"`
	RequestID   string    `json:"request_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type RequestFilter struct {
	RequesterID string
	ReviewerID  string
	Status      Status
}

func ValidateActor(actor Actor) error {
	if strings.TrimSpace(actor.ID) == "" {
		return errors.New("actor id is required")
	}
	switch actor.Role {
	case RoleEmployee, RoleReviewer, RoleAdmin:
		return nil
	default:
		return fmt.Errorf("invalid actor role %q", actor.Role)
	}
}

func NewID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		now := time.Now().UnixNano()
		return fmt.Sprintf("%s_%d", prefix, now)
	}
	return prefix + "_" + hex.EncodeToString(buf)
}
