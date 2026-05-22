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

func TestSetup_RetryAfterTransientError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	blocklistStore := store.NewTokenBlocklistStore(db)
	h := NewAuthHandler(settingsStore, blocklistStore)

	r := gin.New()
	r.POST("/auth/setup", h.Setup)

	// First successful setup
	body1, _ := json.Marshal(map[string]string{"password": "password1"})
	req1 := httptest.NewRequest(http.MethodPost, "/auth/setup", bytes.NewReader(body1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("first setup: expected 200, got %d: %s", w1.Code, w1.Body.String())
	}

	var resp1 map[string]interface{}
	_ = json.Unmarshal(w1.Body.Bytes(), &resp1)
	if resp1["access_token"] == "" {
		t.Error("first setup: expected non-empty access_token")
	}

	// Second setup should be rejected (already completed)
	body2, _ := json.Marshal(map[string]string{"password": "password2"})
	req2 := httptest.NewRequest(http.MethodPost, "/auth/setup", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Errorf("second setup: expected 409, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestSetup_FastRejectOnDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	blocklistStore := store.NewTokenBlocklistStore(db)

	// Pre-set password hash to simulate setup already done
	hash, _ := bcrypt.GenerateFromPassword([]byte("existing"), bcrypt.MinCost)
	_ = settingsStore.SetAuthPasswordHash(context.Background(), string(hash))

	h := NewAuthHandler(settingsStore, blocklistStore)
	r := gin.New()
	r.POST("/auth/setup", h.Setup)

	body, _ := json.Marshal(map[string]string{"password": "newpassword"})
	req := httptest.NewRequest(http.MethodPost, "/auth/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 (setup already done), got %d: %s", w.Code, w.Body.String())
	}
}
