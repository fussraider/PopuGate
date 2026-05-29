package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	r.PUT("/api/v1/instances/:id", handler.Update)
	r.DELETE("/api/v1/instances/:id", handler.Remove)
	r.GET("/api/v1/instances/check-port", handler.CheckPort)
	r.POST("/api/v1/instances/:id/start", handler.StartInstance)
	r.POST("/api/v1/instances/:id/stop", handler.StopInstance)
	r.POST("/api/v1/instances/:id/reload", handler.ReloadInstance)
	r.POST("/api/v1/instances/:id/refresh-fronting", handler.RefreshFronting)
	r.GET("/api/v1/instances/:id/status", handler.InstanceStatus)
	r.GET("/api/v1/instances/:id/logs", handler.InstanceLogs)

	return r, handler
}

// addInstance is a test helper that adds an instance and returns the response code.
func addInstance(t *testing.T, r *gin.Engine, port, metricsPort int, label string) int {
	t.Helper()
	return addInstanceFull(t, r, port, metricsPort, label, nil)
}

// addInstanceFull adds an instance and returns the response code.
// If respOut is provided, the response JSON is decoded into it.
func addInstanceFull(t *testing.T, r *gin.Engine, port, metricsPort int, label string, respOut *map[string]interface{}) int {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"port":         port,
		"metrics_port": metricsPort,
		"label":        label,
		"tls_domain":   "example.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if respOut != nil {
		_ = json.Unmarshal(w.Body.Bytes(), respOut)
	}
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
		"port":       8443,
		"label":      "Second",
		"tls_domain": "example.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["metrics_port"] == nil {
		t.Error("expected auto-assigned metrics_port")
	}
}

func TestInstanceHandler_Add_PortEqualsMetricsPort(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"port":         443,
		"metrics_port": 443,
		"label":        "Bad",
		"tls_domain":   "example.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "metrics_port must differ from port" {
		t.Errorf("unexpected error: %v", resp["error"])
	}
}

func TestInstanceHandler_Add_InvalidTLSDomains(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"port":        443,
		"label":       "Bad",
		"tls_domain":  "example.com",
		"tls_domains": "not-json",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInstanceHandler_Add_InvalidTags(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"port":       443,
		"label":      "Bad",
		"tls_domain": "example.com",
		"tags":       "not-json",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
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

func TestInstanceHandler_Update(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	var inst map[string]interface{}
	code := addInstanceFull(t, r, 443, 9091, "Original", &inst)
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}
	id := int(inst["id"].(float64))

	body, _ := json.Marshal(map[string]interface{}{
		"label": "Updated",
	})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/instances/%d", id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["label"] != "Updated" {
		t.Errorf("expected label=Updated, got %v", resp["label"])
	}
}

func TestInstanceHandler_Update_NotFound(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{"label": "X"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/instances/999", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestInstanceHandler_Update_PortEqualsMetricsPort(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	var inst map[string]interface{}
	code := addInstanceFull(t, r, 443, 9091, "Test", &inst)
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}
	id := int(inst["id"].(float64))

	body, _ := json.Marshal(map[string]interface{}{
		"metrics_port": 443,
	})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/instances/%d", id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInstanceHandler_Update_AutoAssignMetricsPort(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	var inst map[string]interface{}
	code := addInstanceFull(t, r, 443, 9091, "Test", &inst)
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}
	id := int(inst["id"].(float64))

	// Set metrics_port to 0 — should auto-assign
	body, _ := json.Marshal(map[string]interface{}{
		"metrics_port": 0,
	})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/instances/%d", id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	mp := int(resp["metrics_port"].(float64))
	if mp == 0 {
		t.Error("expected auto-assigned metrics_port, got 0")
	}
}

func TestInstanceHandler_Remove(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	var first map[string]interface{}
	code := addInstanceFull(t, r, 443, 9091, "First", &first)
	if code != http.StatusCreated {
		t.Fatalf("first: expected 201, got %d", code)
	}
	code = addInstance(t, r, 8443, 9092, "Second")
	if code != http.StatusCreated {
		t.Fatalf("second: expected 201, got %d", code)
	}

	id := int(first["id"].(float64))
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/instances/%d", id), nil)
	w := httptest.NewRecorder()
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

func TestInstanceHandler_RemoveLastInstance(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	var inst map[string]interface{}
	code := addInstanceFull(t, r, 443, 9091, "Only", &inst)
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}

	id := int(inst["id"].(float64))
	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/instances/%d", id), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "cannot delete the last instance" {
		t.Errorf("expected 'cannot delete the last instance', got %v", resp["error"])
	}
}

func TestInstanceHandler_Remove_InvalidID(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/instances/abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInstanceHandler_CheckPort_Available(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/check-port?port=9999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["available"] != true {
		t.Errorf("expected available=true, got %v", resp["available"])
	}
}

func TestInstanceHandler_CheckPort_ConflictWithInstance(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	code := addInstance(t, r, 443, 9091, "Test")
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}

	// Check the proxy port — should conflict
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/check-port?port=443", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["available"] != false {
		t.Error("expected available=false for conflicting port")
	}
	if resp["reason"] == nil || resp["reason"] == "" {
		t.Error("expected reason for conflict")
	}
}

