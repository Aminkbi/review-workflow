package httptransport

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"review-workflow/internal/application"
	"review-workflow/internal/domain"
	"review-workflow/internal/repository/memory"
)

func TestServerCreateAndListRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := memory.NewStore()
	service := application.NewService(store, application.SimulatedExecutor{}, "reviewer-1", time.Minute, time.Second, 3)
	server := NewServer(service, nil, "review-workflow")
	handler := server.Routes()

	createReq := httptest.NewRequest(http.MethodPost, "/v1/requests", bytes.NewBufferString(`{
		"type":"system_access",
		"target_resource":"crm",
		"justification":"need access"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-Actor-Id", "alice")
	createReq.Header.Set("X-Actor-Role", string(domain.RoleEmployee))

	createResp := httptest.NewRecorder()
	handler.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d", createResp.Code, http.StatusCreated)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/requests", nil)
	listReq.Header.Set("X-Actor-Id", "alice")
	listReq.Header.Set("X-Actor-Role", string(domain.RoleEmployee))

	listResp := httptest.NewRecorder()
	handler.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listResp.Code, http.StatusOK)
	}
}

func TestServerRequiresActorHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := memory.NewStore()
	service := application.NewService(store, application.SimulatedExecutor{}, "reviewer-1", time.Minute, time.Second, 3)
	server := NewServer(service, nil, "review-workflow")

	req := httptest.NewRequest(http.MethodGet, "/v1/requests", nil)
	resp := httptest.NewRecorder()
	server.Routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
}

func TestServerServesUI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := memory.NewStore()
	service := application.NewService(store, application.SimulatedExecutor{}, "reviewer-1", time.Minute, time.Second, 3)
	server := NewServer(service, nil, "review-workflow")

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	server.Routes().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
	if body := resp.Body.String(); !bytes.Contains([]byte(body), []byte("Workflow Showcase")) {
		t.Fatalf("body missing expected ui marker: %s", body)
	}
}
