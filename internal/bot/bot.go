package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/fussraider/PopuGate/internal/store"
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
	GetPublicIP      func(ctx context.Context) string
	IsProxyRunning   func(ctx context.Context) bool
	GetUptime        func(ctx context.Context) string
	GetEngineVersion func() string
	RestartProxy     func(ctx context.Context) error
	GenerateQR       func(ctx context.Context, link string) ([]byte, error)
}

// Bot represents the Telegram bot.
type Bot struct {
	token   string
	chatID  string
	label   string
	client  *http.Client
	running bool
	cancel  context.CancelFunc
	deps    *Dependencies
}

// New creates a new Telegram bot.
func New(token, chatID, label string, deps *Dependencies) *Bot {
	return &Bot{
		token:  token,
		chatID: chatID,
		label:  label,
		client: &http.Client{Timeout: 15 * time.Second},
		deps:   deps,
	}
}

// Start begins the long-polling loop.
func (b *Bot) Start(ctx context.Context) {
	ctx, b.cancel = context.WithCancel(ctx)
	b.running = true

	var offset int64
	for {
		select {
		case <-ctx.Done():
			b.running = false
			return
		default:
		}

		updates, err := b.getUpdates(ctx, offset)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}

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
	b.running = false
}

// IsRunning returns whether the bot is active.
func (b *Bot) IsRunning() bool {
	return b.running
}

// SendMessage sends a Markdown message to the configured chat.
func (b *Bot) SendMessage(ctx context.Context, text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", b.token)
	body := fmt.Sprintf(`chat_id=%s&text=%s&parse_mode=Markdown`,
		b.chatID, escapeURL(text))

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(body))
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
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendPhoto", b.token)

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
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30",
		b.token, offset)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
	text := strings.TrimSpace(update.Message.Text)

	if !strings.HasPrefix(text, "/mp_") {
		return
	}

	log.Debugf("command from %s: %s", update.Message.From.Username, text)

	// Strip @botname suffix from commands
	if idx := strings.Index(text, "@"); idx > 0 && strings.HasPrefix(text, "/mp_") {
		text = text[:idx]
	}

	var response string
	switch {
	case text == "/mp_status":
		response = b.cmdStatus(ctx)
	case text == "/mp_secrets":
		response = b.cmdSecrets(ctx)
	case strings.HasPrefix(text, "/mp_link"):
		response = b.cmdLink(ctx, text)
	case strings.HasPrefix(text, "/mp_add"):
		response = b.cmdAdd(ctx, text)
	case strings.HasPrefix(text, "/mp_remove"):
		response = b.cmdRemove(ctx, text)
	case strings.HasPrefix(text, "/mp_rotate"):
		response = b.cmdRotate(ctx, text)
	case text == "/mp_restart":
		response = b.cmdRestart(ctx)
	case strings.HasPrefix(text, "/mp_enable"):
		response = b.cmdEnable(ctx, text)
	case strings.HasPrefix(text, "/mp_disable"):
		response = b.cmdDisable(ctx, text)
	case text == "/mp_health":
		response = b.cmdHealth(ctx)
	case text == "/mp_traffic":
		response = b.cmdTraffic(ctx)
	case text == "/mp_update":
		response = b.cmdUpdate(ctx)
	case text == "/mp_limits":
		response = b.cmdLimits(ctx)
	case strings.HasPrefix(text, "/mp_setlimit"):
		response = b.cmdSetLimit(ctx, text)
	case text == "/mp_upstreams":
		response = b.cmdUpstreams(ctx)
	case text == "/mp_help":
		response = b.cmdHelp()
	default:
		if strings.HasPrefix(text, "/mp_") {
			response = "Unknown command. Send /mp_help for available commands."
		}
	}

	if response != "" {
		if err := b.SendMessage(ctx, response); err != nil {
			log.Errorf("send error: %v", err)
		}
	}
}

// --- Command helpers ---

func (b *Bot) args(text string) string {
	parts := strings.SplitN(text, " ", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func formatBytes(n int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(GB))
	case n >= MB:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(MB))
	case n >= KB:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(KB))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func escapeURL(s string) string {
	s = strings.ReplaceAll(s, "\n", "%0A")
	s = strings.ReplaceAll(s, "_", "%5F")
	s = strings.ReplaceAll(s, "*", "%2A")
	s = strings.ReplaceAll(s, "[", "%5B")
	s = strings.ReplaceAll(s, "]", "%5D")
	return s
}
