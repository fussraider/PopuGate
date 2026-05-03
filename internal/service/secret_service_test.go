package service

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func newTestSecretService(t *testing.T) (*SecretService, *store.SecretStore) {
	t.Helper()
	db := testutil.OpenTestDB(t)
	secrets := store.NewSecretStore(db)
	return NewSecretService(secrets), secrets
}

func TestSecretService_Add_Valid(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	sec, err := svc.Add(ctx, "user1", "")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if sec.Label != "user1" {
		t.Errorf("Label = %q, want %q", sec.Label, "user1")
	}
	if sec.SecretKey == "" {
		t.Error("SecretKey should be auto-generated")
	}
	if !sec.Enabled {
		t.Error("secret should be enabled by default")
	}
}

func TestSecretService_Add_WithCustomKey(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	customKey := "0123456789abcdef0123456789abcdef"
	sec, err := svc.Add(ctx, "user1", customKey)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if sec.SecretKey != customKey {
		t.Errorf("SecretKey = %q, want %q", sec.SecretKey, customKey)
	}
}

func TestSecretService_Add_DuplicateLabel(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, err := svc.Add(ctx, "user1", "")
	if err != nil {
		t.Fatalf("first Add: %v", err)
	}

	_, err = svc.Add(ctx, "user1", "")
	if err == nil {
		t.Fatal("expected error for duplicate label")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want 'already exists'", err.Error())
	}
}

func TestSecretService_Add_InvalidLabel(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	tests := []string{
		"",
		"has space",
		"a/b",
		strings.Repeat("x", 33),
	}

	for _, label := range tests {
		_, err := svc.Add(ctx, label, "")
		if err == nil {
			t.Errorf("Add(%q) should fail, but got nil", label)
		}
	}
}

func TestSecretService_Add_InvalidKey(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, err := svc.Add(ctx, "user1", "not-hex!")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
	if !strings.Contains(err.Error(), "32 hex") {
		t.Errorf("error = %q, want mention of 32 hex", err.Error())
	}
}

func TestSecretService_Add_MaxCountExceeded(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	// Fill up to MaxSecrets
	for i := 0; i < model.MaxSecrets; i++ {
		label := strings.Repeat("a", 32-len(strconv.Itoa(i))) + strconv.Itoa(i)
		if len(label) > 32 {
			label = label[:32]
		}
		_, err := svc.Add(ctx, label, "")
		if err != nil {
			t.Fatalf("Add secret %d: %v", i, err)
		}
	}

	_, err := svc.Add(ctx, "overflow", "")
	if err == nil {
		t.Fatal("expected error when max secrets exceeded")
	}
	if !strings.Contains(err.Error(), "maximum") {
		t.Errorf("error = %q, want 'maximum'", err.Error())
	}
}

func TestSecretService_Remove(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	// Create two secrets so removing one is safe
	_, _ = svc.Add(ctx, "user1", "")
	_, _ = svc.Add(ctx, "user2", "")

	err := svc.Remove(ctx, "user1", false)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	sec, err := svc.Get(ctx, "user1")
	if err != nil {
		t.Fatalf("Get after remove: %v", err)
	}
	if sec != nil {
		t.Error("secret should be deleted")
	}
}

