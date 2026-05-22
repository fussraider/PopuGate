package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func setupConfigTestRouter(t *testing.T) (*gin.Engine, *ConfigHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	handler := NewConfigHandler(settingsStore)

	r := gin.New()
	r.PUT("/config", handler.Update)
	r.GET("/config", handler.GetAll)
	r.GET("/config/:key", handler.GetKey)
	return r, handler
}

// --- GetAll tests ---

func TestConfigHandler_GetAll(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Verify key settings fields are present with defaults
	if _, ok := resp["proxy_port"]; !ok {
		t.Error("expected 'proxy_port' field in response")
	}
	if _, ok := resp["geoblock_mode"]; !ok {
		t.Error("expected 'geoblock_mode' field in response")
	}
	if _, ok := resp["proxy_domain"]; !ok {
		t.Error("expected 'proxy_domain' field in response")
	}

	// Verify default values
	if resp["geoblock_mode"] != "blacklist" {
		t.Errorf("expected geoblock_mode 'blacklist', got %v", resp["geoblock_mode"])
	}
}

// --- Update tests ---

func TestConfigUpdate_RejectsInternalKeys(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	tests := []struct {
		name   string
		key    string
		value  string
		wantOK bool
	}{
		{"jwt_secret rejected", "jwt_secret", "hacked", false},
		{"auth_password_hash rejected", "auth_password_hash", "hacked", false},
		{"proxy_port allowed", "proxy_port", "8443", true},
		{"debug allowed", "debug", "true", true},
		{"telegram_enabled allowed", "telegram_enabled", "true", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{tt.key: tt.value})
			req := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			var resp map[string]interface{}
			_ = json.Unmarshal(w.Body.Bytes(), &resp)

			if tt.wantOK {
				if w.Code != http.StatusOK {
					t.Errorf("expected 200, got %d: %v", w.Code, resp)
				}
				applied := resp["applied"].([]interface{})
				found := false
				for _, k := range applied {
					if k == tt.key {
						found = true
					}
				}
				if !found {
					t.Errorf("expected %q in applied keys, got %v", tt.key, applied)
				}
			} else {
				if w.Code == http.StatusOK {
					applied := resp["applied"].([]interface{})
					for _, k := range applied {
						if k == tt.key {
							t.Errorf("internal key %q should not be in applied", tt.key)
						}
					}
				}
				// Check it appears in rejected
				rejected, _ := resp["rejected"].([]interface{})
				found := false
				for _, k := range rejected {
					if k == tt.key {
						found = true
					}
				}
				if !found {
					t.Errorf("expected %q in rejected keys, got %v", tt.key, rejected)
				}
			}
		})
	}
}

func TestConfigUpdate_BoolType(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"debug": true,
	})
	req := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	applied := resp["applied"].([]interface{})
	found := false
	for _, k := range applied {
		if k == "debug" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'debug' in applied keys, got %v", applied)
	}

	// Verify the value was stored as "true"
	req2 := httptest.NewRequest(http.MethodGet, "/config/debug", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	var getResp map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &getResp)
	if getResp["value"] != "true" {
		t.Errorf("expected debug='true', got %v", getResp["value"])
	}
}

func TestConfigUpdate_FloatType(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	// JSON numbers are decoded as float64 in Go
	body, _ := json.Marshal(map[string]interface{}{
		"proxy_port": 8443,
	})
	req := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify stored as integer string
	req2 := httptest.NewRequest(http.MethodGet, "/config/proxy_port", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	var resp map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp["value"] != "8443" {
		t.Errorf("expected proxy_port='8443', got %v", resp["value"])
	}
}

func TestConfigUpdate_StringType(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"proxy_domain": "example.com",
	})
	req := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify stored value
	req2 := httptest.NewRequest(http.MethodGet, "/config/proxy_domain", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	var resp map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp["value"] != "example.com" {
		t.Errorf("expected proxy_domain='example.com', got %v", resp["value"])
	}
}

