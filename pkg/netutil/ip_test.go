package netutil

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
