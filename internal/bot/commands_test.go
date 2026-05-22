package bot

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

// testEnv holds a fully wired bot with in-memory stores for command testing.
type testEnv struct {
	bot  *Bot
	db   *sql.DB
	deps *Dependencies
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	db := testutil.OpenTestDB(t)

	deps := &Dependencies{
		Settings:  store.NewSettingsStore(db),
		Secrets:   store.NewSecretStore(db),
		Upstreams: store.NewUpstreamStore(db),
		Traffic:   store.NewTrafficStore(db),
		Instances: store.NewInstanceStore(db),
		Slaves:    store.NewSlaveStore(db),
		Backups:   store.NewBackupStore(t.TempDir()),
		Geoblock:  store.NewGeoblockCacheStore(db),
	}

	b := New("test-token", "123456789", "test-server", deps)
	return &testEnv{bot: b, db: db, deps: deps}
}

// seedSecret inserts a secret directly into the DB for testing.
func seedSecret(t *testing.T, db *sql.DB, s *model.Secret) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO secrets (label, secret_key, enabled, max_conns, max_ips, quota_bytes, expires_at, notes, tags)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Label, s.SecretKey, boolToInt(s.Enabled), s.MaxConns, s.MaxIPs, s.QuotaBytes,
		s.ExpiresAt, s.Notes, s.Tags)
	if err != nil {
		t.Fatalf("seedSecret %s: %v", s.Label, err)
	}
}

// seedInstance inserts an instance directly into the DB.
func seedInstance(t *testing.T, db *sql.DB, inst *model.Instance) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO instances (port, metrics_port, enabled, label, tls_domain, tls_domains, fake_tls, mask_host, mask_port, tags, tcp_mss_enabled, tcp_mss, tls_fronting)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inst.Port, inst.MetricsPort, boolToInt(inst.Enabled), inst.Label, inst.TLSDomain,
		inst.TLSDomains, boolToInt(inst.FakeTLS), inst.MaskHost, inst.MaskPort,
		inst.Tags, boolToInt(inst.TCPMSSEnabled), inst.TCPMSS, boolToInt(inst.TLSFronting))
	if err != nil {
		t.Fatalf("seedInstance %s: %v", inst.Label, err)
	}
}

// seedUpstream inserts an upstream directly into the DB.
func seedUpstream(t *testing.T, db *sql.DB, name, addr string, enabled bool) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO upstreams (name, type, address, weight, enabled) VALUES (?, 'socks5', ?, 1, ?)`,
		name, addr, boolToInt(enabled))
	if err != nil {
		t.Fatalf("seedUpstream %s: %v", name, err)
	}
}

// seedSlave inserts a slave directly into the DB.
func seedSlave(t *testing.T, db *sql.DB, label, host string, port int, enabled bool) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO slaves (host, port, label, enabled, status) VALUES (?, ?, ?, ?, 'ok')`,
		host, port, label, boolToInt(enabled))
	if err != nil {
		t.Fatalf("seedSlave %s: %v", label, err)
	}
}

