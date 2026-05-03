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