func TestSecretService_Remove_NotFound(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	err := svc.Remove(ctx, "nonexistent", false)
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestSecretService_Remove_LastEnabled(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")

	err := svc.Remove(ctx, "user1", false)
	if err == nil {
		t.Fatal("expected error removing last enabled secret")
	}
	if !strings.Contains(err.Error(), "last enabled") {
		t.Errorf("error = %q, want 'last enabled'", err.Error())
	}
}

func TestSecretService_Remove_LastEnabled_WithForce(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")

	err := svc.Remove(ctx, "user1", true)
	if err != nil {
		t.Fatalf("Remove with force: %v", err)
	}

	sec, _ := svc.Get(ctx, "user1")
	if sec != nil {
		t.Error("secret should be deleted with force")
	}
}

func TestSecretService_Remove_DisabledLastEnabled(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")
	_, _ = svc.Add(ctx, "user2", "")

	// Disable user2, remove user1 => only user2 remains but disabled
	_ = svc.Toggle(ctx, "user2", false)

	// user1 is the last enabled, removing without force should fail
	err := svc.Remove(ctx, "user1", false)
	if err == nil {
		t.Fatal("expected error removing last enabled secret")
	}
}

func TestSecretService_Toggle_DisableLastEnabled(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")

	err := svc.Toggle(ctx, "user1", false)
	if err == nil {
		t.Fatal("expected error disabling last enabled secret")
	}
	if !strings.Contains(err.Error(), "last enabled") {
		t.Errorf("error = %q, want 'last enabled'", err.Error())
	}
}

func TestSecretService_Toggle_Enable(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")
	_, _ = svc.Add(ctx, "user2", "")

	// Disable user1 (user2 still enabled)
	err := svc.Toggle(ctx, "user1", false)
	if err != nil {
		t.Fatalf("Toggle disable: %v", err)
	}

	sec, _ := svc.Get(ctx, "user1")
	if sec.Enabled {
		t.Error("secret should be disabled")
	}

	// Re-enable
	err = svc.Toggle(ctx, "user1", true)
	if err != nil {
		t.Fatalf("Toggle enable: %v", err)
	}

	sec, _ = svc.Get(ctx, "user1")
	if !sec.Enabled {
		t.Error("secret should be enabled")
	}
}

func TestSecretService_Toggle_NotFound(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	err := svc.Toggle(ctx, "nonexistent", true)
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
}

func TestSecretService_Rotate(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	sec, _ := svc.Add(ctx, "user1", "")
	oldKey := sec.SecretKey

	rotated, err := svc.Rotate(ctx, "user1")
	if err != nil {
		t.Fatalf("Rotate: %v", err)
	}
	if rotated.SecretKey == oldKey {
		t.Error("key should have changed after rotation")
	}
	if rotated.Label != "user1" {
		t.Errorf("Label = %q, want %q", rotated.Label, "user1")
	}
}

func TestSecretService_Rotate_NotFound(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, err := svc.Rotate(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
}

func TestSecretService_SetLimits(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")

	err := svc.SetLimits(ctx, "user1", 10, 5, 1024, "2025-12-31")
	if err != nil {
		t.Fatalf("SetLimits: %v", err)
	}

	sec, _ := svc.Get(ctx, "user1")
	if sec.MaxConns != 10 {
		t.Errorf("MaxConns = %d, want 10", sec.MaxConns)
	}
	if sec.MaxIPs != 5 {
		t.Errorf("MaxIPs = %d, want 5", sec.MaxIPs)
	}
	if sec.QuotaBytes != 1024 {
		t.Errorf("QuotaBytes = %d, want 1024", sec.QuotaBytes)
	}
	if sec.ExpiresAt != "2025-12-31" {
		t.Errorf("ExpiresAt = %q, want %q", sec.ExpiresAt, "2025-12-31")
	}
}

func TestSecretService_SetLimits_NotFound(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	err := svc.SetLimits(ctx, "nonexistent", 10, 5, 1024, "")
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
}

func TestSecretService_SetLimits_ExceedsMax(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")

	err := svc.SetLimits(ctx, "user1", 1_000_001, -1, -1, "")
	if err == nil {
		t.Fatal("expected error for max_conns exceeding limit")
	}
	if !strings.Contains(err.Error(), "cannot exceed") {
		t.Errorf("error = %q, want 'cannot exceed'", err.Error())
	}

	err = svc.SetLimits(ctx, "user1", -1, 1_000_001, -1, "")
	if err == nil {
		t.Fatal("expected error for max_ips exceeding limit")
	}
}

func TestSecretService_SetLimits_NegativePreservesOldValue(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")

	// Set initial limits
	_ = svc.SetLimits(ctx, "user1", 10, 5, 1024, "")

	// Update only quota, keep others by passing -1
	err := svc.SetLimits(ctx, "user1", -1, -1, 2048, "")
	if err != nil {
		t.Fatalf("SetLimits: %v", err)
	}

	sec, _ := svc.Get(ctx, "user1")
	if sec.MaxConns != 10 {
		t.Errorf("MaxConns = %d, want 10 (unchanged)", sec.MaxConns)
	}
	if sec.MaxIPs != 5 {
		t.Errorf("MaxIPs = %d, want 5 (unchanged)", sec.MaxIPs)
	}
	if sec.QuotaBytes != 2048 {
		t.Errorf("QuotaBytes = %d, want 2048", sec.QuotaBytes)
	}
}

func TestSecretService_GetLink(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "0123456789abcdef0123456789abcdef")

	result, err := svc.GetLink(ctx, "user1", "1.2.3.4", 443, false, "example.com")
	if err != nil {
		t.Fatalf("GetLink: %v", err)
	}
	if result.TGLink == "" {
		t.Error("TGLink should not be empty")
	}
	if result.WebLink == "" {
		t.Error("WebLink should not be empty")
	}
	if result.Label != "user1" {
		t.Errorf("Label = %q, want %q", result.Label, "user1")
	}
}

func TestSecretService_GetLink_NotFound(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, err := svc.GetLink(ctx, "nonexistent", "1.2.3.4", 443, false, "")
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
}

func TestSecretService_UpdateNotes(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")

	err := svc.UpdateNotes(ctx, "user1", "test notes")
	if err != nil {
		t.Fatalf("UpdateNotes: %v", err)
	}

	sec, _ := svc.Get(ctx, "user1")
	if sec.Notes != "test notes" {
		t.Errorf("Notes = %q, want %q", sec.Notes, "test notes")
	}
}

func TestSecretService_UpdateNotes_NotFound(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	err := svc.UpdateNotes(ctx, "nonexistent", "notes")
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
}

func TestSecretService_GetEnabledLabels(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")
	_, _ = svc.Add(ctx, "user2", "")
	_, _ = svc.Add(ctx, "user3", "")

	// Disable one
	_ = svc.Toggle(ctx, "user2", false)

	labels, err := svc.GetEnabledLabels(ctx)
	if err != nil {
		t.Fatalf("GetEnabledLabels: %v", err)
	}

	wantCount := 2
	if len(labels) != wantCount {
		t.Fatalf("len(labels) = %d, want %d", len(labels), wantCount)
	}

	hasUser1 := false
	hasUser3 := false
	for _, l := range labels {
		if l == "user1" {
			hasUser1 = true
		}
		if l == "user3" {
			hasUser3 = true
		}
	}
	if !hasUser1 || !hasUser3 {
		t.Errorf("enabled labels = %v, want user1 and user3", labels)
	}
}

func TestSecretService_List(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")
	_, _ = svc.Add(ctx, "user2", "")

	secrets, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(secrets) != 2 {
		t.Errorf("len(secrets) = %d, want 2", len(secrets))
	}
}

func TestSecretService_Get(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")

	sec, err := svc.Get(ctx, "user1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sec == nil {
		t.Fatal("secret should exist")
	}
	if sec.Label != "user1" {
		t.Errorf("Label = %q, want %q", sec.Label, "user1")
	}
}

func TestSecretService_Get_NotFound(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	sec, err := svc.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sec != nil {
		t.Error("secret should be nil")
	}
}

func TestSecretService_Rename(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "old-name", "")

	err := svc.Rename(ctx, "old-name", "new-name")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	sec, _ := svc.Get(ctx, "new-name")
	if sec == nil {
		t.Fatal("secret should exist with new name")
	}
	if sec.Label != "new-name" {
		t.Errorf("Label = %q, want %q", sec.Label, "new-name")
	}
}

