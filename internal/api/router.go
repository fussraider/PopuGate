package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fussraider/PopuGate/internal/api/handler"
	"github.com/fussraider/PopuGate/internal/bot"
	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/logger"
	"golang.org/x/time/rate"
)

// RouterConfig holds dependencies for router setup.
type RouterConfig struct {
	Debug     bool
	JWTSecret JWTSecretProvider
	Settings  *store.SettingsStore
	Secrets   *store.SecretStore
	Upstreams *store.UpstreamStore
	Instances *store.InstanceStore
	Slaves    *store.SlaveStore
	Traffic   *store.TrafficStore
	Blocklist *store.TokenBlocklistStore
	Backups   *store.BackupStore
	Docker    *dockerutil.DockerClient
	// Services
	SecretSvc    *service.SecretService
	UpstreamSvc  *service.UpstreamService
	ContainerSvc *service.ContainerService
	DockerSvc    *service.DockerService
	GeoblockSvc  *service.GeoblockService
	BotDeps      *bot.Dependencies
	HealthSvc    *service.HealthService
	TrafficSvc   *service.TrafficService
	ReplSvc      *service.ReplicationService
	UpdateSvc    *service.UpdateService
	CORSOrigins  []string // defaults to ["*"] if empty
}

// SetupRouter creates and configures the Gin router.
func SetupRouter(cfg RouterConfig) *gin.Engine {
	if cfg.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(logger.GinLogger(), gin.Recovery())

	// Limit request body size to 2MB
	r.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
		c.Next()
	})

	// CORS
	origins := cfg.CORSOrigins
	if len(origins) == 0 {
		origins = []string{"*"}
	}
	r.Use(CORSMiddleware(origins))

	// Health endpoint (no auth)
	healthHandler := handler.NewHealthHandler()
	healthHandler.SetHealthService(cfg.HealthSvc)
	r.GET("/api/v1/health", healthHandler.Check)

	// Auth endpoints (no auth, rate-limited)
	authLimiter := NewIPRateLimiter(rate.Every(0), 10) // 10 requests per second per IP
	authHandler := handler.NewAuthHandler(cfg.Settings, cfg.Blocklist)
	auth := r.Group("/api/v1/auth")
	auth.Use(RateLimitMiddleware(authLimiter))
	{
		auth.POST("/setup", authHandler.Setup)
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh)
		auth.POST("/logout", authHandler.Logout)
		auth.PUT("/password", AuthMiddleware(cfg.JWTSecret, cfg.Blocklist), authHandler.ChangePassword)
	}

	// Protected endpoints
	protected := r.Group("/api/v1")
	protected.Use(AuthMiddleware(cfg.JWTSecret, cfg.Blocklist))
	{
		// Config
		configHandler := handler.NewConfigHandler(cfg.Settings)
		protected.GET("/config", configHandler.GetAll)
		protected.PUT("/config", configHandler.Update)
		protected.GET("/config/:key", configHandler.GetKey)

		// Secrets
		secretHandler := handler.NewSecretHandler(cfg.SecretSvc, cfg.Settings)
		protected.GET("/secrets", secretHandler.List)
		protected.POST("/secrets", secretHandler.Add)
		protected.GET("/secrets/:label", secretHandler.Get)
		protected.DELETE("/secrets/:label", secretHandler.Remove)
		protected.POST("/secrets/:label/rotate", secretHandler.Rotate)
		protected.PUT("/secrets/:label/toggle", secretHandler.Toggle)
		protected.PUT("/secrets/:label/limits", secretHandler.SetLimits)
		protected.GET("/secrets/:label/limits", secretHandler.GetLimits)
		protected.GET("/secrets/:label/link", secretHandler.GetLink)
		protected.GET("/secrets/:label/qr", secretHandler.GetQR)
		protected.PUT("/secrets/:label/notes", secretHandler.UpdateNotes)
		protected.POST("/secrets/:label/reset-traffic", secretHandler.ResetTraffic)
		protected.POST("/secrets/reset-traffic", secretHandler.ResetAllTraffic)

		// Upstreams
		upstreamHandler := handler.NewUpstreamHandler(cfg.UpstreamSvc)
		protected.GET("/upstreams", upstreamHandler.List)
		protected.GET("/upstreams/interfaces", upstreamHandler.Interfaces)
		protected.POST("/upstreams/test", upstreamHandler.TestConfig)
		protected.POST("/upstreams", upstreamHandler.Add)
		protected.DELETE("/upstreams/:name", upstreamHandler.Remove)
		protected.PUT("/upstreams/:name/toggle", upstreamHandler.Toggle)
		protected.POST("/upstreams/:name/test", upstreamHandler.Test)

		// Instances
		instanceHandler := handler.NewInstanceHandler(cfg.Instances)
		protected.GET("/instances", instanceHandler.List)
		protected.POST("/instances", instanceHandler.Add)
		protected.DELETE("/instances/:port", instanceHandler.Remove)

		// Proxy control
		proxyHandler := handler.NewProxyHandler(cfg.ContainerSvc, cfg.Secrets, cfg.Settings)
		proxyHandler.SetDockerClient(cfg.Docker)
		protected.POST("/proxy/start", proxyHandler.Start)
		protected.POST("/proxy/stop", proxyHandler.Stop)
		protected.POST("/proxy/restart", proxyHandler.Restart)
		protected.POST("/proxy/reload", proxyHandler.Reload)
		protected.GET("/proxy/status", proxyHandler.Status)
		protected.GET("/proxy/logs", proxyHandler.Logs)

		// Docker/Engine
		dockerHandler := handler.NewDockerHandler(cfg.Docker, cfg.DockerSvc, cfg.Settings)
		protected.POST("/docker/install", dockerHandler.Install)
		protected.GET("/docker/status", dockerHandler.Status)
		protected.GET("/engine/status", dockerHandler.EngineStatus)
		protected.POST("/engine/build", dockerHandler.Build)

		// Geoblock
		geoblockHandler := handler.NewGeoblockHandler(cfg.Settings, cfg.GeoblockSvc)
		protected.GET("/geoblock", geoblockHandler.Get)
		protected.POST("/geoblock/add", geoblockHandler.Add)
		protected.POST("/geoblock/remove", geoblockHandler.Remove)
		protected.POST("/geoblock/clear", geoblockHandler.Clear)
		protected.PUT("/geoblock/mode", geoblockHandler.SetMode)

		// Traffic
		trafficHandler := handler.NewTrafficHandler(cfg.Traffic, cfg.Settings)
		trafficHandler.SetTrafficService(cfg.TrafficSvc)
		protected.GET("/traffic", trafficHandler.Get)
		protected.GET("/traffic/live", trafficHandler.GetLive)
		protected.GET("/traffic/:label", trafficHandler.GetUser)

		// Bot
		botHandler := handler.NewBotHandler(cfg.Settings, cfg.BotDeps)
		protected.POST("/bot/setup", botHandler.Setup)
		protected.POST("/bot/test", botHandler.Test)
		protected.GET("/bot/status", botHandler.Status)
		protected.PUT("/bot/toggle", botHandler.Toggle)
		protected.GET("/bot/detect-chat-id", botHandler.DetectChatID)

		// Replication
		replHandler := handler.NewReplicationHandler(cfg.Settings, cfg.Slaves)
		replHandler.SetReplicationService(cfg.ReplSvc)
		protected.GET("/replication/status", replHandler.Status)
		protected.POST("/replication/setup", replHandler.Setup)
		protected.POST("/replication/slaves", replHandler.AddSlave)
		protected.DELETE("/replication/slaves/:host", replHandler.RemoveSlave)
		protected.GET("/replication/slaves", replHandler.ListSlaves)
		protected.POST("/replication/sync", replHandler.Sync)
		protected.POST("/replication/test", replHandler.Test)
		protected.POST("/replication/ssh-keygen", replHandler.SSHKeygen)
		protected.GET("/replication/ssh-key", replHandler.GetSSHKey)

		// Update
		updateHandler := handler.NewUpdateHandler(cfg.UpdateSvc)
		protected.GET("/update/check", updateHandler.Check)
		protected.POST("/update/apply", updateHandler.Apply)

		// Backup
		backupHandler := handler.NewBackupHandler(cfg.Backups)
		protected.GET("/backups", backupHandler.List)
		protected.POST("/backups", backupHandler.Create)
		protected.POST("/backups/restore", backupHandler.Restore)
		protected.GET("/backups/download/:filename", backupHandler.Download)
		protected.DELETE("/backups/:filename", backupHandler.Delete)

		// System
		systemHandler := handler.NewSystemHandler()
		protected.GET("/system/os", systemHandler.GetOS)
		protected.POST("/system/service/install", systemHandler.InstallService)
		protected.DELETE("/system/service/uninstall", systemHandler.UninstallService)
		protected.GET("/system/service/status", systemHandler.ServiceStatus)
		protected.POST("/system/service/restart", systemHandler.RestartService)
		protected.POST("/system/service/reload", systemHandler.ReloadService)
	}

	return r
}
