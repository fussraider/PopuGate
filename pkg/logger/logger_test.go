package logger

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestFatalf_ExitsWithCode1(t *testing.T) {
	if os.Getenv("TEST_FATALF") == "1" {
		Fatalf("fatal test message: %d", 42)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestFatalf_ExitsWithCode1")
	cmd.Env = append(os.Environ(), "TEST_FATALF=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("Fatalf should cause the process to exit with a non-zero status")
	}
	if !strings.Contains(string(output), "fatal test message") {
		t.Errorf("output should contain 'fatal test message', got: %s", output)
	}
}

func TestFatalf_PrintsMessage(t *testing.T) {
	if os.Getenv("TEST_FATALF") == "1" {
		Fatalf("should print this message before exiting")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestFatalf_PrintsMessage")
	cmd.Env = append(os.Environ(), "TEST_FATALF=1")
	output, _ := cmd.CombinedOutput()
	if !strings.Contains(string(output), "should print this message before exiting") {
		t.Errorf("output should contain the fatal message, got: %s", output)
	}
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

func TestColorize(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{LevelDebug, ansiBlue + "DEBUG" + ansiReset},
		{LevelInfo, ansiGreen + "INFO" + ansiReset},
		{LevelWarn, ansiYellow + "WARN" + ansiReset},
		{LevelError, ansiRed + "ERROR" + ansiReset},
		{LevelFatal, ansiRed + ansiBold + "FATAL" + ansiReset},
	}
	for _, tt := range tests {
		got := colorize(tt.level)
		if got != tt.want {
			t.Errorf("colorize(%s) = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestLevelEmoji(t *testing.T) {
	origNoColor := noColor
	defer func() { noColor = origNoColor }()

	// With colors enabled
	noColor = false
	if emoji := levelEmoji(LevelDebug); emoji != emojiDebug {
		t.Errorf("levelEmoji(Debug) = %q, want %q", emoji, emojiDebug)
	}
	if emoji := levelEmoji(LevelInfo); emoji != emojiInfo {
		t.Errorf("levelEmoji(Info) = %q, want %q", emoji, emojiInfo)
	}

	// With noColor
	noColor = true
	if emoji := levelEmoji(LevelDebug); emoji != "" {
		t.Errorf("levelEmoji with noColor should return empty, got %q", emoji)
	}
}

func TestGinLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinLogger())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestWrite_SuppressesBelowLevel(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)

	SetLevel(LevelError)
	// These should not panic or fail - they're suppressed
	Debugf("should be suppressed")
	Infof("should be suppressed")
	Warnf("should be suppressed")
	// This should actually write
	Errorf("should be visible")
}

func TestScopedLogger_Methods(t *testing.T) {
	origLevel := GetLevel()
	defer SetLevel(origLevel)

	SetLevel(LevelDebug)
	l := WithScope("testscope")

	// Should not panic
	l.Debugf("debug msg: %d", 1)
	l.Infof("info msg: %s", "hello")
	l.Warnf("warn msg")
	l.Errorf("error msg")
}
