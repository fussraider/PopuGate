package store

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestUpstreamStore_ListEmpty(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)

	upstreams, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(upstreams) != 0 {
		t.Fatalf("expected empty list, got %d", len(upstreams))
	}
}

func TestUpstreamStore_CreateAndGetByName(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	u := &model.Upstream{
		Name:     "my-proxy",
		Type:     model.UpstreamSOCKS5,
		Address:  "10.0.0.1:1080",
		Username: "user",
		Password: "pass",
		Weight:   50,
		Iface:    "eth0",
		Enabled:  true,
	}
	if err := s.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("expected ID to be set after Create")
	}

	got, err := s.GetByName(ctx, "my-proxy")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got == nil {
		t.Fatal("expected upstream, got nil")
	}
	if got.Name != "my-proxy" {
		t.Fatalf("expected name my-proxy, got %s", got.Name)
	}
	if got.Type != model.UpstreamSOCKS5 {
		t.Fatalf("expected type socks5, got %s", got.Type)
	}
	if got.Address != "10.0.0.1:1080" {
		t.Fatalf("expected address 10.0.0.1:1080, got %s", got.Address)
	}
	if got.Username != "user" {
		t.Fatalf("expected username user, got %s", got.Username)
	}
	if got.Password != "pass" {
		t.Fatalf("expected password pass, got %s", got.Password)
	}
	if got.Weight != 50 {
		t.Fatalf("expected weight 50, got %d", got.Weight)
	}
	if got.Iface != "eth0" {
		t.Fatalf("expected iface eth0, got %s", got.Iface)
	}
	if !got.Enabled {
		t.Fatal("expected enabled=true")
	}
}

func TestUpstreamStore_CreateMultipleAndList(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	for _, name := range []string{"up1", "up2", "up3"} {
		if err := s.Create(ctx, &model.Upstream{
			Name:    name,
			Type:    model.UpstreamDirect,
			Weight:  10,
			Enabled: true,
		}); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
	}

	upstreams, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(upstreams) != 3 {
		t.Fatalf("expected 3 upstreams, got %d", len(upstreams))
	}
}

