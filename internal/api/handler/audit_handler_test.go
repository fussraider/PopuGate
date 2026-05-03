package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func setupAuditTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	auditStore := store.NewAuditStore(db)
	auditSvc := service.NewAuditService(auditStore)
	handler := NewAuditHandler(auditSvc)

	r := gin.New()
	r.GET("/api/v1/audit", handler.List)
	return r
}

func TestAuditHandler_List_Empty(t *testing.T) {
	r := setupAuditTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var entries []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty list, got %d items", len(entries))
	}
}

func TestAuditHandler_List_WithPagination(t *testing.T) {
	r := setupAuditTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit?limit=5&offset=0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