func TestConfigUpdate_AllOnlyInternalKeys_ReturnsBadRequest(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	body, _ := json.Marshal(map[string]string{
		"jwt_secret":         "leaked",
		"auth_password_hash": "leaked",
	})
	req := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "no valid settings provided" {
		t.Errorf("expected 'no valid settings provided', got %v", resp["error"])
	}
}

func TestConfigUpdate_NoValidKeys(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"nonexistent_key": "value",
		"another_bad_key": "value",
	})
	req := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "no valid settings provided" {
		t.Errorf("expected 'no valid settings provided', got %v", resp["error"])
	}
}

func TestConfigUpdate_MixedKeys_AppliesOnlyAllowed(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	body, _ := json.Marshal(map[string]string{
		"proxy_port":  "9090",
		"jwt_secret":  "hacked",
		"debug":       "true",
		"invalid_key": "whatever",
	})
	req := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	applied := resp["applied"].([]interface{})
	if len(applied) != 2 {
		t.Errorf("expected 2 applied keys, got %d: %v", len(applied), applied)
	}

	rejected := resp["rejected"].([]interface{})
	if len(rejected) != 2 {
		t.Errorf("expected 2 rejected keys, got %d: %v", len(rejected), rejected)
	}
}

func TestConfigUpdate_InvalidJSON(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	req := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader([]byte(`{invalid`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestConfigUpdate_NullValueSkipped(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	// null values should be skipped, not cause errors
	body, _ := json.Marshal(map[string]interface{}{
		"debug":      nil,
		"proxy_port": 443,
	})
	req := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	applied := resp["applied"].([]interface{})
	if len(applied) != 1 {
		t.Errorf("expected 1 applied key (null should be skipped), got %d: %v", len(applied), applied)
	}
	if applied[0] != "proxy_port" {
		t.Errorf("expected 'proxy_port' to be applied, got %v", applied[0])
	}
}

// --- GetKey tests ---

func TestConfigHandler_GetKey(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/config/geoblock_mode", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["key"] != "geoblock_mode" {
		t.Errorf("expected key='geoblock_mode', got %v", resp["key"])
	}
	if resp["value"] != "blacklist" {
		t.Errorf("expected value='blacklist', got %v", resp["value"])
	}
}

func TestConfigHandler_GetKey_UnknownKey(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/config/nonexistent_key", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["key"] != "nonexistent_key" {
		t.Errorf("expected key='nonexistent_key', got %v", resp["key"])
	}
	// Unknown keys return empty string (not an error)
	if resp["value"] != "" {
		t.Errorf("expected empty value for unknown key, got %v", resp["value"])
	}
}

func TestConfigHandler_GetKey_AfterUpdate(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	// Update a value first
	body, _ := json.Marshal(map[string]interface{}{
		"debug": true,
	})
	req := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Now retrieve it
	req2 := httptest.NewRequest(http.MethodGet, "/config/debug", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	var resp map[string]interface{}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp["value"] != "true" {
		t.Errorf("expected debug='true' after update, got %v", resp["value"])
	}
}

func TestConfigHandler_GetKey_RejectsSensitiveKeys(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	// Save jwt_secret first so it exists in the DB
	body, _ := json.Marshal(map[string]interface{}{
		"jwt_secret": "super-secret-value",
	})
	req := httptest.NewRequest(http.MethodPut, "/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Now try to read it back via GetKey — should be forbidden
	req2 := httptest.NewRequest(http.MethodGet, "/config/jwt_secret", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for sensitive key, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestConfigHandler_GetKey_RejectsPasswordHash(t *testing.T) {
	r, _ := setupConfigTestRouter(t)

	// auth_password_hash is blocked by sensitiveKeys
	req := httptest.NewRequest(http.MethodGet, "/config/auth_password_hash", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for auth_password_hash, got %d: %s", w.Code, w.Body.String())
	}
}
