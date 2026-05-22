package service

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

// captureNotify creates a NotifyFunc that records calls for assertions.
func captureNotify() (NotifyFunc, *atomic.Int32, *captured) {
	var n atomic.Int32
	var c captured
	fn := func(_ context.Context, format string, args ...any) {
		c.format = format
		c.args = args
		n.Add(1)
	}
	return fn, &n, &c
}

type captured struct {
	format string
	args   []any
}

// --- ContainerService ---

func TestContainerService_Notify_Start(t *testing.T) {
	db := testutil.OpenTestDB(t)
	fn, called, cap := captureNotify()

	svc := NewContainerService(t.TempDir(), nil, nil, nil, nil, nil, store.NewSettingsStore(db), nil)
	svc.SetNotify(fn)
	svc.notifyEngineState(context.Background(), "🟢 *%s* Proxy engine started")

	if called.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", called.Load())
	}
	if cap.format != "🟢 *%s* Proxy engine started" {
		t.Errorf("format = %q", cap.format)
	}
	if len(cap.args) != 0 {
		t.Errorf("expected no args (label resolved by implementation), got %d", len(cap.args))
	}
}

func TestContainerService_Notify_Stop(t *testing.T) {
	db := testutil.OpenTestDB(t)
	fn, called, _ := captureNotify()

	svc := NewContainerService(t.TempDir(), nil, nil, nil, nil, nil, store.NewSettingsStore(db), nil)
	svc.SetNotify(fn)
	svc.notifyEngineState(context.Background(), "🔴 *%s* Proxy engine stopped")

	if called.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", called.Load())
	}
}

func TestContainerService_Notify_NilDoesNotPanic(t *testing.T) {
	db := testutil.OpenTestDB(t)
	svc := NewContainerService(t.TempDir(), nil, nil, nil, nil, nil, store.NewSettingsStore(db), nil)
	svc.notifyEngineState(context.Background(), "🟢 *%s* Proxy engine started")
}

// --- TelemtUpdateService ---

func TestTelemtUpdateService_Notify_UpdateStart(t *testing.T) {
	db := testutil.OpenTestDB(t)
	fn, called, cap := captureNotify()

	svc := &TelemtUpdateService{settings: store.NewSettingsStore(db)}
	svc.SetNotify(fn)
	svc.notifyUpdate(context.Background(), "⏳ *%s* Updating telemt engine to %s...", "4.0.0-abcd")

	if called.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", called.Load())
	}
	if !strings.Contains(cap.format, "Updating telemt engine") {
		t.Errorf("format should mention update, got: %s", cap.format)
	}
	if len(cap.args) != 1 || cap.args[0] != "4.0.0-abcd" {
		t.Errorf("args = %v, want [4.0.0-abcd]", cap.args)
	}
}

func TestTelemtUpdateService_Notify_Success(t *testing.T) {
	db := testutil.OpenTestDB(t)
	fn, _, cap := captureNotify()

	svc := &TelemtUpdateService{settings: store.NewSettingsStore(db)}
	svc.SetNotify(fn)
	svc.notifyUpdate(context.Background(), "✅ *%s* Telemt engine updated to %s", "4.0.0-abcd")

	if !strings.Contains(cap.format, "updated") {
		t.Errorf("format should mention updated, got: %s", cap.format)
	}
	if len(cap.args) != 1 || cap.args[0] != "4.0.0-abcd" {
		t.Errorf("args = %v, want [4.0.0-abcd]", cap.args)
	}
}

func TestTelemtUpdateService_Notify_Failure(t *testing.T) {
	db := testutil.OpenTestDB(t)
	fn, _, cap := captureNotify()

	svc := &TelemtUpdateService{settings: store.NewSettingsStore(db)}
	svc.SetNotify(fn)
	svc.notifyUpdate(context.Background(), "❌ *%s* Telemt engine update failed: %s", "build error")

	if !strings.Contains(cap.format, "failed") {
		t.Errorf("format should mention failed, got: %s", cap.format)
	}
	if len(cap.args) != 1 || cap.args[0] != "build error" {
		t.Errorf("args = %v, want [build error]", cap.args)
	}
}

func TestTelemtUpdateService_Notify_NilDoesNotPanic(t *testing.T) {
	db := testutil.OpenTestDB(t)
	svc := &TelemtUpdateService{settings: store.NewSettingsStore(db)}
	svc.notifyUpdate(context.Background(), "⏳ *%s* Updating to %s", "4.0.0")
}

// --- SetNotify ---

func TestContainerService_SetNotify(t *testing.T) {
	db := testutil.OpenTestDB(t)
	svc := NewContainerService(t.TempDir(), nil, nil, nil, nil, nil, store.NewSettingsStore(db), nil)

	var called atomic.Int32
	svc.SetNotify(func(_ context.Context, _ string, _ ...any) { called.Add(1) })

	svc.notifyEngineState(context.Background(), "🟢 *%s* test")
	if called.Load() != 1 {
		t.Fatalf("expected 1 call after SetNotify, got %d", called.Load())
	}
}

func TestTelemtUpdateService_SetNotify(t *testing.T) {
	db := testutil.OpenTestDB(t)
	svc := NewTelemtUpdateService(store.NewSettingsStore(db), nil, nil, nil)

	var called atomic.Int32
	svc.SetNotify(func(_ context.Context, _ string, _ ...any) { called.Add(1) })

	svc.notifyUpdate(context.Background(), "⏳ *%s* test")
	if called.Load() != 1 {
		t.Fatalf("expected 1 call after SetNotify, got %d", called.Load())
	}
}
