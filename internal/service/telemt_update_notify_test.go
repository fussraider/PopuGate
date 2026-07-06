package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

// notifyRecorder emulates makeNotifyFn from cmd/popugate/server.go: the
// implementation prepends the server label as the first format argument,
// so every format's first %s verb is the label and callers pass arguments
// only for the remaining verbs.
type notifyRecorder struct {
	mu   sync.Mutex
	msgs []string
}

func (r *notifyRecorder) fn() NotifyFunc {
	return func(_ context.Context, format string, args ...any) {
		full := append([]any{"TestServer"}, args...)
		msg := fmt.Sprintf(format, full...)
		r.mu.Lock()
		defer r.mu.Unlock()
		r.msgs = append(r.msgs, msg)
	}
}

func (r *notifyRecorder) messages() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.msgs...)
}

// waitForMessage polls until a recorded message contains substr.
func (r *notifyRecorder) waitForMessage(t *testing.T, substr string) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, m := range r.messages() {
			if strings.Contains(m, substr) {
				return m
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no notification containing %q, got: %q", substr, r.messages())
	return ""
}

// assertWellFormed fails on fmt verb/argument mismatches (%!s(MISSING),
// %!(EXTRA ...) markers) — the exact bug class where notifyUpdate callers
// pass an argument for the auto-filled server-label verb.
func assertWellFormed(t *testing.T, msgs []string) {
	t.Helper()
	for _, m := range msgs {
		if strings.Contains(m, "%!") {
			t.Errorf("malformed notification (format/args mismatch): %q", m)
		}
	}
}

type fakeBuilder struct {
	err     error
	result  *BuildResult
	running bool
}

func (f *fakeBuilder) BuildRunning() bool { return f.running }

func (f *fakeBuilder) BuildEngine(_ context.Context, _ bool, _ string) (*BuildResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	return &BuildResult{Method: "registry", Version: "9.9.9-abc"}, nil
}

// blockingBuilder blocks until the update context is cancelled, mimicking a
// long build interrupted via Cancel. It closes started so the test can wait
// for the build phase before cancelling.
type blockingBuilder struct {
	started chan struct{}
}

func (b *blockingBuilder) BuildRunning() bool { return false }

func (b *blockingBuilder) BuildEngine(ctx context.Context, _ bool, _ string) (*BuildResult, error) {
	close(b.started)
	<-ctx.Done()
	return nil, ctx.Err()
}

type fakeRestarter struct {
	err error
}

func (f *fakeRestarter) Restart(context.Context) error { return f.err }

func newNotifyTestService(t *testing.T) (*TelemtUpdateService, *store.SettingsStore, *notifyRecorder) {
	t.Helper()
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	telemtCfg := NewDBTelemtConfig(settingsStore)
	telemtCfg.SetCacheTTL(0)

	svc := NewTelemtUpdateService(settingsStore, nil, nil, telemtCfg)
	rec := &notifyRecorder{}
	svc.SetNotify(rec.fn())
	return svc, settingsStore, rec
}

func TestTelemtUpdate_Notify_Success(t *testing.T) {
	svc, _, rec := newNotifyTestService(t)
	svc.dockerSvc = &fakeBuilder{}
	svc.containerSvc = &fakeRestarter{}

	svc.runUpdateBackground(context.Background(), "9.9.9", "abc", "9.9.9-abc")

	msgs := rec.messages()
	assertWellFormed(t, msgs)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 notification, got %d: %q", len(msgs), msgs)
	}
	want := "✅ *TestServer* Telemt engine updated to 9.9.9-abc"
	if msgs[0] != want {
		t.Errorf("notification = %q, want %q", msgs[0], want)
	}
}

func TestTelemtUpdate_Notify_BuildError_RevertsVersion(t *testing.T) {
	svc, settingsStore, rec := newNotifyTestService(t)
	svc.dockerSvc = &fakeBuilder{err: errors.New("boom")}

	_ = settingsStore.Save(context.Background(), map[string]string{
		"telemt_version": "1.0.0",
		"telemt_commit":  "old123",
	})

	svc.runUpdateBackground(context.Background(), "9.9.9", "abc", "9.9.9-abc")

	msgs := rec.messages()
	assertWellFormed(t, msgs)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 notification, got %d: %q", len(msgs), msgs)
	}
	want := "❌ *TestServer* Telemt engine update to 9.9.9-abc failed: build error\nboom"
	if msgs[0] != want {
		t.Errorf("notification = %q, want %q", msgs[0], want)
	}

	v, _ := settingsStore.Get(context.Background(), "telemt_version")
	if v != "1.0.0" {
		t.Errorf("telemt_version = %q, want reverted 1.0.0", v)
	}
	c, _ := settingsStore.Get(context.Background(), "telemt_commit")
	if c != "old123" {
		t.Errorf("telemt_commit = %q, want reverted old123", c)
	}
}

