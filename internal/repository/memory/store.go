package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"review-workflow/internal/application"
	"review-workflow/internal/domain"
)

type Store struct {
	mu          sync.RWMutex
	requests    map[string]domain.Request
	audit       map[string][]domain.AuditEntry
	reviews     map[string][]domain.ReviewAction
	idempotency map[string]domain.IdempotencyRecord
	jobRuns     map[string][]domain.JobRun
}

func NewStore() *Store {
	return &Store{
		requests:    make(map[string]domain.Request),
		audit:       make(map[string][]domain.AuditEntry),
		reviews:     make(map[string][]domain.ReviewAction),
		idempotency: make(map[string]domain.IdempotencyRecord),
		jobRuns:     make(map[string][]domain.JobRun),
	}
}

func (s *Store) CreateRequest(_ context.Context, req domain.Request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests[req.ID] = req
	return nil
}

func (s *Store) GetRequest(_ context.Context, id string) (domain.Request, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	req, ok := s.requests[id]
	if !ok {
		return domain.Request{}, application.ErrNotFound
	}
	return req, nil
}

func (s *Store) ListRequests(_ context.Context, filter domain.RequestFilter) ([]domain.Request, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var requests []domain.Request
	for _, req := range s.requests {
		if filter.RequesterID != "" && req.RequesterID != filter.RequesterID {
			continue
		}
		if filter.ReviewerID != "" && req.AssignedReviewerID != filter.ReviewerID {
			continue
		}
		if filter.Status != "" && req.Status != filter.Status {
			continue
		}
		requests = append(requests, req)
	}
	sort.Slice(requests, func(i, j int) bool {
		return requests[i].CreatedAt.Before(requests[j].CreatedAt)
	})
	return requests, nil
}

func (s *Store) UpdateRequest(_ context.Context, req domain.Request, expectedVersion int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.requests[req.ID]
	if !ok {
		return application.ErrNotFound
	}
	if current.Version != expectedVersion {
		return application.ErrConflict
	}
	s.requests[req.ID] = req
	return nil
}

func (s *Store) AppendAudit(_ context.Context, entry domain.AuditEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audit[entry.RequestID] = append(s.audit[entry.RequestID], entry)
	return nil
}

func (s *Store) ListAudit(_ context.Context, requestID string) ([]domain.AuditEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := append([]domain.AuditEntry(nil), s.audit[requestID]...)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt.Before(entries[j].CreatedAt)
	})
	return entries, nil
}

func (s *Store) CreateReview(_ context.Context, review domain.ReviewAction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reviews[review.RequestID] = append(s.reviews[review.RequestID], review)
	return nil
}

func (s *Store) GetIdempotency(_ context.Context, scope, key string) (domain.IdempotencyRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.idempotency[scope+"|"+key]
	if !ok {
		return domain.IdempotencyRecord{}, application.ErrNotFound
	}
	return record, nil
}

func (s *Store) CreateIdempotency(_ context.Context, record domain.IdempotencyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	compound := record.Scope + "|" + record.Key
	if _, ok := s.idempotency[compound]; ok {
		return application.ErrConflict
	}
	s.idempotency[compound] = record
	return nil
}

func (s *Store) ListRequestsNeedingReminder(_ context.Context, before time.Time, limit int) ([]domain.Request, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var results []domain.Request
	for _, req := range s.requests {
		if req.Status != domain.StatusPendingReview {
			continue
		}
		if req.SubmittedAt == nil || req.SubmittedAt.After(before) {
			continue
		}
		if req.LastRemindedAt != nil && !req.LastRemindedAt.Before(before) {
			continue
		}
		results = append(results, req)
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (s *Store) ListRequestsReadyForExecution(_ context.Context, before time.Time, limit int) ([]domain.Request, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var results []domain.Request
	for _, req := range s.requests {
		if req.Status != domain.StatusApproved && req.Status != domain.StatusExecutionFailed {
			continue
		}
		if req.NextExecutionAt == nil || req.NextExecutionAt.After(before) {
			continue
		}
		results = append(results, req)
		if limit > 0 && len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (s *Store) AppendJobRun(_ context.Context, run domain.JobRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobRuns[run.RequestID] = append(s.jobRuns[run.RequestID], run)
	return nil
}
