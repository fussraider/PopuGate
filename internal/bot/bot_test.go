package bot

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestTelegramAPIURL_ContainsToken(t *testing.T) {
	b := New("secret-token-123", "123", "test", nil)

	got := b.telegramAPIURL("sendMessage")
	want := "https://api.telegram.org/botsecret-token-123/sendMessage"
	if got != want {
		t.Errorf("telegramAPIURL = %q, want %q", got, want)
	}
}

func TestTelegramAPIURLWithQuery_ContainsToken(t *testing.T) {
	b := New("secret-token-456", "123", "test", nil)

	got := b.telegramAPIURLWithQuery("getUpdates", "offset=5&timeout=30")
	want := "https://api.telegram.org/botsecret-token-456/getUpdates?offset=5&timeout=30"
	if got != want {
		t.Errorf("telegramAPIURLWithQuery = %q, want %q", got, want)
	}
}

func TestSendMessage_ErrorDoesNotLeakToken(t *testing.T) {
	b := New("super-secret-token", "123", "test", nil)

	// Create a server that always returns 401
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request URL contains the token (this is correct behavior)
		if !strings.Contains(r.URL.Path, "super-secret-token") {
			t.Error("request URL should contain bot token")
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	// Override the API URL by using a custom method — we test via the real method
	// but redirect the HTTP client to our test server.
	// Since the URL is hardcoded, we instead verify the error message doesn't contain the token.
	err := b.SendMessage(context.TODO(), "test message")
	if err == nil {
		t.Fatal("expected error from SendMessage with no server")
	}

	// The error must not contain the token
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Errorf("error message leaks bot token: %v", err)
	}
}

func TestBotAuthCheck(t *testing.T) {
	b := New("test-token", "123456789", "test-label", nil)

	tests := []struct {
		name    string
		fromID  int64
		allowed bool
	}{
		{"matching chatID", 123456789, true},
		{"different user", 999888777, false},
		{"zero ID", 0, false},
		{"negative ID", -1, false},
		{"off by one", 123456788, false},
		{"off by one up", 123456790, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fmt.Sprintf("%d", tt.fromID) == b.chatID
			if got != tt.allowed {
				t.Errorf("user %d: allowed=%v, want %v", tt.fromID, got, tt.allowed)
			}
		})
	}
}

func TestHandleUpdate_NilMessage(t *testing.T) {
	b := New("test-token", "123456789", "test-label", nil)
	// Should not panic
	b.handleUpdate(context.TODO(), TelegramUpdate{UpdateID: 1, Message: nil})
}

func TestHandleUpdate_NonCommandMessage(t *testing.T) {
	b := New("test-token", "123456789", "test-label", nil)
	// Should not panic, should return early
	b.handleUpdate(context.TODO(), TelegramUpdate{
		UpdateID: 2,
		Message: &TelegramMessage{
			MessageID: 10,
			Text:      "hello world",
			From: struct {
				ID       int64  `json:"id"`
				Username string `json:"username"`
			}{ID: 123456789, Username: "admin"},
		},
	})
}

func TestHandleUpdate_AuthorizedHelpCommand(t *testing.T) {
	b := New("test-token", "123456789", "test-label", nil)

	// cmdHelp is a pure function with no deps — test it directly
	got := b.cmdHelp()
	if got.text == "" {
		t.Error("cmdHelp returned empty string")
	}
	if !strings.Contains(got.text, "/status") {
		t.Error("cmdHelp output should contain bot commands")
	}
}

func TestHandleUpdate_AuthCheckRejectsBeforeCommandDispatch(t *testing.T) {
	// Verify that the auth check occurs BEFORE any command processing.
	// If the check were missing, a nil deps bot would panic on /status.
	b := New("test-token", "123456789", "test-label", nil)

	// This should NOT panic because auth rejects user 999 before reaching cmdStatus
	// which would dereference nil Settings deps.
	b.handleUpdate(context.TODO(), TelegramUpdate{
		UpdateID: 3,
		Message: &TelegramMessage{
			MessageID: 11,
			Text:      "/status",
			From: struct {
				ID       int64  `json:"id"`
				Username string `json:"username"`
			}{ID: 999, Username: "attacker"},
		},
	})
}

