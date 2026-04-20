package store

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestUpstreamStore_ListEmpty(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)

	upstreams, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(upstreams) != 0 {
		t.Fatalf("expected empty list, got %d", len(upstreams))
	}
}

func TestUpstreamStore_CreateAndGetByName(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	u := &model.Upstream{
		Name:     "my-proxy",
		Type:     model.UpstreamSOCKS5,
		Address:  "10.0.0.1:1080",
		Username: "user",
		Password: "pass",
		Weight:   50,
		Iface:    "eth0",
		Enabled:  true,
	}
	if err := s.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("expected ID to be set after Create")
	}

	got, err := s.GetByName(ctx, "my-proxy")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got == nil {
		t.Fatal("expected upstream, got nil")
	}
	if got.Name != "my-proxy" {
		t.Fatalf("expected name my-proxy, got %s", got.Name)
	}
	if got.Type != model.UpstreamSOCKS5 {
		t.Fatalf("expected type socks5, got %s", got.Type)
	}
	if got.Address != "10.0.0.1:1080" {
		t.Fatalf("expected address 10.0.0.1:1080, got %s", got.Address)
	}
	if got.Username != "user" {
		t.Fatalf("expected username user, got %s", got.Username)
	}
	if got.Password != "pass" {
		t.Fatalf("expected password pass, got %s", got.Password)
	}
	if got.Weight != 50 {
		t.Fatalf("expected weight 50, got %d", got.Weight)
	}
	if got.Iface != "eth0" {
		t.Fatalf("expected iface eth0, got %s", got.Iface)
	}
	if !got.Enabled {
		t.Fatal("expected enabled=true")
	}
}

func TestUpstreamStore_CreateMultipleAndList(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	for _, name := range []string{"up1", "up2", "up3"} {
		if err := s.Create(ctx, &model.Upstream{
			Name:    name,
			Type:    model.UpstreamDirect,
			Weight:  10,
			Enabled: true,
		}); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}

	upstreams, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(upstreams) != 3 {
		t.Fatalf("expected 3 upstreams, got %d", len(upstreams))
	}
}

func TestUpstreamStore_Delete(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Upstream{
		Name: "del-me", Type: model.UpstreamDirect, Weight: 10, Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Delete(ctx, "del-me"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := s.GetByName(ctx, "del-me")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestUpstreamStore_UpdateEnabled(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Upstream{
		Name: "toggle", Type: model.UpstreamDirect, Weight: 10, Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Disable
	if err := s.UpdateEnabled(ctx, "toggle", false); err != nil {
		t.Fatalf("UpdateEnabled false: %v", err)
	}
	got, err := s.GetByName(ctx, "toggle")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.Enabled {
		t.Fatal("expected enabled=false after disabling")
	}

	// Re-enable
	if err := s.UpdateEnabled(ctx, "toggle", true); err != nil {
		t.Fatalf("UpdateEnabled true: %v", err)
	}
	got, err = s.GetByName(ctx, "toggle")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if !got.Enabled {
		t.Fatal("expected enabled=true after re-enabling")
	}
}

func TestUpstreamStore_GetByNameNonexistent(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)

	got, err := s.GetByName(context.Background(), "nope")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent name")
	}
}