// seedGeoblockCache inserts a geoblock cache entry.
func seedGeoblockCache(t *testing.T, db *sql.DB, code, filePath string, downloadedAt int64) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT OR REPLACE INTO geoblock_cache (country_code, file_path, downloaded_at) VALUES (?, ?, ?)`,
		code, filePath, downloadedAt)
	if err != nil {
		t.Fatalf("seedGeoblockCache %s: %v", code, err)
	}
}

// updateSetting updates a single settings key-value pair.
func updateSetting(t *testing.T, db *sql.DB, key string, val any) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)`, key, val)
	if err != nil {
		t.Fatalf("updateSetting %s: %v", key, err)
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// --- Tests ---

func TestCmdWelcome(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdWelcome()
	if resp.text == "" {
		t.Fatal("cmdWelcome returned empty text")
	}
	if !strings.Contains(resp.text, "/status") {
		t.Error("cmdWelcome should show commands")
	}
}

func TestCmdStatus_NoInstances(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdStatus(context.Background())
	if !strings.Contains(resp.text, "Status") {
		t.Errorf("cmdStatus should contain 'Status', got: %s", resp.text)
	}
	if !strings.Contains(resp.text, "No instances configured") {
		t.Error("cmdStatus should show no instances message")
	}
}

func TestCmdStatus_WithRunningInstance(t *testing.T) {
	env := newTestEnv(t)
	seedInstance(t, env.db, &model.Instance{
		Port: 443, MetricsPort: 9093, Enabled: true, Label: "main",
		TLSDomain: "example.com", FakeTLS: true,
	})
	env.deps.IsInstanceRunning = func(_ context.Context, _ string) bool { return true }
	env.deps.GetEngineVersion = func() string { return "3.14" }
	env.deps.GetUptime = func(_ context.Context) string { return "2h30m" }

	resp := env.bot.cmdStatus(context.Background())
	if !strings.Contains(resp.text, "running") {
		t.Errorf("cmdStatus should show running instance, got: %s", resp.text)
	}
	if !strings.Contains(resp.text, "Engine") {
		t.Error("cmdStatus should show engine version")
	}
	if !strings.Contains(resp.text, "main") {
		t.Error("cmdStatus should show instance label")
	}
}

func TestCmdStatus_AllStopped(t *testing.T) {
	env := newTestEnv(t)
	seedInstance(t, env.db, &model.Instance{
		Port: 443, MetricsPort: 9093, Enabled: true, Label: "main",
		TLSDomain: "example.com",
	})
	env.deps.IsInstanceRunning = func(_ context.Context, _ string) bool { return false }

	resp := env.bot.cmdStatus(context.Background())
	if !strings.Contains(resp.text, "stopped") {
		t.Error("cmdStatus should show stopped instance")
	}
	if !strings.Contains(resp.text, "No instances are running") {
		t.Error("cmdStatus should warn about no running instances")
	}
}

func TestCmdSecrets_Empty(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdSecrets(context.Background())
	if !strings.Contains(resp.text, "No secrets") {
		t.Errorf("cmdSecrets empty should say 'No secrets', got: %s", resp.text)
	}
}

func TestCmdSecrets_WithSecrets(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{Label: "user1", SecretKey: "aa111111111111111111111111111111", Enabled: true})
	seedSecret(t, env.db, &model.Secret{Label: "user2", SecretKey: "bb222222222222222222222222222222", Enabled: false})

	resp := env.bot.cmdSecrets(context.Background())
	if !strings.Contains(resp.text, "Secrets (2)") {
		t.Errorf("should show 2 secrets, got: %s", resp.text)
	}
	if !strings.Contains(resp.text, "user1") || !strings.Contains(resp.text, "user2") {
		t.Error("should list both secrets")
	}
}

func TestCmdSecrets_WithLimits(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{
		Label: "limited", SecretKey: "aa111111111111111111111111111111", Enabled: true,
		MaxConns: 5, MaxIPs: 3, QuotaBytes: 1024 * 1024 * 100,
	})

	resp := env.bot.cmdSecrets(context.Background())
	if !strings.Contains(resp.text, "conns=5") {
		t.Errorf("should show conns limit, got: %s", resp.text)
	}
	if !strings.Contains(resp.text, "ips=3") {
		t.Error("should show ips limit")
	}
	if !strings.Contains(resp.text, "quota=") {
		t.Error("should show quota")
	}
}

func TestCmdAdd(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdAdd(context.Background(), "/add newuser")
	if !strings.Contains(resp.text, "added") {
		t.Errorf("cmdAdd should confirm addition, got: %s", resp.text)
	}

	// Verify it's in the DB
	sec, err := env.deps.Secrets.GetByLabel(context.Background(), "newuser")
	if err != nil || sec == nil {
		t.Fatal("secret should exist after /add")
	}
	if !sec.Enabled {
		t.Error("newly added secret should be enabled")
	}
}

func TestCmdAdd_NoLabel(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdAdd(context.Background(), "/add")
	if !strings.Contains(resp.text, "Usage") {
		t.Errorf("cmdAdd without label should show usage, got: %s", resp.text)
	}
}

func TestCmdAdd_InvalidLabel(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdAdd(context.Background(), "/add bad label!")
	if !strings.Contains(resp.text, "Invalid") {
		t.Errorf("cmdAdd with invalid label should show error, got: %s", resp.text)
	}
}

func TestCmdAdd_Duplicate(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{Label: "existing", SecretKey: "aa111111111111111111111111111111", Enabled: true})

	resp := env.bot.cmdAdd(context.Background(), "/add existing")
	if !strings.Contains(resp.text, "already exists") {
		t.Errorf("cmdAdd duplicate should say 'already exists', got: %s", resp.text)
	}
}

