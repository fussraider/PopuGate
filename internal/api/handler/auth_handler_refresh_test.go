package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestRefresh_RejectsReuse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	blocklistStore := store.NewTokenBlocklistStore(db)
	handler := NewAuthHandler(settingsStore, blocklistStore)

	// Setup password
	hash, _ := bcrypt.GenerateFromPassword([]byte("testpass123"), bcrypt.MinCost)
	_ = settingsStore.SetAuthPasswordHash(context.Background(), string(hash))

	r := gin.New()
	r.POST("/auth/login", handler.Login)
	r.POST("/auth/refresh", handler.Refresh)

	// Login to get tokens
	loginBody, _ := json.Marshal(map[string]string{"password": "testpass123"})
	loginReq := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginW := httptest.NewRecorder()
	r.ServeHTTP(loginW, loginReq)

	var loginResp map[string]interface{}
	_ = json.Unmarshal(loginW.Body.Bytes(), &loginResp)
	refreshToken := loginResp["refresh_token"].(string)

	// First refresh — should succeed
	refreshBody, _ := json.Marshal(map[string]string{"refresh_token": refreshToken})
	req1 := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(refreshBody))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("first refresh: expected 200, got %d: %s", w1.Code, w1.Body.String())
	}

	// Second refresh with same token — should be rejected (H-03)
	req2 := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(refreshBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("second refresh: expected 401 (replay rejected), got %d: %s", w2.Code, w2.Body.String())
	}

	var resp map[string]string
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp["error"] != "refresh token already used" {
		t.Errorf("unexpected error message: %s", resp["error"])
	}
}

func TestSetup_MinPasswordLength(t *testing.T) {
	tests := []struct {
		password string
		wantCode int
	}{
		{"short", http.StatusBadRequest},   // 5 chars < 8
		{"1234567", http.StatusBadRequest}, // 7 chars < 8
		{"12345678", http.StatusOK},        // 8 chars = ok
		{"longpassword", http.StatusOK},    // >8 = ok
	}

	for _, tt := range tests {
		t.Run(tt.password, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			// Fresh DB per test case to avoid setup-already-done conflicts
			db := testutil.OpenTestDB(t)
			settingsStore := store.NewSettingsStore(db)
			blocklistStore := store.NewTokenBlocklistStore(db)

			h := NewAuthHandler(settingsStore, blocklistStore)
			rr := gin.New()
			rr.POST("/auth/setup", h.Setup)

			body, _ := json.Marshal(map[string]string{"password": tt.password})
			req := httptest.NewRequest(http.MethodPost, "/auth/setup", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			rr.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("password %q: expected %d, got %d: %s", tt.password, tt.wantCode, w.Code, w.Body.String())
			}
		})
	}
}