func TestInstanceHandler_CheckPort_ExcludeSelf(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	var inst map[string]interface{}
	code := addInstanceFull(t, r, 443, 9091, "Test", &inst)
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}
	id := int(inst["id"].(float64))

	// Check metrics port excluding own ID — should still conflict (port != metrics_port check)
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/instances/check-port?port=9091&exclude=%d", id), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	// Port 9091 is metrics port of this instance, excluded — should be available
	if resp["available"] != true {
		t.Errorf("expected available=true when excluding self, got %v: %v", resp["available"], resp["reason"])
	}
}

func TestInstanceHandler_CheckPort_InvalidPort(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/check-port?port=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestInstanceHandler_Start_NoContainerSvc(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/1/start", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestInstanceHandler_Stop_NoContainerSvc(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/1/stop", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestInstanceHandler_Reload_NoContainerSvc(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/1/reload", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestInstanceHandler_Status_NoContainerSvc(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/1/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestInstanceHandler_Logs_NoDocker(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instances/1/logs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestInstanceHandler_Start_InvalidID(t *testing.T) {
	r, handler := setupInstanceTestRouter(t)
	handler.SetContainerSvc(nil) // ensure nil but route registered

	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/abc/start", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestInstanceHandler_NextMetricsPort_Empty(t *testing.T) {
	oldIsPortFree := isPortFree
	isPortFree = func(port int) bool { return true }
	defer func() { isPortFree = oldIsPortFree }()

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
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if int(resp["port"].(float64)) != 9091 {
		t.Errorf("expected 9091, got %v", resp["port"])
	}
}

func TestInstanceHandler_NextMetricsPort_WithExisting(t *testing.T) {
	oldIsPortFree := isPortFree
	isPortFree = func(port int) bool { return true }
	defer func() { isPortFree = oldIsPortFree }()

	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	instanceStore := store.NewInstanceStore(db)

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
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if int(resp["port"].(float64)) != 9092 {
		t.Errorf("expected 9092, got %v", resp["port"])
	}
}

func TestInstanceHandler_Update_InvalidTLSDomains(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)
	addInstance(t, r, 443, 9091, "Test")

	body, _ := json.Marshal(map[string]interface{}{
		"tls_domains": "not-json",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/instances/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInstanceHandler_Update_InvalidTags(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)
	addInstance(t, r, 443, 9091, "Test")

	body, _ := json.Marshal(map[string]interface{}{
		"tags": "not-json",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/instances/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInstanceHandler_Update_TagsInvalidFormat(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)
	addInstance(t, r, 443, 9091, "Test")

	body, _ := json.Marshal(map[string]interface{}{
		"tags": `["tag with spaces"]`,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/instances/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInstanceHandler_Update_ValidTags(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)
	addInstance(t, r, 443, 9091, "Test")

	body, _ := json.Marshal(map[string]interface{}{
		"tags": `["valid-tag", "another_tag"]`,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/instances/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInstanceHandler_Add_InvalidTagFormat(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"port":         8443,
		"metrics_port": 9092,
		"label":        "BadTags",
		"tls_domain":   "example.com",
		"tags":         `["tag with spaces"]`,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestInstanceHandler_Add_WithTCPMSS(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"port":            443,
		"metrics_port":    9091,
		"label":           "TCPMSS",
		"tls_domain":      "example.com",
		"tcp_mss_enabled": true,
		"tcp_mss":         120,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["tcp_mss_enabled"] != true {
		t.Errorf("expected tcp_mss_enabled=true, got %v", resp["tcp_mss_enabled"])
	}
	if int(resp["tcp_mss"].(float64)) != 120 {
		t.Errorf("expected tcp_mss=120, got %v", resp["tcp_mss"])
	}
}

func TestInstanceHandler_Add_WithTLSFronting(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"port":         443,
		"metrics_port": 9091,
		"label":        "Fronting",
		"tls_domain":   "example.com",
		"tls_fronting": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["tls_fronting"] != true {
		t.Errorf("expected tls_fronting=true, got %v", resp["tls_fronting"])
	}
}

func TestInstanceHandler_Update_TCPMSS(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	var inst map[string]interface{}
	code := addInstanceFull(t, r, 443, 9091, "Test", &inst)
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}
	id := int(inst["id"].(float64))

	body, _ := json.Marshal(map[string]interface{}{
		"tcp_mss_enabled": true,
		"tcp_mss":         200,
	})
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/instances/%d", id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["tcp_mss_enabled"] != true {
		t.Errorf("expected tcp_mss_enabled=true, got %v", resp["tcp_mss_enabled"])
	}
}

func TestInstanceHandler_RefreshFronting_NoSvc(t *testing.T) {
	r, _ := setupInstanceTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances/1/refresh-fronting", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}
