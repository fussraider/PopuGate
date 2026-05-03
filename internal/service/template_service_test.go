package service

import (
	"context"
	"strings"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func newTestTemplateService(t *testing.T) (*TemplateService, *store.SecretStore) {
	t.Helper()
	db := testutil.OpenTestDB(t)
	templates := store.NewTemplateStore(db)
	secrets := store.NewSecretStore(db)
	return NewTemplateService(templates, secrets), secrets
}

func TestTemplateService_CRUD(t *testing.T) {
	svc, _ := newTestTemplateService(t)
	ctx := context.Background()

	tmpl := &model.SecretTemplate{
		Name: "basic", MaxConns: 5, MaxIPs: 3,
		QuotaBytes: 1024, ExpiresDays: 30, Notes: "test",
	}

	if err := svc.Create(ctx, tmpl); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.Get(ctx, "basic")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.MaxConns != 5 {
		t.Errorf("MaxConns = %d, want 5", got.MaxConns)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("len(list) = %d, want 1", len(list))
	}

	if err := svc.Delete(ctx, "basic"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, _ = svc.Get(ctx, "basic")
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestTemplateService_Create_EmptyName(t *testing.T) {
	svc, _ := newTestTemplateService(t)
	ctx := context.Background()

	err := svc.Create(ctx, &model.SecretTemplate{Name: ""})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestTemplateService_Create_Duplicate(t *testing.T) {
	svc, _ := newTestTemplateService(t)
	ctx := context.Background()

	_ = svc.Create(ctx, &model.SecretTemplate{Name: "basic", MaxConns: 5})

	err := svc.Create(ctx, &model.SecretTemplate{Name: "basic", MaxConns: 10})
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want 'already exists'", err.Error())
	}
}

func TestTemplateService_Delete_NotFound(t *testing.T) {
	svc, _ := newTestTemplateService(t)
	ctx := context.Background()

	err := svc.Delete(ctx, "ghost")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestTemplateService_ApplyToSecret(t *testing.T) {
	svc, secrets := newTestTemplateService(t)
	ctx := context.Background()

	_ = svc.Create(ctx, &model.SecretTemplate{
		Name: "premium", MaxConns: 50, MaxIPs: 10,
		QuotaBytes: 10 * 1024 * 1024 * 1024, ExpiresDays: 90,
	})

	secrets.Create(ctx, &model.Secret{
		Label: "user1", SecretKey: "aa000000000000000000000000000000",
		Enabled: true, MaxConns: 5,
	})

	err := svc.ApplyToSecret(ctx, "premium", "user1")
	if err != nil {
		t.Fatalf("ApplyToSecret: %v", err)
	}

	sec, _ := secrets.GetByLabel(ctx, "user1")
	if sec.MaxConns != 50 {
		t.Errorf("MaxConns = %d, want 50", sec.MaxConns)
	}
	if sec.MaxIPs != 10 {
		t.Errorf("MaxIPs = %d, want 10", sec.MaxIPs)
	}
	if sec.QuotaBytes != 10*1024*1024*1024 {
		t.Errorf("QuotaBytes = %d, want 10GB", sec.QuotaBytes)
	}
	if sec.ExpiresAt == "" || sec.ExpiresAt == "0" {
		t.Error("expected ExpiresAt to be set")
	}
}

func TestTemplateService_ApplyToSecret_TemplateNotFound(t *testing.T) {
	svc, secrets := newTestTemplateService(t)
	ctx := context.Background()

	secrets.Create(ctx, &model.Secret{
		Label: "user1", SecretKey: "aa000000000000000000000000000000", Enabled: true,
	})

	err := svc.ApplyToSecret(ctx, "ghost", "user1")
	if err == nil {
		t.Fatal("expected error for nonexistent template")
	}
}

func TestTemplateService_ApplyToSecret_SecretNotFound(t *testing.T) {
	svc, _ := newTestTemplateService(t)
	ctx := context.Background()

	_ = svc.Create(ctx, &model.SecretTemplate{Name: "basic", MaxConns: 5})

	err := svc.ApplyToSecret(ctx, "basic", "ghost")
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
}
