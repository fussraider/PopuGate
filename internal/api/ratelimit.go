package api

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// IPRateLimiter provides per-IP rate limiting.
type IPRateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

// NewIPRateLimiter creates a new per-IP rate limiter.
// r is the token refill rate (tokens per second), burst is the bucket size.
func NewIPRateLimiter(r rate.Limit, burst int) *IPRateLimiter {
	return &IPRateLimiter{
		visitors: make(map[string]*rate.Limiter),
		rate:     r,
		burst:    burst,
	}
}

// GetLimiter returns the rate limiter for the given IP.
func (l *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	if limiter, exists := l.visitors[ip]; exists {
		return limiter
	}

	limiter := rate.NewLimiter(l.rate, l.burst)
	l.visitors[ip] = limiter
	return limiter
}

// RateLimitMiddleware creates a Gin middleware that limits requests per IP.
func RateLimitMiddleware(limiter *IPRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !limiter.GetLimiter(ip).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "too many requests, please try again later",
			})
			return
		}
		c.Next()
	}
}
