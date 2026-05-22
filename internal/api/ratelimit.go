package api

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// IPRateLimiter provides per-IP rate limiting with periodic cleanup.
type IPRateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitorEntry
	rate     rate.Limit
	burst    int
	stopCh   chan struct{}
	stopOnce sync.Once
}

type visitorEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewIPRateLimiter creates a new per-IP rate limiter.
// r is the token refill rate (tokens per second), burst is the bucket size.
func NewIPRateLimiter(r rate.Limit, burst int) *IPRateLimiter {
	l := &IPRateLimiter{
		visitors: make(map[string]*visitorEntry),
		rate:     r,
		burst:    burst,
		stopCh:   make(chan struct{}),
	}
	go l.cleanupStale()
	return l
}

// Close stops the background cleanup goroutine.
func (l *IPRateLimiter) Close() {
	l.stopOnce.Do(func() { close(l.stopCh) })
}

// cleanupStale removes entries not seen in the last 10 minutes.
func (l *IPRateLimiter) cleanupStale() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-l.stopCh:
			return
		case <-ticker.C:
			l.mu.Lock()
			for ip, v := range l.visitors {
				if time.Since(v.lastSeen) > 10*time.Minute {
					delete(l.visitors, ip)
				}
			}
			l.mu.Unlock()
		}
	}
}

// GetLimiter returns the rate limiter for the given IP.
func (l *IPRateLimiter) GetLimiter(ip string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	if entry, exists := l.visitors[ip]; exists {
		entry.lastSeen = time.Now()
		return entry.limiter
	}

	limiter := rate.NewLimiter(l.rate, l.burst)
	l.visitors[ip] = &visitorEntry{limiter: limiter, lastSeen: time.Now()}
	return limiter
}

// RateLimitMiddleware creates a Gin middleware that limits requests per IP.
func RateLimitMiddleware(limiter *IPRateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !limiter.GetLimiter(ip).Allow() {
			retryAfter := "1"
			if limiter.rate > 0 {
				retryAfter = fmt.Sprintf("%.0f", 1.0/float64(limiter.rate))
			}
			c.Header("Retry-After", retryAfter)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "too many requests, please try again later",
			})
			return
		}
		c.Next()
	}
}
