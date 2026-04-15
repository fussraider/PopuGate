package logger

import "os"

// ANSI color codes.
const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiBlue   = "\033[34m"
	ansiBold   = "\033[1m"
)

// Emoji squares for each level.
const (
	emojiDebug = "\U0001F7E6" // 🟦
	emojiInfo  = "\U0001F7E9" // 🟩
	emojiWarn  = "\U0001F7E8" // 🟨
	emojiError = "\U0001F7E7" // 🟧
	emojiFatal = "\U0001F7E5" // 🟥
)

// noColor is set to true when NO_COLOR env is present.
var noColor bool

func init() {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		noColor = true
	}
}

// colorize returns the level string wrapped in the appropriate ANSI color.
func colorize(l Level) string {
	var code string
	switch l {
	case LevelDebug:
		code = ansiBlue
	case LevelInfo:
		code = ansiGreen
	case LevelWarn:
		code = ansiYellow
	case LevelError:
		code = ansiRed
	case LevelFatal:
		code = ansiRed + ansiBold
	}
	return code + l.String() + ansiReset
}

// levelEmoji returns the emoji square for the given level, or empty if noColor.
func levelEmoji(l Level) string {
	if noColor {
		return ""
	}
	switch l {
	case LevelDebug:
		return emojiDebug
	case LevelInfo:
		return emojiInfo
	case LevelWarn:
		return emojiWarn
	case LevelError:
		return emojiError
	case LevelFatal:
		return emojiFatal
	}
	return ""
}
