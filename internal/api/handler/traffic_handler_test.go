package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func setupTrafficTestRouter(t *testing.T) (*gin.Engine, *TrafficHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	trafficStore := store.NewTrafficStore(db)
	settingsStore := store.NewSettingsStore(db)
	handler := NewTrafficHandler(trafficStore, settingsStore)

	r := gin.New()
	r.GET("/api/v1/traffic", handler.Get)
	r.GET("/api/v1/traffic/live", handler.GetLive)
	r.GET("/api/v1/traffic/:label", handler.GetUser)
	r.GET("/api/v1/traffic/history", handler.GetHistory)

	return r, handler
}

func TestTrafficHandler_Get(t *testing.T) {
	r, _ := setupTrafficTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traffic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	global, ok := resp["global"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'global' key in response")
	}
	if _, ok := global["bytes_in"]; !ok {
		t.Error("expected 'bytes_in' in global")
	}
	if _, ok := global["bytes_out"]; !ok {
		t.Error("expected 'bytes_out' in global")
	}

	users, ok := resp["users"]
	if !ok {
		t.Error("expected 'users' key in response")
	}
	// Empty DB should return an empty array (or nil which marshals to null)
	if users != nil {
		arr, ok := users.([]interface{})
		if !ok {
			t.Errorf("expected users to be an array, got %T", users)
		}
		if len(arr) != 0 {
			t.Errorf("expected empty users array, got %d items", len(arr))
		}
	}
}

func TestTrafficHandler_Get_WithTraffic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	trafficStore := store.NewTrafficStore(db)
	settingsStore := store.NewSettingsStore(db)

	// Insert some user traffic
	ctx := context.Background()
	if err := trafficStore.UpdateUserTraffic(ctx, "user1", 1, 1000, 2000, 500, 1000); err != nil {
		t.Fatalf("setup traffic: %v", err)
	}

	handler := NewTrafficHandler(trafficStore, settingsStore)
	r := gin.New()
	r.GET("/api/v1/traffic", handler.Get)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traffic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	users := resp["users"].([]interface{})
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	user := users[0].(map[string]interface{})
	if user["label"] != "user1" {
		t.Errorf("expected label 'user1', got %v", user["label"])
	}
}

func TestTrafficHandler_GetLive_NoService(t *testing.T) {
	r, _ := setupTrafficTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traffic/live", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "traffic service not available" {
		t.Errorf("unexpected error: %v", resp["error"])
	}
}

func TestTrafficHandler_GetUser(t *testing.T) {
	r, _ := setupTrafficTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traffic/testuser", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["label"] != "testuser" {
		t.Errorf("expected label 'testuser', got %v", resp["label"])
	}
}

func TestTrafficHandler_GetUser_WithTraffic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	trafficStore := store.NewTrafficStore(db)
	settingsStore := store.NewSettingsStore(db)

	ctx := context.Background()
	if err := trafficStore.UpdateUserTraffic(ctx, "alice", 1, 500, 800, 250, 400); err != nil {
		t.Fatalf("setup: %v", err)
	}

	handler := NewTrafficHandler(trafficStore, settingsStore)
	r := gin.New()
	r.GET("/api/v1/traffic/:label", handler.GetUser)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traffic/alice", nil)
	w := httptest.NewRecorder()
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

func TestTrafficHandler_GetHistory_NoService(t *testing.T) {
	r, _ := setupTrafficTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traffic/history", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTrafficHandler_GetHistory_InvalidAggregate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	trafficStore := store.NewTrafficStore(db)
	settingsStore := store.NewSettingsStore(db)
	instanceStore := store.NewInstanceStore(db)
	handler := NewTrafficHandler(trafficStore, settingsStore)

	// Create a real TrafficService (nil docker is fine - we won't reach GetLiveMetrics)
	trafficSvc := service.NewTrafficService(trafficStore, settingsStore, nil, instanceStore)
	handler.SetTrafficService(trafficSvc)

	r := gin.New()
	r.GET("/api/v1/traffic/history", handler.GetHistory)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traffic/history?aggregate=invalid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "aggregate must be 'none', 'hour', or 'day'" {
		t.Errorf("unexpected error: %v", resp["error"])
	}
}

func TestTrafficHandler_GetHistory_ValidParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	trafficStore := store.NewTrafficStore(db)
	settingsStore := store.NewSettingsStore(db)
	instanceStore := store.NewInstanceStore(db)
	handler := NewTrafficHandler(trafficStore, settingsStore)

	trafficSvc := service.NewTrafficService(trafficStore, settingsStore, nil, instanceStore)
	handler.SetTrafficService(trafficSvc)

	r := gin.New()
	r.GET("/api/v1/traffic/history", handler.GetHistory)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traffic/history?aggregate=none&label=test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["history"]; !ok {
		t.Error("expected 'history' key in response")
	}
}

func TestTrafficHandler_GetHistory_ValidHourAggregate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	trafficStore := store.NewTrafficStore(db)
	settingsStore := store.NewSettingsStore(db)
	instanceStore := store.NewInstanceStore(db)
	handler := NewTrafficHandler(trafficStore, settingsStore)

	trafficSvc := service.NewTrafficService(trafficStore, settingsStore, nil, instanceStore)
	handler.SetTrafficService(trafficSvc)

	r := gin.New()
	r.GET("/api/v1/traffic/history", handler.GetHistory)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traffic/history?aggregate=hour", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestTrafficHandler_GetHistory_ValidDayAggregate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	trafficStore := store.NewTrafficStore(db)
	settingsStore := store.NewSettingsStore(db)
	instanceStore := store.NewInstanceStore(db)
	handler := NewTrafficHandler(trafficStore, settingsStore)

	trafficSvc := service.NewTrafficService(trafficStore, settingsStore, nil, instanceStore)
	handler.SetTrafficService(trafficSvc)

	r := gin.New()
	r.GET("/api/v1/traffic/history", handler.GetHistory)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traffic/history?aggregate=day", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
