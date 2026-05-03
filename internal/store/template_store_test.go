package store

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestTemplateStore_CRUD(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTemplateStore(db)
	ctx := context.Background()

	tmpl := &model.SecretTemplate{
		Name:        "basic",
		MaxConns:    5,
		MaxIPs:      3,
		QuotaBytes:  1024 * 1024 * 1024,
		ExpiresDays: 30,
		Notes:       "basic plan",
	}

	if err := s.Create(ctx, tmpl); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tmpl.ID == 0 {
		t.Fatal("expected ID to be set")
	}

	got, err := s.GetByName(ctx, "basic")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got == nil {
		t.Fatal("expected template")
	}
	if got.MaxConns != 5 {
		t.Fatalf("expected max_conns=5, got %d", got.MaxConns)
	}

	templates, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("expected 1 template, got %d", len(templates))
	}

	if err := s.Delete(ctx, "basic"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, _ = s.GetByName(ctx, "basic")
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestTemplateStore_GetByName_NotFound(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewTemplateStore(db)

	got, err := s.GetByName(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent template")
	}
}
