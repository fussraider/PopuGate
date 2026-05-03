package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

func TestNewIPRateLimiter(t *testing.T) {
	rateVal := rate.Limit(5)
	burst := 10
	limiter := NewIPRateLimiter(rateVal, burst)

	if limiter == nil {
		t.Fatal("expected non-nil limiter")
	}
	if limiter.rate != rateVal {
		t.Errorf("expected rate %v, got %v", rateVal, limiter.rate)
	}
	if limiter.burst != burst {
		t.Errorf("expected burst %d, got %d", burst, limiter.burst)
	}
}

func TestIPRateLimiter_GetLimiter_SameIP(t *testing.T) {
	limiter := NewIPRateLimiter(1, 5)

	l1 := limiter.GetLimiter("1.2.3.4")
	l2 := limiter.GetLimiter("1.2.3.4")

	if l1 != l2 {
		t.Error("expected same limiter instance for the same IP")
	}
}

func TestIPRateLimiter_GetLimiter_DifferentIPs(t *testing.T) {
	limiter := NewIPRateLimiter(1, 5)

	l1 := limiter.GetLimiter("1.2.3.4")
	l2 := limiter.GetLimiter("5.6.7.8")

	if l1 == l2 {
		t.Error("expected different limiter instances for different IPs")
	}
}

func TestRateLimitMiddleware_Allow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewIPRateLimiter(rate.Inf, 5) // infinite rate, always allows

	r := gin.New()
	r.Use(RateLimitMiddleware(limiter))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRateLimitMiddleware_Reject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// rate=0 means no refill; burst=2 allows exactly 2 requests
	limiter := NewIPRateLimiter(0, 2)

	r := gin.New()
	r.Use(RateLimitMiddleware(limiter))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// First two requests should succeed
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// Third request should be rejected
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
}

func TestRateLimitMiddleware_AllowsUnderLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewIPRateLimiter(rate.Every(0), 3) // 3 req/s burst

	r := gin.New()
	r.Use(RateLimitMiddleware(limiter))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}
}

func TestRateLimitMiddleware_BlocksOverLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewIPRateLimiter(0, 1) // rate=0 (no refill), burst=1

	r := gin.New()
	r.Use(RateLimitMiddleware(limiter))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// Use up the single burst token
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", w.Code)
	}

	// Second request should be blocked (bucket empty, zero refill rate)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
}

func TestRateLimitMiddleware_DifferentIPsIndependent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewIPRateLimiter(rate.Every(0), 1) // 1 req/s burst per IP

	r := gin.New()
	r.Use(RateLimitMiddleware(limiter))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// First IP uses its burst
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first IP request 1: expected 200, got %d", w.Code)
	}

	// Second IP should have its own bucket
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "5.6.7.8:5678"
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("second IP request 1: expected 200, got %d", w2.Code)
	}
}
