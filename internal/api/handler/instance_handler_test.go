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

func setupInstanceTestRouter(t *testing.T) (*gin.Engine, *InstanceHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	instanceStore := store.NewInstanceStore(db)
	handler := NewInstanceHandler(instanceStore)

	r := gin.New()
	r.GET("/api/v1/instances", handler.List)
	r.POST("/api/v1/instances", handler.Add)
	r.DELETE("/api/v1/instances/:port", handler.Remove)

	return r, handler
}

// addInstance is a test helper that adds an instance and returns the response code.
func addInstance(t *testing.T, r *gin.Engine, port, metricsPort int, label string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"port":         port,
		"metrics_port": metricsPort,
		"label":        label,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestInstanceHandler_List_Empty(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var instances []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &instances); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(instances) != 0 {
		t.Errorf("expected empty list, got %d items", len(instances))
	}
}

func TestInstanceHandler_Add_ValidPort(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	code := addInstance(t, r, 443, 9091, "Default")
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}
}

func TestInstanceHandler_Add_WithAutoMetricsPort(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	// Add first instance with explicit metrics port
	code := addInstance(t, r, 443, 9091, "First")
	if code != http.StatusCreated {
		t.Fatalf("first: expected 201, got %d", code)
	}

	// Add second instance without metrics_port (auto-assign)
	body, _ := json.Marshal(map[string]interface{}{
		"port":  8443,
		"label": "Second",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	// metrics_port should be auto-assigned (9091 + 1 = 9092)
	if resp["metrics_port"] == nil {
		t.Error("expected auto-assigned metrics_port")
	}
}

func TestInstanceHandler_Add_InvalidPort(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	tests := []struct {
		name string
		port int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too high", 70000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"port":         tt.port,
				"metrics_port": 9091,
			})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for port %d, got %d: %s", tt.port, w.Code, w.Body.String())
			}
		})
	}
}

func TestInstanceHandler_Add_MissingPort(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInstanceHandler_Remove(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	// Add two instances so we can remove one
	code := addInstance(t, r, 443, 9091, "First")
	if code != http.StatusCreated {
		t.Fatalf("first: expected 201, got %d", code)
	}
	code = addInstance(t, r, 8443, 9092, "Second")
	if code != http.StatusCreated {
		t.Fatalf("second: expected 201, got %d", code)
	}

	// Remove first
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/instances/443", nil)
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

func TestInstanceHandler_RemoveLastInstance(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	// Add a single instance
	code := addInstance(t, r, 443, 9091, "Only")
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}

	// Try to remove the last instance
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/instances/443", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "cannot delete the last instance" {
		t.Errorf("expected 'cannot delete the last instance', got %v", resp["error"])
	}
}

func TestInstanceHandler_Remove_InvalidPort(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/instances/abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInstanceHandler_NextMetricsPort_Empty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	instanceStore := store.NewInstanceStore(db)
	handler := NewInstanceHandler(instanceStore)

	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		port := handler.nextMetricsPort(c)
		c.JSON(http.StatusOK, gin.H{"port": port})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	// Default minimum is 9091
	if int(resp["port"].(float64)) != 9091 {
		t.Errorf("expected 9091, got %v", resp["port"])
	}
}

func TestInstanceHandler_NextMetricsPort_WithExisting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	instanceStore := store.NewInstanceStore(db)

	// Add an instance with metrics_port 9091
	code := addInstance(t, func() *gin.Engine {
		r := gin.New()
		r.POST("/api/v1/instances", NewInstanceHandler(instanceStore).Add)
		return r
	}(), 443, 9091, "First")
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}

	handler := NewInstanceHandler(instanceStore)
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		port := handler.nextMetricsPort(c)
		c.JSON(http.StatusOK, gin.H{"port": port})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	// Should be 9091 + 1 = 9092
	if int(resp["port"].(float64)) != 9092 {
		t.Errorf("expected 9092, got %v", resp["port"])
	}
}
