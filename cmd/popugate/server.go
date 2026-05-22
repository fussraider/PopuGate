// @title           PopuGate API
// @version         1.0
// @description     PopuGate is a Telegram MTProto proxy manager with a REST API.
// @contact.name    PopuGate Support
// @contact.url     https://github.com/fussraider/PopuGate
// @license.name    MIT
// @license.url     https://github.com/fussraider/PopuGate/blob/main/LICENSE
// @host      localhost:8090
// @BasePath  /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/fussraider/PopuGate/internal/api"
	"github.com/fussraider/PopuGate/internal/bot"
	"github.com/fussraider/PopuGate/internal/database"
	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/scheduler"
	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/logger"
	"github.com/fussraider/PopuGate/pkg/qrutil"
)

// version is set at build time via -ldflags "-X main.version=..."
var version string

// commit is set at build time via -ldflags "-X main.commit=..."
var commit string

var srvLog = logger.WithScope("server")

// serverCmd represents the server command.
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the HTTP API server and web UI",
	Long: `Start the PopuGate HTTP API server and serve the web UI.

Usage:
  popugate server [flags]

Flags:
  -p, --port int       HTTP server port (default 8090)
  -d, --db string      SQLite database path
      --data string    Data directory`,
	Run: runServer,
}

func init() {
	serverCmd.Flags().IntP("port", "p", 8090, "HTTP server port")
	serverCmd.Flags().String("db", "", "SQLite database path")
	serverCmd.Flags().StringP("data", "d", "", "Data directory")
	rootCmd.AddCommand(serverCmd)
}

