package service

import (
	"context"
	"strings"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func newTestUpstreamService(t *testing.T) *UpstreamService {
	t.Helper()
	db := testutil.OpenTestDB(t)
	return NewUpstreamService(store.NewUpstreamStore(db))
}

func TestUpstreamService_List_Empty(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()

	upstreams, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(upstreams) != 0 {
		t.Errorf("len(upstreams) = %d, want 0", len(upstreams))
	}
}

func TestUpstreamService_List_WithData(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()

	err := svc.Add(ctx, &model.Upstream{
		Name:    "test-socks5",
		Type:    model.UpstreamSOCKS5,
		Address: "127.0.0.1:1080",
		Weight:  10,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	err = svc.Add(ctx, &model.Upstream{
		Name:   "test-direct",
		Type:   model.UpstreamDirect,
		Weight: 5,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	upstreams, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(upstreams) != 2 {
		t.Errorf("len(upstreams) = %d, want 2", len(upstreams))
	}
}

func TestUpstreamService_Get_Found(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()

	err := svc.Add(ctx, &model.Upstream{
		Name:    "my-proxy",
		Type:    model.UpstreamSOCKS5,
		Address: "10.0.0.1:1080",
		Weight:  10,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	u, err := svc.Get(ctx, "my-proxy")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if u == nil {
		t.Fatal("upstream should exist")
	}
	if u.Name != "my-proxy" {
		t.Errorf("Name = %q, want %q", u.Name, "my-proxy")
	}
	if u.Type != model.UpstreamSOCKS5 {
		t.Errorf("Type = %q, want %q", u.Type, model.UpstreamSOCKS5)
	}
	if u.Address != "10.0.0.1:1080" {
		t.Errorf("Address = %q, want %q", u.Address, "10.0.0.1:1080")
	}
}

func TestUpstreamService_Get_NotFound(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()

	u, err := svc.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if u != nil {
		t.Error("upstream should be nil")
	}
}

func TestUpstreamService_Add_Valid(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()

	tests := []struct {
		name string
		u    *model.Upstream
	}{
		{
			name: "direct upstream",
			u: &model.Upstream{
				Name:   "direct-1",
				Type:   model.UpstreamDirect,
				Weight: 50,
			},
		},
		{
			name: "socks5 upstream",
			u: &model.Upstream{
				Name:     "socks5-1",
				Type:     model.UpstreamSOCKS5,
				Address:  "192.168.1.1:1080",
				Username: "user",
				Password: "pass",
				Weight:   10,
			},
		},
		{
			name: "socks4 upstream",
			u: &model.Upstream{
				Name:    "socks4-1",
				Type:    model.UpstreamSOCKS4,
				Address: "192.168.1.2:1080",
				Weight:  1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.Add(ctx, tt.u)
			if err != nil {
				t.Fatalf("Add: %v", err)
			}

			got, err := svc.Get(ctx, tt.u.Name)
			if err != nil {
				t.Fatalf("Get after Add: %v", err)
			}
			if got == nil {
				t.Fatal("upstream should exist after Add")
			}
			if !got.Enabled {
				t.Error("upstream should be enabled by default after Add")
			}
		})
	}
}

func TestUpstreamService_Add_DuplicateName(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()

	err := svc.Add(ctx, &model.Upstream{
		Name:   "dup",
		Type:   model.UpstreamDirect,
		Weight: 10,
	})
	if err != nil {
		t.Fatalf("first Add: %v", err)
	}

	err = svc.Add(ctx, &model.Upstream{
		Name:   "dup",
		Type:   model.UpstreamDirect,
		Weight: 10,
	})
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want 'already exists'", err.Error())
	}
}

func TestUpstreamService_Add_InvalidData(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()

	tests := []struct {
		name    string
		u       *model.Upstream
		wantErr string
	}{
		{
			name: "empty name",
			u: &model.Upstream{
				Name:   "",
				Type:   model.UpstreamDirect,
				Weight: 10,
			},
			wantErr: "name must be",
		},
		{
			name: "name too long",
			u: &model.Upstream{
				Name:   strings.Repeat("a", 33),
				Type:   model.UpstreamDirect,
				Weight: 10,
			},
			wantErr: "name must be",
		},
		{
			name: "invalid type",
			u: &model.Upstream{
				Name:   "test",
				Type:   "invalid",
				Weight: 10,
			},
			wantErr: "invalid upstream type",
		},
		{
			name: "socks5 without address",
			u: &model.Upstream{
				Name:   "test",
				Type:   model.UpstreamSOCKS5,
				Weight: 10,
			},
			wantErr: "address is required",
		},
		{
			name: "socks4 without address",
			u: &model.Upstream{
				Name:   "test",
				Type:   model.UpstreamSOCKS4,
				Weight: 10,
			},
			wantErr: "address is required",
		},
		{
			name: "weight zero",
			u: &model.Upstream{
				Name:   "test",
				Type:   model.UpstreamDirect,
				Weight: 0,
			},
			wantErr: "weight must be",
		},
		{
			name: "weight too high",
			u: &model.Upstream{
				Name:   "test",
				Type:   model.UpstreamDirect,
				Weight: 101,
			},
			wantErr: "weight must be",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.Add(ctx, tt.u)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestUpstreamService_Update_Found(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()

	err := svc.Add(ctx, &model.Upstream{
		Name:    "my-proxy",
		Type:    model.UpstreamSOCKS5,
		Address: "10.0.0.1:1080",
		Weight:  10,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	err = svc.Update(ctx, "my-proxy", &model.Upstream{
		Name:    "my-proxy",
		Type:    model.UpstreamSOCKS5,
		Address: "10.0.0.2:1080",
		Weight:  20,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := svc.Get(ctx, "my-proxy")
	if got.Address != "10.0.0.2:1080" {
		t.Errorf("Address = %q, want %q", got.Address, "10.0.0.2:1080")
	}
	if got.Weight != 20 {
		t.Errorf("Weight = %d, want 20", got.Weight)
	}
}

func TestUpstreamService_Update_NotFound(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()

	err := svc.Update(ctx, "nonexistent", &model.Upstream{
		Name:   "test",
		Type:   model.UpstreamDirect,
		Weight: 10,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent upstream")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestUpstreamService_Remove_Found(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()

	err := svc.Add(ctx, &model.Upstream{
		Name:   "to-remove",
		Type:   model.UpstreamDirect,
		Weight: 10,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	err = svc.Remove(ctx, "to-remove")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	got, _ := svc.Get(ctx, "to-remove")
	if got != nil {
		t.Error("upstream should be deleted")
	}
}

func TestUpstreamService_Remove_NotFound(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()

	err := svc.Remove(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent upstream")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestUpstreamService_Toggle_Found(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()

	err := svc.Add(ctx, &model.Upstream{
		Name:   "toggle-me",
		Type:   model.UpstreamDirect,
		Weight: 10,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Disable
	err = svc.Toggle(ctx, "toggle-me", false)
	if err != nil {
		t.Fatalf("Toggle disable: %v", err)
	}

	got, _ := svc.Get(ctx, "toggle-me")
	if got.Enabled {
		t.Error("upstream should be disabled")
	}

	// Re-enable
	err = svc.Toggle(ctx, "toggle-me", true)
	if err != nil {
		t.Fatalf("Toggle enable: %v", err)
	}

	got, _ = svc.Get(ctx, "toggle-me")
	if !got.Enabled {
		t.Error("upstream should be enabled")
	}
}

func TestUpstreamService_Toggle_NotFound(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()

	err := svc.Toggle(ctx, "nonexistent", true)
	if err == nil {
		t.Fatal("expected error for nonexistent upstream")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}
