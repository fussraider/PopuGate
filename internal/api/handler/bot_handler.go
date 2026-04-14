package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/bot"
	"github.com/fussraider/PopuGate/internal/store"
)

// BotHandler handles Telegram bot endpoints.
type BotHandler struct {
	settings *store.SettingsStore
	deps     *bot.Dependencies

	mu  sync.Mutex
	bot *bot.Bot
}

// NewBotHandler creates a new BotHandler.
func NewBotHandler(settings *store.SettingsStore, deps *bot.Dependencies) *BotHandler {
	return &BotHandler{settings: settings, deps: deps}
}

type botSetupRequest struct {
	BotToken string `json:"token" binding:"required"`
	ChatID   string `json:"chat_id" binding:"required"`
	Interval int    `json:"interval" binding:"required"`
	Label    string `json:"label" binding:"required"`
}

// Setup handles POST /api/v1/bot/setup
func (h *BotHandler) Setup(c *gin.Context) {
	var req botSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Stop existing bot if running
	h.mu.Lock()
	if h.bot != nil && h.bot.IsRunning() {
		h.bot.Stop()
	}
	h.mu.Unlock()

	_ = h.settings.Save(c.Request.Context(), map[string]string{
		"telegram_bot_token":    req.BotToken,
		"telegram_chat_id":      req.ChatID,
		"telegram_enabled":      "true",
		"telegram_interval":     fmt.Sprintf("%d", req.Interval),
		"telegram_server_label": req.Label,
	})

	// Start the bot with new config
	h.startBot(c.Request.Context(), req.BotToken, req.ChatID)

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Test handles POST /api/v1/bot/test
func (h *BotHandler) Test(c *gin.Context) {
	settings, _ := h.settings.Load(c.Request.Context())
	if settings.TelegramBotToken == "" || settings.TelegramChatID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bot token and chat_id must be configured first"})
		return
	}

	testBot := bot.New(settings.TelegramBotToken, settings.TelegramChatID, settings.TelegramServerLabel, nil)
	msg := fmt.Sprintf("🧪 Test message from %s v%s", settings.TelegramServerLabel, model.Version)
	if err := testBot.SendMessage(c.Request.Context(), msg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to send: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "test message sent"})
}

// Status handles GET /api/v1/bot/status
func (h *BotHandler) Status(c *gin.Context) {
	settings, _ := h.settings.Load(c.Request.Context())

	h.mu.Lock()
	running := h.bot != nil && h.bot.IsRunning()
	h.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"enabled":        settings.TelegramEnabled,
		"running":        running,
		"interval_hours": settings.TelegramInterval,
		"server_label":   settings.TelegramServerLabel,
	})
}

type botToggleRequest struct {
	Enabled *bool `json:"enable" binding:"required"`
}

// Toggle handles PUT /api/v1/bot/toggle
func (h *BotHandler) Toggle(c *gin.Context) {
	var req botToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	val := "false"
	if *req.Enabled {
		val = "true"
	}
	_ = h.settings.Save(c.Request.Context(), map[string]string{"telegram_enabled": val})

	if *req.Enabled {
		settings, _ := h.settings.Load(c.Request.Context())
		if settings.TelegramBotToken != "" && settings.TelegramChatID != "" {
			h.startBot(c.Request.Context(), settings.TelegramBotToken, settings.TelegramChatID)
		}
	} else {
		h.mu.Lock()
		if h.bot != nil {
			h.bot.Stop()
		}
		h.mu.Unlock()
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "enabled": req.Enabled})
}

// DetectChatID handles GET /api/v1/bot/detect-chat-id
func (h *BotHandler) DetectChatID(c *gin.Context) {
	settings, _ := h.settings.Load(c.Request.Context())
	if settings.TelegramBotToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bot token not configured"})
		return
	}

	chatID, err := detectChatID(c.Request.Context(), settings.TelegramBotToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("detection failed: %v. Send any message to the bot first, then retry.", err)})
		return
	}

	// Auto-save the detected chat ID
	_ = h.settings.Save(c.Request.Context(), map[string]string{
		"telegram_chat_id": chatID,
	})

	c.JSON(http.StatusOK, gin.H{"ok": true, "chat_id": chatID})
}

func (h *BotHandler) startBot(ctx context.Context, token, chatID string) {
	settings, _ := h.settings.Load(ctx)

	h.mu.Lock()
	if h.bot != nil && h.bot.IsRunning() {
		h.bot.Stop()
	}
	h.bot = bot.New(token, chatID, settings.TelegramServerLabel, h.deps)
	h.mu.Unlock()

	go h.bot.Start(context.Background())
}

// detectChatID queries Telegram getUpdates to find the most recent chat_id.
func detectChatID(ctx context.Context, botToken string) (string, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?limit=5", botToken)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool `json:"ok"`
		Result []struct {
			Message *struct {
				Chat struct {
					ID int64 `json:"id"`
				} `json:"chat"`
			} `json:"message"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if !result.OK || len(result.Result) == 0 {
		return "", fmt.Errorf("no messages found — send a message to the bot first")
	}

	// Get the last message's chat ID
	for i := len(result.Result) - 1; i >= 0; i-- {
		if result.Result[i].Message != nil {
			return fmt.Sprintf("%d", result.Result[i].Message.Chat.ID), nil
		}
	}

	return "", fmt.Errorf("no messages with chat_id found")
}
