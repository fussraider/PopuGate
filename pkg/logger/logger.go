package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Level represents a log severity level.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	}
	return "UNKNOWN"
}

// Logger provides leveled, scoped, colored logging.
type Logger struct {
	scope string
}

var (
	mu    sync.Mutex
	level Level = LevelDebug
)

// Init configures the global logger from environment variables.
// Reads NO_COLOR (any value disables colors) and LOG_LEVEL (debug/info/warn/error/fatal).
func Init() {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		noColor = true
	}
	if l := os.Getenv("LOG_LEVEL"); l != "" {
		level = parseLevel(l)
	}
}

func parseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	case "fatal":
		return LevelFatal
	default:
		return LevelDebug
	}
}

// WithScope returns a Logger that prefixes all messages with [scope].
func WithScope(scope string) *Logger {
	return &Logger{scope: scope}
}

// SetLevel changes the minimum log level at runtime.
func SetLevel(l Level) {
	mu.Lock()
	level = l
	mu.Unlock()
}

// GetLevel returns the current minimum log level.
func GetLevel() Level {
	mu.Lock()
	l := level
	mu.Unlock()
	return l
}

func write(l Level, scope string, format string, args ...any) {
	mu.Lock()
	currentLevel := level
	mu.Unlock()

	if l < currentLevel {
		return
	}

	msg := fmt.Sprintf(format, args...)
	ts := time.Now().Format("2006/01/02 15:04:05")

	var out io.Writer = os.Stdout
	if l >= LevelError {
		out = os.Stderr
	}

	mu.Lock()
	if noColor {
		if scope != "" {
			_, _ = fmt.Fprintf(out, "%-5s %s [%s] %s\n", l.String(), ts, scope, msg)
		} else {
			_, _ = fmt.Fprintf(out, "%-5s %s %s\n", l.String(), ts, msg)
		}
	} else {
		emoji := levelEmoji(l)
		clr := colorize(l)
		if scope != "" {
			_, _ = fmt.Fprintf(out, "%s %s %s [%s] %s\n", emoji, clr, ts, scope, msg)
		} else {
			_, _ = fmt.Fprintf(out, "%s %s %s %s\n", emoji, clr, ts, msg)
		}
	}
	mu.Unlock()

	if l == LevelFatal {
		os.Exit(1)
	}
}

// --- Package-level functions (no scope) ---

func Debugf(format string, args ...any) { write(LevelDebug, "", format, args...) }
func Infof(format string, args ...any)  { write(LevelInfo, "", format, args...) }
func Warnf(format string, args ...any)  { write(LevelWarn, "", format, args...) }
func Errorf(format string, args ...any) { write(LevelError, "", format, args...) }
func Fatalf(format string, args ...any) { write(LevelFatal, "", format, args...) }

// --- Scoped Logger methods ---

func (l *Logger) Debugf(format string, args ...any) { write(LevelDebug, l.scope, format, args...) }
func (l *Logger) Infof(format string, args ...any)  { write(LevelInfo, l.scope, format, args...) }
func (l *Logger) Warnf(format string, args ...any)  { write(LevelWarn, l.scope, format, args...) }
func (l *Logger) Errorf(format string, args ...any) { write(LevelError, l.scope, format, args...) }
func (l *Logger) Fatalf(format string, args ...any) { write(LevelFatal, l.scope, format, args...) }