func TestTelemtUpdate_Notify_RestartError(t *testing.T) {
	svc, _, rec := newNotifyTestService(t)
	svc.dockerSvc = &fakeBuilder{}
	svc.containerSvc = &fakeRestarter{err: errors.New("restart boom")}

	svc.runUpdateBackground(context.Background(), "9.9.9", "abc", "9.9.9-abc")

	msgs := rec.messages()
	assertWellFormed(t, msgs)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 notification, got %d: %q", len(msgs), msgs)
	}
	want := "❌ *TestServer* Telemt engine update to 9.9.9-abc failed: restart error\nrestart boom"
	if msgs[0] != want {
		t.Errorf("notification = %q, want %q", msgs[0], want)
	}
}

func TestTelemtUpdate_Notify_SaveError(t *testing.T) {
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	telemtCfg := NewDBTelemtConfig(settingsStore)
	telemtCfg.SetCacheTTL(0)

	svc := NewTelemtUpdateService(settingsStore, nil, nil, telemtCfg)
	rec := &notifyRecorder{}
	svc.SetNotify(rec.fn())
	svc.dockerSvc = &fakeBuilder{}

	_ = db.Close() // force settings.Save to fail

	svc.runUpdateBackground(context.Background(), "9.9.9", "abc", "9.9.9-abc")

	msgs := rec.messages()
	assertWellFormed(t, msgs)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 notification, got %d: %q", len(msgs), msgs)
	}
	wantPrefix := "❌ *TestServer* Telemt engine update to 9.9.9-abc failed: save error\n"
	if !strings.HasPrefix(msgs[0], wantPrefix) {
		t.Errorf("notification = %q, want prefix %q", msgs[0], wantPrefix)
	}
}

// cancellingRestarter cancels the update from inside Restart and records
// whether its own context survived — the restart must run on a context that a
// user cancel cannot abort (point of no return after a successful build).
type cancellingRestarter struct {
	svc      *TelemtUpdateService
	called   bool
	ctxAlive bool
}

func (r *cancellingRestarter) Restart(ctx context.Context) error {
	r.called = true
	_ = r.svc.Cancel(context.Background())
	r.ctxAlive = ctx.Err() == nil
	return nil
}

func TestTelemtUpdate_CancelDuringRestart_DoesNotAbortRestart(t *testing.T) {
	svc, settingsStore, rec := newNotifyTestService(t)
	svc.dockerSvc = &fakeBuilder{}
	restarter := &cancellingRestarter{svc: svc}
	svc.containerSvc = restarter

	if err := svc.Apply(context.Background(), "9.9.9", "abc"); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got := rec.waitForMessage(t, "✅")
	want := "✅ *TestServer* Telemt engine updated to 9.9.9-abc"
	if got != want {
		t.Errorf("success notification = %q, want %q", got, want)
	}
	if !restarter.called {
		t.Fatal("restart was never called")
	}
	if !restarter.ctxAlive {
		t.Error("restart context was cancelled by user Cancel — restart must be a point of no return")
	}

	v, _ := settingsStore.Get(context.Background(), "telemt_version")
	if v != "9.9.9" {
		t.Errorf("telemt_version = %q, want 9.9.9 (no revert after successful build)", v)
	}
}

func TestTelemtUpdate_RestartError_RecordsLastError(t *testing.T) {
	svc, _, rec := newNotifyTestService(t)
	svc.dockerSvc = &fakeBuilder{}
	svc.containerSvc = &fakeRestarter{err: errors.New("daemon flaky")}

	svc.runUpdateBackground(context.Background(), "9.9.9", "abc", "9.9.9-abc")

	assertWellFormed(t, rec.messages())
	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.LastError, "restart failed") || !strings.Contains(status.LastError, "daemon flaky") {
		t.Errorf("status.LastError = %q, want restart failure recorded", status.LastError)
	}
}

