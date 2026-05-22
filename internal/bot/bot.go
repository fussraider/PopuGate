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
	"unicode/utf8"

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
	Slaves    *store.SlaveStore
	Backups   *store.BackupStore
	Geoblock  *store.GeoblockCacheStore

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
	CreateBackup      func(ctx context.Context) (store.Backup, error)
	ResetTraffic      func(ctx context.Context, label string) error
}

// Bot represents the Telegram bot.
type Bot struct {
	token    string
	chatID   string
	label    string
	client   *http.Client
	running  atomic.Bool
	cancel   context.CancelFunc
	deps     *Dependencies
	dispatch map[string]commandEntry
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
		// Management
		{Command: "status", Description: "Proxy status, engine, uptime"},
		{Command: "instances", Description: "List instances with config"},
		{Command: "restart", Description: "Restart proxy"},
		{Command: "start", Description: "Start instance by label"},
		{Command: "stop", Description: "Stop instance"},
		// Secrets
		{Command: "secrets", Description: "List secrets with limits"},
		{Command: "info", Description: "Detailed secret card"},
		{Command: "link", Description: "Proxy links + QR"},
		{Command: "add", Description: "Add secret"},
		{Command: "remove", Description: "Remove secret"},
		{Command: "rotate", Description: "Rotate secret key"},
		{Command: "enable", Description: "Enable secret"},
		{Command: "disable", Description: "Disable secret"},
		{Command: "setlimit", Description: "Set limits (conns, IPs, quota, expiry)"},
		{Command: "resetquota", Description: "Reset user traffic counters"},
		// Traffic
		{Command: "traffic", Description: "Traffic report"},
		// System
		{Command: "geoblock", Description: "Geo-blocking status"},
		{Command: "replication", Description: "Replication status"},
		{Command: "backup", Description: "Backup status & create"},
		{Command: "upstreams", Description: "List upstreams"},
		{Command: "tasks", Description: "Scheduled tasks status"},
		{Command: "update", Description: "Version info"},
		{Command: "help", Description: "Bot description & command list"},
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
	defer func() { _ = resp.Body.Close() }()

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
	bot := &Bot{
		token:  token,
		chatID: chatID,
		label:  label,
		client: &http.Client{Timeout: 35 * time.Second},
		deps:   deps,
	}
	bot.dispatch = bot.buildCommandDispatch()
	return bot
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
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram API returned %d", resp.StatusCode)
	}
	return nil
}

// InlineKeyboardButton is a Telegram Bot API InlineKeyboardButton.
type InlineKeyboardButton struct {
	Text string `json:"text"`
	URL  string `json:"url,omitempty"`
}

// InlineKeyboardMarkup is a Telegram Bot API reply markup for inline keyboards.
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// cmdResp is a bot command response with optional inline keyboard buttons.
type cmdResp struct {
	text string
	kb   [][]InlineKeyboardButton
}

func reply(text string) cmdResp { return cmdResp{text: text} }

func replyKB(text string, kb [][]InlineKeyboardButton) cmdResp {
	return cmdResp{text: text, kb: kb}
}