func runServer(cmd *cobra.Command, args []string) {
	printBanner()
	dataDir, dbPath, port := resolveServerFlags(cmd)

	model.InstallDir = dataDir

	db, err := database.Open(database.Config{Path: dbPath})
	if err != nil {
		logger.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	stores := initStores(db, dataDir)
	svcs := initServices(stores, dataDir)

	botCtx, botCancel := context.WithCancel(context.Background())
	defer botCancel()

	botDeps := buildBotDeps(stores, svcs)

	var activeBot *bot.Bot
	var botMu sync.Mutex
	registerBotCommands(botCtx, stores.settings)
	startBotIfNeeded(botCtx, stores.settings, botDeps, &activeBot, &botMu)

	notifyFn := makeNotifyFn(&activeBot, &botMu, stores.settings)
	notifyWithBtns := makeNotifyWithButtonsFn(&activeBot, &botMu, stores.settings)
	wireNotifyCallbacks(svcs, notifyFn, notifyWithBtns, stores.settings)

	ctx := context.Background()
	settings, err := stores.settings.Load(ctx)
	if err != nil {
		srvLog.Warnf("Failed to load settings: %v", err)
	}
	seedDefaultInstance(ctx, svcs.container, settings)

	isDebug := resolveDebugMode(settings)

	auditSvc := service.NewAuditService(stores.audit)
	service.InitResourceMonitor(notifyFn)

	sched, tasks := prepareSchedulerTasks(schedulerTaskParams{
		trafficSvc: svcs.traffic, healthSvc: svcs.health, replSvc: svcs.repl,
		blocklist: stores.blocklist, settings: stores.settings, secrets: stores.secret,
		backupStore: stores.backup, activeBot: &activeBot, botMu: &botMu,
		updateSvc: svcs.update, telemtUpdateSvc: svcs.telemtUpdate, telemtCfg: svcs.telemtCfg,
		notify: notifyFn, notifyWithBtns: notifyWithBtns, trafficStore: stores.traffic, secretSvc: svcs.secret,
		upstreamSvc: svcs.upstream, auditSvc: auditSvc,
	})

	startScheduler(sched, tasks, stores.scheduler, botDeps, stores.scheduler)

	schedulerSvc := service.NewSchedulerService(stores.scheduler, sched)
	templateSvc := service.NewTemplateService(stores.template, stores.secret)

	router := api.SetupRouter(api.RouterConfig{
		Debug:           isDebug,
		JWTSecret:       api.NewCachedJWTSecretProvider(stores.settings, 5*time.Minute),
		Settings:        stores.settings,
		Secrets:         stores.secret,
		Upstreams:       stores.upstream,
		Instances:       stores.instance,
		Slaves:          stores.slave,
		Traffic:         stores.traffic,
		Blocklist:       stores.blocklist,
		Backups:         stores.backup,
		Docker:          svcs.dockerCli,
		SecretSvc:       svcs.secret,
		UpstreamSvc:     svcs.upstream,
		ContainerSvc:    svcs.container,
		DockerSvc:       svcs.docker,
		GeoblockSvc:     svcs.geoblock,
		BotDeps:         botDeps,
		HealthSvc:       svcs.health,
		TrafficSvc:      svcs.traffic,
		ReplSvc:         svcs.repl,
		UpdateSvc:       svcs.update,
		TelemtUpdateSvc: svcs.telemtUpdate,
		TelemtCfg:       svcs.telemtCfg,
		SchedulerSvc:    schedulerSvc,
		AuditSvc:        auditSvc,
		TemplateSvc:     templateSvc,
	})

	runHTTPServer(port, router, botCancel, &activeBot, svcs.container)
}

func resolveServerFlags(cmd *cobra.Command) (dataDir, dbPath string, port int) {
	defaultDataDir := resolveDataDir()
	dataDir, _ = cmd.Flags().GetString("data")
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	port, _ = cmd.Flags().GetInt("port")
	dbPath, _ = cmd.Flags().GetString("db")
	if dbPath == "" {
		dbPath = filepath.Join(dataDir, "settings.db")
	}
	return
}

type appStores struct {
	db        *sql.DB
	settings  *store.SettingsStore
	secret    *store.SecretStore
	upstream  *store.UpstreamStore
	instance  *store.InstanceStore
	slave     *store.SlaveStore
	traffic   *store.TrafficStore
	blocklist *store.TokenBlocklistStore
	quota     *store.QuotaAlertStore
	geoblock  *store.GeoblockCacheStore
	backup    *store.BackupStore
	scheduler *store.SchedulerStore
	audit     *store.AuditStore
	template  *store.TemplateStore
}

func initStores(db *sql.DB, dataDir string) appStores {
	var backupEncKey []byte
	if keyHex := os.Getenv("BACKUP_ENCRYPTION_KEY"); keyHex != "" {
		if len(keyHex) != 64 {
			logger.Fatalf("BACKUP_ENCRYPTION_KEY must be 64 hex characters (32 bytes), got %d", len(keyHex))
		}
		key, err := hex.DecodeString(keyHex)
		if err != nil {
			logger.Fatalf("BACKUP_ENCRYPTION_KEY must be a valid hex string: %v", err)
		}
		backupEncKey = key
		srvLog.Infof("Backup encryption key loaded from environment")
	}
	return appStores{
		db:        db,
		settings:  store.NewSettingsStore(db),
		secret:    store.NewSecretStore(db),
		upstream:  store.NewUpstreamStore(db),
		instance:  store.NewInstanceStore(db),
		slave:     store.NewSlaveStore(db),
		traffic:   store.NewTrafficStore(db),
		blocklist: store.NewTokenBlocklistStore(db),
		quota:     store.NewQuotaAlertStore(db),
		geoblock:  store.NewGeoblockCacheStore(db),
		backup:    store.NewBackupStore(dataDir, backupEncKey),
		scheduler: store.NewSchedulerStore(db),
		audit:     store.NewAuditStore(db),
		template:  store.NewTemplateStore(db),
	}
}

type appServices struct {
	dockerCli    *dockerutil.DockerClient
	secret       *service.SecretService
	upstream     *service.UpstreamService
	traffic      *service.TrafficService
	geoblock     *service.GeoblockService
	update       *service.UpdateService
	telemtCfg    *service.DBTelemtConfig
	docker       *service.DockerService
	container    *service.ContainerService
	health       *service.HealthService
	repl         *service.ReplicationService
	telemtUpdate *service.TelemtUpdateService
}

func initServices(s appStores, dataDir string) appServices {
	// Get JWT secret (auto-generated on first run)
	if _, err := s.settings.GetJWTSecret(context.Background()); err != nil {
		logger.Fatalf("Failed to get JWT secret: %v", err)
	}

	dockerClient, err := dockerutil.NewDockerClient()
	if err != nil {
		srvLog.Warnf("Docker client unavailable: %v", err)
	}

	svcs := appServices{
		dockerCli: dockerClient,
		secret:    service.NewSecretService(s.secret, s.instance, s.settings, s.traffic),
		upstream:  service.NewUpstreamService(s.upstream),
		traffic:   service.NewTrafficService(s.traffic, s.settings, dockerClient, s.instance),
		geoblock:  service.NewGeoblockService(s.settings, s.instance, s.geoblock),
		update:    service.NewUpdateService(dockerClient),
		telemtCfg: service.NewDBTelemtConfig(s.settings),
	}
	svcs.traffic.SetSecretStore(s.secret, s.quota)

	if dockerClient != nil {
		svcs.docker = service.NewDockerService(dockerClient, svcs.telemtCfg)
		svcs.container = service.NewContainerService(
			dataDir, dockerClient, s.secret, s.upstream, s.instance, s.traffic, s.settings, svcs.traffic,
		)
		svcs.health = service.NewHealthService(dockerClient, s.settings, s.instance)
		svcs.health.SetContainerSvc(svcs.container)
		svcs.repl = service.NewReplicationService(s.settings, s.slave, s.instance)
		svcs.telemtUpdate = service.NewTelemtUpdateService(s.settings, svcs.docker, svcs.container, svcs.telemtCfg)
		svcs.telemtUpdate.ResetStaleUpdate(context.Background())
	}

	return svcs
}

func buildBotDeps(s appStores, svcs appServices) *bot.Dependencies {
	deps := &bot.Dependencies{
		Settings:  s.settings,
		Secrets:   s.secret,
		Upstreams: s.upstream,
		Traffic:   s.traffic,
		Instances: s.instance,
		Slaves:    s.slave,
		Backups:   s.backup,
		Geoblock:  s.geoblock,
	}
	if svcs.container != nil {
		deps.RestartProxy = func(ctx context.Context) error { return svcs.container.Restart(ctx) }
		deps.IsProxyRunning = func(ctx context.Context) bool {
			status, err := svcs.container.Status(ctx)
			return err == nil && status != nil && status.Running
		}
		deps.GetUptime = func(ctx context.Context) string {
			status, err := svcs.container.Status(ctx)
			if err != nil || status == nil {
				return ""
			}
			return status.Uptime
		}
		deps.IsInstanceRunning = func(ctx context.Context, containerName string) bool {
			r, err := svcs.dockerCli.IsInstanceRunning(ctx, containerName)
			return err == nil && r
		}
		deps.StartInstance = func(ctx context.Context, id int64) error {
			return svcs.container.StartInstance(ctx, id)
		}
		deps.StopInstance = func(ctx context.Context, id int64) error {
			return svcs.container.StopInstance(ctx, id)
		}
	}
	if svcs.docker != nil {
		deps.GetEngineVersion = svcs.docker.GetInstalledVersion
	}
	deps.GenerateQR = func(ctx context.Context, link string) ([]byte, error) {
		return qrutil.GeneratePNG(link, 256)
	}
	deps.CreateBackup = func(ctx context.Context) (store.Backup, error) {
		return s.backup.Create(ctx)
	}
	deps.ResetTraffic = func(ctx context.Context, label string) error {
		return s.traffic.ResetTraffic(ctx, label)
	}
	return deps
}

func makeNotifyFn(activeBot **bot.Bot, botMu *sync.Mutex, settingsStore *store.SettingsStore) service.NotifyFunc {
	return func(ctx context.Context, format string, args ...any) {
		botMu.Lock()
		b := *activeBot
		botMu.Unlock()
		if b == nil || !b.IsRunning() {
			return
		}
		s, err := settingsStore.Load(ctx)
		if err != nil || !s.TelegramAlertsEnabled {
			return
		}
		label := s.TelegramServerLabel
		if label == "" {
			label = "PopuGate"
		}
		fullArgs := append([]any{label}, args...)
		msg := fmt.Sprintf(format, fullArgs...)
		if err := b.SendMessage(ctx, msg); err != nil {
			srvLog.Warnf("notify: %v", err)
		}
	}
}

func makeNotifyWithButtonsFn(activeBot **bot.Bot, botMu *sync.Mutex, settingsStore *store.SettingsStore) service.NotifyWithButtonsFunc {
	return func(ctx context.Context, format string, buttons []service.KeyboardButton, args ...any) {
		botMu.Lock()
		b := *activeBot
		botMu.Unlock()
		if b == nil || !b.IsRunning() {
			return
		}
		s, err := settingsStore.Load(ctx)
		if err != nil || !s.TelegramAlertsEnabled {
			return
		}
		label := s.TelegramServerLabel
		if label == "" {
			label = "PopuGate"
		}
		fullArgs := append([]any{label}, args...)
		msg := fmt.Sprintf(format, fullArgs...)

		var rows [][]bot.InlineKeyboardButton
		for _, btn := range buttons {
			if btn.URL == "" {
				continue
			}
			rows = append(rows, []bot.InlineKeyboardButton{{Text: btn.Text, URL: btn.URL}})
		}

		if len(rows) > 0 {
			if err := b.SendMessageWithKeyboard(ctx, msg, bot.InlineKeyboardMarkup{InlineKeyboard: rows}); err != nil {
				srvLog.Warnf("notify: %v", err)
			}
		} else {
			if err := b.SendMessage(ctx, msg); err != nil {
				srvLog.Warnf("notify: %v", err)
			}
		}
	}
}

func wireNotifyCallbacks(svcs appServices, notify service.NotifyFunc, notifyWithBtns service.NotifyWithButtonsFunc, settingsStore *store.SettingsStore) {
	if svcs.container != nil {
		svcs.container.SetNotify(notify)
		svcs.container.SetNotifyWithButtons(notifyWithBtns)
	}
	if svcs.telemtUpdate != nil {
		svcs.telemtUpdate.SetNotify(notify)
	}
	if svcs.upstream != nil {
		svcs.upstream.SetNotify(notify)
		svcs.upstream.SetNotifyWithButtons(notifyWithBtns)
		svcs.upstream.SetSettings(settingsStore)
	}
}

func seedDefaultInstance(ctx context.Context, svc *service.ContainerService, settings *model.Settings) {
	if settings == nil || svc == nil {
		return
	}
	if err := svc.EnsureDefaultInstance(ctx, settings.ProxyPort, settings.ProxyMetricsPort, settings.ProxyDomain, settings.MaskingHost, settings.MaskingEnabled); err != nil {
		srvLog.Warnf("Failed to seed default instance: %v", err)
	}
}

func resolveDebugMode(settings *model.Settings) bool {
	isDebug := settings.Debug
	if os.Getenv("DEBUG") == "true" || os.Getenv("GIN_MODE") == "debug" {
		isDebug = true
	} else if os.Getenv("DEBUG") == "false" || os.Getenv("GIN_MODE") == "release" {
		isDebug = false
	}
	return isDebug
}

func startScheduler(sched *scheduler.Scheduler, tasks []scheduler.Task, schedulerStore *store.SchedulerStore, botDeps *bot.Dependencies, storeRef *store.SchedulerStore) {
	ctx := context.Background()
	overrides, _ := schedulerStore.GetOverrides(ctx)
	overrideMap := make(map[string]scheduler.TaskOverride)
	for _, o := range overrides {
		overrideMap[o.TaskName] = o
	}
	sched.StartWith(tasks, overrideMap, schedulerStore)

	botDeps.GetSchedulerTasks = func(ctx context.Context) []string {
		statuses := sched.GetTaskStatuses()
		var lines []string
		lines = append(lines, "📋 *Scheduled Tasks*")
		for _, t := range statuses {
			status := "✅"
			if !t.Enabled {
				status = "❌"
			}
			schedule := t.EffectiveSchedule
			lastRun := ""
			if rec, _ := storeRef.GetLatestHistory(ctx, t.Name); rec != nil {
				if rec.Status == "success" {
					lastRun = " ✅"
				} else {
					errMsg := rec.Error
					if len(errMsg) > 40 {
						errMsg = errMsg[:40] + "..."
					}
					errMsg = strings.NewReplacer("_", "-", "*", "", "`", "'", "[", "(", "]", ")").Replace(errMsg)
					lastRun = fmt.Sprintf(" ❌ %s", errMsg)
				}
			}
			lines = append(lines, fmt.Sprintf("%s `%s` (%s)%s", status, t.Name, schedule, lastRun))
		}
		return lines
	}
}

func runHTTPServer(port int, router http.Handler, botCancel context.CancelFunc, activeBot **bot.Bot, containerSvc *service.ContainerService) {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router,
	}

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		srvLog.Infof("Shutting down server...")

		botCancel()
		if *activeBot != nil {
			(*activeBot).Stop()
		}

		service.GetResourceMonitor().Stop()

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		if containerSvc != nil {
			srvLog.Infof("Stopping proxy containers...")
			if err := containerSvc.Stop(ctx); err != nil {
				srvLog.Errorf("Proxy stop error: %v", err)
			}
		}

		if err := srv.Shutdown(ctx); err != nil {
			srvLog.Errorf("Server shutdown error: %v", err)
		}
	}()

	srvLog.Infof("PopuGate API server starting on :%d", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("Server error: %v", err)
	}
	srvLog.Infof("Server stopped")
}