func TestCmdRemove(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{Label: "user1", SecretKey: "aa111111111111111111111111111111", Enabled: true})

	resp := env.bot.cmdRemove(context.Background(), "/remove user1")
	if !strings.Contains(resp.text, "removed") {
		t.Errorf("cmdRemove should confirm removal, got: %s", resp.text)
	}

	// Verify it's gone
	sec, _ := env.deps.Secrets.GetByLabel(context.Background(), "user1")
	if sec != nil {
		t.Error("secret should be gone after /remove")
	}
}

func TestCmdRemove_NotFound(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdRemove(context.Background(), "/remove ghost")
	if !strings.Contains(resp.text, "not found") {
		t.Errorf("cmdRemove nonexistent should say 'not found', got: %s", resp.text)
	}
}

func TestCmdRemove_NoLabel(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdRemove(context.Background(), "/remove")
	if !strings.Contains(resp.text, "Usage") {
		t.Errorf("cmdRemove without label should show usage, got: %s", resp.text)
	}
}

func TestCmdRotate(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{Label: "user1", SecretKey: "aa111111111111111111111111111111", Enabled: true})

	resp := env.bot.cmdRotate(context.Background(), "/rotate user1")
	if !strings.Contains(resp.text, "rotated") {
		t.Errorf("cmdRotate should confirm rotation, got: %s", resp.text)
	}

	// Verify key changed
	sec, _ := env.deps.Secrets.GetByLabel(context.Background(), "user1")
	if sec.SecretKey == "aa111111111111111111111111111111" {
		t.Error("secret key should have changed after rotation")
	}
}

func TestCmdRotate_NotFound(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdRotate(context.Background(), "/rotate ghost")
	if !strings.Contains(resp.text, "not found") {
		t.Errorf("cmdRotate nonexistent should say 'not found', got: %s", resp.text)
	}
}

func TestCmdEnable(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{Label: "user1", SecretKey: "aa111111111111111111111111111111", Enabled: false})

	resp := env.bot.cmdEnable(context.Background(), "/enable user1")
	if !strings.Contains(resp.text, "enabled") {
		t.Errorf("cmdEnable should confirm, got: %s", resp.text)
	}

	sec, _ := env.deps.Secrets.GetByLabel(context.Background(), "user1")
	if !sec.Enabled {
		t.Error("secret should be enabled after /enable")
	}
}

func TestCmdEnable_AlreadyEnabled(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{Label: "user1", SecretKey: "aa111111111111111111111111111111", Enabled: true})

	resp := env.bot.cmdEnable(context.Background(), "/enable user1")
	if !strings.Contains(resp.text, "already enabled") {
		t.Errorf("should say already enabled, got: %s", resp.text)
	}
}

func TestCmdDisable(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{Label: "user1", SecretKey: "aa111111111111111111111111111111", Enabled: true})
	seedSecret(t, env.db, &model.Secret{Label: "user2", SecretKey: "bb222222222222222222222222222222", Enabled: true})

	resp := env.bot.cmdDisable(context.Background(), "/disable user1")
	if !strings.Contains(resp.text, "disabled") {
		t.Errorf("cmdDisable should confirm, got: %s", resp.text)
	}

	sec, _ := env.deps.Secrets.GetByLabel(context.Background(), "user1")
	if sec.Enabled {
		t.Error("secret should be disabled after /disable")
	}
}

func TestCmdDisable_LastSecret(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{Label: "only", SecretKey: "aa111111111111111111111111111111", Enabled: true})

	resp := env.bot.cmdDisable(context.Background(), "/disable only")
	if !strings.Contains(resp.text, "Cannot disable the last") {
		t.Errorf("should refuse to disable last secret, got: %s", resp.text)
	}
}

func TestCmdDisable_AlreadyDisabled(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{Label: "user1", SecretKey: "aa111111111111111111111111111111", Enabled: false})
	seedSecret(t, env.db, &model.Secret{Label: "user2", SecretKey: "bb222222222222222222222222222222", Enabled: true})

	resp := env.bot.cmdDisable(context.Background(), "/disable user1")
	if !strings.Contains(resp.text, "already disabled") {
		t.Errorf("should say already disabled, got: %s", resp.text)
	}
}

func TestCmdRestart(t *testing.T) {
	env := newTestEnv(t)
	restarted := false
	env.deps.RestartProxy = func(_ context.Context) error {
		restarted = true
		return nil
	}

	resp := env.bot.cmdRestart(context.Background())
	if !restarted {
		t.Error("RestartProxy should have been called")
	}
	if !strings.Contains(resp.text, "restarted") {
		t.Errorf("cmdRestart should confirm, got: %s", resp.text)
	}
}