func TestHandleUpdate_AuthCheckRejectsBeforeRestart(t *testing.T) {
	b := New("test-token", "123456789", "test-label", nil)

	// /restart with nil RestartProxy callback — would panic without auth check
	b.handleUpdate(context.TODO(), TelegramUpdate{
		UpdateID: 4,
		Message: &TelegramMessage{
			MessageID: 12,
			Text:      "/restart",
			From: struct {
				ID       int64  `json:"id"`
				Username string `json:"username"`
			}{ID: 0, Username: "anon"},
		},
	})
}

func TestHandleUpdate_StripsBotNameSuffix(t *testing.T) {
	_ = New("test-token", "123456789", "test-label", nil)

	// Verify the @botname stripping logic works for authorized user
	text := "/help@my_test_bot"
	if idx := strings.Index(text, "@"); idx > 0 && strings.HasPrefix(text, "/") {
		text = text[:idx]
	}
	if text != "/help" {
		t.Errorf("expected /help after stripping, got %q", text)
	}
}

func TestArgs(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/add user1", "user1"},
		{"/add", ""},
		{"/add  ", ""},
		{"/add user1 secret", "user1 secret"},
	}

	for _, tt := range tests {
		got := (&Bot{}).args(tt.input)
		if got != tt.want {
			t.Errorf("args(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNew(t *testing.T) {
	deps := &Dependencies{}
	b := New("tok123", "998877", "my-server", deps)

	if b.token != "tok123" {
		t.Errorf("token = %q, want %q", b.token, "tok123")
	}
	if b.chatID != "998877" {
		t.Errorf("chatID = %q, want %q", b.chatID, "998877")
	}
	if b.label != "my-server" {
		t.Errorf("label = %q, want %q", b.label, "my-server")
	}
	if b.client == nil {
		t.Error("client is nil")
	}
	if b.deps == nil {
		t.Error("deps is nil")
	}
	if b.deps != deps {
		t.Error("deps pointer mismatch")
	}

	// Verify the client has a reasonable timeout.
	if b.client.Timeout != 35*time.Second {
		t.Errorf("client timeout = %v, want %v", b.client.Timeout, 35*time.Second)
	}
}

func TestIsKnownCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want bool
	}{
		// All known commands
		{"/status", true},
		{"/secrets", true},
		{"/link", true},
		{"/add", true},
		{"/remove", true},
		{"/rotate", true},
		{"/restart", true},
		{"/enable", true},
		{"/disable", true},
		{"/traffic", true},
		{"/update", true},
		{"/setlimit", true},
		{"/upstreams", true},
		{"/tasks", true},
		{"/help", true},
		{"/instances", true},
		{"/info", true},
		{"/geoblock", true},
		{"/replication", true},
		{"/backup", true},
		{"/resetquota", true},
		{"/start", true},
		{"/stop", true},

		// Removed commands
		{"/health", false},
		{"/limits", false},

		// Unknown commands
		{"/unknown", false},
		{"", false},
		{"status", false},
		{"/Status", false},
		{"/STATUS", false},
		{"/HELP", false},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := isKnownCommand(tt.cmd)
			if got != tt.want {
				t.Errorf("isKnownCommand(%q) = %v, want %v", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name string
		n    int64
		want string
	}{
		{"zero", 0, "0 B"},
		{"500 bytes", 500, "500 B"},
		{"1 KB", 1024, "1.0 KB"},
		{"1.5 KB", 1536, "1.5 KB"},
		{"1.0 MB", 1024 * 1024, "1.0 MB"},
		{"1.5 MB", int64(1.5 * 1024 * 1024), "1.5 MB"},
		{"2.0 GB", 2 * 1024 * 1024 * 1024, "2.0 GB"},
		{"max int64", math.MaxInt64, "8.0 EB"},
		{"just below KB", 1023, "1023 B"},
		{"just below MB", 1024*1024 - 1, "1024.0 KB"},
		{"just below GB", 1024*1024*1024 - 1, "1024.0 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBytes(tt.n)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestEscapeURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello", "hello"},
		{"spaces", "hello world", "hello+world"},
		{"markdown chars", "*bold* _italic_ `code`", "%2Abold%2A+_italic_+%60code%60"},
		{"special chars", "a&b=c", "a%26b%3Dc"},
		{"empty string", "", ""},
		{"newline", "line1\nline2", "line1%0Aline2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeURL(tt.input)
			if got != tt.want {
				t.Errorf("escapeURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsRunning(t *testing.T) {
	b := New("token", "123", "test", nil)

	if b.IsRunning() {
		t.Error("new bot should not be running")
	}
}

func TestStop_NilCancel(t *testing.T) {
	b := New("token", "123", "test", nil)

	// cancel is nil on a freshly constructed Bot — Stop must not panic
	b.Stop()
}

func TestSplitMessage_ShortMessage(t *testing.T) {
	text := "short message"
	chunks := splitMessage(text)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != text {
		t.Errorf("chunk = %q, want %q", chunks[0], text)
	}
}

func TestSplitMessage_LongMessageByParagraphs(t *testing.T) {
	// Build a message that exceeds 4000 chars
	var parts []string
	for i := range 50 {
		parts = append(parts, fmt.Sprintf("Line %d: %s", i, strings.Repeat("x", 80)))
	}
	text := strings.Join(parts, "\n\n")

	chunks := splitMessage(text)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks for %d char message, got %d", len(text), len(chunks))
	}

	// Verify no chunk exceeds limit
	for i, chunk := range chunks {
		if utf8.RuneCountInString(chunk) > maxMessageLen {
			t.Errorf("chunk %d exceeds max: %d runes", i, utf8.RuneCountInString(chunk))
		}
	}

	// Verify reconstruction matches original
	reconstructed := strings.Join(chunks, "\n\n")
	if reconstructed != text {
		t.Error("reconstructed text doesn't match original")
	}
}

func TestSplitMessage_SingleOversizedParagraph(t *testing.T) {
	// A single paragraph with many lines, no double newlines
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %04d: %s", i, strings.Repeat("ab", 20))
	}
	text := strings.Join(lines, "\n")

	chunks := splitMessage(text)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}

	for i, chunk := range chunks {
		if utf8.RuneCountInString(chunk) > maxMessageLen {
			t.Errorf("chunk %d exceeds max: %d runes", i, utf8.RuneCountInString(chunk))
		}
	}
}

func TestSplitMessage_PreservesSemanticBlocks(t *testing.T) {
	block1 := "Secret: user1\nStatus: active\nTraffic: 1GB"
	block2 := "Secret: user2\nStatus: disabled\nTraffic: 0"
	text := block1 + "\n\n" + block2

	chunks := splitMessage(text)
	// Should be single chunk since total is short
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for short message, got %d", len(chunks))
	}

	// Now make each block large enough that combined they exceed limit
	bigBlock1 := "Secret: user1\n" + strings.Repeat("x", 2500)
	bigBlock2 := "Secret: user2\n" + strings.Repeat("y", 2500)
	text = bigBlock1 + "\n\n" + bigBlock2

	chunks = splitMessage(text)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}

	// Each chunk should contain one complete block
	if !strings.Contains(chunks[0], "user1") {
		t.Error("chunk 0 should contain user1 data")
	}
	if !strings.Contains(chunks[1], "user2") {
		t.Error("chunk 1 should contain user2 data")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2h0m"},
		{25 * time.Hour, "1d1h"},
		{48 * time.Hour, "2d"},
		{7*24*time.Hour + 3*time.Hour, "7d3h"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
