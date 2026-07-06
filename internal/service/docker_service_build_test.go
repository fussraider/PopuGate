package service

import (
	"context"
	"errors"
	"testing"
)

func TestDockerService_BuildCancelRegistration(t *testing.T) {
	s := NewDockerService(nil, nil)

	if err := s.CancelBuild(); !errors.Is(err, ErrNoBuildRunning) {
		t.Errorf("CancelBuild with no build = %v, want ErrNoBuildRunning", err)
	}

	ctx, done, err := s.beginBuild(context.Background())
	if err != nil {
		t.Fatalf("beginBuild: %v", err)
	}
	if !s.BuildRunning() {
		t.Error("BuildRunning = false, want true after beginBuild")
	}

	// Only one build may run at a time.
	if _, _, err := s.beginBuild(context.Background()); !errors.Is(err, ErrBuildInProgress) {
		t.Errorf("second beginBuild = %v, want ErrBuildInProgress", err)
	}

	if err := s.CancelBuild(); err != nil {
		t.Fatalf("CancelBuild: %v", err)
	}
	if ctx.Err() != context.Canceled {
		t.Errorf("build ctx.Err() = %v, want Canceled", ctx.Err())
	}
	// Repeated cancel while the build is winding down is idempotent.
	if err := s.CancelBuild(); err != nil {
		t.Errorf("second CancelBuild while winding down = %v, want nil", err)
	}

	done()
	if s.BuildRunning() {
		t.Error("BuildRunning = true, want false after done")
	}
	if err := s.CancelBuild(); !errors.Is(err, ErrNoBuildRunning) {
		t.Errorf("CancelBuild after done = %v, want ErrNoBuildRunning", err)
	}

	// A new build can be registered after the previous one finished.
	_, done2, err := s.beginBuild(context.Background())
	if err != nil {
		t.Fatalf("beginBuild after done: %v", err)
	}
	done2()
}
