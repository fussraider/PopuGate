package netutil

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestGetPublicIPFromServices_Success(t *testing.T) {
	wantIP := "203.0.113.42"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(wantIP))
	}))
	defer ts.Close()

	ip, err := GetPublicIPFromServices([]string{ts.URL})
	if err != nil {
		t.Fatalf("GetPublicIPFromServices() returned error: %v", err)
	}
	if ip != wantIP {
		t.Errorf("GetPublicIPFromServices() = %q, want %q", ip, wantIP)
	}
}

func TestGetPublicIPFromServices_TrimmedOutput(t *testing.T) {
	// The response may include a trailing newline (common for ip services)
	wantIP := "198.51.100.7"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(wantIP + "\n"))
	}))
	defer ts.Close()

	ip, err := GetPublicIPFromServices([]string{ts.URL})
	if err != nil {
		t.Fatalf("GetPublicIPFromServices() returned error: %v", err)
	}
	if ip != wantIP {
		t.Errorf("GetPublicIPFromServices() = %q, want %q", ip, wantIP)
	}
}

func TestGetPublicIPFromServices_FirstServiceSucceeds(t *testing.T) {
	wantIP := "10.0.0.1"
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failServer.Close()

	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(wantIP))
	}))
	defer okServer.Close()

	ip, err := GetPublicIPFromServices([]string{failServer.URL, okServer.URL})
	if err != nil {
		t.Fatalf("GetPublicIPFromServices() returned error: %v", err)
	}
	if ip != wantIP {
		t.Errorf("GetPublicIPFromServices() = %q, want %q", ip, wantIP)
	}
}

func TestGetPublicIPFromServices_AllServicesFail(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	_, err := GetPublicIPFromServices([]string{ts.URL})
	if err == nil {
		t.Fatal("GetPublicIPFromServices() should return error when all services fail")
	}
}

func TestGetPublicIPFromServices_EmptyServices(t *testing.T) {
	_, err := GetPublicIPFromServices([]string{})
	if err == nil {
		t.Fatal("GetPublicIPFromServices() should return error with empty service list")
	}
}

func TestGetPublicIPFromServices_InvalidURL(t *testing.T) {
	_, err := GetPublicIPFromServices([]string{"http://[::1]:named"})
	if err == nil {
		t.Fatal("GetPublicIPFromServices() should return error when all URLs are invalid")
	}
}

func TestGetPublicIPFromServices_NonOKStatusCodes(t *testing.T) {
	statusCodes := []int{400, 401, 403, 404, 500, 502, 503}
	for _, code := range statusCodes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(code)
			}))
			defer ts.Close()

			_, err := GetPublicIPFromServices([]string{ts.URL})
			if err == nil {
				t.Errorf("GetPublicIPFromServices() should fail for status %d", code)
			}
		})
	}
}

func TestGetPublicIPFromServices_EmptyResponseBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// write nothing (empty body)
	}))
	defer ts.Close()

	_, err := GetPublicIPFromServices([]string{ts.URL})
	if err == nil {
		t.Fatal("GetPublicIPFromServices() should return error for empty response body")
	}
}

func TestGetPublicIPFromServices_WhitespaceOnlyBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("   \n\t  "))
	}))
	defer ts.Close()

	_, err := GetPublicIPFromServices([]string{ts.URL})
	if err == nil {
		t.Fatal("GetPublicIPFromServices() should return error for whitespace-only body")
	}
}

// --- Cache tests (L-P02 regression) ---

func TestGetPublicIP_CachesResult(t *testing.T) {
	InvalidatePublicIPCache()

	callCount := 0
	wantIP := "203.0.113.99"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(wantIP))
	}))
	defer ts.Close()

	// Override default services for this test
	origServices := defaultServices
	defaultServices = []string{ts.URL}
	defer func() { defaultServices = origServices }()

	ip1, err := GetPublicIP()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if ip1 != wantIP {
		t.Errorf("first call: got %q, want %q", ip1, wantIP)
	}

	ip2, err := GetPublicIP()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if ip2 != wantIP {
		t.Errorf("second call: got %q, want %q", ip2, wantIP)
	}

	if callCount != 1 {
		t.Errorf("expected 1 HTTP call (cached), got %d", callCount)
	}
}

func TestInvalidatePublicIPCache_ForceRefresh(t *testing.T) {
	InvalidatePublicIPCache()

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("1.2.3.4"))
	}))
	defer ts.Close()

	origServices := defaultServices
	defaultServices = []string{ts.URL}
	defer func() { defaultServices = origServices }()

	GetPublicIP() // cache
	InvalidatePublicIPCache()
	GetPublicIP() // fresh

	if callCount != 2 {
		t.Errorf("expected 2 calls after invalidation, got %d", callCount)
	}
}

func TestGetPublicIP_ReturnsStaleOnError(t *testing.T) {
	InvalidatePublicIPCache()

	firstIP := "10.0.0.1"
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(firstIP))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer ts.Close()

	origServices := defaultServices
	defaultServices = []string{ts.URL}
	defer func() { defaultServices = origServices }()

	// First call succeeds and caches
	ip1, _ := GetPublicIP()
	if ip1 != firstIP {
		t.Fatalf("first call: got %q, want %q", ip1, firstIP)
	}

	// Expire the cache
	ipCacheMu.Lock()
	ipCacheAt = time.Now().Add(-10 * time.Minute)
	ipCacheMu.Unlock()

	// Second call should return stale cache despite service error
	ip2, err := GetPublicIP()
	if err != nil {
		t.Fatalf("second call should use stale cache: %v", err)
	}
	if ip2 != firstIP {
		t.Errorf("second call: got %q, want stale %q", ip2, firstIP)
	}
}

func TestGetPublicIP_ConcurrentCalls(t *testing.T) {
	InvalidatePublicIPCache()

	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("192.0.2.1"))
	}))
	defer ts.Close()

	origServices := defaultServices
	defaultServices = []string{ts.URL}
	defer func() { defaultServices = origServices }()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ip, err := GetPublicIP()
			if err != nil {
				t.Errorf("concurrent call failed: %v", err)
			}
			if ip != "192.0.2.1" {
				t.Errorf("concurrent call: got %q", ip)
			}
		}()
	}
	wg.Wait()

	// With thundering herd protection, only 1 HTTP call should be made
	if callCount != 1 {
		t.Errorf("expected 1 HTTP call (deduplication), got %d", callCount)
	}
}
