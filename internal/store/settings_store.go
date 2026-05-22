package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
)

// SettingsStore handles settings persistence.
type SettingsStore struct {
	db *sql.DB
}

// NewSettingsStore creates a new SettingsStore.
func NewSettingsStore(db *sql.DB) *SettingsStore {
	return &SettingsStore{db: db}
}

// Load reads all settings from the database into a Settings struct.
func (s *SettingsStore) Load(ctx context.Context) (*model.Settings, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT key, value FROM settings")
	if err != nil {
		return nil, fmt.Errorf("query settings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	kv := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan setting: %w", err)
		}
		kv[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate settings: %w", err)
	}

	settings := model.DefaultSettings()
	// Proxy
	settings.ProxyPort = getInt(kv, "proxy_port", 443)
	settings.ProxyMetricsPort = getInt(kv, "proxy_metrics_port", 9090)
	settings.ProxyDomain = getString(kv, "proxy_domain", "cloudflare.com")
	settings.ProxyConcurrency = getInt(kv, "proxy_concurrency", 8192)
	settings.ProxyCPUs = getString(kv, "proxy_cpus", "")
	settings.ProxyMemory = getString(kv, "proxy_memory", "")
	settings.CustomIP = getString(kv, "custom_ip", "")
	settings.FakeCertLen = getInt(kv, "fake_cert_len", 2048)
	settings.ProxyProtocol = getBool(kv, "proxy_protocol")
	settings.ProxyProtocolTrustedCIDRs = getString(kv, "proxy_protocol_trusted_cidrs", "")
	// Ad tag
	settings.AdTag = getString(kv, "ad_tag", "")
	// Geo-blocking
	settings.GeoblockMode = getString(kv, "geoblock_mode", "blacklist")
	settings.BlocklistCountries = getString(kv, "blocklist_countries", "")
	// Masking
	settings.MaskingEnabled = getBool(kv, "masking_enabled")
	settings.MaskingHost = getString(kv, "masking_host", "")
	settings.MaskingPort = getInt(kv, "masking_port", 443)
	settings.MaskingRelayMaxBytes = getInt64(kv, "masking_relay_max_bytes", 0)
	settings.UnknownSNIAction = getString(kv, "unknown_sni_action", "mask")
	settings.ProxySecretURL = getString(kv, "proxy_secret_url", "")
	settings.ProxyConfigV4URL = getString(kv, "proxy_config_v4_url", "")
	settings.ProxyConfigV6URL = getString(kv, "proxy_config_v6_url", "")
	// Telegram
	settings.TelegramEnabled = getBool(kv, "telegram_enabled")
	settings.TelegramBotToken = getString(kv, "telegram_bot_token", "")
	settings.TelegramChatID = getString(kv, "telegram_chat_id", "")
	settings.TelegramInterval = getInt(kv, "telegram_interval", 6)
	settings.TelegramAlertsEnabled = getBool(kv, "telegram_alerts_enabled")
	settings.TelegramServerLabel = getString(kv, "telegram_server_label", "PopuGate")
	settings.WebURL = getString(kv, "web_url", "")
	// Auto-update
	settings.AutoUpdateEnabled = getBool(kv, "auto_update_enabled")
	settings.SecretAutoRotateDays = getInt(kv, "secret_auto_rotate_days", 0)
	settings.MaintenanceMode = getBool(kv, "maintenance_mode")
	// Replication
	settings.ReplicationEnabled = getBool(kv, "replication_enabled")
	settings.ReplicationRole = getString(kv, "replication_role", "standalone")
	settings.ReplicationSyncInterval = getInt(kv, "replication_sync_interval", 60)
	settings.ReplicationSSHPort = getInt(kv, "replication_ssh_port", 22)
	settings.ReplicationSSHUser = getString(kv, "replication_ssh_user", "root")
	settings.ReplicationDeleteExtra = getBool(kv, "replication_delete_extra")
	settings.ReplicationSSHKeyPath = getString(kv, "replication_ssh_key_path", "")
	settings.ReplicationExclude = getString(kv, "replication_exclude", "")
	settings.ReplicationRestartOnChange = getBool(kv, "replication_restart_on_change")
	settings.ReplicationLog = getString(kv, "replication_log", "")
	settings.Debug = getBool(kv, "debug")
	// Backup
	settings.BackupRetentionDays = getInt(kv, "backup_retention_days", 7)
	// telemt engine
	settings.TelemtVersion = getString(kv, "telemt_version", "")
	settings.TelemtCommit = getString(kv, "telemt_commit", "")
	settings.TelemtRepo = getString(kv, "telemt_repo", "")

	settings.Validate()
	return &settings, nil
}

// Save writes a partial set of settings to the database.
func (s *SettingsStore) Save(ctx context.Context, updates map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, "INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for key, value := range updates {
		if _, err := stmt.ExecContext(ctx, key, value); err != nil {
			return fmt.Errorf("save setting %s: %w", key, err)
		}
	}

	return tx.Commit()
}

// Get reads a single setting value.
func (s *SettingsStore) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// GetAuthPasswordHash returns the stored bcrypt password hash.
func (s *SettingsStore) GetAuthPasswordHash(ctx context.Context) (string, error) {
	return s.Get(ctx, "auth_password_hash")
}

// SetAuthPasswordHash stores the bcrypt password hash.
func (s *SettingsStore) SetAuthPasswordHash(ctx context.Context, hash string) error {
	return s.Save(ctx, map[string]string{"auth_password_hash": hash})
}

// GetJWTSecret returns the JWT signing secret, generating one if empty.
func (s *SettingsStore) GetJWTSecret(ctx context.Context) (string, error) {
	secret, err := s.Get(ctx, "jwt_secret")
	if err != nil {
		return "", err
	}
	if secret != "" {
		return secret, nil
	}
	// Generate a new secret and store it
	secret, err = generateRandomHex(32)
	if err != nil {
		return "", err
	}
	if err := s.Save(ctx, map[string]string{"jwt_secret": secret}); err != nil {
		return "", err
	}
	return secret, nil
}

// Helper functions

func getInt(m map[string]string, key string, def int) int {
	v, ok := m[key]
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getInt64(m map[string]string, key string, def int64) int64 {
	v, ok := m[key]
	if !ok || v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func getString(m map[string]string, key, def string) string {
	v, ok := m[key]
	if !ok {
		return def
	}
	return v
}

func getBool(m map[string]string, key string) bool {
	v, ok := m[key]
	return ok && v == "true"
}

func generateRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func parseExpiry(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
		return time.Unix(n, 0), nil
	}
	return time.Time{}, fmt.Errorf("cannot parse expiry: %s", s)
}
