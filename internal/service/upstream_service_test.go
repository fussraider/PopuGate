package service

import (
	"context"
	"regexp"
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

func TestUpstreamService_AutoRecovery(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()

	// 1. Add upstream
	err := svc.Add(ctx, &model.Upstream{
		Name:    "test-rec",
		Type:    model.UpstreamSOCKS5,
		Address: "127.0.0.1:9999",
		Weight:  10,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	// 2. Set fail_count to 3 and trigger handleFailover
	for i := 0; i < 3; i++ {
		_ = svc.upstreams.UpdateHealth(ctx, "test-rec", false, 0, "offline")
	}

	var notified bool
	svc.SetNotify(func(ctx context.Context, format string, args ...any) {
		notified = true
	})

	svc.handleFailover(ctx, "test-rec", "offline")

	got, _ := svc.Get(ctx, "test-rec")
	if got.Enabled {
		t.Fatal("expected enabled=false after failover")
	}
	if !got.AutoDisabled {
		t.Fatal("expected auto_disabled=true after failover")
	}
	if !notified {
		t.Fatal("expected Telegram notification on failover")
	}

	// 3. Test handleAutoRecovery
	var recoveredNotified bool
	svc.SetNotify(func(ctx context.Context, format string, args ...any) {
		recoveredNotified = true
	})

	recovered, _ := svc.Get(ctx, "test-rec")
	svc.handleAutoRecovery(ctx, recovered, 45)

	got, _ = svc.Get(ctx, "test-rec")
	if !got.Enabled {
		t.Fatal("expected enabled=true after recovery")
	}
	if got.AutoDisabled {
		t.Fatal("expected auto_disabled=false after recovery")
	}
	if !recoveredNotified {
		t.Fatal("expected Telegram notification on recovery")
	}
}

func TestParseProxyLine(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *model.Upstream
		wantErr bool
	}{
		{
			name:  "socks5 with credentials",
			input: "socks5://user:pass@1.2.3.4:1080",
			want: &model.Upstream{
				Type:     model.UpstreamSOCKS5,
				Address:  "1.2.3.4:1080",
				Username: "user",
				Password: "pass",
				Weight:   10,
			},
			wantErr: false,
		},
		{
			name:  "socks4 scheme only",
			input: "socks4://8.8.8.8:8080",
			want: &model.Upstream{
				Type:    model.UpstreamSOCKS4,
				Address: "8.8.8.8:8080",
				Weight:  10,
			},
			wantErr: false,
		},
		{
			name:  "no scheme default socks5",
			input: "192.168.1.1:1080",
			want: &model.Upstream{
				Type:    model.UpstreamSOCKS5,
				Address: "192.168.1.1:1080",
				Weight:  10,
			},
			wantErr: false,
		},
		{
			name:  "suffix credentials",
			input: "1.2.3.4:1080:username:password123",
			want: &model.Upstream{
				Type:     model.UpstreamSOCKS5,
				Address:  "1.2.3.4:1080",
				Username: "username",
				Password: "password123",
				Weight:   10,
			},
			wantErr: false,
		},
		{
			name:  "suffix credentials username only",
			input: "1.2.3.4:1080:usernameOnly",
			want: &model.Upstream{
				Type:     model.UpstreamSOCKS5,
				Address:  "1.2.3.4:1080",
				Username: "usernameOnly",
				Weight:   10,
			},
			wantErr: false,
		},
		{
			name:  "ipv6 bracketed host only",
			input: "[2001:db8::1]:1080",
			want: &model.Upstream{
				Type:    model.UpstreamSOCKS5,
				Address: "[2001:db8::1]:1080",
				Weight:  10,
			},
			wantErr: false,
		},
		{
			name:  "ipv6 with scheme and credentials",
			input: "socks5://user:pass@[::1]:1080",
			want: &model.Upstream{
				Type:     model.UpstreamSOCKS5,
				Address:  "[::1]:1080",
				Username: "user",
				Password: "pass",
				Weight:   10,
			},
			wantErr: false,
		},
		{
			name:  "ipv6 with suffix credentials",
			input: "[::1]:1080:bob:secret",
			want: &model.Upstream{
				Type:     model.UpstreamSOCKS5,
				Address:  "[::1]:1080",
				Username: "bob",
				Password: "secret",
				Weight:   10,
			},
			wantErr: false,
		},
		{
			name:  "ipv6 suffix password with colons",
			input: "[fe80::1]:1080:bob:se:cr:et",
			want: &model.Upstream{
				Type:     model.UpstreamSOCKS5,
				Address:  "[fe80::1]:1080",
				Username: "bob",
				Password: "se:cr:et",
				Weight:   10,
			},
			wantErr: false,
		},
		{
			name:    "ipv6 missing closing bracket",
			input:   "[::1:1080",
			wantErr: true,
		},
		{
			name:    "ipv6 missing port",
			input:   "[::1]",
			wantErr: true,
		},
		{
			name:    "invalid address format",
			input:   "not-an-ip",
			wantErr: true,
		},
		{
			name:    "empty line",
			input:   "   ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseProxyLine(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseProxyLine() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Type != tt.want.Type {
				t.Errorf("Type = %v, want %v", got.Type, tt.want.Type)
			}
			if got.Address != tt.want.Address {
				t.Errorf("Address = %q, want %q", got.Address, tt.want.Address)
			}
			if got.Username != tt.want.Username {
				t.Errorf("Username = %q, want %q", got.Username, tt.want.Username)
			}
			if got.Password != tt.want.Password {
				t.Errorf("Password = %q, want %q", got.Password, tt.want.Password)
			}
		})
	}
}

