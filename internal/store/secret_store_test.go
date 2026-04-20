package store

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestSecretStore_ListEmpty(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)

	secrets, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(secrets) != 0 {
		t.Fatalf("expected empty list, got %d", len(secrets))
	}
}

func TestSecretStore_CreateAndGetByLabel(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	sec := &model.Secret{
		Label:      "user1",
		SecretKey:  "aa111111111111111111111111111111",
		Enabled:    true,
		MaxConns:   10,
		MaxIPs:     5,
		QuotaBytes: 1024 * 1024,
		ExpiresAt:  "0",
		Notes:      "test user",
	}
	if err := s.Create(ctx, sec); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sec.ID == 0 {
		t.Fatal("expected ID to be set after Create")
	}

	got, err := s.GetByLabel(ctx, "user1")
	if err != nil {
		t.Fatalf("GetByLabel: %v", err)
	}
	if got == nil {
		t.Fatal("expected secret, got nil")
	}
	if got.Label != "user1" {
		t.Fatalf("expected label user1, got %s", got.Label)
	}
	if got.SecretKey != "aa111111111111111111111111111111" {
		t.Fatalf("unexpected secret key: %s", got.SecretKey)
	}
	if !got.Enabled {
		t.Fatal("expected enabled=true")
	}
	if got.MaxConns != 10 {
		t.Fatalf("expected max_conns=10, got %d", got.MaxConns)
	}
	if got.MaxIPs != 5 {
		t.Fatalf("expected max_ips=5, got %d", got.MaxIPs)
	}
	if got.QuotaBytes != 1024*1024 {
		t.Fatalf("expected quota_bytes=%d, got %d", 1024*1024, got.QuotaBytes)
	}
	if got.Notes != "test user" {
		t.Fatalf("expected notes 'test user', got %s", got.Notes)
	}
	if got.TrafficIn != 0 || got.TrafficOut != 0 {
		t.Fatalf("expected zero traffic, got in=%d out=%d", got.TrafficIn, got.TrafficOut)
	}
}

func TestSecretStore_CreateMultipleAndList(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	for _, label := range []string{"alpha", "beta", "gamma"} {
		if err := s.Create(ctx, &model.Secret{
			Label:     label,
			SecretKey: "aa" + label + "0000000000000000000000000000",
			Enabled:   true,
		}); err != nil {
			t.Fatalf("Create %s: %v", label, err)
		}
	}

	secrets, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(secrets) != 3 {
		t.Fatalf("expected 3 secrets, got %d", len(secrets))
	}
}

