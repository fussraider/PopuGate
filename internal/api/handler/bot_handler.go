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
	BotToken string `json:"token" binding:"required,min=10"`
	ChatID   string `json:"chat_id" binding:"required,numeric"`
	Interval int    `json:"interval" binding:"required,min=1"`
	Label    string `json:"label" binding:"required,max=64"`
}

// Setup handles POST /api/v1/bot/setup
// @Summary      Setup Telegram bot
// @Description  Configures and starts the Telegram notification bot with the provided token, chat ID, interval, and label
// @Tags         bot
// @Accept       json
// @Produce      json
// @Param        body  body  object{token=string,chat_id=string,interval=int,label=string}  true  "Bot configuration"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /bot/setup [post]
func (h *BotHandler) Setup(c *gin.Context) {
	var req botSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	// Stop existing bot if running
	h.mu.Lock()
	if h.bot != nil && h.bot.IsRunning() {
		h.bot.Stop()
	}
	h.mu.Unlock()

	if err := h.settings.Save(c.Request.Context(), map[string]string{
		"telegram_bot_token":    req.BotToken,
		"telegram_chat_id":      req.ChatID,
		"telegram_enabled":      "true",
		"telegram_interval":     fmt.Sprintf("%d", req.Interval),
		"telegram_server_label": req.Label,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Start the bot with new config
	h.startBot(c.Request.Context(), req.BotToken, req.ChatID)

	auditLog(c, "bot.setup", "bot configured")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Test handles POST /api/v1/bot/test
// @Summary      Test bot notification
// @Description  Sends a test message via the configured Telegram bot
// @Tags         bot
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /bot/test [post]
func (h *BotHandler) Test(c *gin.Context) {
	settings, _ := h.settings.Load(c.Request.Context())
	if settings.TelegramBotToken == "" || settings.TelegramChatID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bot token and chat_id must be configured first"})
		return
	}

	testBot := bot.New(settings.TelegramBotToken, settings.TelegramChatID, settings.TelegramServerLabel, nil)
	msg := fmt.Sprintf("🧪 Test message from %s %s", settings.TelegramServerLabel, model.Version)
	if err := testBot.SendMessage(c.Request.Context(), msg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "test message sent"})
}

// Status handles GET /api/v1/bot/status
// @Summary      Bot status
// @Description  Returns the current Telegram bot status including enabled state, running state, and configuration
// @Tags         bot
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Security     BearerAuth
// @Router       /bot/status [get]
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
// @Summary      Toggle bot on/off
// @Description  Enables or disables the Telegram notification bot
// @Tags         bot
// @Accept       json
// @Produce      json
// @Param        body  body  object{enable=bool}  true  "Enable flag"
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /bot/toggle [put]
func (h *BotHandler) Toggle(c *gin.Context) {
	var req botToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		HandleBindError(c, err)
		return
	}

	val := "false"
	if *req.Enabled {
		val = "true"
	}
	if err := h.settings.Save(c.Request.Context(), map[string]string{"telegram_enabled": val}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	if *req.Enabled {
		settings, _ := h.settings.Load(c.Request.Context())
		if settings.TelegramBotToken != "" && settings.TelegramChatID != "" {
			h.startBot(c.Request.Context(), settings.TelegramBotToken, settings.TelegramChatID)
		}
	} else {
		h.mu.Lock()
		if h.bot != nil {
			h.bot.Stop()
			h.bot = nil
		}
		h.mu.Unlock()
	}

	auditLog(c, "bot.toggle", fmt.Sprintf("enabled=%v", *req.Enabled))
	c.JSON(http.StatusOK, gin.H{"ok": true, "enabled": req.Enabled})
}

// DetectChatID handles GET /api/v1/bot/detect-chat-id
// @Summary      Detect Telegram chat ID
// @Description  Queries Telegram API for recent messages to detect and auto-save the chat ID
// @Tags         bot
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Security     BearerAuth
// @Router       /bot/detect-chat-id [get]
func (h *BotHandler) DetectChatID(c *gin.Context) {
	settings, _ := h.settings.Load(c.Request.Context())
	if settings.TelegramBotToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bot token not configured"})
		return
	}

	chatID, err := detectChatID(c.Request.Context(), settings.TelegramBotToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Auto-save the detected chat ID
	if err := h.settings.Save(c.Request.Context(), map[string]string{
		"telegram_chat_id": chatID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

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
// The botToken is used for the API URL but is never included in error messages.
func detectChatID(ctx context.Context, botToken string) (string, error) {
	// Build URL without exposing token in error traces
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?limit=5", botToken)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("telegram getUpdates request failed")
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
