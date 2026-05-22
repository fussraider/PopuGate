package service

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestDBTelemtConfig_DBValue(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := store.NewSettingsStore(db)
	_ = s.Save(context.Background(), map[string]string{"telemt_version": "9.9.9"})

	cfg := NewDBTelemtConfig(s)
	cfg.SetCacheTTL(0)

	if got := cfg.TelemtVersion(); got != "9.9.9" {
		t.Errorf("got %q, want 9.9.9", got)
	}
}

func TestDBTelemtConfig_EnvFallback(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := store.NewSettingsStore(db)

	orig := os.Getenv("TELEMT_VERSION")
	t.Cleanup(func() { _ = os.Setenv("TELEMT_VERSION", orig) })
	_ = os.Setenv("TELEMT_VERSION", "8.8.8")

	cfg := NewDBTelemtConfig(s)
	cfg.SetCacheTTL(0)

	if got := cfg.TelemtVersion(); got != "8.8.8" {
		t.Errorf("got %q, want 8.8.8", got)
	}
}

func TestDBTelemtConfig_DefaultFallback(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := store.NewSettingsStore(db)

	orig := os.Getenv("TELEMT_VERSION")
	t.Cleanup(func() { _ = os.Setenv("TELEMT_VERSION", orig) })
	_ = os.Unsetenv("TELEMT_VERSION")

	cfg := NewDBTelemtConfig(s)
	cfg.SetCacheTTL(0)

	if got := cfg.TelemtVersion(); got != model.DefaultTelemtVer {
		t.Errorf("got %q, want %q", got, model.DefaultTelemtVer)
	}
}

func TestDBTelemtConfig_CommitDBValue(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := store.NewSettingsStore(db)
	_ = s.Save(context.Background(), map[string]string{"telemt_commit": "abcdef1"})

	cfg := NewDBTelemtConfig(s)
	cfg.SetCacheTTL(0)

	if got := cfg.TelemtCommit(); got != "abcdef1" {
		t.Errorf("got %q, want abcdef1", got)
	}
}

func TestDBTelemtConfig_RepoDBValue(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := store.NewSettingsStore(db)
	_ = s.Save(context.Background(), map[string]string{"telemt_repo": "https://example.com/repo.git"})

	cfg := NewDBTelemtConfig(s)
	cfg.SetCacheTTL(0)

	if got := cfg.TelemtRepo(); got != "https://example.com/repo.git" {
		t.Errorf("got %q, want https://example.com/repo.git", got)
	}
}

func TestDBTelemtConfig_InvalidateCache(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := store.NewSettingsStore(db)

	cfg := NewDBTelemtConfig(s)
	cfg.SetCacheTTL(time.Hour)
	cfg.TelemtVersion() // populate cache

	_ = s.Save(context.Background(), map[string]string{"telemt_version": "7.7.7"})

	if got := cfg.TelemtVersion(); got != model.DefaultTelemtVer {
		t.Errorf("expected cached default, got %q", got)
	}

	cfg.InvalidateCache()
	if got := cfg.TelemtVersion(); got != "7.7.7" {
		t.Errorf("got %q, want 7.7.7 after invalidation", got)
	}
}

func TestDBTelemtConfig_CacheTTL(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := store.NewSettingsStore(db)

	cfg := NewDBTelemtConfig(s)
	cfg.SetCacheTTL(100 * time.Millisecond)

	cfg.TelemtVersion() // populate cache

	_ = s.Save(context.Background(), map[string]string{"telemt_version": "5.5.5"})

	// Should still return cached value
	if got := cfg.TelemtVersion(); got != model.DefaultTelemtVer {
		t.Errorf("expected cached default, got %q", got)
	}

	time.Sleep(150 * time.Millisecond)

	// Cache should have expired, read fresh value
	if got := cfg.TelemtVersion(); got != "5.5.5" {
		t.Errorf("got %q, want 5.5.5 after TTL expiry", got)
	}
}

func TestDefaultTelemtConfig(t *testing.T) {
	cfg := &defaultTelemtConfig{}

	if got := cfg.TelemtVersion(); got != model.TelemtVersion() {
		t.Errorf("got %q, want %q", got, model.TelemtVersion())
	}
	if got := cfg.TelemtCommit(); got != model.TelemtCommit() {
		t.Errorf("got %q, want %q", got, model.TelemtCommit())
	}
	if got := cfg.TelemtRepo(); got != model.TelemtRepo() {
		t.Errorf("got %q, want %q", got, model.TelemtRepo())
	}
}