func TestTelemtUpdate_BuildInProgress_AbortMessage(t *testing.T) {
	svc, settingsStore, rec := newNotifyTestService(t)
	svc.dockerSvc = &fakeBuilder{err: ErrBuildInProgress}

	_ = settingsStore.Save(context.Background(), map[string]string{
		"telemt_version": "1.0.0",
		"telemt_commit":  "old123",
	})

	svc.runUpdateBackground(context.Background(), "9.9.9", "abc", "9.9.9-abc")

	msgs := rec.messages()
	assertWellFormed(t, msgs)
	want := "⚠️ *TestServer* Telemt engine update to 9.9.9-abc aborted: engine build already in progress"
	if len(msgs) != 1 || msgs[0] != want {
		t.Errorf("notification = %q, want %q", msgs, want)
	}

	v, _ := settingsStore.Get(context.Background(), "telemt_version")
	if v != "1.0.0" {
		t.Errorf("telemt_version = %q, want reverted 1.0.0", v)
	}
	status, _ := svc.GetStatus(context.Background())
	if !strings.Contains(status.LastError, "another engine build") {
		t.Errorf("status.LastError = %q, want build-in-progress abort recorded", status.LastError)
	}
}

func TestTelemtUpdate_Timeout_Message(t *testing.T) {
	svc, _, rec := newNotifyTestService(t)
	svc.dockerSvc = &fakeBuilder{err: context.DeadlineExceeded}

	svc.runUpdateBackground(context.Background(), "9.9.9", "abc", "9.9.9-abc")

	msgs := rec.messages()
	assertWellFormed(t, msgs)
	if len(msgs) != 1 || !strings.Contains(msgs[0], "timed out") {
		t.Errorf("notification = %q, want timeout message", msgs)
	}
	status, _ := svc.GetStatus(context.Background())
	if !strings.Contains(status.LastError, "timed out") {
		t.Errorf("status.LastError = %q, want timeout recorded", status.LastError)
	}
}

func TestTelemtUpdate_Apply_ClearsPreviousError(t *testing.T) {
	svc, settingsStore, rec := newNotifyTestService(t)
	builder := &blockingBuilder{started: make(chan struct{})}
	svc.dockerSvc = builder

	_ = settingsStore.Save(context.Background(), map[string]string{
		"telemt_update_error": "old failure",
	})

	if err := svc.Apply(context.Background(), "9.9.9", "abc"); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.LastError != "" {
		t.Errorf("status.LastError = %q, want cleared on new Apply", status.LastError)
	}

	// Wind the background goroutine down before the test DB closes.
	<-builder.started
	_ = svc.Cancel(context.Background())
	rec.waitForMessage(t, "⏹️")
}

func TestTelemtUpdate_Apply_RejectedWhileManualBuildRunning(t *testing.T) {
	svc, _, rec := newNotifyTestService(t)
	svc.dockerSvc = &fakeBuilder{running: true}

	err := svc.Apply(context.Background(), "9.9.9", "abc")
	if !errors.Is(err, ErrBuildInProgress) {
		t.Errorf("Apply during manual build = %v, want ErrBuildInProgress", err)
	}
	if len(rec.messages()) != 0 {
		t.Errorf("expected no notifications, got %q", rec.messages())
	}
}

func TestTelemtUpdate_Notify_ApplyAndCancel(t *testing.T) {
	svc, settingsStore, rec := newNotifyTestService(t)
	builder := &blockingBuilder{started: make(chan struct{})}
	svc.dockerSvc = builder

	if err := svc.Apply(context.Background(), "9.9.9", "abc"); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	msgs := rec.messages()
	assertWellFormed(t, msgs)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 notification after Apply, got %d: %q", len(msgs), msgs)
	}
	want := "⏳ *TestServer* Updating telemt engine to 9.9.9-abc..."
	if msgs[0] != want {
		t.Errorf("start notification = %q, want %q", msgs[0], want)
	}

	if err := svc.Apply(context.Background(), "9.9.9", "abc"); err == nil ||
		!strings.Contains(err.Error(), "already in progress") {
		t.Errorf("second Apply error = %v, want 'update already in progress'", err)
	}

	select {
	case <-builder.started:
	case <-time.After(3 * time.Second):
		t.Fatal("build phase never started")
	}

	if err := svc.Cancel(context.Background()); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	got := rec.waitForMessage(t, "⏹️")
	wantCancel := "⏹️ *TestServer* Telemt engine update to 9.9.9-abc cancelled by user"
	if got != wantCancel {
		t.Errorf("cancel notification = %q, want %q", got, wantCancel)
	}
	assertWellFormed(t, rec.messages())

	// The background goroutine clears the updating flag on exit.
	deadline := time.Now().Add(3 * time.Second)
	for {
		v, _ := settingsStore.Get(context.Background(), "telemt_updating")
		if v == "false" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("telemt_updating = %q, want false after cancel", v)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := svc.Cancel(context.Background()); err == nil {
		t.Error("Cancel with no update in progress should return an error")
	}
}
