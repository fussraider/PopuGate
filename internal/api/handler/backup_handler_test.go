package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/store"
)

func setupBackupTestRouter(t *testing.T) (*gin.Engine, *BackupHandler, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir := t.TempDir()
	backupStore := store.NewBackupStore(tmpDir)
	handler := NewBackupHandler(backupStore)

	r := gin.New()
	r.GET("/api/v1/backups", handler.List)
	r.POST("/api/v1/backups", handler.Create)
	r.POST("/api/v1/backups/restore", handler.Restore)
	r.DELETE("/api/v1/backups/:filename", handler.Delete)
	r.GET("/api/v1/backups/download/:filename", handler.Download)

	return r, handler, tmpDir
}

func TestBackupHandler_List(t *testing.T) {
	r, _, _ := setupBackupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var backups []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &backups); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	// Empty list is valid
}

func TestBackupHandler_Create(t *testing.T) {
	r, _, tmpDir := setupBackupTestRouter(t)

	// Create a dummy settings.db so backup has something to include
	dbPath := filepath.Join(tmpDir, "settings.db")
	if err := os.WriteFile(dbPath, []byte("dummy"), 0644); err != nil {
		t.Fatalf("create settings.db: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups", nil)
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
	if resp["filename"] == nil || resp["filename"] == "" {
		t.Error("expected non-empty filename")
	}
	if resp["size"] == nil {
		t.Error("expected size field")
	}
}

func TestBackupHandler_Restore_PathTraversal(t *testing.T) {
	r, _, _ := setupBackupTestRouter(t)

	tests := []struct {
		name     string
		filename string
	}{
		{"double dot", "../../etc/passwd"},
		{"slash", "/etc/passwd"},
		{"mixed", "../secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"filename": tt.filename})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/restore", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)
			if resp["error"] != "invalid filename" {
				t.Errorf("expected 'invalid filename', got %v", resp["error"])
			}
		})
	}
}

func TestBackupHandler_Restore_MissingFilename(t *testing.T) {
	r, _, _ := setupBackupTestRouter(t)

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/restore", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBackupHandler_Delete_PathTraversal(t *testing.T) {
	r, _, _ := setupBackupTestRouter(t)

	tests := []struct {
		name     string
		filename string
	}{
		{"double dot", ".."},
		{"mixed", "a..b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/backups/"+tt.filename, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}

			var resp map[string]interface{}
			json.Unmarshal(w.Body.Bytes(), &resp)
			if resp["error"] != "invalid filename" {
				t.Errorf("expected 'invalid filename', got %v", resp["error"])
			}
		})
	}
}

func TestBackupHandler_Delete_Nonexistent(t *testing.T) {
	r, _, _ := setupBackupTestRouter(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/backups/nonexistent.tar.gz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Delete of nonexistent file returns 500 (os.Remove error)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBackupHandler_Download_MissingFile(t *testing.T) {
	r, _, _ := setupBackupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups/download/missing.tar.gz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "backup not found" {
		t.Errorf("expected 'backup not found', got %v", resp["error"])
	}
}

func TestBackupHandler_Download_PathTraversal(t *testing.T) {
	r, _, _ := setupBackupTestRouter(t)

	tests := []struct {
		name     string
		filename string
	}{
		{"double dot", ".."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/backups/download/"+tt.filename, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