// registerBotCommands registers the bot command list via Telegram setMyCommands API.
// Runs on every server startup so the command list stays in sync after updates.
// Silently skips if the bot is not configured (no token/chat ID).
func registerBotCommands(ctx context.Context, settingsStore *store.SettingsStore) {
	settings, err := settingsStore.Load(ctx)
	if err != nil {
		srvLog.Warnf("bot commands: cannot read settings, skipping registration: %v", err)
		return
	}
	if settings.TelegramBotToken == "" {
		srvLog.Infof("bot commands: Telegram bot token not configured, skipping command registration")
		return
	}
	if err := bot.SetCommandsForToken(ctx, settings.TelegramBotToken); err != nil {
		srvLog.Warnf("bot commands: failed to register commands: %v", err)
		return
	}
	srvLog.Infof("bot commands: registered via setMyCommands")
}

func startBotIfNeeded(ctx context.Context, settingsStore *store.SettingsStore, deps *bot.Dependencies, activeBot **bot.Bot, mu *sync.Mutex) {
	settings, err := settingsStore.Load(ctx)
	if err != nil {
		return
	}
	if !settings.TelegramEnabled || settings.TelegramBotToken == "" || settings.TelegramChatID == "" {
		return
	}
	b := bot.New(settings.TelegramBotToken, settings.TelegramChatID, settings.TelegramServerLabel, deps)
	mu.Lock()
	*activeBot = b
	mu.Unlock()
	if err := b.SetCommands(ctx); err != nil {
		logger.WithScope("bot").Warnf("setMyCommands failed: %v", err)
	}
	go b.Start(ctx)
	logger.WithScope("bot").Infof("started")
}

