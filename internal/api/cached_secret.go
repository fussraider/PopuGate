package api

import (
	"context"
	"sync"
	"time"
)

// CachedJWTSecretProvider wraps a JWTSecretProvider with in-memory caching.
// The secret is refreshed from the underlying source every refreshInterval.
type CachedJWTSecretProvider struct {
	source          JWTSecretProvider
	refreshInterval time.Duration
	mu              sync.RWMutex
	cachedSecret    string
	lastRefresh     time.Time
}

// NewCachedJWTSecretProvider creates a new cached JWT secret provider.
func NewCachedJWTSecretProvider(source JWTSecretProvider, refreshInterval time.Duration) *CachedJWTSecretProvider {
	return &CachedJWTSecretProvider{
		source:          source,
		refreshInterval: refreshInterval,
	}
}

// GetJWTSecret returns the cached secret, refreshing from the source if stale.
func (p *CachedJWTSecretProvider) GetJWTSecret(ctx context.Context) (string, error) {
	p.mu.RLock()
	if p.cachedSecret != "" && time.Since(p.lastRefresh) < p.refreshInterval {
		secret := p.cachedSecret
		p.mu.RUnlock()
		return secret, nil
	}
	p.mu.RUnlock()

	// Upgrade to write lock to refresh
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have refreshed)
	if p.cachedSecret != "" && time.Since(p.lastRefresh) < p.refreshInterval {
		return p.cachedSecret, nil
	}

	secret, err := p.source.GetJWTSecret(ctx)
	if err != nil {
		// Return stale cached value if available, rather than failing the request
		if p.cachedSecret != "" {
			return p.cachedSecret, nil
		}
		return "", err
	}

	p.cachedSecret = secret
	p.lastRefresh = time.Now()
	return secret, nil
}