// SendMessageWithKeyboard sends a Markdown message with an inline keyboard.
func (b *Bot) SendMessageWithKeyboard(ctx context.Context, text string, keyboard InlineKeyboardMarkup) error {
	apiURL := b.telegramAPIURL("sendMessage")
	payload := struct {
		ChatID      string               `json:"chat_id"`
		Text        string               `json:"text"`
		ParseMode   string               `json:"parse_mode"`
		ReplyMarkup InlineKeyboardMarkup `json:"reply_markup"`
	}{
		ChatID:      b.chatID,
		Text:        text,
		ParseMode:   "Markdown",
		ReplyMarkup: keyboard,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

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
	_ = w.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

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
	defer func() { _ = resp.Body.Close() }()

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

type commandHandler func(ctx context.Context, text string) cmdResp

type commandEntry struct {
	exact   bool
	handler commandHandler
}

func (b *Bot) buildCommandDispatch() map[string]commandEntry {
	return map[string]commandEntry{
		"/status":      {exact: true, handler: func(ctx context.Context, _ string) cmdResp { return b.cmdStatus(ctx) }},
		"/instances":   {exact: true, handler: func(ctx context.Context, _ string) cmdResp { return b.cmdInstances(ctx) }},
		"/secrets":     {exact: true, handler: func(ctx context.Context, _ string) cmdResp { return b.cmdSecrets(ctx) }},
		"/info":        {exact: false, handler: b.cmdInfo},
		"/link":        {exact: false, handler: b.cmdLink},
		"/add":         {exact: false, handler: b.cmdAdd},
		"/remove":      {exact: false, handler: b.cmdRemove},
		"/rotate":      {exact: false, handler: b.cmdRotate},
		"/restart":     {exact: true, handler: func(ctx context.Context, _ string) cmdResp { return b.cmdRestart(ctx) }},
		"/enable":      {exact: false, handler: b.cmdEnable},
		"/disable":     {exact: false, handler: b.cmdDisable},
		"/start":       {exact: false, handler: b.dispatchStart},
		"/stop":        {exact: false, handler: b.cmdStopInstance},
		"/traffic":     {exact: true, handler: func(ctx context.Context, _ string) cmdResp { return b.cmdTraffic(ctx) }},
		"/setlimit":    {exact: false, handler: b.cmdSetLimit},
		"/resetquota":  {exact: false, handler: b.cmdResetQuota},
		"/geoblock":    {exact: true, handler: func(ctx context.Context, _ string) cmdResp { return b.cmdGeoblock(ctx) }},
		"/replication": {exact: true, handler: func(ctx context.Context, _ string) cmdResp { return b.cmdReplication(ctx) }},
		"/backup":      {exact: false, handler: b.cmdBackup},
		"/upstreams":   {exact: true, handler: func(ctx context.Context, _ string) cmdResp { return b.cmdUpstreams(ctx) }},
		"/tasks":       {exact: true, handler: func(ctx context.Context, _ string) cmdResp { return b.cmdTasks(ctx) }},
		"/update":      {exact: true, handler: func(ctx context.Context, _ string) cmdResp { return b.cmdUpdate(ctx) }},
		"/help":        {exact: true, handler: func(_ context.Context, _ string) cmdResp { return b.cmdHelp() }},
	}
}

func (b *Bot) dispatchStart(ctx context.Context, text string) cmdResp {
	if text == "/start" {
		return b.cmdWelcome()
	}
	return b.cmdStartInstance(ctx, text)
}

func (b *Bot) resolveCommand(text string) (commandHandler, string) {
	for cmd, entry := range b.dispatch {
		if entry.exact {
			if text == cmd {
				return entry.handler, cmd
			}
			continue
		}
		if text == cmd || strings.HasPrefix(text, cmd+" ") || strings.HasPrefix(text, cmd+"@") {
			return entry.handler, cmd
		}
	}
	return nil, ""
}

func (b *Bot) handleUpdate(ctx context.Context, update TelegramUpdate) {
	if update.Message == nil {
		return
	}

	if fmt.Sprintf("%d", update.Message.From.ID) != b.chatID {
		log.Debugf("unauthorized command from user %d (%s), expected chatID %s",
			update.Message.From.ID, update.Message.From.Username, b.chatID)
		return
	}

	text := strings.TrimSpace(update.Message.Text)

	if !strings.HasPrefix(text, "/") {
		return
	}

	// Strip @botname suffix before validation so /status@mybot is recognized
	if idx := strings.Index(text, "@"); idx > 0 {
		text = text[:idx]
	}

	cmd := strings.SplitN(text, " ", 2)[0]
	if !isKnownCommand(cmd) {
		return
	}

	log.Debugf("command from %s: %s", update.Message.From.Username, text)

	handler, _ := b.resolveCommand(text)
	var resp cmdResp
	if handler != nil {
		resp = handler(ctx, text)
	} else {
		resp = reply("Unknown command. Send /help for available commands.")
	}

	if resp.text != "" {
		b.sendChunked(ctx, resp.text, resp.kb)
	}
}

// --- Command helpers ---

const maxMessageLen = 4000

// splitMessage splits text into chunks of at most maxMessageLen Unicode characters.
// It splits only on paragraph boundaries (double newlines) to preserve semantic blocks.
// If a single block exceeds maxMessageLen, it is further split on single newlines.
func splitMessage(text string) []string {
	if utf8.RuneCountInString(text) <= maxMessageLen {
		return []string{text}
	}

	// Split into paragraphs by double newline
	paragraphs := strings.Split(text, "\n\n")
	var chunks []string
	var current strings.Builder

	for _, p := range paragraphs {
		pRunes := utf8.RuneCountInString(p)
		curRunes := utf8.RuneCountInString(current.String())

		// Single paragraph too large — split by lines
		if pRunes > maxMessageLen {
			if current.Len() > 0 {
				chunks = append(chunks, current.String())
				current.Reset()
			}
			lines := strings.Split(p, "\n")
			for _, line := range lines {
				lineRunes := utf8.RuneCountInString(line)
				curRunes = utf8.RuneCountInString(current.String())
				if curRunes > 0 && curRunes+1+lineRunes > maxMessageLen {
					chunks = append(chunks, current.String())
					current.Reset()
				}
				if current.Len() > 0 {
					current.WriteByte('\n')
				}
				current.WriteString(line)
			}
			continue
		}

		if curRunes > 0 && curRunes+2+pRunes > maxMessageLen {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(p)
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

// sendChunked splits a message into chunks if needed and sends them.
// Keyboard buttons are attached only to the last chunk.
func (b *Bot) sendChunked(ctx context.Context, text string, kb [][]InlineKeyboardButton) {
	chunks := splitMessage(text)
	for i, chunk := range chunks {
		if i == len(chunks)-1 && len(kb) > 0 {
			if err := b.SendMessageWithKeyboard(ctx, chunk, InlineKeyboardMarkup{InlineKeyboard: kb}); err != nil {
				log.Errorf("send error (chunk %d): %v", i, err)
			}
		} else {
			if err := b.SendMessage(ctx, chunk); err != nil {
				log.Errorf("send error (chunk %d): %v", i, err)
			}
		}
	}
}

// isKnownCommand checks if the command is one of ours.
func isKnownCommand(cmd string) bool {
	switch cmd {
	case "/status", "/secrets", "/link", "/add", "/remove", "/rotate",
		"/restart", "/enable", "/disable", "/start", "/stop", "/traffic",
		"/update", "/setlimit", "/upstreams", "/tasks", "/help",
		"/instances", "/info", "/geoblock", "/replication", "/backup", "/resetquota":
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

// webURL returns the configured web UI base URL, or empty string.
func (b *Bot) webURL(ctx context.Context) string {
	s, _ := b.deps.Settings.Load(ctx)
	if s == nil {
		return ""
	}
	return s.WebURL
}

// dashboardKB returns a single-row keyboard with a Dashboard button, or nil.
func (b *Bot) dashboardKB(ctx context.Context) [][]InlineKeyboardButton {
	wu := b.webURL(ctx)
	if wu == "" {
		return nil
	}
	return [][]InlineKeyboardButton{{{Text: "Dashboard", URL: wu}}}
}

// pageKB returns a single-row keyboard with a named button linking to a web UI path.
func (b *Bot) pageKB(ctx context.Context, label, path string) [][]InlineKeyboardButton {
	wu := b.webURL(ctx)
	if wu == "" {
		return nil
	}
	return [][]InlineKeyboardButton{{{Text: label, URL: wu + path}}}
}

func escapeURL(s string) string {
	return url.QueryEscape(s)
}