// schedulerTaskParams holds dependencies for building scheduler task callbacks.
type schedulerTaskParams struct {
	trafficSvc      *service.TrafficService
	healthSvc       *service.HealthService
	replSvc         *service.ReplicationService
	blocklist       *store.TokenBlocklistStore
	settings        *store.SettingsStore
	secrets         *store.SecretStore
	backupStore     *store.BackupStore
	activeBot       **bot.Bot
	botMu           *sync.Mutex
	updateSvc       *service.UpdateService
	telemtUpdateSvc *service.TelemtUpdateService
	telemtCfg       *service.DBTelemtConfig
	notify          service.NotifyFunc
	notifyWithBtns  service.NotifyWithButtonsFunc
	trafficStore    *store.TrafficStore
	secretSvc       *service.SecretService
	upstreamSvc     *service.UpstreamService
	auditSvc        *service.AuditService
}

func prepareSchedulerTasks(p schedulerTaskParams) (*scheduler.Scheduler, []scheduler.Task) {
	sched := scheduler.New()
	tasks := scheduler.DefaultTasks()

	for i := range tasks {
		tasks[i].Fn = buildTaskFn(tasks[i].Name, p)
	}

	return sched, tasks
}

func buildTaskFn(name string, p schedulerTaskParams) func(context.Context) error {
	builders := map[string]func(schedulerTaskParams) func(context.Context) error{
		"traffic-flush":    buildTrafficFlush,
		"quota-check":      buildQuotaCheck,
		"expiry-check":     buildExpiryCheck,
		"health-check":     buildHealthCheck,
		"replication-sync": buildReplicationSync,
		"token-cleanup":    buildTokenCleanup,
		"daily-backup":     buildDailyBackup,
		"backup-cleanup":   buildBackupCleanup,
		"telegram-report":  buildTelegramReport,
		"update-check":     buildUpdateCheck,
		"telemt-check":     buildTelemtCheck,
		"history-cleanup":  buildHistoryCleanup,
		"quota-reset":      buildQuotaReset,
		"auto-rotate":      buildAutoRotate,
		"upstream-health":  buildUpstreamHealth,
	}
	fn, ok := builders[name]
	if !ok {
		return nil
	}
	return fn(p)
}

