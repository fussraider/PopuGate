package main

import (
	"context"
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
  -p, --port int       HTTP server port (default 8080)
  -d, --db string      SQLite database path
      --data string    Data directory`,
	Run: runServer,
}

func init() {
	serverCmd.Flags().IntP("port", "p", 8080, "HTTP server port")
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
	backupStore := store.NewBackupStore(dataDir)

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
	secretSvc := service.NewSecretService(secretStore)
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
		replSvc = service.NewReplicationService(settingsStore, slaveStore)
		telemtUpdateSvc = service.NewTelemtUpdateService(settingsStore, dockerSvc, containerSvc, telemtCfg)
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
			r, err := dockerClient.IsRunning(ctx)
			return err == nil && r
		}
		botDeps.GetUptime = func(ctx context.Context) string {
			status, err := containerSvc.Status(ctx)
			if err != nil || status == nil {
				return ""
			}
			return status.Uptime
		}
	}
	if dockerSvc != nil {
		botDeps.GetEngineVersion = dockerSvc.GetInstalledVersion
	}
	botDeps.GenerateQR = func(ctx context.Context, link string) ([]byte, error) {
		return qrutil.GeneratePNG(link, 256)
	}

	startBotIfNeeded(botCtx, settingsStore, botDeps, &activeBot, &botMu)

	// Get settings from DB
	ctx := context.Background()
	settings, err := settingsStore.Load(ctx)
	if err != nil {
		srvLog.Warnf("Failed to load settings: %v", err)
	}

	// Seed default instance if table is empty (migration from single-port to multi-port)
	if settings != nil {
		if err := instanceStore.EnsureDefaultInstance(ctx, settings.ProxyPort, settings.ProxyMetricsPort); err != nil {
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
	})

	// Start scheduler
	sched := setupScheduler(trafficSvc, healthSvc, replSvc, blocklistStore, settingsStore, secretStore, &activeBot, &botMu, updateSvc, telemtUpdateSvc, telemtCfg)
	defer sched.Stop()

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
	go b.Start(ctx)
	logger.WithScope("bot").Infof("started")
}

func setupScheduler(
	trafficSvc *service.TrafficService,
	healthSvc *service.HealthService,
	replSvc *service.ReplicationService,
	blocklist *store.TokenBlocklistStore,
	settings *store.SettingsStore,
	secrets *store.SecretStore,
	activeBot **bot.Bot,
	botMu *sync.Mutex,
	updateSvc *service.UpdateService,
	telemtUpdateSvc *service.TelemtUpdateService,
	telemtCfg *service.DBTelemtConfig,
) *scheduler.Scheduler {
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
					}
					return nil
				}
			}
		}
	}

	sched.Start(tasks)
	return sched
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
