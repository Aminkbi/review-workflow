package httptransport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"review-workflow/internal/application"
	"review-workflow/internal/domain"
)

type Server struct {
	service     *application.Service
	ready       func(context.Context) error
	serviceName string
}

func NewServer(service *application.Service, ready func(context.Context) error, serviceName string) *Server {
	if strings.TrimSpace(serviceName) == "" {
		serviceName = "review-workflow"
	}
	return &Server{service: service, ready: ready, serviceName: serviceName}
}

func (s *Server) Routes() http.Handler {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(otelgin.Middleware(s.serviceName))
	registerUI(router)

	router.GET("/healthz", s.handleHealth)
	router.GET("/readyz", s.handleReady)

	v1 := router.Group("/v1")
	{
		v1.POST("/requests", s.handleCreateRequest)
		v1.GET("/requests", s.handleListRequests)
		v1.GET("/requests/:id", s.handleGetRequest)
		v1.POST("/requests/:id/submit", s.handleSubmitRequest)
		v1.POST("/requests/:id/approve", s.handleApproveRequest)
		v1.POST("/requests/:id/reject", s.handleRejectRequest)
		v1.POST("/requests/:id/execution-result", s.handleExecutionResult)
		v1.GET("/requests/:id/audit", s.handleAudit)
	}

	return router
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleReady(c *gin.Context) {
	if s.ready != nil {
		if err := s.ready(c.Request.Context()); err != nil {
			writeError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func (s *Server) handleCreateRequest(c *gin.Context) {
	actor, err := actorFromRequest(c.Request)
	if err != nil {
		writeError(c, err)
		return
	}
	var input application.CreateDraftInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, fmt.Errorf("%w: %v", application.ErrInvalidInput, err))
		return
	}
	req, err := s.service.CreateDraft(c.Request.Context(), actor, input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, req)
}

func (s *Server) handleListRequests(c *gin.Context) {
	actor, err := actorFromRequest(c.Request)
	if err != nil {
		writeError(c, err)
		return
	}
	filter := domain.RequestFilter{
		RequesterID: strings.TrimSpace(c.Query("requester_id")),
		ReviewerID:  strings.TrimSpace(c.Query("reviewer_id")),
		Status:      domain.Status(strings.TrimSpace(c.Query("status"))),
	}
	requests, err := s.service.ListRequests(c.Request.Context(), actor, filter)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": requests})
}

func (s *Server) handleGetRequest(c *gin.Context) {
	actor, err := actorFromRequest(c.Request)
	if err != nil {
		writeError(c, err)
		return
	}
	req, err := s.service.GetRequest(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, req)
}

func (s *Server) handleSubmitRequest(c *gin.Context) {
	actor, err := actorFromRequest(c.Request)
	if err != nil {
		writeError(c, err)
		return
	}
	req, err := s.service.SubmitRequest(c.Request.Context(), actor, c.Param("id"), strings.TrimSpace(c.GetHeader("Idempotency-Key")))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, req)
}

func (s *Server) handleApproveRequest(c *gin.Context) {
	actor, err := actorFromRequest(c.Request)
	if err != nil {
		writeError(c, err)
		return
	}
	var input application.ReviewInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, fmt.Errorf("%w: %v", application.ErrInvalidInput, err))
		return
	}
	req, err := s.service.ApproveRequest(c.Request.Context(), actor, c.Param("id"), strings.TrimSpace(c.GetHeader("Idempotency-Key")), input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, req)
}

func (s *Server) handleRejectRequest(c *gin.Context) {
	actor, err := actorFromRequest(c.Request)
	if err != nil {
		writeError(c, err)
		return
	}
	var input application.ReviewInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, fmt.Errorf("%w: %v", application.ErrInvalidInput, err))
		return
	}
	req, err := s.service.RejectRequest(c.Request.Context(), actor, c.Param("id"), strings.TrimSpace(c.GetHeader("Idempotency-Key")), input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, req)
}

func (s *Server) handleExecutionResult(c *gin.Context) {
	actor, err := actorFromRequest(c.Request)
	if err != nil {
		writeError(c, err)
		return
	}
	var input application.ExecutionResultInput
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, fmt.Errorf("%w: %v", application.ErrInvalidInput, err))
		return
	}
	req, err := s.service.RecordExecutionResult(c.Request.Context(), actor, c.Param("id"), input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, req)
}

func (s *Server) handleAudit(c *gin.Context) {
	actor, err := actorFromRequest(c.Request)
	if err != nil {
		writeError(c, err)
		return
	}
	entries, err := s.service.ListAudit(c.Request.Context(), actor, c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": entries})
}

func actorFromRequest(r *http.Request) (domain.Actor, error) {
	actor := domain.Actor{
		ID:   strings.TrimSpace(r.Header.Get("X-Actor-Id")),
		Role: domain.Role(strings.TrimSpace(r.Header.Get("X-Actor-Role"))),
	}
	if err := domain.ValidateActor(actor); err != nil {
		return domain.Actor{}, fmt.Errorf("%w: %v", application.ErrUnauthorized, err)
	}
	return actor, nil
}

func writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, application.ErrInvalidInput):
		status = http.StatusBadRequest
	case errors.Is(err, application.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, application.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, application.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, application.ErrConflict), errors.Is(err, application.ErrIdempotencyKeyUsed):
		status = http.StatusConflict
	}
	c.JSON(status, gin.H{"error": err.Error()})
}
