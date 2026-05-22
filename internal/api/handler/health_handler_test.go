package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHealthHandler_Check_WithoutService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHealthHandler()

	r := gin.New()
	r.GET("/api/v1/health", handler.Check)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}
	if _, ok := resp["version"]; !ok {
		t.Error("expected 'version' field")
	}
	if _, ok := resp["commit"]; !ok {
		t.Error("expected 'commit' field")
	}
	if _, ok := resp["version_url"]; !ok {
		t.Error("expected 'version_url' field")
	}
	// Without healthSvc, docker/container/port/metrics should NOT be present
	if _, ok := resp["docker"]; ok {
		t.Error("did not expect 'docker' field without healthSvc")
	}
}

func TestHealthHandler_Check_WithService_Fields(t *testing.T) {
	// When healthSvc is set, the response should include docker/container/port/metrics.
	// We can't create a real HealthService without Docker, but we can verify that
	// the code path works by checking that the handler calls healthSvc.Check().
	// This test is covered by integration tests with a running Docker daemon.
	// Here we just verify the no-service path includes the expected fields.
	t.Skip("requires Docker daemon for HealthService")
}

func TestHealthHandler_Check_ResponseFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHealthHandler()

	r := gin.New()
	r.GET("/health", handler.Check)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify response has correct content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json; charset=utf-8" {
		t.Errorf("expected JSON content type, got %q", contentType)
	}
}

func TestHealthHandler_Check_VersionFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHealthHandler()

	r := gin.New()
	r.GET("/health", handler.Check)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	// These fields should always be present
	requiredFields := []string{"status", "version", "commit", "version_url"}
	for _, field := range requiredFields {
		if _, ok := resp[field]; !ok {
			t.Errorf("expected '%s' field in response", field)
		}
	}
}
