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

// renderLikeNotifier reproduces makeNotifyFn's behavior: the implementation
// prepends the server label as the first format argument before Sprintf. Tests
// use it so a verb/argument mismatch surfaces exactly as a user would see it
// (a "%!" badverb / EXTRA marker in the rendered message).
func renderLikeNotifier(label, format string, args ...any) string {
	return fmt.Sprintf(format, append([]any{label}, args...)...)
}

func assertNoBadVerb(t *testing.T, msg string) {
	t.Helper()
	if strings.Contains(msg, "%!") {
		t.Fatalf("notification has a format verb/argument mismatch: %q", msg)
	}
}

// seedFailedUpstream creates an enabled upstream and drives its fail_count to 3
// (the auto-disable threshold) via the real UpdateHealth path.
func seedFailedUpstream(t *testing.T, s *UpstreamService, name string) {
	t.Helper()
	ctx := context.Background()
	u := &model.Upstream{Name: name, Type: model.UpstreamDirect, Address: "1.2.3.4:1080", Enabled: true, Weight: 1}
	if err := s.upstreams.Create(ctx, u); err != nil {
		t.Fatalf("create upstream: %v", err)
	}
	for range 3 {
		if err := s.upstreams.UpdateHealth(ctx, name, false, 0, "boom"); err != nil {
			t.Fatalf("UpdateHealth: %v", err)
		}
	}
}

func TestUpstreamNotify_AutoDisable_NoBadVerb(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()
	const name = "proxy6uae"
	seedFailedUpstream(t, svc, name)

	var msg string
	svc.SetNotify(func(_ context.Context, format string, args ...any) {
		msg = renderLikeNotifier("PopuGate", format, args...)
	})

	errMsg := `proxy reachable (TCP 41ms) but HTTP request failed: Get "https://icanhazip.com": context deadline exceeded`
	svc.handleFailover(ctx, name, errMsg)

	if msg == "" {
		t.Fatal("expected a notification to be sent")
	}
	assertNoBadVerb(t, msg)
	if !strings.Contains(msg, name) {
		t.Errorf("message missing upstream name: %q", msg)
	}
	if !strings.Contains(msg, errMsg) {
		t.Errorf("message missing error detail: %q", msg)
	}
	if !strings.Contains(msg, "PopuGate") {
		t.Errorf("message missing server label: %q", msg)
	}
}

func TestUpstreamNotify_AutoRecover_NoBadVerb(t *testing.T) {
	svc := newTestUpstreamService(t)
	ctx := context.Background()
	const name = "proxy6uae"
	// The upstream must exist and be (auto-)disabled for EnableAutomatically.
	seedFailedUpstream(t, svc, name)
	if err := svc.upstreams.DisableAutomatically(ctx, name, 1); err != nil {
		t.Fatalf("DisableAutomatically: %v", err)
	}

	var msg string
	svc.SetNotify(func(_ context.Context, format string, args ...any) {
		msg = renderLikeNotifier("PopuGate", format, args...)
	})

	u, err := svc.upstreams.GetByName(ctx, name)
	if err != nil || u == nil {
		t.Fatalf("GetByName: %v", err)
	}
	svc.handleAutoRecovery(ctx, u, 407)

	if msg == "" {
		t.Fatal("expected a notification to be sent")
	}
	assertNoBadVerb(t, msg)
	if !strings.Contains(msg, name) {
		t.Errorf("message missing upstream name: %q", msg)
	}
	if !strings.Contains(msg, "407ms") {
		t.Errorf("message missing latency: %q", msg)
	}
}

// The button variants use their own (duplicated) format-string literals, so
// exercise them too: with settings.web_url set and a notifyWithBtns handler,
// handleFailover/handleAutoRecovery take the keyboard-button branch.
func TestUpstreamNotify_ButtonVariants_NoBadVerb(t *testing.T) {
	db := testutil.OpenTestDB(t)
	settings := store.NewSettingsStore(db)
	ctx := context.Background()
	if err := settings.Save(ctx, map[string]string{"web_url": "https://panel.example"}); err != nil {
		t.Fatalf("save web_url: %v", err)
	}
	svc := NewUpstreamService(store.NewUpstreamStore(db))
	svc.SetSettings(settings)

	var msg string
	svc.SetNotifyWithButtons(func(_ context.Context, format string, _ []KeyboardButton, args ...any) {
		msg = renderLikeNotifier("PopuGate", format, args...)
	})

	const name = "proxy6uae"
	seedFailedUpstream(t, svc, name)

	svc.handleFailover(ctx, name, "boom detail")
	assertNoBadVerb(t, msg)
	if !strings.Contains(msg, name) || !strings.Contains(msg, "boom detail") {
		t.Errorf("disable button message wrong: %q", msg)
	}

	if err := svc.upstreams.DisableAutomatically(ctx, name, 1); err != nil {
		t.Fatalf("DisableAutomatically: %v", err)
	}
	u, err := svc.upstreams.GetByName(ctx, name)
	if err != nil || u == nil {
		t.Fatalf("GetByName: %v", err)
	}
	msg = ""
	svc.handleAutoRecovery(ctx, u, 407)
	assertNoBadVerb(t, msg)
	if !strings.Contains(msg, name) || !strings.Contains(msg, "407ms") {
		t.Errorf("recover button message wrong: %q", msg)
	}
}