func TestCmdRestart_Failure(t *testing.T) {
	env := newTestEnv(t)
	env.deps.RestartProxy = func(_ context.Context) error {
		return fmt.Errorf("container not found")
	}

	resp := env.bot.cmdRestart(context.Background())
	if !strings.Contains(resp.text, "failed") {
		t.Errorf("cmdRestart should show failure, got: %s", resp.text)
	}
}

func TestCmdRestart_NilCallback(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdRestart(context.Background())
	if !strings.Contains(resp.text, "not available") {
		t.Errorf("cmdRestart with nil callback should say not available, got: %s", resp.text)
	}
}

func TestCmdStartInstance(t *testing.T) {
	env := newTestEnv(t)
	seedInstance(t, env.db, &model.Instance{
		Port: 443, MetricsPort: 9093, Enabled: true, Label: "main",
		TLSDomain: "example.com",
	})

	var startedID int64
	env.deps.StartInstance = func(_ context.Context, id int64) error {
		startedID = id
		return nil
	}

	resp := env.bot.dispatchStart(context.Background(), "/start main")
	if startedID == 0 {
		t.Error("StartInstance should have been called")
	}
	if !strings.Contains(resp.text, "started") {
		t.Errorf("should confirm start, got: %s", resp.text)
	}
}

func TestCmdStartInstance_NotFound(t *testing.T) {
	env := newTestEnv(t)
	seedInstance(t, env.db, &model.Instance{
		Port: 443, MetricsPort: 9093, Enabled: true, Label: "main",
		TLSDomain: "example.com",
	})
	resp := env.bot.cmdStartInstance(context.Background(), "/start ghost")
	if !strings.Contains(resp.text, "not found") {
		t.Errorf("should say not found, got: %s", resp.text)
	}
}

func TestCmdStartInstance_NoLabel(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdStartInstance(context.Background(), "/start ")
	if !strings.Contains(resp.text, "Usage") {
		t.Errorf("should show usage, got: %s", resp.text)
	}
}

func TestCmdStopInstance(t *testing.T) {
	env := newTestEnv(t)
	seedInstance(t, env.db, &model.Instance{
		Port: 443, MetricsPort: 9093, Enabled: true, Label: "main",
		TLSDomain: "example.com",
	})

	var stoppedID int64
	env.deps.StopInstance = func(_ context.Context, id int64) error {
		stoppedID = id
		return nil
	}

	resp := env.bot.cmdStopInstance(context.Background(), "/stop main")
	if stoppedID == 0 {
		t.Error("StopInstance should have been called")
	}
	if !strings.Contains(resp.text, "stopped") {
		t.Errorf("should confirm stop, got: %s", resp.text)
	}
}

func TestCmdStopInstance_Failure(t *testing.T) {
	env := newTestEnv(t)
	seedInstance(t, env.db, &model.Instance{
		Port: 443, MetricsPort: 9093, Enabled: true, Label: "main",
		TLSDomain: "example.com",
	})
	env.deps.StopInstance = func(_ context.Context, _ int64) error {
		return fmt.Errorf("timeout")
	}

	resp := env.bot.cmdStopInstance(context.Background(), "/stop main")
	if !strings.Contains(resp.text, "Failed") {
		t.Errorf("should show failure, got: %s", resp.text)
	}
}

func TestCmdTraffic(t *testing.T) {
	env := newTestEnv(t)
	testutil.SeedTraffic(t, env.db, "user1", 1024, 2048)

	resp := env.bot.cmdTraffic(context.Background())
	if !strings.Contains(resp.text, "Traffic Report") {
		t.Errorf("should contain 'Traffic Report', got: %s", resp.text)
	}
	if !strings.Contains(resp.text, "user1") {
		t.Error("should show per-user traffic")
	}
}

func TestCmdTraffic_NoData(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdTraffic(context.Background())
	if !strings.Contains(resp.text, "Traffic") {
		t.Errorf("should show traffic section, got: %s", resp.text)
	}
}

func TestCmdUpdate(t *testing.T) {
	env := newTestEnv(t)
	env.deps.GetEngineVersion = func() string { return "3.14" }

	resp := env.bot.cmdUpdate(context.Background())
	if !strings.Contains(resp.text, "PopuGate") {
		t.Errorf("should show PopuGate version, got: %s", resp.text)
	}
	if !strings.Contains(resp.text, "Engine") {
		t.Error("should show engine version")
	}
}

