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
	return r, handler
}

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
			json.Unmarshal(w.Body.Bytes(), &resp)

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
	json.Unmarshal(w.Body.Bytes(), &resp)

	applied := resp["applied"].([]interface{})
	if len(applied) != 2 {
		t.Errorf("expected 2 applied keys, got %d: %v", len(applied), applied)
	}

	rejected := resp["rejected"].([]interface{})
	if len(rejected) != 2 {
		t.Errorf("expected 2 rejected keys, got %d: %v", len(rejected), rejected)
	}
}
