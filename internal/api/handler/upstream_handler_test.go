package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func setupUpstreamTestRouter(t *testing.T) (*gin.Engine, *UpstreamHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	upstreamStore := store.NewUpstreamStore(db)
	upstreamSvc := service.NewUpstreamService(upstreamStore)
	handler := NewUpstreamHandler(upstreamSvc)

	r := gin.New()
	r.GET("/api/v1/upstreams", handler.List)
	r.POST("/api/v1/upstreams", handler.Add)
	r.DELETE("/api/v1/upstreams/:name", handler.Remove)
	r.PUT("/api/v1/upstreams/:name", handler.Update)
	r.PUT("/api/v1/upstreams/:name/toggle", handler.Toggle)

	return r, handler
}

func TestUpstreamHandler_List_Empty(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/upstreams", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var upstreams []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &upstreams); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(upstreams) != 0 {
		t.Errorf("expected empty list, got %d items", len(upstreams))
	}
}

func TestUpstreamHandler_Add_ValidDirect(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"name":   "mydirect",
		"type":   "direct",
		"weight": 10,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["name"] != "mydirect" {
		t.Errorf("expected name 'mydirect', got %v", resp["name"])
	}
	if resp["type"] != "direct" {
		t.Errorf("expected type 'direct', got %v", resp["type"])
	}
}

func TestUpstreamHandler_Add_ValidSOCKS5(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"name":     "mysocks5",
		"type":     "socks5",
		"address":  "127.0.0.1:1080",
		"username": "user",
		"password": "pass",
		"weight":   20,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpstreamHandler_Add_DuplicateName(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	// Add first
	body, _ := json.Marshal(map[string]interface{}{
		"name": "dup", "type": "direct", "weight": 10,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("first add: expected 201, got %d", w.Code)
	}

	// Add duplicate
	body, _ = json.Marshal(map[string]interface{}{
		"name": "dup", "type": "direct", "weight": 10,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for duplicate, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpstreamHandler_Add_InvalidType(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "bad", "type": "http", "weight": 10,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpstreamHandler_Add_MissingName(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"type": "direct", "weight": 10,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpstreamHandler_Add_InvalidWeight(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "badweight", "type": "direct", "weight": 200,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpstreamHandler_Add_SOCKS5MissingAddress(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "nosocks", "type": "socks5", "weight": 10,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing address, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpstreamHandler_Remove(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	// Add first
	body, _ := json.Marshal(map[string]interface{}{
		"name": "toremove", "type": "direct", "weight": 10,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("add: expected 201, got %d", w.Code)
	}

	// Remove
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/upstreams/toremove", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Error("expected ok=true")
	}
}

func TestUpstreamHandler_Remove_NotFound(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/upstreams/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpstreamHandler_Toggle(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	// Add first
	body, _ := json.Marshal(map[string]interface{}{
		"name": "toggleme", "type": "direct", "weight": 10,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Toggle off
	body, _ = json.Marshal(map[string]interface{}{"enabled": false})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/upstreams/toggleme/toggle", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Error("expected ok=true")
	}
}

func TestUpstreamHandler_Toggle_NotFound(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{"enabled": true})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/upstreams/nonexistent/toggle", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpstreamHandler_Toggle_InvalidBody(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/upstreams/test/toggle", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpstreamHandler_Update(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	// Add first
	body, _ := json.Marshal(map[string]interface{}{
		"name": "toupdate", "type": "direct", "weight": 10,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Update
	body, _ = json.Marshal(map[string]interface{}{
		"type":    "socks5",
		"address": "10.0.0.1:1080",
		"weight":  50,
	})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/upstreams/toupdate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["type"] != "socks5" {
		t.Errorf("expected type 'socks5', got %v", resp["type"])
	}
}

func TestUpstreamHandler_Update_NotFound(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"type": "direct",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/upstreams/nonexistent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpstreamHandler_Update_InvalidType(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	// Add first
	body, _ := json.Marshal(map[string]interface{}{
		"name": "upd", "type": "direct", "weight": 10,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Try to update with invalid type
	body, _ = json.Marshal(map[string]interface{}{
		"type": "invalid",
	})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/upstreams/upd", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
