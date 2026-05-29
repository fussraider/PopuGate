package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/fussraider/PopuGate/internal/bot"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
	"github.com/gin-gonic/gin"
)

func TestBotHandler_Status(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)

	var activeBot *bot.Bot
	var botMu sync.Mutex

	handler := NewBotHandler(settingsStore, nil, &activeBot, &botMu)

	r := gin.New()
	r.GET("/api/v1/bot/status", handler.Status)

	// 1. Test when activeBot is nil
	{
		req := httptest.NewRequest(http.MethodGet, "/api/v1/bot/status", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)

		if resp["running"] != false {
			t.Errorf("expected running to be false, got %v", resp["running"])
		}
		if resp["enabled"] != false {
			t.Errorf("expected enabled to be false, got %v", resp["enabled"])
		}
	}

	// 2. Test when activeBot is not nil and is running
	{
		ctx, cancel := context.WithCancel(context.Background())
		// Instantiate a real bot
		b := bot.New("1234567890:ABCdefGhIJKlmNoPQRsTUVwxyZ", "1234567", "test", nil)
		go b.Start(ctx)
		defer cancel()

		// Allow Go scheduler to execute b.Start and set atomic boolean to true
		time.Sleep(5 * time.Millisecond)

		botMu.Lock()
		activeBot = b
		botMu.Unlock()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/bot/status", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		var resp map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)

		if resp["running"] != true {
			t.Errorf("expected running to be true, got %v", resp["running"])
		}

		// Stop bot and clear activeBot
		cancel()
		b.Stop()
		botMu.Lock()
		activeBot = nil
		botMu.Unlock()
	}
}

func TestBotHandler_Toggle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)

	var activeBot *bot.Bot
	var botMu sync.Mutex

	handler := NewBotHandler(settingsStore, nil, &activeBot, &botMu)

	// Save configured bot details first
	_ = settingsStore.Save(context.Background(), map[string]string{
		"telegram_bot_token": "1234567890:ABCdefGhIJKlmNoPQRsTUVwxyZ",
		"telegram_chat_id":   "1234567",
	})

	r := gin.New()
	r.PUT("/api/v1/bot/toggle", handler.Toggle)

	// Toggle ON
	{
		body := `{"enable": true}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/bot/toggle", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		botMu.Lock()
		defer botMu.Unlock()
		if activeBot == nil {
			t.Error("expected activeBot to be initialized after toggle ON")
		} else {
			activeBot.Stop()
			activeBot = nil
		}
	}
}
