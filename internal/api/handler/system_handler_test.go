package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupSystemTestRouter(t *testing.T) (*gin.Engine, *SystemHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler := NewSystemHandler(nil)

	r := gin.New()
	r.GET("/api/v1/system/os", handler.GetOS)
	r.GET("/api/v1/system/service/status", handler.ServiceStatus)

	return r, handler
}

func TestSystemHandler_GetOS(t *testing.T) {
	r, _ := setupSystemTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/os", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Must contain family, version, and arch fields
	if resp["family"] == nil || resp["family"] == "" {
		t.Error("expected non-empty family field")
	}
	if resp["version"] == nil || resp["version"] == "" {
		t.Error("expected non-empty version field")
	}
	if resp["arch"] == nil || resp["arch"] == "" {
		t.Error("expected non-empty arch field")
	}
}

func TestSystemHandler_ServiceStatus(t *testing.T) {
	r, _ := setupSystemTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/service/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// On macOS (no systemd), status should indicate unsupported
	if active, ok := resp["active"].(string); ok {
		if active == "" {
			t.Error("expected non-empty active status")
		}
	}
}