func buildTrafficFlush(p schedulerTaskParams) func(context.Context) error {
	if p.trafficSvc == nil {
		return nil
	}
	return func(ctx context.Context) error { return p.trafficSvc.Flush(ctx) }
}

func buildQuotaCheck(p schedulerTaskParams) func(context.Context) error {
	if p.trafficSvc == nil {
		return nil
	}
	return func(ctx context.Context) error {
		p.trafficSvc.CheckQuotas(ctx)
		return nil
	}
}

func buildExpiryCheck(p schedulerTaskParams) func(context.Context) error {
	if p.trafficSvc == nil {
		return nil
	}
	return func(ctx context.Context) error {
		p.trafficSvc.CheckExpirations(ctx)
		return nil
	}
}

func buildHealthCheck(p schedulerTaskParams) func(context.Context) error {
	if p.healthSvc == nil {
		return nil
	}
	return p.healthSvc.AutoRecover
}

func buildReplicationSync(p schedulerTaskParams) func(context.Context) error {
	if p.replSvc == nil {
		return nil
	}
	return func(ctx context.Context) error {
		for _, r := range p.replSvc.SyncAll(ctx) {
			if r.Error != "" {
				return fmt.Errorf("sync %s: %s", r.Host, r.Error)
			}
		}
		return nil
	}
}

