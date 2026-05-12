package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/fmtutil"
	"github.com/fussraider/PopuGate/pkg/logger"
)

var log = logger.WithScope("bot")

// Dependencies holds the stores/services the bot needs for command execution.
type Dependencies struct {
	Settings  *store.SettingsStore
	Secrets   *store.SecretStore
	Upstreams *store.UpstreamStore
	Traffic   *store.TrafficStore
	Instances *store.InstanceStore

	// Callbacks for actions that need service layer (set by caller)
	GetPublicIP       func(ctx context.Context) string
	IsProxyRunning    func(ctx context.Context) bool
	IsInstanceRunning func(ctx context.Context, containerName string) bool
	GetUptime         func(ctx context.Context) string
	GetEngineVersion  func() string
	RestartProxy      func(ctx context.Context) error
	StartInstance     func(ctx context.Context, id int64) error
	StopInstance      func(ctx context.Context, id int64) error
	GenerateQR        func(ctx context.Context, link string) ([]byte, error)
	GetSchedulerTasks func(ctx context.Context) []string
}

// Bot represents the Telegram bot.
type Bot struct {
	token   string
	chatID  string
	label   string
	client  *http.Client
	running atomic.Bool
	cancel  context.CancelFunc
	deps    *Dependencies
}

// telegramAPIURL builds a Telegram Bot API URL with the bot token.
// The URL is safe for use in HTTP requests but must never be logged.
func (b *Bot) telegramAPIURL(method string) string {
	return fmt.Sprintf("https://api.telegram.org/bot%s/%s", b.token, method)
}

// telegramAPIURLWithQuery builds a Telegram Bot API URL with query parameters.
func (b *Bot) telegramAPIURLWithQuery(method, query string) string {
	return fmt.Sprintf("https://api.telegram.org/bot%s/%s?%s", b.token, method, query)
}

// telegramCommand is a BotCommand for setMyCommands.
type telegramCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// defaultCommands returns the full list of bot commands for setMyCommands.
func defaultCommands() []telegramCommand {
	return []telegramCommand{
		{Command: "status", Description: "Proxy status"},
		{Command: "secrets", Description: "List secrets"},
		{Command: "link", Description: "Proxy links + QR"},
		{Command: "add", Description: "Add secret"},
		{Command: "remove", Description: "Remove secret"},
		{Command: "rotate", Description: "Rotate secret"},
		{Command: "restart", Description: "Restart proxy"},
		{Command: "start", Description: "Start instance"},
		{Command: "stop", Description: "Stop instance"},
		{Command: "enable", Description: "Enable secret"},
		{Command: "disable", Description: "Disable secret"},
		{Command: "health", Description: "Health check"},
		{Command: "traffic", Description: "Traffic report"},
		{Command: "update", Description: "Version info"},
		{Command: "limits", Description: "User limits"},
		{Command: "setlimit", Description: "Set limits"},
		{Command: "upstreams", Description: "List upstreams"},
		{Command: "tasks", Description: "Scheduled tasks"},
		{Command: "help", Description: "Show help"},
	}
}

// SetCommands registers bot commands via setMyCommands so they appear as
// autocomplete suggestions in Telegram clients.
func (b *Bot) SetCommands(ctx context.Context) error {
	return setCommandsForTokenWithClient(ctx, b.client, b.token)
}

// SetCommandsForToken registers bot commands using only a token (no Bot instance needed).
func SetCommandsForToken(ctx context.Context, token string) error {
	return setCommandsForTokenWithClient(ctx, &http.Client{Timeout: 10 * time.Second}, token)
}