func TestGenerateBulkUpstreamName(t *testing.T) {
	// Every generated name ends with an 8-hex identity hash: "<...>-<hash8>".
	hashSuffix := regexp.MustCompile(`-[0-9a-f]{8}$`)

	t.Run("structure and length", func(t *testing.T) {
		cases := []struct {
			proto  model.UpstreamType
			addr   string
			prefix string
		}{
			{model.UpstreamSOCKS5, "192.168.1.1:1080", "s5-192-168-1-1-1080-"},
			{model.UpstreamSOCKS4, "8.8.8.8:80", "s4-8-8-8-8-80-"},
			{model.UpstreamDirect, "1.2.3.4:1080", "dir-1-2-3-4-1080-"},
			{model.UpstreamSOCKS5, "very-long-subdomain-that-exceeds-thirty-two-chars.example.com:1080", "s5-"},
		}
		for _, c := range cases {
			u := &model.Upstream{Type: c.proto, Address: c.addr}
			got := GenerateBulkUpstreamName(u)
			if len(got) > 32 {
				t.Errorf("addr %q: len(got)=%d exceeds 32 (%q)", c.addr, len(got), got)
			}
			if !strings.HasPrefix(got, c.prefix) {
				t.Errorf("addr %q: got %q, want prefix %q", c.addr, got, c.prefix)
			}
			if !hashSuffix.MatchString(got) {
				t.Errorf("addr %q: got %q, expected trailing 8-hex identity hash", c.addr, got)
			}
		}
	})

	t.Run("distinct credentials produce distinct names", func(t *testing.T) {
		noCreds := GenerateBulkUpstreamName(&model.Upstream{Type: model.UpstreamSOCKS5, Address: "1.2.3.4:1080"})
		creds1 := GenerateBulkUpstreamName(&model.Upstream{Type: model.UpstreamSOCKS5, Address: "1.2.3.4:1080", Username: "u1", Password: "p1"})
		creds2 := GenerateBulkUpstreamName(&model.Upstream{Type: model.UpstreamSOCKS5, Address: "1.2.3.4:1080", Username: "u2", Password: "p2"})
		if noCreds == creds1 || creds1 == creds2 || noCreds == creds2 {
			t.Errorf("expected distinct names for differing credentials, got %q / %q / %q", noCreds, creds1, creds2)
		}
	})

	t.Run("identical identity produces identical name", func(t *testing.T) {
		u1 := &model.Upstream{Type: model.UpstreamSOCKS5, Address: "1.2.3.4:1080", Username: "u", Password: "p", Iface: "eth0"}
		u2 := &model.Upstream{Type: model.UpstreamSOCKS5, Address: "1.2.3.4:1080", Username: "u", Password: "p", Iface: "eth0"}
		if GenerateBulkUpstreamName(u1) != GenerateBulkUpstreamName(u2) {
			t.Error("expected identical names for identical upstream identity")
		}
	})
}
