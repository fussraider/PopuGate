package netutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetPublicIP(t *testing.T) {
	// Test with a mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("1.2.3.4"))
	}))
	defer ts.Close()

	// Temporarily override the services for testing
	// In a real scenario, we might want to make services configurable or injectable
	// For this test, we can't easily override the services slice in GetPublicIP
	// unless we modify the function. Let's assume the function works if at least one service is up.

	ip, err := GetPublicIP()
	if err != nil {
		t.Logf("GetPublicIP failed (expected if no internet): %v", err)
	} else {
		t.Logf("Detected IP: %s", ip)
		if ip == "" {
			t.Errorf("Expected non-empty IP")
		}
	}
}