func TestSecretService_Rename_NotFound(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	err := svc.Rename(ctx, "ghost", "new-ghost")
	if err == nil {
		t.Fatal("expected error renaming nonexistent secret")
	}
}

func TestSecretService_Rename_DuplicateNew(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")
	_, _ = svc.Add(ctx, "user2", "")

	err := svc.Rename(ctx, "user1", "user2")
	if err == nil {
		t.Fatal("expected error renaming to existing label")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want 'already exists'", err.Error())
	}
}

func TestSecretService_Rename_InvalidNewLabel(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")

	err := svc.Rename(ctx, "user1", "bad label!")
	if err == nil {
		t.Fatal("expected error with invalid new label")
	}
}

func TestSecretService_Extend(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")

	err := svc.Extend(ctx, "user1", 30)
	if err != nil {
		t.Fatalf("Extend: %v", err)
	}

	sec, _ := svc.Get(ctx, "user1")
	if sec.ExpiresAt == "" || sec.ExpiresAt == "0" {
		t.Error("expected non-empty expiry after extend")
	}
}

func TestSecretService_Extend_Reenables(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")
	_, _ = svc.Add(ctx, "user2", "")
	_ = svc.Toggle(ctx, "user1", false)

	err := svc.Extend(ctx, "user1", 30)
	if err != nil {
		t.Fatalf("Extend: %v", err)
	}

	sec, _ := svc.Get(ctx, "user1")
	if !sec.Enabled {
		t.Error("expected secret to be re-enabled after extend")
	}
}

