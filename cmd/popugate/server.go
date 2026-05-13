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
	// Resolve data directory
	defaultDataDir := resolveDataDir()
	dataDir, _ := cmd.Flags().GetString("data")
	if dataDir == "" {
		dataDir = defaultDataDir
	}

	port, _ := cmd.Flags().GetInt("port")
	dbPath, _ := cmd.Flags().GetString("db")
	if dbPath == "" {
		dbPath = filepath.Join(dataDir, "settings.db")
	}

	model.InstallDir = dataDir

	// Open database
	db, err := database.Open(database.Config{Path: dbPath})
	if err != nil {
		logger.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Initialize stores
	settingsStore := store.NewSettingsStore(db)
	secretStore := store.NewSecretStore(db)
	upstreamStore := store.NewUpstreamStore(db)
	instanceStore := store.NewInstanceStore(db)
	slaveStore := store.NewSlaveStore(db)
	trafficStore := store.NewTrafficStore(db)
	blocklistStore := store.NewTokenBlocklistStore(db)
	quotaStore := store.NewQuotaAlertStore(db)
	geoblockCache := store.NewGeoblockCacheStore(db)
	// Resolve backup encryption key from environment
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
	backupStore := store.NewBackupStore(dataDir, backupEncKey)
	schedulerStore := store.NewSchedulerStore(db)
	auditStore := store.NewAuditStore(db)
	templateStore := store.NewTemplateStore(db)

	// Get JWT secret (auto-generated on first run)
	jwtSecret, err := settingsStore.GetJWTSecret(context.Background())
	_ = jwtSecret
	if err != nil {
		logger.Fatalf("Failed to get JWT secret: %v", err)
	}

	// Docker client
	dockerClient, err := dockerutil.NewDockerClient()
	if err != nil {
		srvLog.Warnf("Docker client unavailable: %v", err)
	}
	if dockerClient != nil {
		defer dockerClient.Close()
	}

	// Initialize services
	secretSvc := service.NewSecretService(secretStore, instanceStore, settingsStore)
	upstreamSvc := service.NewUpstreamService(upstreamStore)
	trafficSvc := service.NewTrafficService(trafficStore, settingsStore, dockerClient, instanceStore)
	trafficSvc.SetSecretStore(secretStore, quotaStore)
	geoblockSvc := service.NewGeoblockService(settingsStore, instanceStore, geoblockCache)
	updateSvc := service.NewUpdateService(dockerClient)
	telemtCfg := service.NewDBTelemtConfig(settingsStore)
	var dockerSvc *service.DockerService
	var containerSvc *service.ContainerService
	var healthSvc *service.HealthService
	var replSvc *service.ReplicationService
	var telemtUpdateSvc *service.TelemtUpdateService
	if dockerClient != nil {
		dockerSvc = service.NewDockerService(dockerClient, telemtCfg)
		containerSvc = service.NewContainerService(
			dockerClient, secretStore, upstreamStore, instanceStore, trafficStore, settingsStore, trafficSvc,
		)
		healthSvc = service.NewHealthService(dockerClient, settingsStore, instanceStore)
		healthSvc.SetContainerSvc(containerSvc)
		replSvc = service.NewReplicationService(settingsStore, slaveStore, instanceStore)
		telemtUpdateSvc = service.NewTelemtUpdateService(settingsStore, dockerSvc, containerSvc, telemtCfg)
		telemtUpdateSvc.ResetStaleUpdate(context.Background())
	}

	// Bot setup
	var activeBot *bot.Bot
	var botMu sync.Mutex
	botCtx, botCancel := context.WithCancel(context.Background())
	defer botCancel()

	botDeps := &bot.Dependencies{
		Settings:  settingsStore,
		Secrets:   secretStore,
		Upstreams: upstreamStore,
		Traffic:   trafficStore,
		Instances: instanceStore,
	}
	if containerSvc != nil {
		botDeps.RestartProxy = func(ctx context.Context) error { return containerSvc.Restart(ctx) }
		botDeps.IsProxyRunning = func(ctx context.Context) bool {
			status, err := containerSvc.Status(ctx)
			return err == nil && status != nil && status.Running
		}
		botDeps.GetUptime = func(ctx context.Context) string {
			status, err := containerSvc.Status(ctx)
			if err != nil || status == nil {
				return ""
			}
			return status.Uptime
		}
		botDeps.IsInstanceRunning = func(ctx context.Context, containerName string) bool {
			r, err := dockerClient.IsInstanceRunning(ctx, containerName)
			return err == nil && r
		}
		botDeps.StartInstance = func(ctx context.Context, id int64) error {
			return containerSvc.StartInstance(ctx, id)
		}
		botDeps.StopInstance = func(ctx context.Context, id int64) error {
			return containerSvc.StopInstance(ctx, id)
		}
	}
	if dockerSvc != nil {
		botDeps.GetEngineVersion = dockerSvc.GetInstalledVersion
	}
	botDeps.GenerateQR = func(ctx context.Context, link string) ([]byte, error) {
		return qrutil.GeneratePNG(link, 256)
	}

	startBotIfNeeded(botCtx, settingsStore, botDeps, &activeBot, &botMu)

	// Notification callback — used by services and scheduler to send alerts via Telegram bot.
	// Resolves the server label and checks TelegramAlertsEnabled in a single settings load.
	notifyFn := func(ctx context.Context, format string, args ...any) {
		botMu.Lock()
		b := activeBot
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

	if containerSvc != nil {
		containerSvc.SetNotify(notifyFn)
	}
	if telemtUpdateSvc != nil {
		telemtUpdateSvc.SetNotify(notifyFn)
	}
	if upstreamSvc != nil {
		upstreamSvc.SetNotify(notifyFn)
	}

	// Get settings from DB
	ctx := context.Background()
	settings, err := settingsStore.Load(ctx)
	if err != nil {
		srvLog.Warnf("Failed to load settings: %v", err)
	}

	// Seed default instance if table is empty (migration from single-port to multi-port)
	if settings != nil {
		if err := instanceStore.EnsureDefaultInstance(ctx, settings.ProxyPort, settings.ProxyMetricsPort, settings.ProxyDomain, settings.MaskingHost, settings.MaskingEnabled); err != nil {
			srvLog.Warnf("Failed to seed default instance: %v", err)
		}
	}
	isDebug := settings.Debug

	// Override from environment
	if os.Getenv("DEBUG") == "true" || os.Getenv("GIN_MODE") == "debug" {
		isDebug = true
	} else if os.Getenv("DEBUG") == "false" || os.Getenv("GIN_MODE") == "release" {
		isDebug = false
	}

	// Setup router
	cachedJWTProvider := api.NewCachedJWTSecretProvider(settingsStore, 5*time.Minute)

	// Create services needed by scheduler
	auditSvc := service.NewAuditService(auditStore)

	// Resource monitor for WebSocket and background threshold checks
	service.InitResourceMonitor(notifyFn)

	// Prepare scheduler tasks (wire Fn callbacks)
	sched, tasks := prepareSchedulerTasks(trafficSvc, healthSvc, replSvc, blocklistStore, settingsStore, secretStore, backupStore, &activeBot, &botMu, updateSvc, telemtUpdateSvc, telemtCfg, notifyFn, trafficStore, secretSvc, upstreamSvc, auditSvc)

	// Load overrides and start scheduler with execution tracking
	overrides, _ := schedulerStore.GetOverrides(ctx)
	overrideMap := make(map[string]scheduler.TaskOverride)
	for _, o := range overrides {
		overrideMap[o.TaskName] = o
	}
	sched.StartWith(tasks, overrideMap, schedulerStore)
	defer sched.Stop()

	// Create scheduler service (needs live scheduler + store)
	schedulerSvc := service.NewSchedulerService(schedulerStore, sched)
	templateSvc := service.NewTemplateService(templateStore, secretStore)

	// Wire scheduler status callback for bot
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
			if rec, _ := schedulerStore.GetLatestHistory(ctx, t.Name); rec != nil {
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

	router := api.SetupRouter(api.RouterConfig{
		Debug:           isDebug,
		JWTSecret:       cachedJWTProvider,
		Settings:        settingsStore,
		Secrets:         secretStore,
		Upstreams:       upstreamStore,
		Instances:       instanceStore,
		Slaves:          slaveStore,
		Traffic:         trafficStore,
		Blocklist:       blocklistStore,
		Backups:         backupStore,
		Docker:          dockerClient,
		SecretSvc:       secretSvc,
		UpstreamSvc:     upstreamSvc,
		ContainerSvc:    containerSvc,
		DockerSvc:       dockerSvc,
		GeoblockSvc:     geoblockSvc,
		BotDeps:         botDeps,
		HealthSvc:       healthSvc,
		TrafficSvc:      trafficSvc,
		ReplSvc:         replSvc,
		UpdateSvc:       updateSvc,
		TelemtUpdateSvc: telemtUpdateSvc,
		TelemtCfg:       telemtCfg,
		SchedulerSvc:    schedulerSvc,
		AuditSvc:        auditSvc,
		TemplateSvc:     templateSvc,
	})

	// HTTP server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: router,
	}

	// Graceful shutdown
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		srvLog.Infof("Shutting down server...")

		botCancel()
		if activeBot != nil {
			activeBot.Stop()
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

func prepareSchedulerTasks(
	trafficSvc *service.TrafficService,
	healthSvc *service.HealthService,
	replSvc *service.ReplicationService,
	blocklist *store.TokenBlocklistStore,
	settings *store.SettingsStore,
	secrets *store.SecretStore,
	backupStore *store.BackupStore,
	activeBot **bot.Bot,
	botMu *sync.Mutex,
	updateSvc *service.UpdateService,
	telemtUpdateSvc *service.TelemtUpdateService,
	telemtCfg *service.DBTelemtConfig,
	notify service.NotifyFunc,
	trafficStore *store.TrafficStore,
	secretSvc *service.SecretService,
	upstreamSvc *service.UpstreamService,
	auditSvc *service.AuditService,
) (*scheduler.Scheduler, []scheduler.Task) {
	sched := scheduler.New()
	tasks := scheduler.DefaultTasks()

	for i := range tasks {
		switch tasks[i].Name {
		case "traffic-flush":
			if trafficSvc != nil {
				tasks[i].Fn = func(ctx context.Context) error {
					return trafficSvc.Flush(ctx)
				}
			}
		case "quota-check":
			if trafficSvc != nil {
				tasks[i].Fn = func(ctx context.Context) error {
					trafficSvc.CheckQuotas(ctx)
					return nil
				}
			}
		case "expiry-check":
			if trafficSvc != nil {
				tasks[i].Fn = func(ctx context.Context) error {
					trafficSvc.CheckExpirations(ctx)
					return nil
				}
			}
		case "health-check":
			if healthSvc != nil {
				tasks[i].Fn = healthSvc.AutoRecover
			}
		case "replication-sync":
			if replSvc != nil {
				tasks[i].Fn = func(ctx context.Context) error {
					results := replSvc.SyncAll(ctx)
					for _, r := range results {
						if r.Error != "" {
							return fmt.Errorf("sync %s: %s", r.Host, r.Error)
						}
					}
					return nil
				}
			}
		case "token-cleanup":
			if blocklist != nil {
				tasks[i].Fn = blocklist.Cleanup
			}
		case "daily-backup":
			if backupStore != nil {
				tasks[i].Fn = func(ctx context.Context) error {
					_, err := backupStore.Create(ctx)
					return err
				}
			}
		case "backup-cleanup":
			if backupStore != nil {
				tasks[i].Fn = func(ctx context.Context) error {
					s, _ := settings.Load(ctx)
					days := 7
					if s != nil && s.BackupRetentionDays > 0 {
						days = s.BackupRetentionDays
					}
					_, err := backupStore.CleanOld(ctx, time.Duration(days)*24*time.Hour)
					return err
				}
			}
		case "telegram-report":
			tasks[i].Fn = func(ctx context.Context) error {
				botMu.Lock()
				botPtr := *activeBot
				botMu.Unlock()
				if botPtr == nil || !botPtr.IsRunning() {
					return nil
				}
				return sendPeriodicReport(ctx, botPtr, settings, secrets, trafficSvc)
			}
		case "update-check":
			if updateSvc != nil {
				tasks[i].Fn = func(ctx context.Context) error {
					status, err := updateSvc.Check(ctx)
					if err != nil {
						return err
					}
					if status.UpdateAvailable {
						srvLog.Infof("update available: v%s (current: v%s)", status.Latest, status.Current)
						notify(ctx, "🆕 *%s* New PopuGate version available: v%s\nCurrent: v%s", status.Latest, status.Current)
					}
					return nil
				}
			}
		case "telemt-check":
			if telemtUpdateSvc != nil {
				tasks[i].Fn = func(ctx context.Context) error {
					release, err := telemtUpdateSvc.CheckRemote(ctx)
					if err != nil {
						return err
					}
					if release.Version != telemtCfg.TelemtVersion() {
						srvLog.Infof("telemt update available: %s (current: %s)", release.Version, telemtCfg.TelemtVersion())
						notify(ctx, "🆕 *%s* New telemt engine version available: %s\nCurrent: %s", release.Version, telemtCfg.TelemtVersion())
					}
					return nil
				}
			}
		case "history-cleanup":
			tasks[i].Fn = func(ctx context.Context) error {
				return trafficStore.CleanOldHistory(ctx, 30*24*time.Hour)
			}
		case "quota-reset":
			if trafficSvc != nil {
				tasks[i].Fn = func(ctx context.Context) error {
					trafficSvc.ResetAllQuotas(ctx)
					if auditSvc != nil {
						auditSvc.Log(ctx, "system", "quota-reset", "monthly quota reset completed")
					}
					return nil
				}
			}
		case "auto-rotate":
			if secretSvc != nil {
				tasks[i].Fn = func(ctx context.Context) error {
					s, _ := settings.Load(ctx)
					if s == nil || s.SecretAutoRotateDays <= 0 {
						return nil
					}
					all, err := secrets.List(ctx)
					if err != nil {
						return err
					}
					cutoff := time.Now().AddDate(0, 0, -s.SecretAutoRotateDays).Unix()
					rotated := 0
					for _, sec := range all {
						if sec.Enabled && sec.CreatedAt > 0 && sec.CreatedAt < cutoff {
							if _, err := secretSvc.Rotate(ctx, sec.Label); err == nil {
								rotated++
							}
						}
					}
					if rotated > 0 && auditSvc != nil {
						auditSvc.Log(ctx, "system", "auto-rotate", fmt.Sprintf("rotated %d secret(s)", rotated))
					}
					return nil
				}
			}
		case "upstream-health":
			if upstreamSvc != nil {
				tasks[i].Fn = func(ctx context.Context) error {
					return upstreamSvc.CheckAllUpstreams(ctx)
				}
			}
		}
	}

	return sched, tasks
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
	fmt.Printf(" PopuGate Server %s\n", model.Version)
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
