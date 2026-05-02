package service

import "context"

// NotifyFunc sends a formatted notification. The first %s verb in format
// is the server label, resolved automatically by the implementation.
// Implementations must be safe for concurrent use and silently skip
// when notifications are disabled or unavailable.
type NotifyFunc func(ctx context.Context, format string, args ...any)