func TestSecretService_Extend_NotFound(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	err := svc.Extend(ctx, "ghost", 30)
	if err == nil {
		t.Fatal("expected error extending nonexistent secret")
	}
}

func TestSecretService_Extend_ZeroDays(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")

	err := svc.Extend(ctx, "user1", 0)
	if err == nil {
		t.Fatal("expected error with zero days")
	}
}

func TestSecretService_DisableExpired(t *testing.T) {
	svc, store := newTestSecretService(t)
	ctx := context.Background()

	// Create secret with past expiry
	store.Create(ctx, &model.Secret{
		Label: "expired", SecretKey: "aa000000000000000000000000000000",
		Enabled: true, ExpiresAt: "2020-01-01T00:00:00Z",
	})
	// Create active secret
	_, _ = svc.Add(ctx, "active", "")

	count, err := svc.DisableExpired(ctx)
	if err != nil {
		t.Fatalf("DisableExpired: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 disabled, got %d", count)
	}

	expired, _ := svc.Get(ctx, "expired")
	if expired.Enabled {
		t.Error("expired secret should be disabled")
	}
}

func TestSecretService_Clone(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "src", "0123456789abcdef0123456789abcdef")

	clone, err := svc.Clone(ctx, "src", "dst")
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if clone.Label != "dst" {
		t.Errorf("Label = %q, want %q", clone.Label, "dst")
	}
	if clone.SecretKey == "0123456789abcdef0123456789abcdef" {
		t.Error("clone should have a new key, not the source key")
	}
	if !clone.Enabled {
		t.Error("clone should be enabled")
	}
}

func TestSecretService_Clone_SourceNotFound(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, err := svc.Clone(ctx, "ghost", "dst")
	if err == nil {
		t.Fatal("expected error cloning nonexistent source")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestSecretService_Clone_DuplicateLabel(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "src", "")
	_, _ = svc.Add(ctx, "dst", "")

	_, err := svc.Clone(ctx, "src", "dst")
	if err == nil {
		t.Fatal("expected error cloning to existing label")
	}
}

func TestSecretService_SetTags(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")

	err := svc.SetTags(ctx, "user1", "vip,paid")
	if err != nil {
		t.Fatalf("SetTags: %v", err)
	}

	sec, _ := svc.Get(ctx, "user1")
	if sec.Tags != "vip,paid" {
		t.Errorf("Tags = %q, want %q", sec.Tags, "vip,paid")
	}
}

func TestSecretService_SetTags_NotFound(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	err := svc.SetTags(ctx, "ghost", "tag")
	if err == nil {
		t.Fatal("expected error setting tags on nonexistent secret")
	}
}

func TestSecretService_Archive_Unarchive(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")

	err := svc.Archive(ctx, "user1")
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}

	sec, _ := svc.Get(ctx, "user1")
	if sec.ArchivedAt == 0 {
		t.Error("expected non-zero archived_at after archive")
	}

	err = svc.Unarchive(ctx, "user1")
	if err != nil {
		t.Fatalf("Unarchive: %v", err)
	}

	sec, _ = svc.Get(ctx, "user1")
	if sec.ArchivedAt != 0 {
		t.Error("expected zero archived_at after unarchive")
	}
}

