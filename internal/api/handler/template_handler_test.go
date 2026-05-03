package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func setupTemplateTestRouter(t *testing.T) (*gin.Engine, *store.SecretStore) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	tmplStore := store.NewTemplateStore(db)
	secStore := store.NewSecretStore(db)
	tmplSvc := service.NewTemplateService(tmplStore, secStore)
	h := NewTemplateHandler(tmplSvc)

	r := gin.New()
	r.GET("/api/v1/templates", h.List)
	r.GET("/api/v1/templates/:name", h.Get)
	r.POST("/api/v1/templates", h.Create)
	r.DELETE("/api/v1/templates/:name", h.Delete)
	r.POST("/api/v1/templates/:name/apply", h.Apply)
	return r, secStore
}

func TestTemplateHandler_List_Empty(t *testing.T) {
	r, _ := setupTemplateTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/templates", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var list []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

func TestTemplateHandler_CreateAndGet(t *testing.T) {
	r, _ := setupTemplateTestRouter(t)

	body := `{"name":"basic","max_conns":5,"max_ips":3,"quota_bytes":1024,"expires_days":30}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var tmpl model.SecretTemplate
	if err := json.Unmarshal(w.Body.Bytes(), &tmpl); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if tmpl.Name != "basic" {
		t.Errorf("name = %q, want basic", tmpl.Name)
	}

	// Get by name
	req = httptest.NewRequest(http.MethodGet, "/api/v1/templates/basic", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Get: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTemplateHandler_Get_NotFound(t *testing.T) {
	r, _ := setupTemplateTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/templates/ghost", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTemplateHandler_Create_EmptyName(t *testing.T) {
	r, _ := setupTemplateTestRouter(t)

	body := `{"name":"","max_conns":5}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTemplateHandler_Delete(t *testing.T) {
	r, _ := setupTemplateTestRouter(t)

	// Create first
	body := `{"name":"temp","max_conns":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}

	// Delete
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/templates/temp", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify gone
	req = httptest.NewRequest(http.MethodGet, "/api/v1/templates/temp", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("after delete: expected 404, got %d", w.Code)
	}
}

func TestTemplateHandler_Delete_NotFound(t *testing.T) {
	r, _ := setupTemplateTestRouter(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/templates/ghost", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTemplateHandler_Apply(t *testing.T) {
	r, secStore := setupTemplateTestRouter(t)
	ctx := context.Background()

	// Create a secret
	secStore.Create(ctx, &model.Secret{
		Label: "user1", SecretKey: "aa000000000000000000000000000000", Enabled: true,
	})

	// Create template
	body := `{"name":"premium","max_conns":50,"max_ips":10,"quota_bytes":1048576,"expires_days":90}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create template: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Apply
	applyBody := `{"secret_label":"user1"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/templates/premium/apply", strings.NewReader(applyBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("apply: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify limits applied
	sec, _ := secStore.GetByLabel(ctx, "user1")
	if sec.MaxConns != 50 {
		t.Errorf("MaxConns = %d, want 50", sec.MaxConns)
	}
}

func TestTemplateHandler_Apply_TemplateNotFound(t *testing.T) {
	r, secStore := setupTemplateTestRouter(t)
	ctx := context.Background()

	secStore.Create(ctx, &model.Secret{
		Label: "user1", SecretKey: "aa000000000000000000000000000000", Enabled: true,
	})

	applyBody := `{"secret_label":"user1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/templates/ghost/apply", strings.NewReader(applyBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