func setCommandsForTokenWithClient(ctx context.Context, client *http.Client, token string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/setMyCommands", token)

	payload, _ := json.Marshal(map[string]any{"commands": defaultCommands()})
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("setMyCommands request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("setMyCommands call: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK bool `json:"ok"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("setMyCommands decode: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("setMyCommands returned ok=false (HTTP %d)", resp.StatusCode)
	}
	return nil
}

// New creates a new Telegram bot.
func New(token, chatID, label string, deps *Dependencies) *Bot {
	return &Bot{
		token:  token,
		chatID: chatID,
		label:  label,
		client: &http.Client{Timeout: 35 * time.Second},
		deps:   deps,
	}
}

// Start begins the long-polling loop with exponential backoff on errors.
func (b *Bot) Start(ctx context.Context) {
	ctx, b.cancel = context.WithCancel(ctx)
	b.running.Store(true)

	var offset int64
	backoff := time.Second
	const maxBackoff = 5 * time.Minute

	for {
		select {
		case <-ctx.Done():
			b.running.Store(false)
			return
		default:
		}

		updates, err := b.getUpdates(ctx, offset)
		if err != nil {
			// Exponential backoff with jitter
			jitter := time.Duration(rand.Int63n(int64(backoff) / 2))
			sleep := backoff + jitter
			log.Warnf("getUpdates error, retrying in %v: %v", sleep, err)

			select {
			case <-ctx.Done():
				b.running.Store(false)
				return
			case <-time.After(sleep):
			}

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Success — reset backoff
		backoff = time.Second

		for _, update := range updates {
			offset = update.UpdateID + 1
			b.handleUpdate(ctx, update)
		}

		time.Sleep(2 * time.Second)
	}
}

// Stop stops the bot.
func (b *Bot) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
}

// IsRunning returns whether the bot is active.
func (b *Bot) IsRunning() bool {
	return b.running.Load()
}

// SendMessage sends a Markdown message to the configured chat.
func (b *Bot) SendMessage(ctx context.Context, text string) error {
	apiURL := b.telegramAPIURL("sendMessage")
	body := fmt.Sprintf(`chat_id=%s&text=%s&parse_mode=Markdown`,
		b.chatID, escapeURL(text))

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram API returned %d", resp.StatusCode)
	}
	return nil
}

// SendPhoto sends a PNG photo with an optional caption to the configured chat.
func (b *Bot) SendPhoto(ctx context.Context, pngData []byte, caption string) error {
	apiURL := b.telegramAPIURL("sendPhoto")

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// chat_id field
	_ = w.WriteField("chat_id", b.chatID)
	if caption != "" {
		_ = w.WriteField("caption", caption)
		_ = w.WriteField("parse_mode", "Markdown")
	}

	// photo file field
	fw, err := w.CreateFormFile("photo", "qr.png")
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(fw, bytes.NewReader(pngData)); err != nil {
		return fmt.Errorf("copy photo data: %w", err)
	}
	w.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram sendPhoto returned %d", resp.StatusCode)
	}
	return nil
}

// TelegramUpdate represents an incoming update.
type TelegramUpdate struct {
	UpdateID int64            `json:"update_id"`
	Message  *TelegramMessage `json:"message,omitempty"`
}

// TelegramMessage represents a message from Telegram.
type TelegramMessage struct {
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
	From      struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"from"`
}

func (b *Bot) getUpdates(ctx context.Context, offset int64) ([]TelegramUpdate, error) {
	apiURL := b.telegramAPIURLWithQuery("getUpdates", fmt.Sprintf("offset=%d&timeout=30", offset))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool             `json:"ok"`
		Result []TelegramUpdate `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram getUpdates not ok")
	}
	return result.Result, nil
}

func (b *Bot) handleUpdate(ctx context.Context, update TelegramUpdate) {
	if update.Message == nil {
		return
	}

	// Only allow commands from the authorized chat user
	if fmt.Sprintf("%d", update.Message.From.ID) != b.chatID {
		log.Debugf("unauthorized command from user %d (%s), expected chatID %s",
			update.Message.From.ID, update.Message.From.Username, b.chatID)
		return
	}

	text := strings.TrimSpace(update.Message.Text)

	if !strings.HasPrefix(text, "/") {
		return
	}

	// Ignore commands from other bots/groups (e.g. /start, /help from general bots)
	cmd := strings.SplitN(text, " ", 2)[0]
	if !isKnownCommand(cmd) {
		return
	}

	log.Debugf("command from %s: %s", update.Message.From.Username, text)

	// Strip @botname suffix from commands
	if idx := strings.Index(text, "@"); idx > 0 && strings.HasPrefix(text, "/") {
		text = text[:idx]
	}

	var response string
	switch {
	case text == "/status":
		response = b.cmdStatus(ctx)
	case text == "/secrets":
		response = b.cmdSecrets(ctx)
	case strings.HasPrefix(text, "/link"):
		response = b.cmdLink(ctx, text)
	case strings.HasPrefix(text, "/add"):
		response = b.cmdAdd(ctx, text)
	case strings.HasPrefix(text, "/remove"):
		response = b.cmdRemove(ctx, text)
	case strings.HasPrefix(text, "/rotate"):
		response = b.cmdRotate(ctx, text)
	case text == "/restart":
		response = b.cmdRestart(ctx)
	case strings.HasPrefix(text, "/enable"):
		response = b.cmdEnable(ctx, text)
	case strings.HasPrefix(text, "/disable"):
		response = b.cmdDisable(ctx, text)
	case strings.HasPrefix(text, "/start"):
		response = b.cmdStartInstance(ctx, text)
	case strings.HasPrefix(text, "/stop"):
		response = b.cmdStopInstance(ctx, text)
	case text == "/health":
		response = b.cmdHealth(ctx)
	case text == "/traffic":
		response = b.cmdTraffic(ctx)
	case text == "/update":
		response = b.cmdUpdate(ctx)
	case text == "/limits":
		response = b.cmdLimits(ctx)
	case strings.HasPrefix(text, "/setlimit"):
		response = b.cmdSetLimit(ctx, text)
	case text == "/upstreams":
		response = b.cmdUpstreams(ctx)
	case text == "/tasks":
		response = b.cmdTasks(ctx)
	case text == "/help":
		response = b.cmdHelp()
	default:
		response = "Unknown command. Send /help for available commands."
	}

	if response != "" {
		if err := b.SendMessage(ctx, response); err != nil {
			log.Errorf("send error: %v", err)
		}
	}
}

// --- Command helpers ---

// isKnownCommand checks if the command is one of ours.
func isKnownCommand(cmd string) bool {
	switch cmd {
	case "/status", "/secrets", "/link", "/add", "/remove", "/rotate",
		"/restart", "/enable", "/disable", "/start", "/stop", "/health", "/traffic",
		"/update", "/limits", "/setlimit", "/upstreams", "/tasks", "/help":
		return true
	}
	return false
}

func (b *Bot) args(text string) string {
	parts := strings.SplitN(text, " ", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// FormatBytes re-exports fmtutil.FormatBytes for internal bot use with int64 inputs.
func formatBytes(n int64) string {
	if n < 0 {
		return "0 B"
	}
	return fmtutil.FormatBytes(uint64(n))
}

func escapeURL(s string) string {
	return url.QueryEscape(s)
}