func TestCmdUpdate_NoEngine(t *testing.T) {
	env := newTestEnv(t)
	env.deps.GetEngineVersion = func() string { return "" }

	resp := env.bot.cmdUpdate(context.Background())
	if strings.Contains(resp.text, "Engine:") {
		t.Error("should not show engine section when version is empty")
	}
}

func TestCmdSetLimit(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{Label: "user1", SecretKey: "aa111111111111111111111111111111", Enabled: true})

	resp := env.bot.cmdSetLimit(context.Background(), "/setlimit user1 10 5 500")
	if !strings.Contains(resp.text, "updated") {
		t.Errorf("cmdSetLimit should confirm update, got: %s", resp.text)
	}

	sec, _ := env.deps.Secrets.GetByLabel(context.Background(), "user1")
	if sec.MaxConns != 10 {
		t.Errorf("MaxConns = %d, want 10", sec.MaxConns)
	}
	if sec.MaxIPs != 5 {
		t.Errorf("MaxIPs = %d, want 5", sec.MaxIPs)
	}
	if sec.QuotaBytes != 500*1024*1024 {
		t.Errorf("QuotaBytes = %d, want %d", sec.QuotaBytes, 500*1024*1024)
	}
}

func TestCmdSetLimit_WithExpiry(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{Label: "user1", SecretKey: "aa111111111111111111111111111111", Enabled: true})

	resp := env.bot.cmdSetLimit(context.Background(), "/setlimit user1 10 5 500 2026-12-31")
	if !strings.Contains(resp.text, "updated") {
		t.Errorf("cmdSetLimit with expiry should confirm, got: %s", resp.text)
	}

	sec, _ := env.deps.Secrets.GetByLabel(context.Background(), "user1")
	if sec.ExpiresAt != "2026-12-31" {
		t.Errorf("ExpiresAt = %q, want %q", sec.ExpiresAt, "2026-12-31")
	}
}

func TestCmdSetLimit_InvalidArgs(t *testing.T) {
	tests := []struct {
		name       string
		cmd        string
		seedSecret bool
		want       string
	}{
		{"too few args", "/setlimit user1 10", true, "Usage"},
		{"not found", "/setlimit ghost 10 5 500", false, "not found"},
		{"invalid conns", "/setlimit user1 abc 5 500", true, "Invalid max_conns"},
		{"invalid ips", "/setlimit user1 10 abc 500", true, "Invalid max_ips"},
		{"invalid quota", "/setlimit user1 10 5 abc", true, "Invalid quota"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv(t)
			if tt.seedSecret {
				seedSecret(t, env.db, &model.Secret{Label: "user1", SecretKey: "aa111111111111111111111111111111", Enabled: true})
			}
			resp := env.bot.cmdSetLimit(context.Background(), tt.cmd)
			if !strings.Contains(resp.text, tt.want) {
				t.Errorf("cmdSetLimit(%q) = %q, want to contain %q", tt.cmd, resp.text, tt.want)
			}
		})
	}
}

func TestCmdUpstreams_Empty(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdUpstreams(context.Background())
	if !strings.Contains(resp.text, "No upstreams") {
		t.Errorf("should say no upstreams, got: %s", resp.text)
	}
}

func TestCmdUpstreams_WithData(t *testing.T) {
	env := newTestEnv(t)
	seedUpstream(t, env.db, "proxy1", "1.2.3.4:1080", true)
	seedUpstream(t, env.db, "proxy2", "5.6.7.8:1080", false)

	resp := env.bot.cmdUpstreams(context.Background())
	if !strings.Contains(resp.text, "Upstreams (2)") {
		t.Errorf("should show 2 upstreams, got: %s", resp.text)
	}
	if !strings.Contains(resp.text, "proxy1") || !strings.Contains(resp.text, "proxy2") {
		t.Error("should list both upstreams")
	}
}

func TestCmdTasks(t *testing.T) {
	env := newTestEnv(t)
	env.deps.GetSchedulerTasks = func(_ context.Context) []string {
		return []string{"traffic-flush: every 1m ✅", "health-check: every 5m ✅"}
	}

	resp := env.bot.cmdTasks(context.Background())
	if !strings.Contains(resp.text, "traffic-flush") {
		t.Errorf("should show tasks, got: %s", resp.text)
	}
}

