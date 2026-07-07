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

	testutil.SeedTraffic(t, db, "user1", 100, 200)
	testutil.SeedTraffic(t, db, "user1", 50, 75)

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

func TestSecretStore_RenameLabel(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	sec := &model.Secret{
		Label:     "old-name",
		SecretKey: "aa000000000000000000000000000000",
		Enabled:   true,
	}
	if err := s.Create(ctx, sec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.RenameLabel(ctx, "old-name", "new-name"); err != nil {
		t.Fatalf("RenameLabel: %v", err)
	}

	got, err := s.GetByLabel(ctx, "new-name")
	if err != nil {
		t.Fatalf("GetByLabel new: %v", err)
	}
	if got == nil {
		t.Fatal("expected secret with new label")
	}
	if got.Label != "new-name" {
		t.Fatalf("expected label 'new-name', got %s", got.Label)
	}

	old, err := s.GetByLabel(ctx, "old-name")
	if err != nil {
		t.Fatalf("GetByLabel old: %v", err)
	}
	if old != nil {
		t.Fatal("expected nil for old label after rename")
	}
}

func TestSecretStore_RenameLabel_Nonexistent(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	err := s.RenameLabel(ctx, "ghost", "new-ghost")
	if err == nil {
		t.Fatal("expected error renaming nonexistent secret")
	}
}

func TestSecretStore_ExtendExpiry(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	sec := &model.Secret{
		Label:     "user1",
		SecretKey: "aa000000000000000000000000000000",
		Enabled:   false,
		ExpiresAt: "2020-01-01T00:00:00Z",
	}
	if err := s.Create(ctx, sec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.ExtendExpiry(ctx, "user1", "2030-12-31T23:59:59Z", true); err != nil {
		t.Fatalf("ExtendExpiry: %v", err)
	}

	got, err := s.GetByLabel(ctx, "user1")
	if err != nil {
		t.Fatalf("GetByLabel: %v", err)
	}
	if got.ExpiresAt != "2030-12-31T23:59:59Z" {
		t.Fatalf("expected new expiry, got %s", got.ExpiresAt)
	}
	if !got.Enabled {
		t.Fatal("expected secret to be re-enabled")
	}
}

func TestSecretStore_DisableExpired(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	past := "2020-01-01T00:00:00Z"

	// Create two secrets, one expired
	if err := s.Create(ctx, &model.Secret{
		Label: "expired-user", SecretKey: "aa000000000000000000000000000000",
		Enabled: true, ExpiresAt: past,
	}); err != nil {
		t.Fatalf("Create expired: %v", err)
	}
	if err := s.Create(ctx, &model.Secret{
		Label: "active-user", SecretKey: "bb000000000000000000000000000000",
		Enabled: true, ExpiresAt: "0",
	}); err != nil {
		t.Fatalf("Create active: %v", err)
	}

	count, err := s.DisableExpired(ctx)
	if err != nil {
		t.Fatalf("DisableExpired: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 disabled, got %d", count)
	}

	expired, _ := s.GetByLabel(ctx, "expired-user")
	if expired.Enabled {
		t.Fatal("expired secret should be disabled")
	}

	active, _ := s.GetByLabel(ctx, "active-user")
	if !active.Enabled {
		t.Fatal("active secret should still be enabled")
	}
}

func TestSecretStore_Search(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	_ = s.Create(ctx, &model.Secret{Label: "alice", SecretKey: "aa000000000000000000000000000000", Enabled: true, Notes: "VIP"})
	_ = s.Create(ctx, &model.Secret{Label: "bob", SecretKey: "bb000000000000000000000000000000", Enabled: true})

	results, err := s.Search(ctx, "ali")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Label != "alice" {
		t.Fatalf("expected [alice], got %v", results)
	}

	results2, err := s.Search(ctx, "VIP")
	if err != nil {
		t.Fatalf("Search notes: %v", err)
	}
	if len(results2) != 1 || results2[0].Label != "alice" {
		t.Fatalf("expected [alice] by notes, got %v", results2)
	}
}

func TestSecretStore_Top(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	_ = s.Create(ctx, &model.Secret{Label: "low", SecretKey: "aa000000000000000000000000000000", Enabled: true})
	_ = s.Create(ctx, &model.Secret{Label: "high", SecretKey: "bb000000000000000000000000000000", Enabled: true})
	testutil.SeedTraffic(t, db, "high", 10000, 5000)
	testutil.SeedTraffic(t, db, "low", 100, 50)

	results, err := s.Top(ctx, 10)
	if err != nil {
		t.Fatalf("Top: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Label != "high" {
		t.Errorf("top = %q, want %q", results[0].Label, "high")
	}
}

func TestSecretStore_Top_DefaultLimit(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	// Invalid limit should default to 10
	results, err := s.Top(ctx, -1)
	if err != nil {
		t.Fatalf("Top with negative limit: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestSecretStore_UpdateTags(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	_ = s.Create(ctx, &model.Secret{Label: "user1", SecretKey: "aa000000000000000000000000000000", Enabled: true})

	if err := s.UpdateTags(ctx, "user1", `["vip","paid"]`); err != nil {
		t.Fatalf("UpdateTags: %v", err)
	}

	got, _ := s.GetByLabel(ctx, "user1")
	if got.Tags != `["vip","paid"]` {
		t.Errorf("Tags = %q, want %q", got.Tags, `["vip","paid"]`)
	}
}

func TestSecretStore_ListByTag(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	_ = s.Create(ctx, &model.Secret{Label: "vip1", SecretKey: "aa000000000000000000000000000000", Enabled: true, Tags: `["vip","paid"]`})
	_ = s.Create(ctx, &model.Secret{Label: "vip2", SecretKey: "bb000000000000000000000000000000", Enabled: true, Tags: `["vip"]`})
	_ = s.Create(ctx, &model.Secret{Label: "free1", SecretKey: "cc000000000000000000000000000000", Enabled: true, Tags: `["free"]`})

	results, err := s.ListByTag(ctx, "vip")
	if err != nil {
		t.Fatalf("ListByTag: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestSecretStore_Archive_Unarchive(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	_ = s.Create(ctx, &model.Secret{Label: "user1", SecretKey: "aa000000000000000000000000000000", Enabled: true})

	if err := s.Archive(ctx, "user1"); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	got, _ := s.GetByLabel(ctx, "user1")
	if got.ArchivedAt == 0 {
		t.Error("expected non-zero archived_at")
	}

	if err := s.Unarchive(ctx, "user1"); err != nil {
		t.Fatalf("Unarchive: %v", err)
	}

	got, _ = s.GetByLabel(ctx, "user1")
	if got.ArchivedAt != 0 {
		t.Error("expected zero archived_at after unarchive")
	}
}

func TestSecretStore_BulkExtendExpiry(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	_ = s.Create(ctx, &model.Secret{Label: "user1", SecretKey: "aa000000000000000000000000000000", Enabled: true})
	_ = s.Create(ctx, &model.Secret{Label: "user2", SecretKey: "bb000000000000000000000000000000", Enabled: true})

	updated, err := s.BulkExtendExpiry(ctx, []string{"user1", "user2", "ghost"}, "2030-01-01T00:00:00Z", true)
	if err != nil {
		t.Fatalf("BulkExtendExpiry: %v", err)
	}
	if updated != 2 {
		t.Errorf("updated = %d, want 2", updated)
	}
}

func TestSecretStore_BulkRotateKeys(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	_ = s.Create(ctx, &model.Secret{Label: "user1", SecretKey: "aa000000000000000000000000000000", Enabled: true})

	keys := map[string]string{"user1": "ff000000000000000000000000000000"}
	updated, err := s.BulkRotateKeys(ctx, []string{"user1"}, keys)
	if err != nil {
		t.Fatalf("BulkRotateKeys: %v", err)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}

	got, _ := s.GetByLabel(ctx, "user1")
	if got.SecretKey != "ff000000000000000000000000000000" {
		t.Errorf("key not rotated, got %s", got.SecretKey)
	}
}

func TestSecretStore_RenameLabel_WithTraffic(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	_ = s.Create(ctx, &model.Secret{Label: "old", SecretKey: "aa000000000000000000000000000000", Enabled: true})
	testutil.SeedTraffic(t, db, "old", 1000, 500)

	if err := s.RenameLabel(ctx, "old", "new"); err != nil {
		t.Fatalf("RenameLabel: %v", err)
	}

	got, _ := s.GetByLabel(ctx, "new")
	if got == nil {
		t.Fatal("expected secret with new label")
	}
	if got.TrafficIn != 1000 || got.TrafficOut != 500 {
		t.Errorf("traffic not preserved: in=%d out=%d", got.TrafficIn, got.TrafficOut)
	}
}

func TestSecretStore_CreateWithTags(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	sec := &model.Secret{
		Label: "tagged", SecretKey: "aa000000000000000000000000000000",
		Enabled: true, Tags: `["vip","paid"]`,
	}
	if err := s.Create(ctx, sec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, _ := s.GetByLabel(ctx, "tagged")
	if got.Tags != `["vip","paid"]` {
		t.Errorf("Tags = %q, want %q", got.Tags, `["vip","paid"]`)
	}
}

func TestSecretStore_UpdatePreservesTags(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	_ = s.Create(ctx, &model.Secret{
		Label: "user1", SecretKey: "aa000000000000000000000000000000",
		Enabled: true, Tags: `["test"]`,
	})

	got, _ := s.GetByLabel(ctx, "user1")
	got.Notes = "updated"
	_ = s.Update(ctx, got)

	got2, _ := s.GetByLabel(ctx, "user1")
	if got2.Tags != `["test"]` {
		t.Errorf("Tags = %q, want '[\"test\"]' (preserved after Update)", got2.Tags)
	}
}

func TestSecretStore_CountEnabledByLabels(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	_ = s.Create(ctx, &model.Secret{Label: "a", SecretKey: "aa000000000000000000000000000000", Enabled: true})
	_ = s.Create(ctx, &model.Secret{Label: "b", SecretKey: "bb000000000000000000000000000000", Enabled: true})
	_ = s.Create(ctx, &model.Secret{Label: "c", SecretKey: "cc000000000000000000000000000000", Enabled: false})

	count, err := s.CountEnabledByLabels(ctx, []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("CountEnabledByLabels: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 enabled among [a,b,c], got %d", count)
	}

	// Empty labels returns 0
	count, err = s.CountEnabledByLabels(ctx, nil)
	if err != nil || count != 0 {
		t.Fatalf("empty labels: count=%d err=%v", count, err)
	}
}

func TestSecretStore_ListAllTags(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	_ = s.Create(ctx, &model.Secret{Label: "u1", SecretKey: "aa000000000000000000000000000000", Enabled: true, Tags: `["vip","paid"]`})
	_ = s.Create(ctx, &model.Secret{Label: "u2", SecretKey: "bb000000000000000000000000000000", Enabled: true, Tags: `["vip","free"]`})
	_ = s.Create(ctx, &model.Secret{Label: "u3", SecretKey: "cc000000000000000000000000000000", Enabled: true, Tags: ""})

	tags, err := s.ListAllTags(ctx)
	if err != nil {
		t.Fatalf("ListAllTags: %v", err)
	}
	if len(tags) != 3 {
		t.Fatalf("expected 3 unique tags, got %d: %v", len(tags), tags)
	}

	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[tag] = true
	}
	if !tagSet["vip"] || !tagSet["paid"] || !tagSet["free"] {
		t.Fatalf("missing expected tags, got %v", tags)
	}
}

func TestSecretStore_LabelsByTag(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	_ = s.Create(ctx, &model.Secret{Label: "vip1", SecretKey: "aa000000000000000000000000000000", Enabled: true, Tags: `["vip"]`})
	_ = s.Create(ctx, &model.Secret{Label: "vip2", SecretKey: "bb000000000000000000000000000000", Enabled: true, Tags: `["vip"]`})
	_ = s.Create(ctx, &model.Secret{Label: "free1", SecretKey: "cc000000000000000000000000000000", Enabled: true, Tags: `["free"]`})

	labels, err := s.LabelsByTag(ctx, "vip")
	if err != nil {
		t.Fatalf("LabelsByTag: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("expected 2 labels with tag 'vip', got %d", len(labels))
	}
	if labels[0] != "vip1" || labels[1] != "vip2" {
		t.Fatalf("expected [vip1, vip2], got %v", labels)
	}

	// Non-existent tag returns empty
	empty, err := s.LabelsByTag(ctx, "ghost")
	if err != nil || len(empty) != 0 {
		t.Fatalf("nonexistent tag: labels=%v err=%v", empty, err)
	}
}

func TestSecretStore_BulkToggleEnabled(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	_ = s.Create(ctx, &model.Secret{Label: "a", SecretKey: "aa000000000000000000000000000000", Enabled: true})
	_ = s.Create(ctx, &model.Secret{Label: "b", SecretKey: "bb000000000000000000000000000000", Enabled: true})
	_ = s.Create(ctx, &model.Secret{Label: "c", SecretKey: "cc000000000000000000000000000000", Enabled: true})

	// Disable two — should work (one remains enabled)
	updated, err := s.BulkToggleEnabled(ctx, []string{"a", "b"}, false)
	if err != nil {
		t.Fatalf("BulkToggleEnabled disable: %v", err)
	}
	if updated != 2 {
		t.Fatalf("expected 2 disabled, got %d", updated)
	}

	// Try to disable remaining — should fail
	_, err = s.BulkToggleEnabled(ctx, []string{"c"}, false)
	if err == nil {
		t.Fatal("expected error when disabling last enabled secret")
	}

	// Re-enable one
	updated, err = s.BulkToggleEnabled(ctx, []string{"a"}, true)
	if err != nil {
		t.Fatalf("BulkToggleEnabled enable: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected 1 enabled, got %d", updated)
	}
}

func TestSecretStore_BulkSetLimits(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewSecretStore(db)
	ctx := context.Background()

	_ = s.Create(ctx, &model.Secret{Label: "a", SecretKey: "aa000000000000000000000000000000", Enabled: true, MaxConns: 5, MaxIPs: 3})
	_ = s.Create(ctx, &model.Secret{Label: "b", SecretKey: "bb000000000000000000000000000000", Enabled: true, MaxConns: 10})

	updated, err := s.BulkSetLimits(ctx, []string{"a", "b"}, 20, 10, -1, "", -1, -1)
	if err != nil {
		t.Fatalf("BulkSetLimits: %v", err)
	}
	if updated != 2 {
		t.Fatalf("expected 2 updated, got %d", updated)
	}

	a, _ := s.GetByLabel(ctx, "a")
	if a.MaxConns != 20 || a.MaxIPs != 10 {
		t.Fatalf("a: expected max_conns=20 max_ips=10, got %d/%d", a.MaxConns, a.MaxIPs)
	}

	b, _ := s.GetByLabel(ctx, "b")
	if b.MaxConns != 20 || b.MaxIPs != 10 {
		t.Fatalf("b: expected max_conns=20 max_ips=10, got %d/%d", b.MaxConns, b.MaxIPs)
	}

	// Negative values should not change existing
	_, err = s.BulkSetLimits(ctx, []string{"a"}, -1, -1, 5000, "2030-01-01T00:00:00Z", -1, -1)
	if err != nil {
		t.Fatalf("BulkSetLimits partial: %v", err)
	}
	a, _ = s.GetByLabel(ctx, "a")
	if a.MaxConns != 20 {
		t.Fatalf("max_conns should not change with -1, got %d", a.MaxConns)
	}
	if a.QuotaBytes != 5000 {
		t.Fatalf("expected quota=5000, got %d", a.QuotaBytes)
	}
	if a.ExpiresAt != "2030-01-01T00:00:00Z" {
		t.Fatalf("expected new expiry, got %s", a.ExpiresAt)
	}

	// Nonexistent label is skipped
	updated, err = s.BulkSetLimits(ctx, []string{"ghost"}, 5, 5, 0, "", -1, -1)
	if err != nil {
		t.Fatalf("BulkSetLimits nonexistent: %v", err)
	}
	if updated != 0 {
		t.Fatalf("expected 0 updated for nonexistent, got %d", updated)
	}
}
