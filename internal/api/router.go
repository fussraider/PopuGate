package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/fussraider/PopuGate/docs"
	_ "github.com/fussraider/PopuGate/docs"

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
	SecretSvc       *service.SecretService
	TemplateSvc     *service.TemplateService
	UpstreamSvc     *service.UpstreamService
	ContainerSvc    *service.ContainerService
	DockerSvc       *service.DockerService
	GeoblockSvc     *service.GeoblockService
	BotDeps         *bot.Dependencies
	ActiveBot       **bot.Bot
	BotMu           *sync.Mutex
	HealthSvc       *service.HealthService
	TrafficSvc      *service.TrafficService
	ReplSvc         *service.ReplicationService
	UpdateSvc       *service.UpdateService
	TelemtUpdateSvc *service.TelemtUpdateService
	DockerUpdateSvc *service.DockerUpdateService
	TelemtCfg       *service.DBTelemtConfig
	SchedulerSvc    *service.SchedulerService
	AuditSvc        *service.AuditService
	CORSOrigins     []string // defaults to ["*"] if empty
	AllowedHosts    []string // anti-phishing: reject unknown Host headers; empty = no enforcement
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

	// Anti-phishing: reject requests with unknown Host headers
	r.Use(HostMiddleware(cfg.AllowedHosts))

	// Limit request body size to 2MB (skip for WebSocket upgrades)
	r.Use(func(c *gin.Context) {
		upgrade := c.GetHeader("Upgrade")
		if !strings.EqualFold(upgrade, "websocket") {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
		}
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

	// Swagger UI (no auth) — clear hardcoded host so UI works behind reverse proxies
	docs.SwaggerInfo.Host = ""
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Auth endpoints (no auth, rate-limited)
	authLimiter := NewIPRateLimiter(rate.Every(time.Second), 10) // 10 requests per second per IP
	authHandler := handler.NewAuthHandler(cfg.Settings, cfg.Blocklist)
	auth := r.Group("/api/v1/auth")
	auth.Use(RateLimitMiddleware(authLimiter))
	{
		auth.POST("/setup", authHandler.Setup)
		auth.POST("/login", authHandler.Login)
		auth.POST("/refresh", authHandler.Refresh)
		auth.PUT("/password", AuthMiddleware(cfg.JWTSecret, cfg.Blocklist), authHandler.ChangePassword)
	}

	// Protected endpoints
	protected := r.Group("/api/v1")
	protected.Use(AuthMiddleware(cfg.JWTSecret, cfg.Blocklist))
	if cfg.AuditSvc != nil {
		protected.Use(func(c *gin.Context) {
			handler.SetAuditSvc(c, cfg.AuditSvc)
			c.Next()
		})
	}
	{
		// Config
		configHandler := handler.NewConfigHandler(cfg.Settings)
		configHandler.SetContainerSvc(cfg.ContainerSvc)
		protected.GET("/config", configHandler.GetAll)
		protected.PUT("/config", configHandler.Update)
		protected.GET("/config/:key", configHandler.GetKey)

		// Auth (authenticated)
		protected.POST("/auth/logout", authHandler.Logout)

		// Secrets — static paths before :label
		secretHandler := handler.NewSecretHandler(cfg.SecretSvc, cfg.Settings)
		secretHandler.SetContainerSvc(cfg.ContainerSvc)
		protected.GET("/secrets", secretHandler.List)
		protected.POST("/secrets", secretHandler.Add)
		protected.GET("/secrets/search", secretHandler.Search)
		protected.GET("/secrets/top", secretHandler.Top)
		protected.GET("/secrets/export", secretHandler.Export)
		protected.GET("/secrets/tags", secretHandler.ListTags)
		protected.GET("/secrets/by-tag/:tag", secretHandler.ListByTag)
		protected.POST("/secrets/import", secretHandler.Import)
		protected.POST("/secrets/bulk-extend", secretHandler.BulkExtend)
		protected.POST("/secrets/bulk-rotate", secretHandler.BulkRotate)
		protected.POST("/secrets/bulk-toggle", secretHandler.BulkToggle)
		protected.POST("/secrets/bulk-set-limits", secretHandler.BulkSetLimits)
		protected.POST("/secrets/reset-traffic", secretHandler.ResetAllTraffic)
		protected.POST("/secrets/disable-expired", secretHandler.DisableExpired)
		// Secrets — per-label paths
		protected.GET("/secrets/:label", secretHandler.Get)
		protected.DELETE("/secrets/:label", secretHandler.Remove)
		protected.POST("/secrets/:label/rotate", secretHandler.Rotate)
		protected.PUT("/secrets/:label/toggle", secretHandler.Toggle)
		protected.PUT("/secrets/:label/limits", secretHandler.SetLimits)
		protected.GET("/secrets/:label/limits", secretHandler.GetLimits)
		protected.GET("/secrets/:label/link", secretHandler.GetLink)
		protected.GET("/secrets/:label/qr", secretHandler.GetQR)
		protected.PUT("/secrets/:label/notes", secretHandler.UpdateNotes)
		protected.PUT("/secrets/:label/rename", secretHandler.Rename)
		protected.POST("/secrets/:label/extend", secretHandler.Extend)
		protected.PUT("/secrets/:label/tags", secretHandler.SetTags)
		protected.POST("/secrets/:label/archive", secretHandler.Archive)
		protected.POST("/secrets/:label/unarchive", secretHandler.Unarchive)
		protected.POST("/secrets/:label/clone", secretHandler.Clone)
		protected.POST("/secrets/:label/reset-traffic", secretHandler.ResetTraffic)

		// Upstreams
		upstreamHandler := handler.NewUpstreamHandler(cfg.UpstreamSvc)
		upstreamHandler.SetContainerSvc(cfg.ContainerSvc)
		protected.GET("/upstreams", upstreamHandler.List)
		protected.GET("/upstreams/interfaces", upstreamHandler.Interfaces)
		protected.POST("/upstreams/test", upstreamHandler.TestConfig)
		protected.POST("/upstreams/bulk-check", upstreamHandler.BulkCheck)
		protected.POST("/upstreams/bulk", upstreamHandler.BulkAdd)
		protected.POST("/upstreams", upstreamHandler.Add)
		protected.PUT("/upstreams/:name", upstreamHandler.Update)
		protected.DELETE("/upstreams/:name", upstreamHandler.Remove)
		protected.PUT("/upstreams/:name/toggle", upstreamHandler.Toggle)
		protected.POST("/upstreams/:name/test", upstreamHandler.Test)

		// Instances
		instanceHandler := handler.NewInstanceHandler(cfg.Instances)
		instanceHandler.SetContainerSvc(cfg.ContainerSvc)
		instanceHandler.SetDockerClient(cfg.Docker)
		protected.GET("/instances", instanceHandler.List)
		protected.POST("/instances", instanceHandler.Add)
		protected.GET("/instances/check-port", instanceHandler.CheckPort)
		protected.PUT("/instances/:id", instanceHandler.Update)
		protected.DELETE("/instances/:id", instanceHandler.Remove)
		protected.POST("/instances/:id/start", instanceHandler.StartInstance)
		protected.POST("/instances/:id/stop", instanceHandler.StopInstance)
		protected.POST("/instances/:id/reload", instanceHandler.ReloadInstance)
		protected.POST("/instances/:id/restart", instanceHandler.RestartInstance)
		protected.POST("/instances/:id/reload-config", instanceHandler.ReloadInstanceConfig)
		protected.POST("/instances/:id/refresh-fronting", instanceHandler.RefreshFronting)
		protected.GET("/instances/:id/status", instanceHandler.InstanceStatus)
		protected.GET("/instances/:id/logs", instanceHandler.InstanceLogs)

		// Proxy control
		proxyHandler := handler.NewProxyHandler(cfg.ContainerSvc, cfg.Settings, cfg.SecretSvc)
		proxyHandler.SetDockerClient(cfg.Docker)
		proxyHandler.SetInstanceStore(cfg.Instances)
		protected.POST("/proxy/start", proxyHandler.Start)
		protected.POST("/proxy/stop", proxyHandler.Stop)
		protected.POST("/proxy/restart", proxyHandler.Restart)
		protected.POST("/proxy/reload", proxyHandler.Reload)
		protected.POST("/proxy/reload-zero-downtime", proxyHandler.ReloadZeroDowntime)
		protected.GET("/proxy/status", proxyHandler.Status)
		protected.GET("/proxy/status/ws", proxyHandler.StatusWS)
		protected.GET("/proxy/logs", proxyHandler.Logs)

		// Docker/Engine
		dockerHandler := handler.NewDockerHandler(cfg.Docker, cfg.DockerSvc, cfg.Settings)
		if cfg.DockerUpdateSvc != nil {
			dockerHandler.SetDockerUpdateService(cfg.DockerUpdateSvc)
		}
		protected.POST("/docker/install", dockerHandler.Install)
		protected.GET("/docker/status", dockerHandler.Status)
		protected.GET("/docker/update/status", dockerHandler.CheckUpdate)
		protected.POST("/docker/update/check", dockerHandler.TriggerCheckUpdate)
		protected.POST("/docker/update/apply", dockerHandler.ApplyUpdate)
		protected.GET("/engine/status", dockerHandler.EngineStatus)
		protected.POST("/engine/build", dockerHandler.Build)

		// telemt engine updates
		if cfg.TelemtUpdateSvc != nil {
			telemtUpdateHandler := handler.NewTelemtUpdateHandler(cfg.TelemtUpdateSvc, cfg.TelemtCfg, cfg.DockerSvc)
			protected.GET("/engine/update", telemtUpdateHandler.GetStatus)
			protected.GET("/engine/releases", telemtUpdateHandler.GetReleases)
			protected.POST("/engine/check", telemtUpdateHandler.CheckRemote)
			protected.POST("/engine/update", telemtUpdateHandler.Apply)
		}

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
		protected.GET("/traffic/history", trafficHandler.GetHistory)
		protected.GET("/traffic/:label", trafficHandler.GetUser)

		// WebSocket — live metrics streaming
		wsHandler := handler.NewWSHandler(cfg.TrafficSvc, cfg.TelemtUpdateSvc)
		if len(origins) > 0 {
			handler.SetWSAllowedOrigins(origins)
		} else {
			handler.SetWSAllowedOrigins([]string{"*"})
		}
		protected.GET("/traffic/live/ws", wsHandler.Handle)
		protected.GET("/engine/update/ws", wsHandler.HandleEngineUpdate)

		// Bot
		botHandler := handler.NewBotHandler(cfg.Settings, cfg.BotDeps, cfg.ActiveBot, cfg.BotMu)
		protected.POST("/bot/setup", botHandler.Setup)
		protected.POST("/bot/test", botHandler.Test)
		protected.GET("/bot/status", botHandler.Status)
		protected.PUT("/bot/toggle", botHandler.Toggle)
		protected.GET("/bot/detect-chat-id", botHandler.DetectChatID)
		protected.POST("/bot/commands", botHandler.SetCommands)

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
		backupHandler := handler.NewBackupHandler(cfg.Backups, cfg.Settings, cfg.ContainerSvc, cfg.AuditSvc)
		protected.GET("/backups", backupHandler.List)
		protected.POST("/backups", backupHandler.Create)
		protected.POST("/backups/restore", backupHandler.Restore)
		protected.GET("/backups/download/:filename", backupHandler.Download)
		protected.DELETE("/backups/:filename", backupHandler.Delete)

		// System
		systemHandler := handler.NewSystemHandler(cfg.Settings)
		protected.GET("/system/resources", systemHandler.GetResources)
		protected.GET("/system/resources/ws", systemHandler.StreamResources)
		protected.GET("/system/os", systemHandler.GetOS)
		protected.POST("/system/service/install", systemHandler.InstallService)
		protected.DELETE("/system/service/uninstall", systemHandler.UninstallService)
		protected.GET("/system/service/status", systemHandler.ServiceStatus)
		protected.POST("/system/service/restart", systemHandler.RestartService)
		protected.POST("/system/service/reload", systemHandler.ReloadService)
		protected.POST("/system/sysctl", systemHandler.ConfigureSysctl)

		// Scheduler
		if cfg.SchedulerSvc != nil {
			schedulerHandler := handler.NewSchedulerHandler(cfg.SchedulerSvc)
			protected.GET("/scheduler/tasks", schedulerHandler.List)
			protected.PUT("/scheduler/tasks/:name", schedulerHandler.Update)
			protected.POST("/scheduler/tasks/:name/run", schedulerHandler.RunNow)
			protected.GET("/scheduler/tasks/:name/history", schedulerHandler.History)
			protected.GET("/scheduler/history", schedulerHandler.AllHistory)
		}

		// Audit
		if cfg.AuditSvc != nil {
			auditHandler := handler.NewAuditHandler(cfg.AuditSvc)
			protected.GET("/audit", auditHandler.List)
			protected.GET("/audit/filters", auditHandler.GetFilters)
		}

		// Templates
		if cfg.TemplateSvc != nil {
			templateHandler := handler.NewTemplateHandler(cfg.TemplateSvc)
			protected.GET("/templates", templateHandler.List)
			protected.POST("/templates", templateHandler.Create)
			protected.GET("/templates/:name", templateHandler.Get)
			protected.DELETE("/templates/:name", templateHandler.Delete)
			protected.POST("/templates/:name/apply", templateHandler.Apply)
		}
	}

	return r
}