func TestUpstreamStore_Delete(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Upstream{
		Name: "del-me", Type: model.UpstreamDirect, Weight: 10, Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Delete(ctx, "del-me"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := s.GetByName(ctx, "del-me")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestUpstreamStore_UpdateEnabled(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Upstream{
		Name: "toggle", Type: model.UpstreamDirect, Weight: 10, Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Disable
	if err := s.UpdateEnabled(ctx, "toggle", false); err != nil {
		t.Fatalf("UpdateEnabled false: %v", err)
	}
	got, err := s.GetByName(ctx, "toggle")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.Enabled {
		t.Fatal("expected enabled=false after disabling")
	}

	// Re-enable
	if err := s.UpdateEnabled(ctx, "toggle", true); err != nil {
		t.Fatalf("UpdateEnabled true: %v", err)
	}
	got, err = s.GetByName(ctx, "toggle")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if !got.Enabled {
		t.Fatal("expected enabled=true after re-enabling")
	}
}

func TestUpstreamStore_Update(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Upstream{
		Name: "proxy1", Type: model.UpstreamSOCKS5, Address: "10.0.0.1:1080",
		Username: "user1", Password: "pass1", Weight: 30, Iface: "eth0", Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated := &model.Upstream{
		Type: model.UpstreamSOCKS4, Address: "10.0.0.2:1081",
		Username: "user2", Password: "pass2", Weight: 70, Iface: "eth1", Enabled: false,
	}
	if err := s.Update(ctx, "proxy1", updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.GetByName(ctx, "proxy1")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.Name != "proxy1" {
		t.Fatalf("expected name proxy1, got %s", got.Name)
	}
	if got.Type != model.UpstreamSOCKS4 {
		t.Fatalf("expected type socks4, got %s", got.Type)
	}
	if got.Address != "10.0.0.2:1081" {
		t.Fatalf("expected address 10.0.0.2:1081, got %s", got.Address)
	}
	if got.Username != "user2" {
		t.Fatalf("expected username user2, got %s", got.Username)
	}
	if got.Password != "pass2" {
		t.Fatalf("expected password pass2, got %s", got.Password)
	}
	if got.Weight != 70 {
		t.Fatalf("expected weight 70, got %d", got.Weight)
	}
	if got.Iface != "eth1" {
		t.Fatalf("expected iface eth1, got %s", got.Iface)
	}
	if got.Enabled {
		t.Fatal("expected enabled=false")
	}
}

func TestUpstreamStore_UpdateNonexistent(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)

	err := s.Update(context.Background(), "nope", &model.Upstream{
		Type: model.UpstreamDirect, Weight: 10, Enabled: true,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent upstream")
	}
}

func TestUpstreamStore_UpdateHealth(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Upstream{
		Name: "health-check", Type: model.UpstreamDirect, Weight: 10, Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Healthy check: should reset fail_count to 0
	if err := s.UpdateHealth(ctx, "health-check", true, 42, ""); err != nil {
		t.Fatalf("UpdateHealth ok: %v", err)
	}
	got, err := s.GetByName(ctx, "health-check")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.LastCheckOK == nil || !*got.LastCheckOK {
		t.Fatal("expected last_check_ok=true")
	}
	if got.LatencyMs != 42 {
		t.Fatalf("expected latency_ms=42, got %d", got.LatencyMs)
	}
	if got.FailCount != 0 {
		t.Fatalf("expected fail_count=0, got %d", got.FailCount)
	}

	// Failed check: should increment fail_count
	if err := s.UpdateHealth(ctx, "health-check", false, 0, "timeout"); err != nil {
		t.Fatalf("UpdateHealth fail: %v", err)
	}
	got, err = s.GetByName(ctx, "health-check")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.LastCheckOK == nil || *got.LastCheckOK {
		t.Fatal("expected last_check_ok=false")
	}
	if got.LastError != "timeout" {
		t.Fatalf("expected last_error='timeout', got %s", got.LastError)
	}
	if got.FailCount != 1 {
		t.Fatalf("expected fail_count=1, got %d", got.FailCount)
	}
}

func TestUpstreamStore_ClearHealth(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Upstream{
		Name: "clear-me", Type: model.UpstreamDirect, Weight: 10, Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Set some health data first
	if err := s.UpdateHealth(ctx, "clear-me", false, 99, "conn refused"); err != nil {
		t.Fatalf("UpdateHealth: %v", err)
	}

	// Clear it
	if err := s.ClearHealth(ctx, "clear-me"); err != nil {
		t.Fatalf("ClearHealth: %v", err)
	}

	got, err := s.GetByName(ctx, "clear-me")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.LastCheckOK != nil {
		t.Fatalf("expected last_check_ok=nil, got %v", got.LastCheckOK)
	}
	if got.LatencyMs != 0 {
		t.Fatalf("expected latency_ms=0, got %d", got.LatencyMs)
	}
	if got.LastError != "" {
		t.Fatalf("expected last_error='', got %s", got.LastError)
	}
	if got.FailCount != 0 {
		t.Fatalf("expected fail_count=0, got %d", got.FailCount)
	}
	if got.LastCheckAt != 0 {
		t.Fatalf("expected last_check_at=0, got %d", got.LastCheckAt)
	}
}

func TestUpstreamStore_GetByNameNonexistent(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)

	got, err := s.GetByName(context.Background(), "nope")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got != nil {
		t.Fatal("expected nil for nonexistent name")
	}
}

func TestUpstreamStore_DisableAndEnableAutomatically(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	if err := s.Create(ctx, &model.Upstream{
		Name: "test-auto", Type: model.UpstreamDirect, Weight: 10, Enabled: true,
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Disable automatically
	ts := int64(123456789)
	if err := s.DisableAutomatically(ctx, "test-auto", ts); err != nil {
		t.Fatalf("DisableAutomatically: %v", err)
	}

	got, err := s.GetByName(ctx, "test-auto")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got.Enabled {
		t.Fatal("expected enabled=false")
	}
	if !got.AutoDisabled {
		t.Fatal("expected auto_disabled=true")
	}
	if got.AutoDisabledAt != ts {
		t.Fatalf("expected auto_disabled_at=%d, got %d", ts, got.AutoDisabledAt)
	}

	// Enable automatically
	if err := s.EnableAutomatically(ctx, "test-auto"); err != nil {
		t.Fatalf("EnableAutomatically: %v", err)
	}

	got, err = s.GetByName(ctx, "test-auto")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if !got.Enabled {
		t.Fatal("expected enabled=true")
	}
	if got.AutoDisabled {
		t.Fatal("expected auto_disabled=false")
	}
	if got.AutoDisabledAt != 0 {
		t.Fatalf("expected auto_disabled_at=0, got %d", got.AutoDisabledAt)
	}
}

func TestUpstreamStore_CreateMultiple(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	ups := []*model.Upstream{
		{Name: "bulk1", Type: model.UpstreamDirect, Weight: 10, Enabled: true},
		{Name: "bulk2", Type: model.UpstreamSOCKS5, Address: "1.2.3.4:1080", Weight: 15, Enabled: true},
	}
	inserted, err := s.CreateMultiple(ctx, ups)
	if err != nil {
		t.Fatalf("CreateMultiple: %v", err)
	}
	if len(inserted) != 2 {
		t.Errorf("expected inserted count = 2, got %d", len(inserted))
	}
	if len(inserted) == 2 && (inserted[0].Name != "bulk1" || inserted[1].Name != "bulk2") {
		t.Errorf("expected inserted to preserve input order [bulk1 bulk2], got [%s %s]", inserted[0].Name, inserted[1].Name)
	}

	// Verify they are saved
	got1, _ := s.GetByName(ctx, "bulk1")
	if got1 == nil || got1.Weight != 10 {
		t.Error("failed to retrieve bulk1")
	}
	got2, _ := s.GetByName(ctx, "bulk2")
	if got2 == nil || got2.Address != "1.2.3.4:1080" {
		t.Error("failed to retrieve bulk2")
	}
}

func TestUpstreamStore_CreateMultiple_IgnoresDuplicateNames(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	existing := &model.Upstream{Name: "bulk2", Type: model.UpstreamSOCKS5, Address: "1.2.3.4:1080", Weight: 15, Enabled: true}
	if err := s.Create(ctx, existing); err != nil {
		t.Fatalf("Create: %v", err)
	}

	ups := []*model.Upstream{
		{Name: "bulk2", Type: model.UpstreamSOCKS5, Address: "1.2.3.4:1080", Weight: 15, Enabled: true}, // duplicate, should be ignored
		{Name: "bulk3", Type: model.UpstreamDirect, Weight: 20, Enabled: true},                          // new
	}
	inserted, err := s.CreateMultiple(ctx, ups)
	if err != nil {
		t.Fatalf("CreateMultiple with duplicate: %v", err)
	}
	if len(inserted) != 1 {
		t.Errorf("expected inserted count = 1, got %d", len(inserted))
	}
	if len(inserted) == 1 && inserted[0].Name != "bulk3" {
		t.Errorf("expected only bulk3 inserted, got %s", inserted[0].Name)
	}
}

func TestUpstreamStore_CreateMultiple_RollsBackOnInvalid(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	ups := []*model.Upstream{
		{Name: "bulk4", Type: model.UpstreamDirect, Weight: 10, Enabled: true},
		{Name: "", Type: model.UpstreamDirect, Weight: 10, Enabled: true}, // invalid: empty name
	}
	if _, err := s.CreateMultiple(ctx, ups); err == nil {
		t.Fatal("expected validation error")
	}

	// Verify transaction rolled back (bulk4 should not be created)
	got4, _ := s.GetByName(ctx, "bulk4")
	if got4 != nil {
		t.Error("bulk4 should not exist due to transaction rollback")
	}
}

func TestUpstreamStore_ShadowsocksRoundTrip(t *testing.T) {
	db := testutil.OpenTestDB(t)
	s := NewUpstreamStore(db)
	ctx := context.Background()

	const ssURL = "ss://2022-blake3-aes-256-gcm:cGFzcw==@127.0.0.1:8388"
	u := &model.Upstream{
		Name:    "my-ss",
		Type:    model.UpstreamShadowsocks,
		URL:     ssURL,
		Weight:  20,
		Enabled: true,
	}
	if err := s.Create(ctx, u); err != nil {
		t.Fatalf("Create shadowsocks: %v", err)
	}

	got, err := s.GetByName(ctx, "my-ss")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if got == nil {
		t.Fatal("expected shadowsocks upstream, got nil")
	}
	if got.Type != model.UpstreamShadowsocks {
		t.Errorf("type = %q, want shadowsocks", got.Type)
	}
	if got.URL != ssURL {
		t.Errorf("url = %q, want %q", got.URL, ssURL)
	}

	// Health columns must still work after the 017 table rebuild.
	if err := s.UpdateHealth(ctx, "my-ss", true, 42, ""); err != nil {
		t.Fatalf("UpdateHealth on shadowsocks row: %v", err)
	}
	got, _ = s.GetByName(ctx, "my-ss")
	if got.LatencyMs != 42 {
		t.Errorf("latency_ms = %d, want 42 (health columns lost in rebuild?)", got.LatencyMs)
	}
}

// TestUpstreamStore_MigrationRelaxedTypeCheck verifies migration 017 relaxed the
// type CHECK constraint to include shadowsocks while still rejecting unknown types.
func TestUpstreamStore_MigrationRelaxedTypeCheck(t *testing.T) {
	db := testutil.OpenTestDB(t)
	ctx := context.Background()

	if _, err := db.ExecContext(ctx,
		`INSERT INTO upstreams (name, type, url, weight) VALUES ('ss-raw', 'shadowsocks', 'ss://x@127.0.0.1:8388', 10)`); err != nil {
		t.Fatalf("insert shadowsocks should be allowed after migration 017: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO upstreams (name, type, weight) VALUES ('bad', 'http', 10)`); err == nil {
		t.Fatal("insert with type 'http' must be rejected by CHECK constraint")
	}
}
