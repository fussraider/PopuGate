package service

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
)

// TelemtConfigProvider abstracts how telemt version/commit/repo are resolved.
type TelemtConfigProvider interface {
	TelemtVersion() string
	TelemtCommit() string
	TelemtRepo() string
}

// DBTelemtConfig reads telemt configuration from the database with env/constant fallback.
// Results are cached for up to cacheTTL to avoid querying the DB on every call.
type DBTelemtConfig struct {
	store    *store.SettingsStore
	cacheTTL time.Duration

	mu      sync.RWMutex
	version string
	commit  string
	repo    string
	cached  time.Time
}

// NewDBTelemtConfig creates a new DBTelemtConfig.
func NewDBTelemtConfig(s *store.SettingsStore) *DBTelemtConfig {
	return &DBTelemtConfig{store: s, cacheTTL: 30 * time.Second}
}

func (c *DBTelemtConfig) refresh() {
	c.mu.RLock()
	if time.Since(c.cached) < c.cacheTTL {
		c.mu.RUnlock()
		return
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.cached) < c.cacheTTL {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if v, _ := c.store.Get(ctx, "telemt_version"); v != "" {
		c.version = v
	} else if v := os.Getenv("TELEMT_VERSION"); v != "" {
		c.version = v
	} else {
		c.version = model.DefaultTelemtVer
	}

	if v, _ := c.store.Get(ctx, "telemt_commit"); v != "" {
		c.commit = v
	} else if v := os.Getenv("TELEMT_COMMIT"); v != "" {
		c.commit = v
	} else {
		c.commit = model.DefaultTelemtRef
	}

	if v, _ := c.store.Get(ctx, "telemt_repo"); v != "" {
		c.repo = v
	} else if v := os.Getenv("TELEMT_REPO"); v != "" {
		c.repo = v
	} else {
		c.repo = model.DefaultTelemtURL
	}

	c.cached = time.Now()
}

func (c *DBTelemtConfig) TelemtVersion() string {
	c.refresh()
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.version
}

func (c *DBTelemtConfig) TelemtCommit() string {
	c.refresh()
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.commit
}

func (c *DBTelemtConfig) TelemtRepo() string {
	c.refresh()
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.repo
}

// InvalidateCache forces a reload on next access.
func (c *DBTelemtConfig) InvalidateCache() {
	c.mu.Lock()
	c.cached = time.Time{}
	c.mu.Unlock()
}

// SetCacheTTL changes the cache TTL (for testing).
func (c *DBTelemtConfig) SetCacheTTL(d time.Duration) {
	c.mu.Lock()
	c.cacheTTL = d
	c.mu.Unlock()
}

// defaultTelemtConfig wraps the existing model package-level functions.
type defaultTelemtConfig struct{}

func (d *defaultTelemtConfig) TelemtVersion() string { return model.TelemtVersion() }
func (d *defaultTelemtConfig) TelemtCommit() string  { return model.TelemtCommit() }
func (d *defaultTelemtConfig) TelemtRepo() string    { return model.TelemtRepo() }