func TestSecretStore_Update(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	sec := &model.Secret{
		Label:      "user1",
		SecretKey:  "aa111111111111111111111111111111",
		Enabled:    true,
		MaxConns:   10,
		QuotaBytes: 1000,
	}
	if err := s.Create(ctx, sec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	sec.SecretKey = "bb222222222222222222222222222222"
	sec.Enabled = false
	sec.MaxConns = 20
	sec.QuotaBytes = 2000
	sec.Notes = "updated"
	if err := s.Update(ctx, sec); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.GetByLabel(ctx, "user1")
	if err != nil {
		t.Fatalf("GetByLabel: %v", err)
	}
	if got.SecretKey != "bb222222222222222222222222222222" {
		t.Fatalf("expected updated secret key, got %s", got.SecretKey)
	}
	if got.Enabled {
		t.Fatal("expected enabled=false after update")
	}
	if got.MaxConns != 20 {
		t.Fatalf("expected max_conns=20, got %d", got.MaxConns)
	}
	if got.QuotaBytes != 2000 {
		t.Fatalf("expected quota_bytes=2000, got %d", got.QuotaBytes)
	}
	if got.Notes != "updated" {
		t.Fatalf("expected notes 'updated', got %s", got.Notes)
	}
}

func TestSecretStore_Delete(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Secret{
		Label:     "del-me",
		SecretKey: "aa000000000000000000000000000000",
		Enabled:   true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Delete(ctx, "del-me"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := s.GetByLabel(ctx, "del-me")
	if err != nil {
		t.Fatalf("GetByLabel: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestSecretStore_CountEnabled(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	for i, label := range []string{"a", "b", "c", "d"} {
		enabled := i < 3 // first 3 enabled, last disabled
		if err := s.Create(ctx, &model.Secret{
			Label:     label,
			SecretKey: "aa" + label + "0000000000000000000000000000",
			Enabled:   enabled,
		}); err != nil {
			t.Fatalf("Create %s: %v", label, err)
		}
	}

	count, err := s.CountEnabled(ctx)
	if err != nil {
		t.Fatalf("CountEnabled: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 enabled, got %d", count)
	}

	total, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if total != 4 {
		t.Fatalf("expected 4 total, got %d", total)
	}
}

func TestSecretStore_ListEnabledLabels(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Secret{
		Label: "enabled1", SecretKey: "aa000000000000000000000000000000", Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Create(ctx, &model.Secret{
		Label: "disabled1", SecretKey: "bb000000000000000000000000000000", Enabled: false,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Create(ctx, &model.Secret{
		Label: "enabled2", SecretKey: "cc000000000000000000000000000000", Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	labels, err := s.ListEnabledLabels(ctx)
	if err != nil {
		t.Fatalf("ListEnabledLabels: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(labels))
	}
	if labels[0] != "enabled1" || labels[1] != "enabled2" {
		t.Fatalf("expected [enabled1, enabled2], got %v", labels)
	}
}

func TestSecretStore_UpdateTrafficCumulative(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Secret{
		Label: "user1", SecretKey: "aa000000000000000000000000000000", Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.UpdateTraffic(ctx, "user1", 100, 200); err != nil {
		t.Fatalf("UpdateTraffic first: %v", err)
	}
	if err := s.UpdateTraffic(ctx, "user1", 50, 75); err != nil {
		t.Fatalf("UpdateTraffic second: %v", err)
	}

	got, err := s.GetByLabel(ctx, "user1")
	if err != nil {
		t.Fatalf("GetByLabel: %v", err)
	}
	if got.TrafficIn != 150 {
		t.Fatalf("expected traffic_in=150, got %d", got.TrafficIn)
	}
	if got.TrafficOut != 275 {
		t.Fatalf("expected traffic_out=275, got %d", got.TrafficOut)
	}
}

func TestSecretStore_ResetTraffic(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Secret{
		Label: "user1", SecretKey: "aa000000000000000000000000000000", Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.UpdateTraffic(ctx, "user1", 500, 600); err != nil {
		t.Fatalf("UpdateTraffic: %v", err)
	}

	if err := s.ResetTraffic(ctx, "user1"); err != nil {
		t.Fatalf("ResetTraffic: %v", err)
	}

	got, err := s.GetByLabel(ctx, "user1")
	if err != nil {
		t.Fatalf("GetByLabel: %v", err)
	}
	if got.TrafficIn != 0 || got.TrafficOut != 0 {
		t.Fatalf("expected zero traffic after reset, got in=%d out=%d", got.TrafficIn, got.TrafficOut)
	}
}

func TestSecretStore_ResetAllTraffic(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	for _, label := range []string{"user1", "user2"} {
		if err := s.Create(ctx, &model.Secret{
			Label: label, SecretKey: "aa" + label + "0000000000000000000000000", Enabled: true,
		}); err != nil {
			t.Fatalf("Create %s: %v", label, err)
		}
	}

	s.UpdateTraffic(ctx, "user1", 100, 200)
	s.UpdateTraffic(ctx, "user2", 300, 400)

	if err := s.ResetAllTraffic(ctx); err != nil {
		t.Fatalf("ResetAllTraffic: %v", err)
	}

	for _, label := range []string{"user1", "user2"} {
		got, err := s.GetByLabel(ctx, label)
		if err != nil {
			t.Fatalf("GetByLabel %s: %v", label, err)
		}
		if got.TrafficIn != 0 || got.TrafficOut != 0 {
			t.Fatalf("%s: expected zero traffic, got in=%d out=%d", label, got.TrafficIn, got.TrafficOut)
		}
	}
}

func TestSecretStore_GetByLabelNonexistent(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)

	got, err := s.GetByLabel(context.Background(), "nope")
	if err != nil {
		t.Fatalf("GetByLabel: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent label")
	}
}
