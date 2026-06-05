package service

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func newTestContainerService(t *testing.T) (*ContainerService, *store.InstanceStore, *store.SecretStore) {
	t.Helper()
	db := testutil.OpenTestDB(t)
	instances := store.NewInstanceStore(db)
	secrets := store.NewSecretStore(db)
	settings := store.NewSettingsStore(db)
	upstreams := store.NewUpstreamStore(db)
	traffic := store.NewTrafficStore(db)
	svc := NewContainerService(t.TempDir(), nil, secrets, upstreams, instances, traffic, settings, nil)
	return svc, instances, secrets
}

func TestContainerService_EnsureDefaultInstance_Seeds(t *testing.T) {
	svc, instances, _ := newTestContainerService(t)
	ctx := context.Background()

	if err := svc.EnsureDefaultInstance(ctx, 443, 9090, "cloudflare.com", "", true); err != nil {
		t.Fatalf("EnsureDefaultInstance: %v", err)
	}

	count, err := instances.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 instance after seeding, got %d", count)
	}

	got, err := instances.GetByPort(ctx, 443)
	if err != nil {
		t.Fatalf("GetByPort: %v", err)
	}
	if got == nil {
		t.Fatal("expected instance, got nil")
	}
	if got.Port != 443 {
		t.Fatalf("expected port 443, got %d", got.Port)
	}
	if !got.Enabled {
		t.Fatal("expected enabled=true")
	}
	if got.Label != "Default" {
		t.Fatalf("expected label 'Default', got %s", got.Label)
	}
	if !got.FakeTLS {
		t.Fatal("expected fake_tls=true when maskingEnabled=true")
	}
}

func TestContainerService_EnsureDefaultInstance_NoOpIfPopulated(t *testing.T) {
	svc, instances, _ := newTestContainerService(t)
	ctx := context.Background()

	if err := svc.EnsureDefaultInstance(ctx, 443, 9090, "cloudflare.com", "", true); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := svc.EnsureDefaultInstance(ctx, 8090, 9091, "cloudflare.com", "", true); err != nil {
		t.Fatalf("second call: %v", err)
	}

	count, err := instances.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 instance (no-op), got %d", count)
	}

	got, err := instances.GetByPort(ctx, 443)
	if err != nil {
		t.Fatalf("GetByPort: %v", err)
	}
	if got.Port != 443 {
		t.Fatalf("expected original port 443, got %d", got.Port)
	}
}

func TestContainerService_EnsureDefaultInstance_NoMasking(t *testing.T) {
	svc, instances, _ := newTestContainerService(t)
	ctx := context.Background()

	if err := svc.EnsureDefaultInstance(ctx, 443, 9090, "cloudflare.com", "", false); err != nil {
		t.Fatalf("EnsureDefaultInstance: %v", err)
	}

	got, err := instances.GetByPort(ctx, 443)
	if err != nil {
		t.Fatalf("GetByPort: %v", err)
	}
	if got.FakeTLS {
		t.Fatal("expected fake_tls=false when maskingEnabled=false")
	}
}

func TestContainerService_BuildSecretCounts(t *testing.T) {
	svc, _, secrets := newTestContainerService(t)
	ctx := context.Background()

	// Create instances
	instStore := svc.instances
	inst1 := &model.Instance{Port: 443, MetricsPort: 9090, Enabled: true, Label: "untagged"}
	_ = inst1.Validate()
	_ = instStore.Create(ctx, inst1)

	inst2 := &model.Instance{Port: 8443, MetricsPort: 9091, Enabled: true, Label: "tagged", Tags: `["vip"]`}
	_ = inst2.Validate()
	_ = instStore.Create(ctx, inst2)

	inst3 := &model.Instance{Port: 9443, MetricsPort: 9092, Enabled: true, Label: "multi", Tags: `["vip","paid"]`}
	_ = inst3.Validate()
	_ = instStore.Create(ctx, inst3)

	// Create secrets
	_ = secrets.Create(ctx, &model.Secret{Label: "s1", SecretKey: "aa000000000000000000000000000000", Enabled: true, Tags: ""})
	_ = secrets.Create(ctx, &model.Secret{Label: "s2", SecretKey: "bb000000000000000000000000000000", Enabled: true, Tags: `["vip"]`})
	_ = secrets.Create(ctx, &model.Secret{Label: "s3", SecretKey: "cc000000000000000000000000000000", Enabled: true, Tags: `["paid"]`})
	_ = secrets.Create(ctx, &model.Secret{Label: "s4", SecretKey: "dd000000000000000000000000000000", Enabled: false, Tags: `["vip"]`})

	insts, _ := instStore.List(ctx)
	counts, _ := svc.buildSecretCounts(ctx, insts)

	// inst1 (no tags) matches all enabled secrets: s1, s2, s3 = 3
	if counts[inst1.ID] != 3 {
		t.Fatalf("untagged instance: expected 3, got %d", counts[inst1.ID])
	}

	// inst2 (tag: vip) matches s2 (vip) = 1 (s4 is disabled)
	if counts[inst2.ID] != 1 {
		t.Fatalf("vip instance: expected 1, got %d", counts[inst2.ID])
	}

	// inst3 (tags: vip,paid) matches s2 (vip) and s3 (paid) = 2
	if counts[inst3.ID] != 2 {
		t.Fatalf("multi-tag instance: expected 2, got %d", counts[inst3.ID])
	}
}

func TestContainerService_BuildSecretCounts_NoSecrets(t *testing.T) {
	svc, _, _ := newTestContainerService(t)
	ctx := context.Background()

	inst := &model.Instance{Port: 443, MetricsPort: 9090, Enabled: true, Label: "alone"}
	_ = inst.Validate()
	_ = svc.instances.Create(ctx, inst)

	insts, _ := svc.instances.List(ctx)
	counts, _ := svc.buildSecretCounts(ctx, insts)

	if counts[inst.ID] != 0 {
		t.Fatalf("expected 0 with no secrets, got %d", counts[inst.ID])
	}
}

func TestContainerService_GetActiveContainerName(t *testing.T) {
	svc, _, _ := newTestContainerService(t)
	ctx := context.Background()

	inst := &model.Instance{Port: 443, MetricsPort: 9090, Enabled: true, Label: "Test"}

	// When s.docker is nil, it should gracefully fall back to the primary container name.
	activeName := svc.GetActiveContainerName(ctx, inst)
	expectedName := inst.ContainerName()
	if activeName != expectedName {
		t.Errorf("expected active container name %q, got %q", expectedName, activeName)
	}
}

func TestContainerService_Reload_DockerNil(t *testing.T) {
	svc, _, _ := newTestContainerService(t)
	ctx := context.Background()

	err := svc.Reload(ctx, "test")
	if err == nil {
		t.Fatal("expected error when Reload is called with nil docker client, got nil")
	}
}
