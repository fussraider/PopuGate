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

func setupGeoblockTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	handler := NewGeoblockHandler(settingsStore, nil)

	r := gin.New()
	r.POST("/geoblock/add", handler.Add)
	r.POST("/geoblock/remove", handler.Remove)
	r.PUT("/geoblock/mode", handler.SetMode)
	r.GET("/geoblock", handler.Get)
	r.POST("/geoblock/clear", handler.Clear)

	return r
}

func TestGeoblockAdd_ValidCountry(t *testing.T) {
	r := setupGeoblockTestRouter(t)

	body, _ := json.Marshal(map[string]string{"country": "ru"})
	req := httptest.NewRequest(http.MethodPost, "/geoblock/add", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGeoblockAdd_InvalidCountry(t *testing.T) {
	r := setupGeoblockTestRouter(t)

	tests := []string{"xx", "../../", "a", "abc", "12"}
	for _, code := range tests {
		t.Run(code, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"country": code})
			req := httptest.NewRequest(http.MethodPost, "/geoblock/add", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for %q, got %d: %s", code, w.Code, w.Body.String())
			}
		})
	}
}

func TestGeoblockRemove_ValidCountry(t *testing.T) {
	r := setupGeoblockTestRouter(t)

	body, _ := json.Marshal(map[string]string{"country": "us"})
	req := httptest.NewRequest(http.MethodPost, "/geoblock/remove", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
