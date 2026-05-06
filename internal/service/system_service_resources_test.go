package service

import (
	"context"
	"testing"

	"github.com/fussraider/PopuGate/internal/model"
)

// ---------------------------------------------------------------------------
// CheckResources
// ---------------------------------------------------------------------------

// mockResources temporarily replaces GetResources for testing.
// It patches the global via a test-local override of readSysinfo/readDisk.
// Since those are platform-specific, we test the logic layer by calling
// CheckResources with a deliberately crafted notify func and injecting
// alertCooldown state.

func TestCheckResources_NoAlert_WhenBelowThresholds(t *testing.T) {
	// Reset alert timers so we start clean.
	alertMu.Lock()
	delete(lastAlertTime, "memory")
	delete(lastAlertTime, "disk")
	alertMu.Unlock()

	var called int
	notify := func(ctx context.Context, format string, args ...any) {
		called++
	}

	// Inject a low-usage resource snapshot via the monitor singleton.
	// CheckResources calls GetResources() which is platform-specific, so we
	// just verify that when resources are well below thresholds no alert fires.
	// We can't easily mock GetResources without interfaces, so we test the
	// threshold logic directly by calling the internal guard.
	testCheckResourcesWithStats(t, &model.SystemResources{
		MemoryTotal: 8 * 1024 * 1024 * 1024, // 8 GB
		MemoryUsed:  4 * 1024 * 1024 * 1024, // 50%
		DiskTotal:   100 * 1024 * 1024 * 1024,
		DiskUsed:    50 * 1024 * 1024 * 1024, // 50%
	}, notify)

	if called != 0 {
		t.Errorf("expected 0 notifications, got %d", called)
	}
}

func TestCheckResources_Alert_OnHighMemory(t *testing.T) {
	alertMu.Lock()
	delete(lastAlertTime, "memory")
	alertMu.Unlock()

	var called int
	notify := func(ctx context.Context, format string, args ...any) {
		called++
	}

	testCheckResourcesWithStats(t, &model.SystemResources{
		MemoryTotal: 8 * 1024 * 1024 * 1024,
		MemoryUsed:  7*1024*1024*1024 + 900*1024*1024, // ~98.6%
		DiskTotal:   100 * 1024 * 1024 * 1024,
		DiskUsed:    50 * 1024 * 1024 * 1024,
	}, notify)

	if called != 1 {
		t.Errorf("expected 1 memory alert, got %d", called)
	}
}

func TestCheckResources_Alert_OnHighDisk(t *testing.T) {
	alertMu.Lock()
	delete(lastAlertTime, "disk")
	alertMu.Unlock()

	var called int
	notify := func(ctx context.Context, format string, args ...any) {
		called++
	}

	testCheckResourcesWithStats(t, &model.SystemResources{
		MemoryTotal: 8 * 1024 * 1024 * 1024,
		MemoryUsed:  1 * 1024 * 1024 * 1024, // 12.5%
		DiskTotal:   100 * 1024 * 1024 * 1024,
		DiskUsed:    95 * 1024 * 1024 * 1024, // 95%
	}, notify)

	if called != 1 {
		t.Errorf("expected 1 disk alert, got %d", called)
	}
}

func TestCheckResources_CooldownPreventsDoubleAlert(t *testing.T) {
	alertMu.Lock()
	delete(lastAlertTime, "memory")
	alertMu.Unlock()

	var called int
	notify := func(ctx context.Context, format string, args ...any) {
		called++
	}

	stats := &model.SystemResources{
		MemoryTotal: 8 * 1024 * 1024 * 1024,
		MemoryUsed:  7*1024*1024*1024 + 900*1024*1024, // ~98.6%
		DiskTotal:   100 * 1024 * 1024 * 1024,
		DiskUsed:    50 * 1024 * 1024 * 1024,
	}

	testCheckResourcesWithStats(t, stats, notify) // first call — fires
	testCheckResourcesWithStats(t, stats, notify) // second call — cooldown active

	if called != 1 {
		t.Errorf("expected exactly 1 alert due to cooldown, got %d", called)
	}
}

// testCheckResourcesWithStats exercises checkResourcesWithStats, the extracted
// pure-logic helper that does not call GetResources().
func testCheckResourcesWithStats(t *testing.T, res *model.SystemResources, notify NotifyFunc) {
	t.Helper()
	checkResourcesWithStats(context.Background(), res, notify)
}