func buildTokenCleanup(p schedulerTaskParams) func(context.Context) error {
	if p.blocklist == nil {
		return nil
	}
	return p.blocklist.Cleanup
}

func buildDailyBackup(p schedulerTaskParams) func(context.Context) error {
	if p.backupStore == nil {
		return nil
	}
	return func(ctx context.Context) error {
		_, err := p.backupStore.Create(ctx)
		return err
	}
}

func buildBackupCleanup(p schedulerTaskParams) func(context.Context) error {
	if p.backupStore == nil {
		return nil
	}
	return func(ctx context.Context) error {
		s, _ := p.settings.Load(ctx)
		days := 7
		if s != nil && s.BackupRetentionDays > 0 {
			days = s.BackupRetentionDays
		}
		_, err := p.backupStore.CleanOld(ctx, time.Duration(days)*24*time.Hour)
		return err
	}
}

func buildTelegramReport(p schedulerTaskParams) func(context.Context) error {
	return func(ctx context.Context) error {
		p.botMu.Lock()
		botPtr := *p.activeBot
		p.botMu.Unlock()
		if botPtr == nil || !botPtr.IsRunning() {
			return nil
		}
		return sendPeriodicReport(ctx, botPtr, p.settings, p.secrets, p.trafficSvc)
	}
}

