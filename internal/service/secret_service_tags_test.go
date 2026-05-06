package service

import (
	"context"
	"testing"
)

// ---------------------------------------------------------------------------
// ListByTag / ListAllTags / LabelsByTag
// ---------------------------------------------------------------------------

func TestSecretService_ListByTag(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	svc.Add(ctx, "alpha", "")
	svc.Add(ctx, "beta", "")
	svc.SetTags(ctx, "alpha", "prod")
	svc.SetTags(ctx, "beta", "dev")

	got, err := svc.ListByTag(ctx, "prod")
	if err != nil {
		t.Fatalf("ListByTag: %v", err)
	}
	if len(got) != 1 || got[0].Label != "alpha" {
		t.Errorf("ListByTag(prod) = %v, want [alpha]", got)
	}

	got, err = svc.ListByTag(ctx, "dev")
	if err != nil {
		t.Fatalf("ListByTag: %v", err)
	}
	if len(got) != 1 || got[0].Label != "beta" {
		t.Errorf("ListByTag(dev) = %v, want [beta]", got)
	}
}

func TestSecretService_ListByTag_Empty(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	got, err := svc.ListByTag(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("ListByTag: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestSecretService_ListAllTags(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	svc.Add(ctx, "s1", "")
	svc.Add(ctx, "s2", "")
	svc.SetTags(ctx, "s1", "tag-a,tag-b")
	svc.SetTags(ctx, "s2", "tag-b,tag-c")

	tags, err := svc.ListAllTags(ctx)
	if err != nil {
		t.Fatalf("ListAllTags: %v", err)
	}
	tagSet := make(map[string]bool, len(tags))
	for _, tg := range tags {
		tagSet[tg] = true
	}
	for _, want := range []string{"tag-a", "tag-b", "tag-c"} {
		if !tagSet[want] {
			t.Errorf("ListAllTags missing %q, got %v", want, tags)
		}
	}
}

func TestSecretService_LabelsByTag(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	svc.Add(ctx, "x1", "")
	svc.Add(ctx, "x2", "")
	svc.SetTags(ctx, "x1", "shared")
	svc.SetTags(ctx, "x2", "shared")

	labels, err := svc.LabelsByTag(ctx, "shared")
	if err != nil {
		t.Fatalf("LabelsByTag: %v", err)
	}
	if len(labels) != 2 {
		t.Errorf("LabelsByTag(shared) = %v, want 2 labels", labels)
	}
}

func TestSecretService_LabelsByTag_EmptyTag(t *testing.T) {
	svc, _ := newTestSecretService(t)
	ctx := context.Background()

	_, err := svc.LabelsByTag(ctx, "")
	if err == nil {
		t.Error("expected error for empty tag, got nil")
	}
}
