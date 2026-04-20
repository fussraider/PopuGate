package store

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestSlaveStore_ListEmpty(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSlaveStore(db)

	slaves, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(slaves) != 0 {
		t.Fatalf("expected empty list, got %d", len(slaves))
	}
}

func TestSlaveStore_CreateAndGetByHost(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSlaveStore(db)
	ctx := context.Background()

	sl := &model.Slave{
		Host:    "192.168.1.10",
		Port:    22,
		Label:   "slave-1",
		Enabled: true,
		Status:  "unknown",
	}
	if err := s.Create(ctx, sl); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sl.ID == 0 {
		t.Fatal("expected ID to be set after Create")
	}

	got, err := s.GetByHost(ctx, "192.168.1.10")
	if err != nil {
		t.Fatalf("GetByHost: %v", err)
	}
	if got == nil {
		t.Fatal("expected slave, got nil")
	}
	if got.Host != "192.168.1.10" {
		t.Fatalf("expected host 192.168.1.10, got %s", got.Host)
	}
	if got.Port != 22 {
		t.Fatalf("expected port 22, got %d", got.Port)
	}
	if got.Label != "slave-1" {
		t.Fatalf("expected label slave-1, got %s", got.Label)
	}
	if !got.Enabled {
		t.Fatal("expected enabled=true by default")
	}
	if got.Status != "unknown" {
		t.Fatalf("expected status unknown, got %s", got.Status)
	}
}

func TestSlaveStore_CreateMultipleAndList(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSlaveStore(db)
	ctx := context.Background()

	hosts := []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}
	for _, host := range hosts {
		if err := s.Create(ctx, &model.Slave{
			Host:   host,
			Port:   22,
			Label:  host,
			Status: "unknown",
		}); err != nil {
			t.Fatalf("Create %s: %v", host, err)
		}
	}

	slaves, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(slaves) != 3 {
		t.Fatalf("expected 3 slaves, got %d", len(slaves))
	}
}

func TestSlaveStore_Delete(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSlaveStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Slave{
		Host: "10.0.0.99", Port: 22, Label: "temp", Status: "unknown",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Delete(ctx, "10.0.0.99"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := s.GetByHost(ctx, "10.0.0.99")
	if err != nil {
		t.Fatalf("GetByHost: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestSlaveStore_UpdateStatus(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSlaveStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Slave{
		Host: "10.0.0.1", Port: 22, Label: "slave-1", Status: "unknown",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.UpdateStatus(ctx, "10.0.0.1", "ok", 1700000000); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	got, err := s.GetByHost(ctx, "10.0.0.1")
	if err != nil {
		t.Fatalf("GetByHost: %v", err)
	}
	if got.Status != "ok" {
		t.Fatalf("expected status ok, got %s", got.Status)
	}
	if got.LastSync != 1700000000 {
		t.Fatalf("expected last_sync=1700000000, got %d", got.LastSync)
	}
}

func TestSlaveStore_GetByHostNonexistent(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSlaveStore(db)

	got, err := s.GetByHost(context.Background(), "nope")
	if err != nil {
		t.Fatalf("GetByHost: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent host")
	}
}
