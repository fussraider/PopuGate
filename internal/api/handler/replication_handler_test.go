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

func setupReplicationTestRouter(t *testing.T) (*gin.Engine, *ReplicationHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	slaveStore := store.NewSlaveStore(db)
	handler := NewReplicationHandler(settingsStore, slaveStore)

	r := gin.New()
	r.GET("/api/v1/replication/status", handler.Status)
	r.POST("/api/v1/replication/setup", handler.Setup)
	r.POST("/api/v1/replication/slaves", handler.AddSlave)
	r.DELETE("/api/v1/replication/slaves/:host", handler.RemoveSlave)
	r.GET("/api/v1/replication/slaves", handler.ListSlaves)
	r.POST("/api/v1/replication/sync", handler.Sync)
	r.POST("/api/v1/replication/test", handler.Test)
	r.GET("/api/v1/replication/ssh-key", handler.GetSSHKey)
	r.POST("/api/v1/replication/ssh-keygen", handler.SSHKeygen)

	return r, handler
}

func TestReplicationHandler_Status(t *testing.T) {
	r, _ := setupReplicationTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/replication/status", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["role"] == nil {
		t.Error("expected role field")
	}
	if resp["enabled"] == nil {
		t.Error("expected enabled field")
	}
	if resp["slaves"] == nil {
		t.Error("expected slaves field")
	}

	// Default settings have empty role and replication disabled
	slaves, ok := resp["slaves"].([]interface{})
	if !ok {
		t.Fatalf("expected slaves to be a slice, got %T", resp["slaves"])
	}
	if len(slaves) != 0 {
		t.Errorf("expected empty slaves list, got %d", len(slaves))
	}
}

func TestReplicationHandler_Setup_Master(t *testing.T) {
	r, _ := setupReplicationTestRouter(t)

	body, _ := json.Marshal(map[string]string{"role": "master"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["ok"] != true {
		t.Error("expected ok=true")
	}
	if resp["role"] != "master" {
		t.Errorf("expected role=master, got %v", resp["role"])
	}
}

func TestReplicationHandler_Setup_Slave(t *testing.T) {
	r, _ := setupReplicationTestRouter(t)

	body, _ := json.Marshal(map[string]string{"role": "slave"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["ok"] != true {
		t.Error("expected ok=true")
	}
	if resp["role"] != "slave" {
		t.Errorf("expected role=slave, got %v", resp["role"])
	}
}

func TestReplicationHandler_Setup_InvalidRole(t *testing.T) {
	r, _ := setupReplicationTestRouter(t)

	body, _ := json.Marshal(map[string]string{"role": "invalid"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReplicationHandler_Setup_WithSSH(t *testing.T) {
	r, _ := setupReplicationTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"role":     "master",
		"ssh_user": "deploy",
		"ssh_port": 2222,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/setup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["ok"] != true {
		t.Error("expected ok=true")
	}
	if resp["role"] != "master" {
		t.Errorf("expected role=master, got %v", resp["role"])
	}
}

func TestReplicationHandler_AddSlave(t *testing.T) {
	r, _ := setupReplicationTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"host":  "192.168.1.10",
		"port":  22,
		"label": "slave-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/slaves", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["host"] != "192.168.1.10" {
		t.Errorf("expected host=192.168.1.10, got %v", resp["host"])
	}
	if resp["label"] != "slave-1" {
		t.Errorf("expected label=slave-1, got %v", resp["label"])
	}
}

func TestReplicationHandler_AddSlave_DefaultPort(t *testing.T) {
	r, _ := setupReplicationTestRouter(t)

	body, _ := json.Marshal(map[string]interface{}{
		"host": "10.0.0.5",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/slaves", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	// Port 0 should default to 22
	port := resp["port"]
	if port == nil {
		t.Error("expected port field")
	} else {
		// JSON numbers decode as float64
		if int(port.(float64)) != 22 {
			t.Errorf("expected port=22 (default), got %v", port)
		}
	}
	// Label defaults to host when empty
	if resp["label"] != "10.0.0.5" {
		t.Errorf("expected label to default to host, got %v", resp["label"])
	}
}

func TestReplicationHandler_AddSlave_InvalidHost(t *testing.T) {
	r, _ := setupReplicationTestRouter(t)

	// Missing host field
	body, _ := json.Marshal(map[string]interface{}{
		"port": 22,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/slaves", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestReplicationHandler_RemoveSlave(t *testing.T) {
	r, _ := setupReplicationTestRouter(t)

	// First, add a slave
	body, _ := json.Marshal(map[string]interface{}{
		"host": "192.168.1.20",
		"port": 22,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/slaves", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("setup: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Now remove it
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/replication/slaves/192.168.1.20", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["ok"] != true {
		t.Error("expected ok=true")
	}
}

func TestReplicationHandler_ListSlaves(t *testing.T) {
	r, _ := setupReplicationTestRouter(t)

	// Add two slaves
	for _, slave := range []map[string]interface{}{
		{"host": "10.0.0.1", "port": 22, "label": "s1"},
		{"host": "10.0.0.2", "port": 2222, "label": "s2"},
	} {
		body, _ := json.Marshal(slave)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/slaves", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("setup: expected 201, got %d: %s", w.Code, w.Body.String())
		}
	}

	// List all slaves
	req := httptest.NewRequest(http.MethodGet, "/api/v1/replication/slaves", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var slaves []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &slaves); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(slaves) != 2 {
		t.Fatalf("expected 2 slaves, got %d", len(slaves))
	}

	hosts := make(map[string]bool)
	for _, s := range slaves {
		hosts[s["host"].(string)] = true
	}
	if !hosts["10.0.0.1"] || !hosts["10.0.0.2"] {
		t.Errorf("expected both slaves, got hosts: %v", hosts)
	}
}

func TestReplicationHandler_Sync_NoService(t *testing.T) {
	r, _ := setupReplicationTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/sync", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["error"] != "replication service not available" {
		t.Errorf("unexpected error message: %v", resp["error"])
	}
}

func TestReplicationHandler_Test_NoService(t *testing.T) {
	r, _ := setupReplicationTestRouter(t)

	body, _ := json.Marshal(map[string]string{"host": "10.0.0.1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/test", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["error"] != "replication service not available" {
		t.Errorf("unexpected error message: %v", resp["error"])
	}
}

func TestReplicationHandler_GetSSHKey_NoService(t *testing.T) {
	r, _ := setupReplicationTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/replication/ssh-key", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["error"] != "replication service not available" {
		t.Errorf("unexpected error message: %v", resp["error"])
	}
}

func TestReplicationHandler_SSHKeygen_NoService(t *testing.T) {
	r, _ := setupReplicationTestRouter(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/replication/ssh-keygen", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["error"] != "replication service not available" {
		t.Errorf("unexpected error message: %v", resp["error"])
	}
}
