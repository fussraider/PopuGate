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
	"net/url"
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
	serverCmd.Flags().String("override-version", "", "Override application version for update testing")
	rootCmd.AddCommand(serverCmd)
}

func runServer(cmd *cobra.Command, args []string) {
	printBanner()
	dataDir, dbPath, port := resolveServerFlags(cmd)

	if v, _ := cmd.Flags().GetString("override-version"); v != "" {
		model.SetVersion(v)
		srvLog.Infof("version overridden to %s", model.Version)
	}

	model.InstallDir = dataDir

	db, err := database.Open(database.Config{Path: dbPath})
	if err != nil {
		logger.Fatalf("Failed to open database: %v", err)
	}
	defer func() { _ = database.Close() }()

	stores := initStores(db, dataDir)
	if n, err := stores.instance.BackfillAPIPorts(context.Background()); err != nil {
		srvLog.Warnf("backfill instance api_port: %v", err)
	} else if n > 0 {
		srvLog.Infof("assigned control-plane api_port to %d existing instance(s)", n)
	}
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
		dockerUpdateSvc: svcs.dockerUpdate,
		notify:          notifyFn, notifyWithBtns: notifyWithBtns, trafficStore: stores.traffic, secretSvc: svcs.secret,
		upstreamSvc: svcs.upstream, auditSvc: auditSvc, containerSvc: svcs.container,
	})

	startScheduler(sched, tasks, stores.scheduler, botDeps, stores.scheduler)

	schedulerSvc := service.NewSchedulerService(stores.scheduler, sched)
	templateSvc := service.NewTemplateService(stores.template, stores.secret)

	var allowedHosts []string
	if settings != nil && settings.WebURL != "" {
		if u, err := url.Parse(settings.WebURL); err == nil && u.Hostname() != "" {
			allowedHosts = append(allowedHosts, u.Hostname())
		}
	}

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
		ActiveBot:       &activeBot,
		BotMu:           &botMu,
		HealthSvc:       svcs.health,
		TrafficSvc:      svcs.traffic,
		ReplSvc:         svcs.repl,
		UpdateSvc:       svcs.update,
		TelemtUpdateSvc: svcs.telemtUpdate,
		DockerUpdateSvc: svcs.dockerUpdate,
		TelemtCfg:       svcs.telemtCfg,
		SchedulerSvc:    schedulerSvc,
		AuditSvc:        auditSvc,
		TemplateSvc:     templateSvc,
		AllowedHosts:    allowedHosts,
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
	dockerUpdate *service.DockerUpdateService
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

	// Engine control-plane API client: propagate quota resets to running engines
	// (loopback [server.api]) without recreating containers.
	engineAPI := service.NewEngineAPIClient()
	svcs.secret.SetEngineAPI(engineAPI)
	svcs.traffic.SetEngineAPI(engineAPI)

	if dockerClient != nil {
		svcs.docker = service.NewDockerService(dockerClient, svcs.telemtCfg)
		svcs.container = service.NewContainerService(
			dataDir, dockerClient, s.secret, s.upstream, s.instance, s.traffic, s.settings, svcs.traffic,
		)
		svcs.upstream.SetContainerSvc(svcs.container)
		svcs.health = service.NewHealthService(dockerClient, s.settings, s.instance)
		svcs.health.SetContainerSvc(svcs.container)
		svcs.repl = service.NewReplicationService(s.settings, s.slave, s.instance)
		svcs.telemtUpdate = service.NewTelemtUpdateService(s.settings, svcs.docker, svcs.container, svcs.telemtCfg)
		svcs.telemtUpdate.ResetStaleUpdate(context.Background())
		svcs.dockerUpdate = service.NewDockerUpdateService(s.settings, dockerClient, svcs.container)
		svcs.dockerUpdate.HandleStartupRecovery(context.Background())
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
		return svcs.secret.ResetTraffic(ctx, label)
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
	if svcs.dockerUpdate != nil {
		svcs.dockerUpdate.SetNotify(notify)
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
	dockerUpdateSvc *service.DockerUpdateService
	telemtCfg       *service.DBTelemtConfig
	notify          service.NotifyFunc
	notifyWithBtns  service.NotifyWithButtonsFunc
	trafficStore    *store.TrafficStore
	secretSvc       *service.SecretService
	upstreamSvc     *service.UpstreamService
	auditSvc        *service.AuditService
	containerSvc    *service.ContainerService
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
		"traffic-flush":     buildTrafficFlush,
		"quota-check":       buildQuotaCheck,
		"expiry-check":      buildExpiryCheck,
		"health-check":      buildHealthCheck,
		"replication-sync":  buildReplicationSync,
		"token-cleanup":     buildTokenCleanup,
		"daily-backup":      buildDailyBackup,
		"backup-cleanup":    buildBackupCleanup,
		"telegram-report":   buildTelegramReport,
		"update-check":      buildUpdateCheck,
		"auto-update":       buildAutoUpdate,
		"telemt-check":      buildTelemtCheck,
		"docker-host-check": buildDockerHostCheck,
		"history-cleanup":   buildHistoryCleanup,
		"quota-reset":       buildQuotaReset,
		"auto-rotate":       buildAutoRotate,
		"upstream-health":   buildUpstreamHealth,
		"fronting-update":   buildFrontingUpdate,
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
				buttons = append(buttons, service.KeyboardButton{Text: "Updates", URL: webURL + "/system"})
			}
			p.notifyWithBtns(ctx, "🆕 *%s* New PopuGate version available: v%s\nCurrent: v%s", buttons, status.Latest, status.Current)
		}
		return nil
	}
}

