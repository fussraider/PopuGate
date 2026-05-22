package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func newTestReplicationService(t *testing.T) (*ReplicationService, *store.SlaveStore) {
	t.Helper()
	db := testutil.OpenTestDB(t)
	settings := store.NewSettingsStore(db)
	slaves := store.NewSlaveStore(db)
	instances := store.NewInstanceStore(db)
	return NewReplicationService(settings, slaves, instances), slaves
}

func TestReplicationService_BuildSyncConfig(t *testing.T) {
	svc, _ := newTestReplicationService(t)
	ctx := context.Background()

	// Save settings so Load returns them
	_ = svc.settings.Save(ctx, map[string]string{
		"replication_ssh_user":     "deploy",
		"replication_delete_extra": "true",
		"replication_exclude":      "backups,.ssh,settings.db",
	})

	loaded, _ := svc.settings.Load(ctx)
	slave := model.Slave{Host: "10.0.0.5", Port: 22}

	cfg := svc.buildSyncConfig(loaded, slave)

	if cfg.Host != "10.0.0.5" {
		t.Fatalf("expected host 10.0.0.5, got %s", cfg.Host)
	}
	if cfg.Port != 22 {
		t.Fatalf("expected port 22, got %d", cfg.Port)
	}
	if cfg.User != "deploy" {
		t.Fatalf("expected user deploy, got %s", cfg.User)
	}
	if !cfg.DeleteExtra {
		t.Fatal("expected DeleteExtra=true")
	}
	if len(cfg.Exclude) != 3 {
		t.Fatalf("expected 3 exclude entries, got %d: %v", len(cfg.Exclude), cfg.Exclude)
	}
	if !strings.Contains(cfg.KnownHostsPath, ".ssh/known_hosts") {
		t.Fatalf("expected known_hosts path, got %s", cfg.KnownHostsPath)
	}
}

func TestReplicationService_GenerateSSHKey(t *testing.T) {
	svc, _ := newTestReplicationService(t)
	ctx := context.Background()

	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "id_ed25519")

	// Save settings with custom key path
	_ = svc.settings.Save(ctx, map[string]string{"replication_ssh_key_path": keyPath})

	pubKey, err := svc.GenerateSSHKey(ctx)
	if err != nil {
		t.Fatalf("GenerateSSHKey: %v", err)
	}
	if !strings.HasPrefix(pubKey, "ssh-ed25519 ") {
		t.Fatalf("expected ed25519 public key, got %s", pubKey)
	}

	// Verify key files exist
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("private key not found: %v", err)
	}
	if _, err := os.Stat(keyPath + ".pub"); err != nil {
		t.Fatalf("public key file not found: %v", err)
	}
}