func buildUpdateCheck(p schedulerTaskParams) func(context.Context) error {
	if p.updateSvc == nil {
		return nil
	}
	return func(ctx context.Context) error {
		status, err := p.updateSvc.Check(ctx)
		if err != nil {
			return err
		}
		if status.UpdateAvailable {
			srvLog.Infof("update available: v%s (current: v%s)", status.Latest, status.Current)
			s, _ := p.settings.Load(ctx)
			webURL := ""
			if s != nil {
				webURL = s.WebURL
			}
			var buttons []service.KeyboardButton
			if status.HTMLURL != "" {
				buttons = append(buttons, service.KeyboardButton{Text: "Release Notes", URL: status.HTMLURL})
			}
			if webURL != "" {
				buttons = append(buttons, service.KeyboardButton{Text: "Updates", URL: webURL + "/updates"})
			}
			p.notifyWithBtns(ctx, "🆕 *%s* New PopuGate version available: v%s\nCurrent: v%s", buttons, status.Latest, status.Current)
		}
		return nil
	}
}

func buildTelemtCheck(p schedulerTaskParams) func(context.Context) error {
	if p.telemtUpdateSvc == nil {
		return nil
	}
	return func(ctx context.Context) error {
		release, err := p.telemtUpdateSvc.CheckRemote(ctx)
		if err != nil {
			return err
		}
		if release.Version != p.telemtCfg.TelemtVersion() {
			srvLog.Infof("telemt update available: %s (current: %s)", release.Version, p.telemtCfg.TelemtVersion())
			s, _ := p.settings.Load(ctx)
			webURL := ""
			if s != nil {
				webURL = s.WebURL
			}
			var buttons []service.KeyboardButton
			if webURL != "" {
				buttons = append(buttons, service.KeyboardButton{Text: "Engine Updates", URL: webURL + "/docker"})
			}
			p.notifyWithBtns(ctx, "🆕 *%s* New telemt engine version available: %s\nCurrent: %s", buttons, release.Version, p.telemtCfg.TelemtVersion())
		}
		return nil
	}
}

func buildHistoryCleanup(p schedulerTaskParams) func(context.Context) error {
	return func(ctx context.Context) error {
		return p.trafficStore.CleanOldHistory(ctx, 30*24*time.Hour)
	}
}

func buildQuotaReset(p schedulerTaskParams) func(context.Context) error {
	if p.trafficSvc == nil {
		return nil
	}
	return func(ctx context.Context) error {
		p.trafficSvc.ResetAllQuotas(ctx)
		if p.auditSvc != nil {
			_ = p.auditSvc.Log(ctx, "system", "quota-reset", "monthly quota reset completed")
		}
		return nil
	}
}