func buildAutoUpdate(p schedulerTaskParams) func(context.Context) error {
	if p.updateSvc == nil {
		return nil
	}
	return func(ctx context.Context) error {
		return p.updateSvc.AutoUpdate(ctx, p.notify)
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
			if release.HTMLURL != "" {
				buttons = append(buttons, service.KeyboardButton{Text: "Release Notes", URL: release.HTMLURL})
			}
			if webURL != "" {
				buttons = append(buttons, service.KeyboardButton{Text: "Engine Updates", URL: webURL + "/docker"})
			}
			p.notifyWithBtns(ctx, "🆕 *%s* New telemt engine version available: %s\nCurrent: %s", buttons, release.Version, p.telemtCfg.TelemtVersion())
		}
		return nil
	}
}

func buildDockerHostCheck(p schedulerTaskParams) func(context.Context) error {
	if p.dockerUpdateSvc == nil {
		return nil
	}
	return func(ctx context.Context) error {
		status, err := p.dockerUpdateSvc.CheckRemote(ctx)
		if err != nil {
			return err
		}
		if status.UpdateAvailable {
			srvLog.Infof("host Docker update available: %s (current: %s)", status.LatestVersion, status.CurrentVersion)
			s, _ := p.settings.Load(ctx)
			webURL := ""
			if s != nil {
				webURL = s.WebURL
			}
			var buttons []service.KeyboardButton
			if status.ChangelogURL != "" {
				buttons = append(buttons, service.KeyboardButton{Text: "Release Notes", URL: status.ChangelogURL})
			}
			if webURL != "" {
				buttons = append(buttons, service.KeyboardButton{Text: "Docker Updates", URL: webURL + "/docker"})
			}
			p.notifyWithBtns(ctx, "🆕 *%s* New host Docker Engine version available: %s\nCurrent: %s", buttons, status.LatestVersion, status.CurrentVersion)
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

func buildFrontingUpdate(p schedulerTaskParams) func(context.Context) error {
	if p.containerSvc == nil {
		return nil
	}
	return func(ctx context.Context) error { return p.containerSvc.RefreshAllFrontingContent(ctx) }
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

// terminalSupportsColor returns true when the current stdout terminal
// can display ANSI TrueColor sequences. It respects three signals:
//  1. NO_COLOR env var (https://no-color.org) — user explicitly opt-out
//  2. TERM=dumb — terminal is known to be incapable of escape codes
//  3. stdout is not a character device — output is piped / redirected
func terminalSupportsColor() bool {
	if _, noColor := os.LookupEnv("NO_COLOR"); noColor {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func printBanner() {
	if terminalSupportsColor() {
		printColorBanner()
	} else {
		printPlainBanner()
	}
}

func printPlainBanner() {
	fmt.Printf(`
  ____                    ____       _       
 |  _ \ ___  _ __  _   _ / ___| __ _| |_ ___ 
 | |_) / _ \| '_ \| | | | |  _ / _`+"`"+` | __/ _ \
 |  __/ (_) | |_) | |_| | |_| | (_| | ||  __/
 |_|   \___/| .__/ \__,_|\____|\__,_|\__\___|
            |_|

 PopuGate Server %s
 %s

`, model.VersionTag(), model.VersionURL())
}

func printColorBanner() {
	banner := "" +
		"         \x1b[38;2;0;82;102m▄\x1b[0m\x1b[38;2;22;139;137m▄\x1b[0m\x1b[38;2;77;175;165m▄\x1b[0m\x1b[48;2;6;139;124m\x1b[38;2;155;212;205m▄\x1b[0m\x1b[48;2;6;141;126m\x1b[38;2;159;213;206m▄\x1b[0m\x1b[38;2;80;176;165m▄\x1b[0m\x1b[38;2;24;149;136m▄\x1b[0m\x1b[38;2;0;112;106m▄\x1b[0m           \n" +
		"   \x1b[38;2;22;66;136m▄\x1b[0m\x1b[38;2;67;106;164m▄\x1b[0m\x1b[48;2;0;0;92m\x1b[38;2;109;142;182m▄\x1b[0m\x1b[48;2;0;54;121m\x1b[38;2;160;188;214m▄\x1b[0m\x1b[48;2;31;98;142m\x1b[38;2;212;228;241m▄\x1b[0m\x1b[48;2;75;136;164m\x1b[38;2;248;253;255m▄\x1b[0m\x1b[48;2;124;178;188m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;190;230;230m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;249;255;255m\x1b[38;2;252;251;252m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;210;218;232m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;181;194;218m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;174;185;216m▄\x1b[0m\x1b[48;2;214;252;242m\x1b[38;2;193;198;226m▄\x1b[0m\x1b[48;2;141;204;196m\x1b[38;2;220;222;238m▄\x1b[0m\x1b[48;2;85;165;168m\x1b[38;2;250;255;255m▄\x1b[0m\x1b[48;2;40;127;144m\x1b[38;2;234;252;251m▄\x1b[0m\x1b[48;2;0;87;124m\x1b[38;2;165;201;216m▄\x1b[0m\x1b[48;2;0;8;85m\x1b[38;2;111;151;183m▄\x1b[0m\x1b[38;2;64;106;165m▄\x1b[0m\x1b[38;2;20;65;137m▄\x1b[0m     \n" +
		"  \x1b[48;2;7;0;85m\x1b[38;2;0;13;104m▄\x1b[0m\x1b[48;2;99;131;181m\x1b[38;2;169;185;218m▄\x1b[0m\x1b[48;2;253;255;255m\x1b[38;2;240;237;246m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;143;160;197m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;176;187;215m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;163;175;210m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;117;137;188m▄\x1b[0m\x1b[48;2;212;221;233m\x1b[38;2;89;118;174m▄\x1b[0m\x1b[48;2;134;154;196m\x1b[38;2;111;150;182m▄\x1b[0m\x1b[48;2;96;120;179m\x1b[38;2;109;178;179m▄\x1b[0m\x1b[48;2;96;125;178m\x1b[38;2;82;183;163m▄\x1b[0m\x1b[48;2;95;133;177m\x1b[38;2;71;184;157m▄\x1b[0m\x1b[48;2;96;139;178m\x1b[38;2;68;187;156m▄\x1b[0m\x1b[48;2;95;135;178m\x1b[38;2;66;186;153m▄\x1b[0m\x1b[48;2;90;121;175m\x1b[38;2;82;188;160m▄\x1b[0m\x1b[48;2;93;117;178m\x1b[38;2;120;186;182m▄\x1b[0m\x1b[48;2;169;182;214m\x1b[38;2;85;120;172m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;106;133;182m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;248;249;250m▄\x1b[0m\x1b[48;2;251;255;255m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;75;108;164m\x1b[38;2;126;150;189m▄\x1b[0m\x1b[38;2;0;0;86m▄\x1b[0m       ____                    ____       _       \n" +
		"  \x1b[48;2;13;50;130m\x1b[38;2;23;67;134m▄\x1b[0m\x1b[48;2;162;195;215m\x1b[38;2;212;226;240m▄\x1b[0m\x1b[48;2;196;231;222m\x1b[38;2;164;213;206m▄\x1b[0m\x1b[48;2;117;159;183m\x1b[38;2;69;147;158m▄\x1b[0m\x1b[48;2;139;177;194m\x1b[38;2;106;157;177m▄\x1b[0m\x1b[48;2;102;157;176m\x1b[38;2;133;185;191m▄\x1b[0m\x1b[48;2;104;166;177m\x1b[38;2;147;206;196m▄\x1b[0m\x1b[48;2;116;190;181m\x1b[38;2;53;164;149m▄\x1b[0m\x1b[48;2;63;175;153m\x1b[38;2;87;181;168m▄\x1b[0m\x1b[48;2;60;172;152m\x1b[38;2;238;247;245m▄\x1b[0m\x1b[48;2;133;201;192m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;190;226;221m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;208;234;229m\x1b[38;2;241;246;251m▄\x1b[0m\x1b[48;2;190;228;219m\x1b[38;2;212;230;240m▄\x1b[0m\x1b[48;2;116;198;179m\x1b[38;2;239;244;251m▄\x1b[0m\x1b[48;2;43;173;138m\x1b[38;2;166;219;203m▄\x1b[0m\x1b[48;2;118;211;175m\x1b[38;2;40;173;133m▄\x1b[0m\x1b[48;2;91;128;172m\x1b[38;2;131;202;183m▄\x1b[0m\x1b[48;2;142;163;199m\x1b[38;2;101;124;181m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;160;181;213m\x1b[38;2;192;208;231m▄\x1b[0m\x1b[48;2;0;27;111m\x1b[38;2;2;52;127m▄\x1b[0m      |  _ \\ ___  _ __  _   _ / ___| __ _| |_ ___ \n" +
		"  \x1b[48;2;39;82;141m\x1b[38;2;54;103;150m▄\x1b[0m\x1b[48;2;235;241;251m\x1b[38;2;240;246;253m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;243;240;249m\x1b[38;2;235;241;244m▄\x1b[0m\x1b[48;2;116;139;187m\x1b[38;2;92;118;176m▄\x1b[0m\x1b[48;2;92;117;175m\x1b[38;2;127;188;188m▄\x1b[0m\x1b[48;2;115;193;181m\x1b[38;2;31;158;138m▄\x1b[0m\x1b[48;2;48;165;148m\x1b[38;2;110;185;180m▄\x1b[0m\x1b[48;2;230;244;242m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;247;250;253m\x1b[38;2;243;248;249m▄\x1b[0m\x1b[48;2;159;206;221m\x1b[38;2;158;204;221m▄\x1b[0m\x1b[48;2;122;189;211m\x1b[38;2;124;176;204m▄\x1b[0m\x1b[48;2;131;189;212m\x1b[38;2;62;139;185m▄\x1b[0m\x1b[48;2;81;164;193m\x1b[38;2;98;170;202m▄\x1b[0m\x1b[48;2;223;236;245m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;234;248;240m\x1b[38;2;136;202;188m▄\x1b[0m\x1b[48;2;56;186;142m\x1b[38;2;15;98;132m▄\x1b[0m\x1b[48;2;62;140;153m\x1b[38;2;113;133;187m▄\x1b[0m\x1b[48;2;68;114;161m\x1b[38;2;121;191;180m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;150;217;194m▄\x1b[0m\x1b[48;2;213;225;242m\x1b[38;2;237;242;254m▄\x1b[0m\x1b[48;2;21;74;136m\x1b[38;2;32;98;141m▄\x1b[0m      | |_) / _ \\| '_ \\| | | | |  _ / _` | __/ _ \\\n" +
		"  \x1b[48;2;62;120;152m\x1b[38;2;62;128;151m▄\x1b[0m\x1b[48;2;240;248;253m\x1b[38;2;244;253;254m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;247;247;250m▄\x1b[0m\x1b[48;2;137;157;197m\x1b[38;2;84;114;173m▄\x1b[0m\x1b[48;2;112;150;184m\x1b[38;2;175;199;213m▄\x1b[0m\x1b[48;2;76;189;160m\x1b[38;2;104;167;177m▄\x1b[0m\x1b[48;2;37;165;142m\x1b[38;2;105;166;177m▄\x1b[0m\x1b[48;2;102;174;175m\x1b[38;2;61;145;152m▄\x1b[0m\x1b[48;2;239;245;246m\x1b[38;2;215;232;233m▄\x1b[0m\x1b[48;2;255;254;254m\x1b[38;2;254;255;255m▄\x1b[0m\x1b[48;2;255;255;254m\x1b[38;2;248;250;250m▄\x1b[0m\x1b[48;2;161;190;212m\x1b[38;2;255;255;254m▄\x1b[0m\x1b[48;2;127;175;206m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;188;218;234m\x1b[38;2;220;226;237m▄\x1b[0m\x1b[48;2;229;236;239m\x1b[38;2;38;78;149m▄\x1b[0m\x1b[48;2;56;104;153m\x1b[38;2;89;116;171m▄\x1b[0m\x1b[48;2;170;181;213m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;243;246;248m▄\x1b[0m\x1b[48;2;252;255;253m\x1b[38;2;255;254;255m▄\x1b[0m\x1b[48;2;97;193;166m\x1b[38;2;158;216;201m▄\x1b[0m\x1b[48;2;194;229;230m\x1b[38;2;128;205;194m▄\x1b[0m\x1b[48;2;58;124;152m\x1b[38;2;78;143;161m▄\x1b[0m      |  __/ (_) | |_) | |_| | |_| | (_| | ||  __/\n" +
		"  \x1b[48;2;50;130;144m\x1b[38;2;30;130;134m▄\x1b[0m\x1b[48;2;247;255;255m\x1b[38;2;230;253;249m▄\x1b[0m\x1b[48;2;198;206;227m\x1b[38;2;151;166;205m▄\x1b[0m\x1b[48;2;83;115;171m\x1b[38;2;95;126;178m▄\x1b[0m\x1b[48;2;146;163;199m\x1b[38;2;102;128;179m▄\x1b[0m\x1b[48;2;81;113;166m\x1b[38;2;89;130;166m▄\x1b[0m\x1b[48;2;63;90;157m\x1b[38;2;69;120;155m▄\x1b[0m\x1b[48;2;42;115;143m\x1b[38;2;42;102;145m▄\x1b[0m\x1b[48;2;136;205;190m\x1b[38;2;43;154;142m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;194;233;220m▄\x1b[0m\x1b[48;2;247;249;250m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;251;253;253m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;211;219;233m\x1b[38;2;103;132;180m▄\x1b[0m\x1b[48;2;70;106;167m\x1b[38;2;172;188;214m▄\x1b[0m\x1b[48;2;175;191;215m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;56;93;157m\x1b[38;2;140;162;198m▄\x1b[0m\x1b[48;2;179;193;217m\x1b[38;2;31;74;146m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;169;186;213m▄\x1b[0m\x1b[48;2;254;254;254m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;192;229;220m\x1b[38;2;187;227;219m▄\x1b[0m\x1b[48;2;99;193;179m\x1b[38;2;82;185;169m▄\x1b[0m\x1b[48;2;70;151;154m\x1b[38;2;62;156;150m▄\x1b[0m      |_|   \\___/| .__/ \\__,_|\\____|\\__,_|\\__\\___| \n" +
		"  \x1b[48;2;5;127;119m\x1b[38;2;0;143;103m▄\x1b[0m\x1b[48;2;187;231;224m\x1b[38;2;103;179;173m▄\x1b[0m\x1b[48;2;117;137;188m\x1b[38;2;93;123;179m▄\x1b[0m\x1b[48;2;166;185;213m\x1b[38;2;237;241;245m▄\x1b[0m\x1b[48;2;61;97;161m\x1b[38;2;52;90;158m▄\x1b[0m\x1b[48;2;46;108;142m\x1b[38;2;27;94;138m▄\x1b[0m\x1b[48;2;61;115;150m\x1b[38;2;30;102;140m▄\x1b[0m\x1b[48;2;73;116;157m\x1b[38;2;59;114;152m▄\x1b[0m\x1b[48;2;28;105;138m\x1b[38;2;78;122;160m▄\x1b[0m\x1b[48;2;52;164;146m\x1b[38;2;43;104;144m▄\x1b[0m\x1b[48;2;154;217;199m\x1b[38;2;53;147;148m▄\x1b[0m\x1b[48;2;233;246;243m\x1b[38;2;91;198;165m▄\x1b[0m\x1b[48;2;94;123;177m\x1b[38;2;41;117;148m▄\x1b[0m\x1b[48;2;233;238;243m\x1b[38;2;146;159;203m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;233;237;245m▄\x1b[0m\x1b[48;2;245;247;249m\x1b[38;2;190;202;223m▄\x1b[0m\x1b[48;2;116;143;186m\x1b[38;2;114;137;185m▄\x1b[0m\x1b[48;2;0;47;129m\x1b[38;2;110;133;185m▄\x1b[0m\x1b[48;2;195;201;226m\x1b[38;2;83;129;169m▄\x1b[0m\x1b[48;2;153;211;201m\x1b[38;2;111;185;181m▄\x1b[0m\x1b[48;2;75;174;161m\x1b[38;2;95;180;167m▄\x1b[0m\x1b[38;2;47;155;141m▀\x1b[0m                 |_|                               \n" +
		"   \x1b[48;2;23;118;136m\x1b[38;2;2;79;127m▄\x1b[0m\x1b[48;2;135;160;204m\x1b[38;2;157;211;210m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;106;134;186m\x1b[38;2;195;208;225m▄\x1b[0m\x1b[48;2;28;76;143m\x1b[38;2;48;85;155m▄\x1b[0m\x1b[48;2;65;110;154m\x1b[38;2;138;169;188m▄\x1b[0m\x1b[48;2;58;110;153m\x1b[38;2;118;160;178m▄\x1b[0m\x1b[48;2;76;111;158m\x1b[38;2;111;155;175m▄\x1b[0m\x1b[48;2;79;105;154m\x1b[38;2;121;168;177m▄\x1b[0m\x1b[48;2;137;189;185m\x1b[38;2;116;196;174m▄\x1b[0m\x1b[48;2;140;204;187m\x1b[38;2;125;195;177m▄\x1b[0m\x1b[48;2;71;180;156m\x1b[38;2;102;201;171m▄\x1b[0m\x1b[48;2;24;99;137m\x1b[38;2;24;112;129m▄\x1b[0m\x1b[48;2;57;92;154m\x1b[38;2;176;182;209m▄\x1b[0m\x1b[48;2;103;131;181m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;164;193;208m\x1b[38;2;104;195;170m▄\x1b[0m\x1b[48;2;190;212;220m\x1b[38;2;196;220;223m▄\x1b[0m\x1b[48;2;56;116;152m\x1b[38;2;243;245;249m▄\x1b[0m\x1b[48;2;182;228;224m\x1b[38;2;138;197;197m▄\x1b[0m\x1b[48;2;63;161;153m\x1b[38;2;0;108;103m▄\x1b[0m     \n" +
		"    \x1b[48;2;60;172;148m\x1b[38;2;0;130;104m▄\x1b[0m\x1b[48;2;228;250;246m\x1b[38;2;98;188;174m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;253;255;255m▄\x1b[0m\x1b[48;2;105;131;184m\x1b[38;2;230;235;242m▄\x1b[0m\x1b[48;2;105;152;178m\x1b[38;2;69;101;167m▄\x1b[0m\x1b[48;2;151;209;192m\x1b[38;2;117;140;183m▄\x1b[0m\x1b[48;2;141;193;189m\x1b[38;2;54;107;146m▄\x1b[0m\x1b[48;2;108;183;174m\x1b[38;2;79;130;164m▄\x1b[0m\x1b[48;2;97;180;172m\x1b[38;2;104;163;181m▄\x1b[0m\x1b[48;2;98;183;169m\x1b[38;2;106;163;180m▄\x1b[0m\x1b[48;2;100;198;169m\x1b[38;2;72;145;156m▄\x1b[0m\x1b[48;2;31;112;142m\x1b[38;2;39;128;144m▄\x1b[0m\x1b[48;2;222;215;240m\x1b[38;2;85;141;167m▄\x1b[0m\x1b[48;2;134;208;187m\x1b[38;2;111;197;175m▄\x1b[0m\x1b[48;2;145;204;193m\x1b[38;2;253;255;255m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;241;249;251m▄\x1b[0m\x1b[48;2;212;237;240m\x1b[38;2;72;136;160m▄\x1b[0m\x1b[38;2;35;125;138m▀\x1b[0m        PopuGate Server __VERSION__\n" +
		"     \x1b[38;2;6;136;124m▀\x1b[0m\x1b[48;2;125;197;194m\x1b[38;2;9;127;127m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;123;183;194m▄\x1b[0m\x1b[48;2;211;220;233m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;58;95;160m\x1b[38;2;223;228;238m▄\x1b[0m\x1b[48;2;99;133;175m\x1b[38;2;80;113;171m▄\x1b[0m\x1b[48;2;68;119;155m\x1b[38;2;86;119;172m▄\x1b[0m\x1b[48;2;44;104;142m\x1b[38;2;89;129;166m▄\x1b[0m\x1b[48;2;45;100;142m\x1b[38;2;104;149;174m▄\x1b[0m\x1b[48;2;79;122;161m\x1b[38;2;103;181;173m▄\x1b[0m\x1b[48;2;53;159;147m\x1b[38;2;115;195;179m▄\x1b[0m\x1b[48;2;63;134;158m\x1b[38;2;251;255;252m▄\x1b[0m\x1b[48;2;241;248;246m\x1b[38;2;249;250;255m▄\x1b[0m\x1b[48;2;252;255;255m\x1b[38;2;94;130;176m▄\x1b[0m\x1b[48;2;94;138;174m\x1b[38;2;0;34;108m▄\x1b[0m\x1b[38;2;0;51;106m▀\x1b[0m         __URL__\n" +
		"       \x1b[38;2;0;92;123m▀\x1b[0m\x1b[48;2;94;145;180m\x1b[38;2;0;56;117m▄\x1b[0m\x1b[48;2;240;248;254m\x1b[38;2;59;131;163m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;177;222;228m▄\x1b[0m\x1b[48;2;141;161;201m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;76;118;169m\x1b[38;2;245;243;249m▄\x1b[0m\x1b[48;2;99;183;172m\x1b[38;2;252;249;253m▄\x1b[0m\x1b[48;2;168;215;208m\x1b[38;2;255;255;255m▄\x1b[0m\x1b[48;2;255;255;255m\x1b[38;2;160;213;213m▄\x1b[0m\x1b[48;2;224;237;246m\x1b[38;2;46;125;148m▄\x1b[0m\x1b[48;2;78;122;167m\x1b[38;2;0;36;94m▄\x1b[0m\x1b[38;2;0;30;108m▀\x1b[0m         \n" +
		"          \x1b[38;2;19;138;143m▀\x1b[0m\x1b[38;2;91;183;172m▀\x1b[0m\x1b[48;2;183;233;223m\x1b[38;2;17;157;130m▄\x1b[0m\x1b[48;2;172;225;214m\x1b[38;2;13;152;127m▄\x1b[0m\x1b[38;2;79;178;163m▀\x1b[0m\x1b[38;2;5;131;126m▀\x1b[0m            \n" +
		""
	banner = strings.ReplaceAll(banner, "__VERSION__", model.VersionTag())
	banner = strings.ReplaceAll(banner, "__URL__", model.VersionURL())
	fmt.Print(banner)
	fmt.Println()
}

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
