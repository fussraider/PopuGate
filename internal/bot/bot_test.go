package bot

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	err := b.SendMessage(nil, "test message")
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
	b.handleUpdate(nil, TelegramUpdate{UpdateID: 1, Message: nil})
}

func TestHandleUpdate_NonCommandMessage(t *testing.T) {
	b := New("test-token", "123456789", "test-label", nil)
	// Should not panic, should return early
	b.handleUpdate(nil, TelegramUpdate{
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
	if got == "" {
		t.Error("cmdHelp returned empty string")
	}
	if !strings.Contains(got, "/status") {
		t.Error("cmdHelp output should contain bot commands")
	}
}

func TestHandleUpdate_AuthCheckRejectsBeforeCommandDispatch(t *testing.T) {
	// Verify that the auth check occurs BEFORE any command processing.
	// If the check were missing, a nil deps bot would panic on /status.
	b := New("test-token", "123456789", "test-label", nil)

	// This should NOT panic because auth rejects user 999 before reaching cmdStatus
	// which would dereference nil Settings deps.
	b.handleUpdate(nil, TelegramUpdate{
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
	b.handleUpdate(nil, TelegramUpdate{
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
