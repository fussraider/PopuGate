package store

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestInstanceStore_CountEmpty(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)

	count, err := s.Count(context.Background())
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
}

func TestInstanceStore_EnsureDefaultInstanceSeeds(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()

	if err := s.EnsureDefaultInstance(ctx, 443, 9090, "cloudflare.com", "", true); err != nil {
		t.Fatalf("EnsureDefaultInstance: %v", err)
	}

	count, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 instance after seeding, got %d", count)
	}

	got, err := s.GetByPort(ctx, 443)
	if err != nil {
		t.Fatalf("GetByPort: %v", err)
	}
	if got == nil {
		t.Fatal("expected instance, got nil")
	}
	if got.Port != 443 {
		t.Fatalf("expected port 443, got %d", got.Port)
	}
	if got.MetricsPort != 9090 {
		t.Fatalf("expected metrics_port 9090, got %d", got.MetricsPort)
	}
	if !got.Enabled {
		t.Fatal("expected enabled=true for default instance")
	}
	if got.Label != "Default" {
		t.Fatalf("expected label 'Default', got %s", got.Label)
	}
}

func TestInstanceStore_EnsureDefaultInstanceNoOpIfPopulated(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()

	if err := s.EnsureDefaultInstance(ctx, 443, 9090, "cloudflare.com", "", true); err != nil {
		t.Fatalf("EnsureDefaultInstance first: %v", err)
	}
	if err := s.EnsureDefaultInstance(ctx, 8090, 9091, "cloudflare.com", "", true); err != nil {
		t.Fatalf("EnsureDefaultInstance second: %v", err)
	}

	count, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 instance (no-op), got %d", count)
	}

	// Verify original data was not overwritten
	got, err := s.GetByPort(ctx, 443)
	if err != nil {
		t.Fatalf("GetByPort: %v", err)
	}
	if got.Port != 443 {
		t.Fatalf("expected original port 443, got %d", got.Port)
	}
}

func TestInstanceStore_CreateAndGetByPort(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()

	inst := &model.Instance{
		Port:        8443,
		MetricsPort: 9091,
		Enabled:     true,
		Label:       "secondary",
	}
	if err := s.Create(ctx, inst); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inst.ID == 0 {
		t.Fatal("expected ID to be set after Create")
	}

	got, err := s.GetByPort(ctx, 8443)
	if err != nil {
		t.Fatalf("GetByPort: %v", err)
	}
	if got == nil {
		t.Fatal("expected instance, got nil")
	}
	if got.Port != 8443 {
		t.Fatalf("expected port 8443, got %d", got.Port)
	}
	if got.MetricsPort != 9091 {
		t.Fatalf("expected metrics_port 9091, got %d", got.MetricsPort)
	}
	if got.Label != "secondary" {
		t.Fatalf("expected label 'secondary', got %s", got.Label)
	}
}

func TestInstanceStore_ListMultiple(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()

	for _, inst := range []model.Instance{
		{Port: 443, MetricsPort: 9090, Enabled: true, Label: "default"},
		{Port: 8443, MetricsPort: 9091, Enabled: true, Label: "secondary"},
		{Port: 9443, MetricsPort: 9092, Enabled: false, Label: "disabled"},
	} {
		if err := s.Create(ctx, &inst); err != nil {
			t.Fatalf("Create port %d: %v", inst.Port, err)
		}
	}

	instances, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(instances) != 3 {
		t.Fatalf("expected 3 instances, got %d", len(instances))
	}

	// Verify ordering by port
	if instances[0].Port != 443 {
		t.Fatalf("expected first port 443, got %d", instances[0].Port)
	}
	if instances[1].Port != 8443 {
		t.Fatalf("expected second port 8443, got %d", instances[1].Port)
	}
	if instances[2].Port != 9443 {
		t.Fatalf("expected third port 9443, got %d", instances[2].Port)
	}
}

func TestInstanceStore_Delete(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Instance{
		Port: 5555, MetricsPort: 9095, Enabled: true, Label: "temp",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Delete(ctx, 5555); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := s.GetByPort(ctx, 5555)
	if err != nil {
		t.Fatalf("GetByPort: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestInstanceStore_GetByPortNonexistent(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewInstanceStore(db)

	got, err := s.GetByPort(context.Background(), 9999)
	if err != nil {
		t.Fatalf("GetByPort: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent port")
	}
}
