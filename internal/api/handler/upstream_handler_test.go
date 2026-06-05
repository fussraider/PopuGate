package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	r.POST("/api/v1/upstreams/bulk-check", handler.BulkCheck)
	r.POST("/api/v1/upstreams/bulk", handler.BulkAdd)

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
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
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
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
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
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
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
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
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

func TestUpstreamHandler_AutoDisabledFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	upstreamStore := store.NewUpstreamStore(db)
	upstreamSvc := service.NewUpstreamService(upstreamStore)
	handler := NewUpstreamHandler(upstreamSvc)

	r := gin.New()
	r.GET("/api/v1/upstreams", handler.List)
	r.POST("/api/v1/upstreams", handler.Add)

	// Add an upstream manually
	body, _ := json.Marshal(map[string]interface{}{
		"name": "auto-check", "type": "direct", "weight": 10,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("add: expected 201, got %d", w.Code)
	}

	// Trigger automatic disablement via store
	ctx := context.Background()
	_ = upstreamStore.DisableAutomatically(ctx, "auto-check", 987654321)

	// Fetch upstreams list
	req = httptest.NewRequest(http.MethodGet, "/api/v1/upstreams", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp []map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp) != 1 {
		t.Fatalf("expected 1 upstream in list, got %d", len(resp))
	}

	u := resp[0]
	if u["auto_disabled"] != true {
		t.Errorf("expected auto_disabled=true, got %v", u["auto_disabled"])
	}
	if u["auto_disabled_at"] != float64(987654321) {
		t.Errorf("expected auto_disabled_at=987654321, got %v", u["auto_disabled_at"])
	}
}

func TestUpstreamHandler_BulkCheck(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"proxies": []string{
			"socks5://user:pass@127.0.0.1:1080",
			"invalid-line-missing-port",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams/bulk-check", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		t.Errorf("expected text/event-stream, got %q", contentType)
	}

	respStr := w.Body.String()
	if !strings.Contains(respStr, "invalid-line-missing-port") {
		t.Errorf("expected error notification in stream, got %q", respStr)
	}
}

func TestUpstreamHandler_BulkAdd(t *testing.T) {
	r, _ := setupUpstreamTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"upstreams": []map[string]interface{}{
			{
				"type":    "socks5",
				"address": "1.1.1.1:1080",
				"weight":  15,
			},
			{
				"type":    "socks4",
				"address": "2.2.2.2:1080",
				"weight":  25,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams/bulk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["ok"] != true {
		t.Errorf("expected ok=true, got %v", resp["ok"])
	}
	if resp["count"] != float64(2) {
		t.Errorf("expected count=2, got %v", resp["count"])
	}
	names, ok := resp["names"].([]interface{})
	if !ok || len(names) != 2 {
		t.Errorf("expected names list in response of length 2, got %v", resp["names"])
	}
}

func TestUpstreamHandler_WithContainerService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	upstreamStore := store.NewUpstreamStore(db)
	upstreamSvc := service.NewUpstreamService(upstreamStore)
	handler := NewUpstreamHandler(upstreamSvc)

	// Create test container service
	instances := store.NewInstanceStore(db)
	secrets := store.NewSecretStore(db)
	settings := store.NewSettingsStore(db)
	traffic := store.NewTrafficStore(db)
	containerSvc := service.NewContainerService(t.TempDir(), nil, secrets, upstreamStore, instances, traffic, settings, nil)

	handler.SetContainerSvc(containerSvc)

	r := gin.New()
	r.POST("/api/v1/upstreams", handler.Add)

	body, _ := json.Marshal(addUpstreamRequest{
		Name:    "test-upstream",
		Type:    "socks5",
		Address: "1.2.3.4:1080",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/upstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}