func TestCmdTasks_Empty(t *testing.T) {
	env := newTestEnv(t)
	env.deps.GetSchedulerTasks = func(_ context.Context) []string { return nil }

	resp := env.bot.cmdTasks(context.Background())
	if !strings.Contains(resp.text, "No scheduled tasks") {
		t.Errorf("should say no tasks, got: %s", resp.text)
	}
}

func TestCmdTasks_NilCallback(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdTasks(context.Background())
	if !strings.Contains(resp.text, "not available") {
		t.Errorf("should say not available, got: %s", resp.text)
	}
}

func TestCmdGeoblock_Disabled(t *testing.T) {
	env := newTestEnv(t)
	// Default after migration is "blacklist" with no countries — show status
	resp := env.bot.cmdGeoblock(context.Background())
	if !strings.Contains(resp.text, "Geo-blocking") {
		t.Errorf("should show geo-blocking section, got: %s", resp.text)
	}
	if !strings.Contains(resp.text, "No countries") {
		t.Errorf("should show no countries, got: %s", resp.text)
	}
}

func TestCmdGeoblock_WithCountries(t *testing.T) {
	env := newTestEnv(t)
	updateSetting(t, env.db, "geoblock_mode", "whitelist")
	updateSetting(t, env.db, "blocklist_countries", "RU, CN, IR")
	seedGeoblockCache(t, env.db, "RU", "/tmp/ru.cidr", time.Now().Add(-2*time.Hour).Unix())

	resp := env.bot.cmdGeoblock(context.Background())
	if !strings.Contains(resp.text, "whitelist") {
		t.Errorf("should show whitelist mode, got: %s", resp.text)
	}
	if !strings.Contains(resp.text, "RU") || !strings.Contains(resp.text, "CN") {
		t.Error("should list countries")
	}
	if !strings.Contains(resp.text, "cached") {
		t.Error("should show cache status for RU")
	}
}

func TestCmdReplication_Standalone(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdReplication(context.Background())
	if !strings.Contains(resp.text, "standalone") {
		t.Errorf("should show standalone mode, got: %s", resp.text)
	}
	if !strings.Contains(resp.text, "not configured") {
		t.Error("should say replication not configured")
	}
}

func TestCmdReplication_Master(t *testing.T) {
	env := newTestEnv(t)
	updateSetting(t, env.db, "replication_role", "master")
	updateSetting(t, env.db, "replication_sync_interval", 60)
	updateSetting(t, env.db, "replication_ssh_user", "popugate")
	updateSetting(t, env.db, "replication_ssh_port", 22)
	seedSlave(t, env.db, "slave1", "10.0.0.2", 8090, true)
	seedSlave(t, env.db, "slave2", "10.0.0.3", 8090, false)

	resp := env.bot.cmdReplication(context.Background())
	if !strings.Contains(resp.text, "master") {
		t.Errorf("should show master role, got: %s", resp.text)
	}
	if !strings.Contains(resp.text, "slave1") || !strings.Contains(resp.text, "slave2") {
		t.Error("should list slaves")
	}
}

func TestCmdReplication_Slave(t *testing.T) {
	env := newTestEnv(t)
	updateSetting(t, env.db, "replication_role", "slave")
	updateSetting(t, env.db, "replication_sync_interval", 120)

	resp := env.bot.cmdReplication(context.Background())
	if !strings.Contains(resp.text, "slave") {
		t.Errorf("should show slave role, got: %s", resp.text)
	}
	if !strings.Contains(resp.text, "120") {
		t.Error("should show sync interval")
	}
}

func TestCmdBackup_StatusEmpty(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdBackup(context.Background(), "/backup")
	if !strings.Contains(resp.text, "No backups") {
		t.Errorf("should show no backups, got: %s", resp.text)
	}
}

func TestCmdBackup_Create(t *testing.T) {
	env := newTestEnv(t)
	env.deps.CreateBackup = func(_ context.Context) (store.Backup, error) {
		return store.Backup{Filename: "backup-2026.tar.gz", Size: 12345}, nil
	}

	resp := env.bot.cmdBackup(context.Background(), "/backup create")
	if !strings.Contains(resp.text, "created") {
		t.Errorf("should confirm creation, got: %s", resp.text)
	}
	if !strings.Contains(resp.text, "backup-2026.tar.gz") {
		t.Error("should show filename")
	}
}

