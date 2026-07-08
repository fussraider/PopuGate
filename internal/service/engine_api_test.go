package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
)

// portOf extracts the numeric port from an httptest server URL (http://127.0.0.1:PORT).
func portOf(t *testing.T, serverURL string) int {
	t.Helper()
	_, port, ok := strings.Cut(strings.TrimPrefix(serverURL, "http://"), ":")
	if !ok {
		t.Fatalf("no port in %q", serverURL)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatalf("parse port from %q: %v", serverURL, err)
	}
	return n
}

func TestEngineAPI_ResetUserQuota_Status(t *testing.T) {
	var gotPath, gotMethod string
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(status)
	}))
	defer srv.Close()
	port := portOf(t, srv.URL)
	c := NewEngineAPIClient()
	ctx := context.Background()

	// 200 → ok, and path/method are correct.
	if err := c.ResetUserQuota(ctx, port, "alice"); err != nil {
		t.Fatalf("200: unexpected error: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/users/alice/reset-quota" {
		t.Errorf("request = %s %s, want POST /v1/users/alice/reset-quota", gotMethod, gotPath)
	}

	// 404 (user not on this instance) → treated as success.
	status = http.StatusNotFound
	if err := c.ResetUserQuota(ctx, port, "ghost"); err != nil {
		t.Errorf("404: expected nil, got %v", err)
	}

	// 500 → error surfaced.
	status = http.StatusInternalServerError
	if err := c.ResetUserQuota(ctx, port, "bob"); err == nil {
		t.Error("500: expected error, got nil")
	}
}

func TestEngineAPI_ResetUserQuota_NoPort(t *testing.T) {
	c := NewEngineAPIClient()
	if err := c.ResetUserQuota(context.Background(), 0, "alice"); err != nil {
		t.Errorf("apiPort 0 must be a no-op, got %v", err)
	}
}

func TestEngineAPI_ResetLabel_SkipsDisabledAndNoPort(t *testing.T) {
	var mu sync.Mutex
	hits := map[int]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[1]++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	port := portOf(t, srv.URL)
	c := NewEngineAPIClient()

	instances := []model.Instance{
		{Port: 443, APIPort: port, Enabled: true},   // called
		{Port: 8443, APIPort: 0, Enabled: true},     // skipped: no API port
		{Port: 9443, APIPort: port, Enabled: false}, // skipped: disabled
	}
	c.ResetLabel(context.Background(), instances, "alice")
	mu.Lock()
	defer mu.Unlock()
	if hits[1] != 1 {
		t.Errorf("expected exactly 1 API call, got %d", hits[1])
	}
}

func TestEngineAPI_ResetAll_TagMatched(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	port := portOf(t, srv.URL)
	c := NewEngineAPIClient()

	// One instance with tag "vip"; two secrets, only the vip one matches.
	instances := []model.Instance{{Port: 443, APIPort: port, Enabled: true, Tags: `["vip"]`}}
	secrets := []model.Secret{
		{Label: "alice", Tags: `["vip"]`},
		{Label: "bob", Tags: `["free"]`},
	}
	c.ResetAll(context.Background(), instances, secrets)

	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 1 || paths[0] != "/v1/users/alice/reset-quota" {
		t.Errorf("expected only alice reset, got %v", paths)
	}
}