func buildAutoRotate(p schedulerTaskParams) func(context.Context) error {
	if p.secretSvc == nil {
		return nil
	}
	return func(ctx context.Context) error {
		s, _ := p.settings.Load(ctx)
		if s == nil || s.SecretAutoRotateDays <= 0 {
			return nil
		}
		all, err := p.secrets.List(ctx)
		if err != nil {
			return err
		}
		cutoff := time.Now().AddDate(0, 0, -s.SecretAutoRotateDays).Unix()
		rotated := 0
		for _, sec := range all {
			if sec.Enabled && sec.CreatedAt > 0 && sec.CreatedAt < cutoff {
				if _, err := p.secretSvc.Rotate(ctx, sec.Label); err == nil {
					rotated++
				}
			}
		}
		if rotated > 0 && p.auditSvc != nil {
			_ = p.auditSvc.Log(ctx, "system", "auto-rotate", fmt.Sprintf("rotated %d secret(s)", rotated))
		}
		return nil
	}
}

func buildUpstreamHealth(p schedulerTaskParams) func(context.Context) error {
	if p.upstreamSvc == nil {
		return nil
	}
	return func(ctx context.Context) error { return p.upstreamSvc.CheckAllUpstreams(ctx) }
}

func sendPeriodicReport(ctx context.Context, b *bot.Bot, settings *store.SettingsStore, secrets *store.SecretStore, trafficSvc *service.TrafficService) error {
	s, _ := settings.Load(ctx)
	label := s.TelegramServerLabel

	var lines []string
	lines = append(lines, fmt.Sprintf("📊 *%s Periodic Report*", label))
	lines = append(lines, "")

	// Traffic
	global, err := trafficSvc.GetReport(ctx)
	if err == nil {
		lines = append(lines, fmt.Sprintf("Traffic: ↓%s ↑%s",
			store.FormatBytes(global.Global.TotalIn),
			store.FormatBytes(global.Global.TotalOut)))
	}

	// Secrets count
	secretList, err := secrets.List(ctx)
	if err == nil {
		enabled := 0
		for _, sec := range secretList {
			if sec.Enabled {
				enabled++
			}
		}
		lines = append(lines, fmt.Sprintf("Secrets: %d/%d active", enabled, len(secretList)))

		// Quota warnings
		for _, sec := range secretList {
			if sec.Enabled && sec.QuotaBytes > 0 {
				pct := float64(sec.TrafficIn+sec.TrafficOut) * 100 / float64(sec.QuotaBytes)
				if pct >= 80 {
					lines = append(lines, fmt.Sprintf("⚠ `%s` quota: %.0f%%", sec.Label, pct))
				}
			}
		}
	}

	return b.SendMessage(ctx, strings.Join(lines, "\n"))
}

func printBanner() {
	banner := `
  ____                    ____       _       
 |  _ \ ___  _ __  _   _ / ___| __ _| |_ ___ 
 | |_) / _ \| '_ \| | | | |  _ / _` + "`" + ` | __/ _ \
 |  __/ (_) | |_) | |_| | |_| | (_| | ||  __/
 |_|   \___/| .__/ \__,_|\____|\__,_|\__\___|
            |_|                              
`
	fmt.Print(banner)
	fmt.Printf(" PopuGate Server %s\n", model.VersionTag())
	fmt.Printf(" %s\n", model.VersionURL())
	fmt.Println(" -------------------------------------------")
	fmt.Println()
}

// resolveDataDir returns the data directory in priority order:
// 1. POPUGATE_DATA_DIR env var
// 2. Directory of the running binary
// 3. Current working directory
// 4. Fallback "/opt/popugate"
func resolveDataDir() string {
	if dir := os.Getenv("POPUGATE_DATA_DIR"); dir != "" {
		return dir
	}
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			return filepath.Dir(resolved)
		}
		return filepath.Dir(exe)
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "/opt/popugate"
}