func TestSecretService_Archive_NotFound(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	err := svc.Archive(ctx, "ghost")
	if err == nil {
		t.Fatal("expected error archiving nonexistent secret")
	}
}

func TestSecretService_BulkExtend(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")
	_, _ = svc.Add(ctx, "user2", "")

	updated, err := svc.BulkExtend(ctx, []string{"user1", "user2"}, 30)
	if err != nil {
		t.Fatalf("BulkExtend: %v", err)
	}
	if updated != 2 {
		t.Errorf("updated = %d, want 2", updated)
	}

	sec, _ := svc.Get(ctx, "user1")
	if sec.ExpiresAt == "" || sec.ExpiresAt == "0" {
		t.Error("expected non-empty expiry after bulk extend")
	}
}

func TestSecretService_BulkExtend_ZeroDays(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.BulkExtend(ctx, []string{"user1"}, 0)
}

func TestSecretService_BulkRotate(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "0123456789abcdef0123456789abcdef")
	_, _ = svc.Add(ctx, "user2", "fedcba9876543210fedcba9876543210")

	updated, labels, err := svc.BulkRotate(ctx, []string{"user1", "user2"})
	if err != nil {
		t.Fatalf("BulkRotate: %v", err)
	}
	if updated != 2 {
		t.Errorf("updated = %d, want 2", updated)
	}
	if len(labels) != 2 {
		t.Errorf("len(labels) = %d, want 2", len(labels))
	}

	sec, _ := svc.Get(ctx, "user1")
	if sec.SecretKey == "0123456789abcdef0123456789abcdef" {
		t.Error("expected new key after bulk rotate")
	}
}

func TestSecretService_Search(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "alice", "")
	_ = svc.UpdateNotes(ctx, "alice", "VIP user")
	_, _ = svc.Add(ctx, "bob", "")

	results, err := svc.Search(ctx, "alice")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Label != "alice" {
		t.Errorf("Label = %q, want %q", results[0].Label, "alice")
	}
}

func TestSecretService_Search_ByNotes(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")
	_ = svc.UpdateNotes(ctx, "user1", "premium account")
	_, _ = svc.Add(ctx, "user2", "")

	results, err := svc.Search(ctx, "premium")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
}

func TestSecretService_Search_Empty(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")

	results, err := svc.Search(ctx, "")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1 (empty query returns all)", len(results))
	}
}

func TestSecretService_Top(t *testing.T) {
	svc, store := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "low", "")
	_, _ = svc.Add(ctx, "high", "")

	// Give "high" more traffic
	store.UpdateTraffic(ctx, "high", 10000, 5000)

	results, err := svc.Top(ctx, 10)
	if err != nil {
		t.Fatalf("Top: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("len(results) = %d, want at least 2", len(results))
	}
	if results[0].Label != "high" {
		t.Errorf("top secret = %q, want %q", results[0].Label, "high")
	}
}

func TestSecretService_ExportJSON(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "0123456789abcdef0123456789abcdef")
	_, _ = svc.Add(ctx, "user2", "")

	exported, err := svc.ExportJSON(ctx)
	if err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	if len(exported) != 2 {
		t.Errorf("len(exported) = %d, want 2", len(exported))
	}
}

