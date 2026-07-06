package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func newUpstreamServiceWithSettings(t *testing.T) (*UpstreamService, *store.SettingsStore) {
	t.Helper()
	db := testutil.OpenTestDB(t)
	svc := NewUpstreamService(store.NewUpstreamStore(db))
	settings := store.NewSettingsStore(db)
	svc.SetSettings(settings)
	return svc, settings
}

// Legacy fully-base64 ss:// URLs carry the host inside the base64 blob, so no
// probe can run. The health check must report OK instead of failing, otherwise
// the scheduler auto-disables a perfectly healthy upstream forever.
func TestTestShadowsocks_LegacyURLSkipsProbe(t *testing.T) {
	svc, _ := newUpstreamServiceWithSettings(t)

	u := &model.Upstream{
		Type: model.UpstreamShadowsocks,
		URL:  "ss://YWVzLTI1Ni1nY206cGFzc3dvcmRAMTI3LjAuMC4xOjgzODg", // no raw '@'
	}
	res, err := svc.testShadowsocks(context.Background(), u)
	if err != nil {
		t.Fatalf("testShadowsocks: %v", err)
	}
	if !res.OK {
		t.Errorf("legacy base64 ss:// URL must skip the probe and report OK, got OK=false (%q)", res.Error)
	}
}

// ADR-001: auto-recovery must not re-enable a shadowsocks upstream while
// Middle-Proxy mode is on — telemt would reject the whole engine config.
func TestHandleAutoRecovery_ShadowsocksBlockedByMiddleProxy(t *testing.T) {
	svc, settings := newUpstreamServiceWithSettings(t)
	ctx := context.Background()

	_ = settings.Save(ctx, map[string]string{"use_middle_proxy": "true"})

	u := &model.Upstream{
		Name:   "ss-rec",
		Type:   model.UpstreamShadowsocks,
		URL:    "ss://method:pw@127.0.0.1:8388",
		Weight: 10,
	}
	if err := svc.upstreams.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.upstreams.DisableAutomatically(ctx, "ss-rec", 123); err != nil {
		t.Fatalf("DisableAutomatically: %v", err)
	}

	stored, _ := svc.Get(ctx, "ss-rec")
	svc.handleAutoRecovery(ctx, stored, 42)

	got, _ := svc.Get(ctx, "ss-rec")
	if got.Enabled {
		t.Error("shadowsocks upstream must stay disabled while Middle-Proxy mode is on")
	}
	if !got.AutoDisabled {
		t.Error("auto_disabled flag must survive the blocked recovery")
	}

	// With Middle-Proxy off the same recovery proceeds.
	_ = settings.Save(ctx, map[string]string{"use_middle_proxy": "false"})
	svc.handleAutoRecovery(ctx, got, 42)

	got, _ = svc.Get(ctx, "ss-rec")
	if !got.Enabled {
		t.Error("shadowsocks upstream must auto-recover once Middle-Proxy mode is off")
	}
}

// Names generated for non-shadowsocks upstreams must hash exactly as before
// the URL field existed: re-imports of old bulk lists rely on name-based
// dedup (INSERT OR IGNORE) matching the rows created by earlier releases.
func TestGenerateBulkUpstreamName_HashBackwardCompatible(t *testing.T) {
	u := &model.Upstream{
		Type:     model.UpstreamSOCKS5,
		Address:  "10.0.0.1:1080",
		Username: "user",
		Password: "pass",
		Iface:    "eth0",
	}

	// The pre-shadowsocks releases hashed exactly these five fields.
	identity := strings.Join([]string{string(u.Type), u.Address, u.Username, u.Password, u.Iface}, "\x00")
	h := uint32(0)
	for i := 0; i < len(identity); i++ {
		h = h*31 + uint32(identity[i])
	}
	wantSuffix := fmt.Sprintf("%08x", h)

	name := GenerateBulkUpstreamName(u)
	if !strings.HasSuffix(name, wantSuffix) {
		t.Errorf("name %q does not end with legacy hash %s — old bulk imports would duplicate", name, wantSuffix)
	}
}

func TestGenerateBulkUpstreamName_ShadowsocksURLsDistinct(t *testing.T) {
	a := &model.Upstream{Type: model.UpstreamShadowsocks, URL: "ss://m:p@10.0.0.1:8388"}
	b := &model.Upstream{Type: model.UpstreamShadowsocks, URL: "ss://m:p@10.0.0.2:8388"}
	if GenerateBulkUpstreamName(a) == GenerateBulkUpstreamName(b) {
		t.Error("distinct shadowsocks URLs must produce distinct names")
	}
}

// With Middle-Proxy on, bulk add must skip ss:// entries (reporting them) and
// still insert the rest of the batch instead of rejecting it wholesale.
func TestAddMultiple_SkipsShadowsocksWhenMiddleProxyOn(t *testing.T) {
	svc, settings := newUpstreamServiceWithSettings(t)
	ctx := context.Background()

	_ = settings.Save(ctx, map[string]string{"use_middle_proxy": "true"})

	batch := []*model.Upstream{
		{Name: "s5-one", Type: model.UpstreamSOCKS5, Address: "10.0.0.1:1080", Weight: 10},
		{Name: "ss-one", Type: model.UpstreamShadowsocks, URL: "ss://m:p@10.0.0.2:8388", Weight: 10},
	}
	inserted, skipped, err := svc.AddMultiple(ctx, batch)
	if err != nil {
		t.Fatalf("AddMultiple: %v", err)
	}
	if len(inserted) != 1 || inserted[0].Name != "s5-one" {
		t.Errorf("inserted = %v, want only s5-one", inserted)
	}
	if len(skipped) != 1 || skipped[0] != "ss-one" {
		t.Errorf("skipped = %v, want [ss-one]", skipped)
	}
	if row, _ := svc.Get(ctx, "ss-one"); row != nil {
		t.Error("skipped shadowsocks upstream must not be inserted")
	}

	// With Middle-Proxy off the same ss entry is accepted.
	_ = settings.Save(ctx, map[string]string{"use_middle_proxy": "false"})
	inserted, skipped, err = svc.AddMultiple(ctx, []*model.Upstream{
		{Name: "ss-two", Type: model.UpstreamShadowsocks, URL: "ss://m:p@10.0.0.3:8388", Weight: 10},
	})
	if err != nil {
		t.Fatalf("AddMultiple (MP off): %v", err)
	}
	if len(inserted) != 1 || len(skipped) != 0 {
		t.Errorf("MP off: inserted=%d skipped=%d, want 1/0", len(inserted), len(skipped))
	}
}