func TestCmdBackup_CreateFailure(t *testing.T) {
	env := newTestEnv(t)
	env.deps.CreateBackup = func(_ context.Context) (store.Backup, error) {
		return store.Backup{}, fmt.Errorf("disk full")
	}

	resp := env.bot.cmdBackup(context.Background(), "/backup create")
	if !strings.Contains(resp.text, "failed") {
		t.Errorf("should show failure, got: %s", resp.text)
	}
}

func TestCmdBackup_CreateNotAvailable(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdBackup(context.Background(), "/backup create")
	if !strings.Contains(resp.text, "not available") {
		t.Errorf("should say not available, got: %s", resp.text)
	}
}

func TestCmdResetQuota(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{Label: "user1", SecretKey: "aa111111111111111111111111111111", Enabled: true})

	reset := false
	env.deps.ResetTraffic = func(_ context.Context, label string) error {
		reset = true
		if label != "user1" {
			t.Errorf("ResetTraffic called with label %q, want %q", label, "user1")
		}
		return nil
	}

	resp := env.bot.cmdResetQuota(context.Background(), "/resetquota user1")
	if !reset {
		t.Error("ResetTraffic should have been called")
	}
	if !strings.Contains(resp.text, "reset") {
		t.Errorf("should confirm reset, got: %s", resp.text)
	}
}

func TestCmdResetQuota_NotFound(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdResetQuota(context.Background(), "/resetquota ghost")
	if !strings.Contains(resp.text, "not found") {
		t.Errorf("should say not found, got: %s", resp.text)
	}
}

func TestCmdResetQuota_NoLabel(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdResetQuota(context.Background(), "/resetquota")
	if !strings.Contains(resp.text, "Usage") {
		t.Errorf("should show usage, got: %s", resp.text)
	}
}

func TestCmdResetQuota_NilCallback(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{Label: "user1", SecretKey: "aa111111111111111111111111111111", Enabled: true})

	resp := env.bot.cmdResetQuota(context.Background(), "/resetquota user1")
	if !strings.Contains(resp.text, "not available") {
		t.Errorf("should say not available, got: %s", resp.text)
	}
}

func TestCmdInstances_Empty(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdInstances(context.Background())
	if !strings.Contains(resp.text, "No instances") {
		t.Errorf("should say no instances, got: %s", resp.text)
	}
}

func TestCmdInstances_WithData(t *testing.T) {
	env := newTestEnv(t)
	seedInstance(t, env.db, &model.Instance{
		Port: 443, MetricsPort: 9093, Enabled: true, Label: "main",
		TLSDomain: "example.com", FakeTLS: true, TCPMSSEnabled: true, TCPMSS: 88,
	})
	seedInstance(t, env.db, &model.Instance{
		Port: 8443, MetricsPort: 9094, Enabled: false, Label: "backup",
		TLSDomain: "backup.com",
	})
	env.deps.IsInstanceRunning = func(_ context.Context, name string) bool {
		return name == "popugate-telemt-443"
	}

	resp := env.bot.cmdInstances(context.Background())
	if !strings.Contains(resp.text, "Instances (2)") {
		t.Errorf("should show 2 instances, got: %s", resp.text)
	}
	if !strings.Contains(resp.text, "main") || !strings.Contains(resp.text, "backup") {
		t.Error("should list both instances")
	}
	if !strings.Contains(resp.text, "FakeTLS") {
		t.Error("should show FakeTLS detail")
	}
	if !strings.Contains(resp.text, "MSS=88") {
		t.Error("should show MSS detail")
	}
}

func TestCmdInfo(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{
		Label: "user1", SecretKey: "aa111111111111111111111111111111", Enabled: true,
		MaxConns: 5, MaxIPs: 3, QuotaBytes: 100 * 1024 * 1024,
		Notes: "test user", Tags: `["premium","vip"]`,
	})

	resp := env.bot.cmdInfo(context.Background(), "/info user1")
	if !strings.Contains(resp.text, "user1") {
		t.Errorf("should show secret label, got: %s", resp.text)
	}
	if !strings.Contains(resp.text, "Enabled") {
		t.Error("should show enabled status")
	}
	if !strings.Contains(resp.text, "conns=5") {
		t.Error("should show connection limit")
	}
	if !strings.Contains(resp.text, "quota=") {
		t.Error("should show quota")
	}
	if !strings.Contains(resp.text, "premium") {
		t.Error("should show tags")
	}
	if !strings.Contains(resp.text, "test user") {
		t.Error("should show notes")
	}
}

func TestCmdInfo_NotFound(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdInfo(context.Background(), "/info ghost")
	if !strings.Contains(resp.text, "not found") {
		t.Errorf("should say not found, got: %s", resp.text)
	}
}

