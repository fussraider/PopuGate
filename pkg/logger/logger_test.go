package logger

import (
	"strings"
	"testing"
)

func TestFatalf_PanicsInsteadOfExit(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Fatalf should panic, but recovered nil")
		}
		msg, ok := r.(string)
		if !ok {
			t.Errorf("expected string panic, got %T: %v", r, r)
			return
		}
		if !strings.Contains(msg, "fatal test message") {
			t.Errorf("panic message should contain 'fatal test message', got: %s", msg)
		}
	}()

	Fatalf("fatal test message: %d", 42)
}

func TestFatalf_AllowsDeferredCleanup(t *testing.T) {
	cleaned := false
	defer func() {
		recover()
		if !cleaned {
			t.Error("deferred cleanup should have run before Fatalf panic")
		}
	}()
	defer func() {
		cleaned = true
	}()

	Fatalf("should not prevent deferred cleanup")
}

func TestSetGetLevel(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)

	SetLevel(LevelError)
	if GetLevel() != LevelError {
		t.Errorf("expected LevelError, got %d", GetLevel())
	}

	SetLevel(LevelDebug)
	if GetLevel() != LevelDebug {
		t.Errorf("expected LevelDebug, got %d", GetLevel())
	}
}

func TestLevel_String(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelFatal, "FATAL"},
		{Level(-1), "UNKNOWN"},
	}
	for _, tt := range tests {
		got := tt.level.String()
		if got != tt.want {
			t.Errorf("Level(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  Level
	}{
		{"debug", LevelDebug},
		{"info", LevelInfo},
		{"warn", LevelWarn},
		{"warning", LevelWarn},
		{"error", LevelError},
		{"fatal", LevelFatal},
		{"", LevelDebug},
		{"unknown", LevelDebug},
		{"  DEBUG  ", LevelDebug},
	}
	for _, tt := range tests {
		got := parseLevel(tt.input)
		if got != tt.want {
			t.Errorf("parseLevel(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestWithScope(t *testing.T) {
	l := WithScope("test")
	if l.scope != "test" {
		t.Errorf("expected scope 'test', got %q", l.scope)
	}
}
