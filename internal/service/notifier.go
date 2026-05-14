package service

import (
	"context"

	"github.com/fussraider/PopuGate/internal/store"
)

// NotifyFunc sends a formatted notification. The first %s verb in format
// is the server label, resolved automatically by the implementation.
// Implementations must be safe for concurrent use and silently skip
// when notifications are disabled or unavailable.
type NotifyFunc func(ctx context.Context, format string, args ...any)

// KeyboardButton represents an inline keyboard URL button for Telegram notifications.
type KeyboardButton struct {
	Text string
	URL  string
}

// NotifyWithButtonsFunc sends a notification with optional inline keyboard buttons.
// When buttons is empty or nil, behavior is identical to NotifyFunc.
type NotifyWithButtonsFunc func(ctx context.Context, format string, buttons []KeyboardButton, args ...any)

func dashboardButton(ctx context.Context, settings *store.SettingsStore) KeyboardButton {
	s, _ := settings.Load(ctx)
	if s == nil || s.WebURL == "" {
		return KeyboardButton{}
	}
	return KeyboardButton{Text: "Dashboard", URL: s.WebURL + "/"}
}
