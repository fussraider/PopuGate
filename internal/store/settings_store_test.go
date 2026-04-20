package store

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestSettingsStore_LoadDefaults(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSettingsStore(db)

	settings, err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if settings.ProxyPort != 443 {
		t.Fatalf("expected ProxyPort=443, got %d", settings.ProxyPort)
	}
	if settings.ProxyMetricsPort != 9090 {
		t.Fatalf("expected ProxyMetricsPort=9090, got %d", settings.ProxyMetricsPort)
	}
	if settings.ProxyDomain != "cloudflare.com" {
		t.Fatalf("expected ProxyDomain=cloudflare.com, got %s", settings.ProxyDomain)
	}
	if settings.ProxyConcurrency != 8192 {
		t.Fatalf("expected ProxyConcurrency=8192, got %d", settings.ProxyConcurrency)
	}
	if settings.MaskingEnabled != true {
		t.Fatal("expected MaskingEnabled=true by default")
	}
	if settings.GeoblockMode != "blacklist" {
		t.Fatalf("expected GeoblockMode=blacklist, got %s", settings.GeoblockMode)
	}
	if settings.TelegramServerLabel != "PopuGate" {
		t.Fatalf("expected TelegramServerLabel=PopuGate, got %s", settings.TelegramServerLabel)
	}
	if settings.ReplicationRole != "standalone" {
		t.Fatalf("expected ReplicationRole=standalone, got %s", settings.ReplicationRole)
	}
}

func TestSettingsStore_SaveLoadRoundtrip(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSettingsStore(db)
	ctx := context.Background()

	updates := map[string]string{
		"proxy_port":       "8443",
		"telegram_enabled": "true",
		"ad_tag":           "deadbeef",
	}
	if err := s.Save(ctx, updates); err != nil {
		t.Fatalf("Save: %v", err)
	}

	settings, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if settings.ProxyPort != 8443 {
		t.Fatalf("expected ProxyPort=8443, got %d", settings.ProxyPort)
	}
	if !settings.TelegramEnabled {
		t.Fatal("expected TelegramEnabled=true")
	}
	if settings.AdTag != "deadbeef" {
		t.Fatalf("expected AdTag=deadbeef, got %s", settings.AdTag)
	}
}

func TestSettingsStore_GetSingleKey(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSettingsStore(db)
	ctx := context.Background()

	val, err := s.Get(ctx, "proxy_port")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "443" {
		t.Fatalf("expected '443', got '%s'", val)
	}
}

func TestSettingsStore_GetNonexistent(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSettingsStore(db)

	val, err := s.Get(context.Background(), "no_such_key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "" {
		t.Fatalf("expected empty string, got '%s'", val)
	}
}

func TestSettingsStore_GetJWTSecretGeneratesAndPersists(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSettingsStore(db)
	ctx := context.Background()

	secret1, err := s.GetJWTSecret(ctx)
	if err != nil {
		t.Fatalf("GetJWTSecret first: %v", err)
	}
	if secret1 == "" {
		t.Fatal("expected non-empty JWT secret")
	}
	if len(secret1) != 64 { // 32 bytes hex-encoded
		t.Fatalf("expected 64-char hex string, got %d chars", len(secret1))
	}

	secret2, err := s.GetJWTSecret(ctx)
	if err != nil {
		t.Fatalf("GetJWTSecret second: %v", err)
	}
	if secret2 != secret1 {
		t.Fatalf("expected same secret on second call, got %s then %s", secret1, secret2)
	}
}

func TestSettingsStore_AuthPasswordHashRoundtrip(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSettingsStore(db)
	ctx := context.Background()

	hash, err := s.GetAuthPasswordHash(ctx)
	if err != nil {
		t.Fatalf("GetAuthPasswordHash (empty): %v", err)
	}
	if hash != "" {
		t.Fatalf("expected empty hash initially, got '%s'", hash)
	}

	fakeHash := "$2a$10$abcdefghijklmnopqrstuvwxyz0123456789"
	if err := s.SetAuthPasswordHash(ctx, fakeHash); err != nil {
		t.Fatalf("SetAuthPasswordHash: %v", err)
	}

	got, err := s.GetAuthPasswordHash(ctx)
	if err != nil {
		t.Fatalf("GetAuthPasswordHash: %v", err)
	}
	if got != fakeHash {
		t.Fatalf("expected '%s', got '%s'", fakeHash, got)
	}
}