func TestSecretService_ImportSecrets(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	entries := []model.Secret{
		{Label: "imported1", SecretKey: "aa000000000000000000000000000000"},
		{Label: "imported2"},
	}

	imported, created, err := svc.ImportSecrets(ctx, entries)
	if err != nil {
		t.Fatalf("ImportSecrets: %v", err)
	}
	if imported != 2 {
		t.Errorf("imported = %d, want 2", imported)
	}
	if len(created) != 2 {
		t.Errorf("len(created) = %d, want 2", len(created))
	}

	sec, _ := svc.Get(ctx, "imported1")
	if sec == nil {
		t.Error("imported1 should exist")
	}
	if sec.SecretKey != "aa000000000000000000000000000000" {
		t.Error("imported secret should preserve provided key")
	}

	sec2, _ := svc.Get(ctx, "imported2")
	if sec2 == nil {
		t.Error("imported2 should exist")
	}
	if sec2.SecretKey == "" {
		t.Error("imported2 should have auto-generated key")
	}
}

func TestSecretService_ImportSecrets_SkipsDuplicates(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "existing", "")

	entries := []model.Secret{
		{Label: "existing"},
		{Label: "newuser"},
	}

	imported, created, err := svc.ImportSecrets(ctx, entries)
	if err != nil {
		t.Fatalf("ImportSecrets: %v", err)
	}
	if imported != 1 {
		t.Errorf("imported = %d, want 1 (duplicate skipped)", imported)
	}
	if len(created) != 1 || created[0] != "newuser" {
		t.Errorf("created = %v, want [newuser]", created)
	}
}

func TestSecretService_ImportSecrets_SkipsInvalidKeys(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	entries := []model.Secret{
		{Label: "badkey", SecretKey: "not-valid"},
		{Label: "goodkey", SecretKey: "aa000000000000000000000000000000"},
	}

	imported, _, err := svc.ImportSecrets(ctx, entries)
	if err != nil {
		t.Fatalf("ImportSecrets: %v", err)
	}
	if imported != 1 {
		t.Errorf("imported = %d, want 1 (bad key skipped)", imported)
	}
}

func TestSecretService_DisableExpired_LastEnabledGuard(t *testing.T) {
	svc, store := newTestSecretService(t)
	ctx := context.Background()

	// Only one secret, and it's expired — should NOT be disabled
	store.Create(ctx, &model.Secret{
		Label: "only", SecretKey: "aa000000000000000000000000000000",
		Enabled: true, ExpiresAt: "2020-01-01T00:00:00Z",
	})

	count, err := svc.DisableExpired(ctx)
	if err != nil {
		t.Fatalf("DisableExpired: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 disabled (last enabled guard), got %d", count)
	}

	sec, _ := svc.Get(ctx, "only")
	if !sec.Enabled {
		t.Error("last enabled secret should remain enabled even if expired")
	}
}

func TestSecretService_ResetTraffic(t *testing.T) {
	svc, store := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")
	store.UpdateTraffic(ctx, "user1", 1000, 500)

	err := svc.ResetTraffic(ctx, "user1")
	if err != nil {
		t.Fatalf("ResetTraffic: %v", err)
	}

	sec, _ := svc.Get(ctx, "user1")
	if sec.TrafficIn != 0 || sec.TrafficOut != 0 {
		t.Errorf("traffic should be zero after reset: in=%d out=%d", sec.TrafficIn, sec.TrafficOut)
	}
}

func TestSecretService_ResetAllTraffic(t *testing.T) {
	svc, store := newTestSecretService(t)
	ctx := context.Background()

	_, _ = svc.Add(ctx, "user1", "")
	_, _ = svc.Add(ctx, "user2", "")
	store.UpdateTraffic(ctx, "user1", 1000, 500)
	store.UpdateTraffic(ctx, "user2", 2000, 1000)

	err := svc.ResetAllTraffic(ctx)
	if err != nil {
		t.Fatalf("ResetAllTraffic: %v", err)
	}

	sec1, _ := svc.Get(ctx, "user1")
	sec2, _ := svc.Get(ctx, "user2")
	if sec1.TrafficIn != 0 || sec2.TrafficIn != 0 {
		t.Error("all traffic should be zero after reset")
	}
}
