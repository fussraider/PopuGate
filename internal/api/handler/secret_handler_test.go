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

func setupSecretTestRouter(t *testing.T) (*gin.Engine, *SecretHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	secretStore := store.NewSecretStore(db)
	settingsStore := store.NewSettingsStore(db)
	secretSvc := service.NewSecretService(secretStore)
	handler := NewSecretHandler(secretSvc, settingsStore)

	r := gin.New()
	r.GET("/api/v1/secrets", handler.List)
	r.POST("/api/v1/secrets", handler.Add)
	r.GET("/api/v1/secrets/:label", handler.Get)
	r.DELETE("/api/v1/secrets/:label", handler.Remove)
	r.PUT("/api/v1/secrets/:label/toggle", handler.Toggle)
	r.PUT("/api/v1/secrets/:label/limits", handler.SetLimits)
	r.GET("/api/v1/secrets/:label/limits", handler.GetLimits)
	r.PUT("/api/v1/secrets/:label/notes", handler.UpdateNotes)
	r.POST("/api/v1/secrets/:label/reset-traffic", handler.ResetTraffic)
	r.POST("/api/v1/secrets/reset-traffic", handler.ResetAllTraffic)

	return r, handler
}

// --- parseHumanBytes tests ---

func TestParseHumanBytes(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"", 0},
		{"0", 0},
		{"5G", 5 * 1024 * 1024 * 1024},
		{"5g", 5 * 1024 * 1024 * 1024},
		{"500M", 500 * 1024 * 1024},
		{"500m", 500 * 1024 * 1024},
		{"100K", 100 * 1024},
		{"100k", 100 * 1024},
		{"1024", 1024},
		{"-5G", -1},
		{"-1", -1},
		{"abc", -1},
		{"5X", -1},   // unknown unit
		{"10GB", -1}, // GB not a recognized unit (only G)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseHumanBytes(tt.input)
			if got != tt.want {
				t.Errorf("parseHumanBytes(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// --- SecretHandler endpoint tests ---

func TestSecretHandler_List_Empty(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var secrets []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &secrets); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(secrets) != 0 {
		t.Errorf("expected empty list, got %d items", len(secrets))
	}
}

func TestSecretHandler_Add_Valid(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	body, _ := json.Marshal(map[string]string{
		"label": "testuser",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["label"] != "testuser" {
		t.Errorf("expected label 'testuser', got %v", resp["label"])
	}
	if resp["secret_key"] == nil || resp["secret_key"] == "" {
		t.Error("expected non-empty secret_key")
	}
}

func TestSecretHandler_Add_DuplicateLabel(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	// Add first
	body, _ := json.Marshal(map[string]string{"label": "dup"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("first add: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Add duplicate
	body, _ = json.Marshal(map[string]string{"label": "dup"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for duplicate, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSecretHandler_Add_InvalidLabel(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	tests := []struct {
		name  string
		label string
	}{
		{"empty", ""},
		{"spaces", "bad label"},
		{"special chars", "bad!label"},
		{"too long", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"label": tt.label})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for label %q, got %d: %s", tt.label, w.Code, w.Body.String())
			}
		})
	}
}

func TestSecretHandler_Get(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	// Add a secret first
	body, _ := json.Marshal(map[string]string{"label": "alice"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("add: expected 201, got %d", w.Code)
	}

	// Get the secret
	req = httptest.NewRequest(http.MethodGet, "/api/v1/secrets/alice", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["label"] != "alice" {
		t.Errorf("expected label 'alice', got %v", resp["label"])
	}
}

func TestSecretHandler_Get_NotFound(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSecretHandler_Remove(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	// Add two secrets so we can remove one
	for _, label := range []string{"user1", "user2"} {
		body, _ := json.Marshal(map[string]string{"label": label})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("add %s: expected 201, got %d", label, w.Code)
		}
	}

	// Remove user1 with force
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/user1?force=true", nil)
	w := httptest.NewRecorder()
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

func TestSecretHandler_Remove_NotFound(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	// Add one secret
	body, _ := json.Marshal(map[string]string{"label": "keep"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Try to remove nonexistent
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/nonexistent?force=true", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSecretHandler_Toggle(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	// Add two secrets so we can toggle one
	for _, label := range []string{"toggle1", "toggle2"} {
		body, _ := json.Marshal(map[string]string{"label": label})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}

	// Disable toggle1
	body, _ := json.Marshal(map[string]interface{}{"enabled": false})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/secrets/toggle1/toggle", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
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

func TestSecretHandler_Toggle_InvalidBody(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/secrets/test/toggle", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSecretHandler_Toggle_DisableLastEnabled(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	// Add only one secret
	body, _ := json.Marshal(map[string]string{"label": "only"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Try to disable the last enabled secret
	body, _ = json.Marshal(map[string]interface{}{"enabled": false})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/secrets/only/toggle", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSecretHandler_SetLimits(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	// Add a secret first
	body, _ := json.Marshal(map[string]string{"label": "limited"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Set limits
	maxConns := 10
	maxIPs := 5
	quotaBytes := int64(1073741824) // 1GB
	body, _ = json.Marshal(map[string]interface{}{
		"max_conns":   maxConns,
		"max_ips":     maxIPs,
		"quota_bytes": quotaBytes,
		"expires_at":  "2030-01-01",
	})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/secrets/limited/limits", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSecretHandler_SetLimits_WithHumanQuota(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	// Add a secret
	body, _ := json.Marshal(map[string]string{"label": "quota"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Set limits with human-readable quota
	body, _ = json.Marshal(map[string]interface{}{
		"quota": "5G",
	})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/secrets/quota/limits", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSecretHandler_SetLimits_NotFound(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	maxConns := 10
	body, _ := json.Marshal(map[string]interface{}{
		"max_conns": maxConns,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/secrets/nonexistent/limits", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSecretHandler_GetLimits(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	// Add a secret
	body, _ := json.Marshal(map[string]string{"label": "limuser"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Get limits
	req = httptest.NewRequest(http.MethodGet, "/api/v1/secrets/limuser/limits", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["label"] != "limuser" {
		t.Errorf("expected label 'limuser', got %v", resp["label"])
	}
}

func TestSecretHandler_GetLimits_NotFound(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets/nonexistent/limits", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSecretHandler_UpdateNotes(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	// Add a secret
	body, _ := json.Marshal(map[string]string{"label": "notes"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Update notes
	body, _ = json.Marshal(map[string]string{"notes": "test notes"})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/secrets/notes/notes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["notes"] != "test notes" {
		t.Errorf("expected notes 'test notes', got %v", resp["notes"])
	}
}

func TestSecretHandler_UpdateNotes_NotFound(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	body, _ := json.Marshal(map[string]string{"notes": "test"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/secrets/nonexistent/notes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSecretHandler_ResetTraffic(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	// Add a secret
	body, _ := json.Marshal(map[string]string{"label": "reset"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Reset traffic
	req = httptest.NewRequest(http.MethodPost, "/api/v1/secrets/reset/reset-traffic", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSecretHandler_ResetAllTraffic(t *testing.T) {
	r, _ := setupSecretTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/secrets/reset-traffic", nil)
	w := httptest.NewRecorder()
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