func TestCmdInfo_NoLabel(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdInfo(context.Background(), "/info")
	if !strings.Contains(resp.text, "Usage") {
		t.Errorf("should show usage, got: %s", resp.text)
	}
}

func TestCmdInfo_ShortKey(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{
		Label: "short", SecretKey: "abc", Enabled: true,
	})

	resp := env.bot.cmdInfo(context.Background(), "/info short")
	if !strings.Contains(resp.text, "abc") {
		t.Errorf("should show short key as-is, got: %s", resp.text)
	}
}

func TestCmdInfo_Disabled(t *testing.T) {
	env := newTestEnv(t)
	seedSecret(t, env.db, &model.Secret{
		Label: "disabled1", SecretKey: "aa111111111111111111111111111111", Enabled: false,
	})

	resp := env.bot.cmdInfo(context.Background(), "/info disabled1")
	if !strings.Contains(resp.text, "Disabled") {
		t.Errorf("should show disabled status, got: %s", resp.text)
	}
}

func TestCmdLink_NoSecrets(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdLink(context.Background(), "/link")
	if !strings.Contains(resp.text, "No enabled secrets") {
		t.Errorf("should say no enabled secrets, got: %s", resp.text)
	}
}

func TestCmdLink_SecretNotFound(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdLink(context.Background(), "/link ghost")
	if !strings.Contains(resp.text, "not found") {
		t.Errorf("should say not found, got: %s", resp.text)
	}
}

func TestCountEnabled(t *testing.T) {
	secrets := []model.Secret{
		{Label: "a", Enabled: true},
		{Label: "b", Enabled: false},
		{Label: "c", Enabled: true},
	}
	if n := countEnabled(secrets); n != 2 {
		t.Errorf("countEnabled = %d, want 2", n)
	}
	if n := countEnabled(nil); n != 0 {
		t.Errorf("countEnabled(nil) = %d, want 0", n)
	}
}

func TestMdSafe(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello_world", "hello-world"},
		{"*bold*", "bold"},
		{"`code`", "'code'"},
		{"[link]", "(link)"},
		{"normal text", "normal text"},
	}
	for _, tt := range tests {
		got := mdSafe(tt.input)
		if got != tt.want {
			t.Errorf("mdSafe(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDispatchStart_NoArgs(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.dispatchStart(context.Background(), "/start")
	if !strings.Contains(resp.text, "/status") {
		t.Errorf("dispatchStart with no args should show welcome, got: %s", resp.text)
	}
}

func TestResolveCommand(t *testing.T) {
	env := newTestEnv(t)

	tests := []struct {
		input   string
		wantNil bool
	}{
		{"/status", false},
		{"/secrets", false},
		{"/help", false},
		{"/info user1", false},
		{"/unknown", true},
		{"hello", true},
	}

	for _, tt := range tests {
		handler, _ := env.bot.resolveCommand(tt.input)
		if (handler == nil) != tt.wantNil {
			t.Errorf("resolveCommand(%q) nil=%v, wantNil=%v", tt.input, handler == nil, tt.wantNil)
		}
	}
}

func TestDispatchStart_ArgsDispatchesToInstanceStart(t *testing.T) {
	env := newTestEnv(t)
	seedInstance(t, env.db, &model.Instance{
		Port: 443, MetricsPort: 9093, Enabled: true, Label: "main",
		TLSDomain: "example.com",
	})

	called := false
	env.deps.StartInstance = func(_ context.Context, _ int64) error {
		called = true
		return nil
	}

	env.bot.dispatchStart(context.Background(), "/start main")
	if !called {
		t.Error("dispatchStart with label should call cmdStartInstance")
	}
}

func TestCmdSetLimit_NoLabel(t *testing.T) {
	env := newTestEnv(t)
	resp := env.bot.cmdSetLimit(context.Background(), "/setlimit")
	if !strings.Contains(resp.text, "Usage") {
		t.Errorf("should show usage, got: %s", resp.text)
	}
}

func TestCmdGeoblock_SettingsUnavailable(t *testing.T) {
	env := newTestEnv(t)
	// Drop the settings table to force Load to fail
	_, _ = env.db.Exec("DROP TABLE settings")

	resp := env.bot.cmdGeoblock(context.Background())
	if !strings.Contains(resp.text, "Settings unavailable") {
		t.Errorf("should say settings unavailable, got: %s", resp.text)
	}
}
