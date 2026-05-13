package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/service"
	"github.com/fussraider/PopuGate/pkg/telemt"
)

// mdSafe strips Telegram Markdown v1 special characters from user-provided text.
// Telegram Markdown v1 treats _, *, `, [ as formatting and fails on unbalanced pairs.
func mdSafe(s string) string {
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, "*", "")
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "[", "(")
	s = strings.ReplaceAll(s, "]", ")")
	return s
}

// cmdWelcome handles /start without arguments — the initial Telegram greeting.
func (b *Bot) cmdWelcome() string {
	return b.cmdHelp()
}

// cmdStatus shows proxy status per instance.
func (b *Bot) cmdStatus(ctx context.Context) string {
	settings, _ := b.deps.Settings.Load(ctx)
	label := b.label
	if label == "" {
		label = settings.TelegramServerLabel
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("🔧 *Status* — `%s`", mdSafe(label)))
	lines = append(lines, "")

	instances, err := b.deps.Instances.List(ctx)
	if err != nil || len(instances) == 0 {
		lines = append(lines, "No instances configured.")
		return strings.Join(lines, "\n")
	}

	anyRunning := false
	for _, inst := range instances {
		if !inst.Enabled {
			continue
		}
		running := false
		if b.deps.IsInstanceRunning != nil {
			running = b.deps.IsInstanceRunning(ctx, inst.ContainerName())
		}
		status := "❌ stopped"
		if running {
			status = "✅ running"
			anyRunning = true
		}

		domain := inst.TLSDomain
		if domain == "" {
			domain = "-"
		}
		lines = append(lines, fmt.Sprintf("`%s` :%d %s — `%s`", inst.Label, inst.Port, status, domain))
	}

	var totalIn, totalOut int64
	global, err := b.deps.Traffic.GetGlobal(ctx)
	if err == nil {
		totalIn = global.BytesIn
		totalOut = global.BytesOut
	}

	secrets, _ := b.deps.Secrets.List(ctx)
	enabled := 0
	for _, s := range secrets {
		if s.Enabled {
			enabled++
		}
	}

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Secrets: %d/%d enabled", enabled, len(secrets)))
	lines = append(lines, fmt.Sprintf("Traffic: ↓%s ↑%s", formatBytes(totalIn), formatBytes(totalOut)))

	if !anyRunning {
		lines = append(lines, "")
		lines = append(lines, "⚠ No instances are running")
	}
	return strings.Join(lines, "\n")
}

// cmdSecrets lists all secrets with per-user stats.
func (b *Bot) cmdSecrets(ctx context.Context) string {
	secrets, err := b.deps.Secrets.List(ctx)
	if err != nil || len(secrets) == 0 {
		return "📋 No secrets configured."
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("📋 *Secrets (%d)*", len(secrets)))
	for _, s := range secrets {
		status := "✅"
		if !s.Enabled {
			status = "❌"
		}
		traffic := ""
		if s.TrafficIn+s.TrafficOut > 0 {
			traffic = fmt.Sprintf(" ↓%s↑%s", formatBytes(s.TrafficIn), formatBytes(s.TrafficOut))
		}
		lines = append(lines, fmt.Sprintf("%s `%s`%s", status, s.Label, traffic))
	}
	return strings.Join(lines, "\n")
}

// cmdLink shows proxy links for a specific secret or all enabled secrets.
func (b *Bot) cmdLink(ctx context.Context, text string) string {
	settings, _ := b.deps.Settings.Load(ctx)
	publicIP := ""
	if b.deps.GetPublicIP != nil {
		publicIP = b.deps.GetPublicIP(ctx)
	}
	if publicIP == "" {
		publicIP = settings.CustomIP
	}

	instances, _ := b.deps.Instances.List(ctx)

	label := b.args(text)
	if label != "" {
		sec, err := b.deps.Secrets.GetByLabel(ctx, label)
		if err != nil || sec == nil {
			return fmt.Sprintf("Secret `%s` not found.", label)
		}

		var lines []string
		lines = append(lines, fmt.Sprintf("🔗 *Links* — `%s`:", sec.Label))

		links := service.BuildLinksForSecret(sec, instances, publicIP)
		for _, link := range links {
			lines = append(lines, fmt.Sprintf("`%s` :%d — `%s`", link.InstanceLabel, link.InstancePort, link.Domain))
			lines = append(lines, fmt.Sprintf("`%s`", link.TGLink))
			lines = append(lines, link.WebLink)

			if b.deps.GenerateQR != nil {
				if qrPNG, err := b.deps.GenerateQR(ctx, link.WebLink); err == nil {
					caption := fmt.Sprintf("🔗 %s — %s :%d — %s", sec.Label, link.InstanceLabel, link.InstancePort, link.Domain)
					if sendErr := b.SendPhoto(ctx, qrPNG, caption); sendErr != nil {
						log.Errorf("QR photo send error: %v", sendErr)
					}
				}
			}
		}

		if len(lines) <= 1 {
			return fmt.Sprintf("No accessible instances for secret `%s`.", label)
		}
		return strings.Join(lines, "\n")
	}

	// Show all enabled secrets
	secrets, _ := b.deps.Secrets.List(ctx)
	var allLines []string
	for _, s := range secrets {
		if !s.Enabled {
			continue
		}
		links := service.BuildLinksForSecret(&s, instances, publicIP)
		for _, link := range links {
			allLines = append(allLines, fmt.Sprintf("🔗 `%s` `%s` :%d `%s`: `%s`", s.Label, link.InstanceLabel, link.InstancePort, link.Domain, link.TGLink))

			if b.deps.GenerateQR != nil {
				if qrPNG, err := b.deps.GenerateQR(ctx, link.WebLink); err == nil {
					caption := fmt.Sprintf("🔗 %s — %s :%d — %s", s.Label, link.InstanceLabel, link.InstancePort, link.Domain)
					if sendErr := b.SendPhoto(ctx, qrPNG, caption); sendErr != nil {
						log.Errorf("QR photo send error for %s: %v", s.Label, sendErr)
					}
				}
			}
		}
	}
	if len(allLines) == 0 {
		return "No enabled secrets or instances."
	}
	return strings.Join(allLines, "\n")
}

// cmdAdd adds a new secret with the given label.
func (b *Bot) cmdAdd(ctx context.Context, text string) string {
	label := b.args(text)
	if label == "" {
		return "Usage: /add <label>"
	}
	if err := model.ValidateLabel(label); err != nil {
		return fmt.Sprintf("Invalid label: %s", err.Error())
	}

	existing, _ := b.deps.Secrets.GetByLabel(ctx, label)
	if existing != nil {
		return fmt.Sprintf("Secret `%s` already exists.", label)
	}

	secretKey, err := telemt.GenerateSecret()
	if err != nil {
		return fmt.Sprintf("Failed to generate secret: %s", err.Error())
	}

	sec := &model.Secret{
		Label:     label,
		SecretKey: secretKey,
		Enabled:   true,
	}
	if err := b.deps.Secrets.Create(ctx, sec); err != nil {
		return fmt.Sprintf("Failed to create secret: %s", err.Error())
	}

	return fmt.Sprintf("✅ Secret `%s` added. Use /restart to apply.", label)
}

// cmdRemove removes a secret by label.
func (b *Bot) cmdRemove(ctx context.Context, text string) string {
	label := b.args(text)
	if label == "" {
		return "Usage: /remove <label>"
	}

	existing, _ := b.deps.Secrets.GetByLabel(ctx, label)
	if existing == nil {
		return fmt.Sprintf("Secret `%s` not found.", label)
	}

	if err := b.deps.Secrets.Delete(ctx, label); err != nil {
		return fmt.Sprintf("Failed to remove: %s", err.Error())
	}

	return fmt.Sprintf("🗑 Secret `%s` removed. Use /restart to apply.", label)
}

// cmdRotate rotates a secret's key.
func (b *Bot) cmdRotate(ctx context.Context, text string) string {
	label := b.args(text)
	if label == "" {
		return "Usage: /rotate <label>"
	}

	existing, err := b.deps.Secrets.GetByLabel(ctx, label)
	if err != nil || existing == nil {
		return fmt.Sprintf("Secret `%s` not found.", label)
	}

	newKey, err := telemt.GenerateSecret()
	if err != nil {
		return fmt.Sprintf("Failed to generate key: %s", err.Error())
	}

	existing.SecretKey = newKey
	if err := b.deps.Secrets.Update(ctx, existing); err != nil {
		return fmt.Sprintf("Failed to rotate: %s", err.Error())
	}

	return fmt.Sprintf("🔄 Secret `%s` rotated. Use /restart to apply.", label)
}

// cmdRestart restarts the proxy.
func (b *Bot) cmdRestart(ctx context.Context) string {
	if b.deps.RestartProxy == nil {
		return "⚠ Restart not available."
	}
	if err := b.deps.RestartProxy(ctx); err != nil {
		return fmt.Sprintf("❌ Restart failed: %s", err.Error())
	}
	return "🔄 Proxy restarted."
}

// cmdStartInstance starts a specific instance by label.
func (b *Bot) cmdStartInstance(ctx context.Context, text string) string {
	label := b.args(text)
	if label == "" {
		return "Usage: /start <label>"
	}

	instances, err := b.deps.Instances.List(ctx)
	if err != nil || len(instances) == 0 {
		return "No instances found."
	}

	for _, inst := range instances {
		if inst.Label == label || inst.Label == strings.TrimSpace(label) {
			if b.deps.StartInstance == nil {
				return "⚠ Start not available."
			}
			if err := b.deps.StartInstance(ctx, inst.ID); err != nil {
				return fmt.Sprintf("❌ Failed to start `%s`: %s", inst.Label, err.Error())
			}
			return fmt.Sprintf("▶ Instance `%s` (:%d) started.", inst.Label, inst.Port)
		}
	}
	return fmt.Sprintf("Instance `%s` not found.", label)
}

// cmdStopInstance stops a specific instance by label.
func (b *Bot) cmdStopInstance(ctx context.Context, text string) string {
	label := b.args(text)
	if label == "" {
		return "Usage: /stop <label>"
	}

	instances, err := b.deps.Instances.List(ctx)
	if err != nil || len(instances) == 0 {
		return "No instances found."
	}

	for _, inst := range instances {
		if inst.Label == label || inst.Label == strings.TrimSpace(label) {
			if b.deps.StopInstance == nil {
				return "⚠ Stop not available."
			}
			if err := b.deps.StopInstance(ctx, inst.ID); err != nil {
				return fmt.Sprintf("❌ Failed to stop `%s`: %s", inst.Label, err.Error())
			}
			return fmt.Sprintf("⏹ Instance `%s` (:%d) stopped.", inst.Label, inst.Port)
		}
	}

	return fmt.Sprintf("Instance `%s` not found.", label)
}

// cmdEnable enables a secret.
func (b *Bot) cmdEnable(ctx context.Context, text string) string {
	label := b.args(text)
	if label == "" {
		return "Usage: /enable <label>"
	}

	sec, err := b.deps.Secrets.GetByLabel(ctx, label)
	if err != nil || sec == nil {
		return fmt.Sprintf("Secret `%s` not found.", label)
	}

	if sec.Enabled {
		return fmt.Sprintf("Secret `%s` is already enabled.", label)
	}

	sec.Enabled = true
	if err := b.deps.Secrets.Update(ctx, sec); err != nil {
		return fmt.Sprintf("Failed: %s", err.Error())
	}

	return fmt.Sprintf("✅ Secret `%s` enabled. Use /restart to apply.", label)
}

// cmdDisable disables a secret.
func (b *Bot) cmdDisable(ctx context.Context, text string) string {
	label := b.args(text)
	if label == "" {
		return "Usage: /disable <label>"
	}

	sec, err := b.deps.Secrets.GetByLabel(ctx, label)
	if err != nil || sec == nil {
		return fmt.Sprintf("Secret `%s` not found.", label)
	}

	if !sec.Enabled {
		return fmt.Sprintf("Secret `%s` is already disabled.", label)
	}

	enabled, _ := b.deps.Secrets.CountEnabled(ctx)
	if enabled <= 1 {
		return "⚠ Cannot disable the last active secret."
	}

	sec.Enabled = false
	if err := b.deps.Secrets.Update(ctx, sec); err != nil {
		return fmt.Sprintf("Failed: %s", err.Error())
	}

	return fmt.Sprintf("❌ Secret `%s` disabled. Use /restart to apply.", label)
}

// cmdHealth runs a health check per instance.
func (b *Bot) cmdHealth(ctx context.Context) string {
	label := b.label

	var lines []string
	lines = append(lines, fmt.Sprintf("🏥 *Health* — `%s`", mdSafe(label)))

	instances, _ := b.deps.Instances.List(ctx)
	for _, inst := range instances {
		if !inst.Enabled {
			continue
		}
		running := false
		if b.deps.IsInstanceRunning != nil {
			running = b.deps.IsInstanceRunning(ctx, inst.ContainerName())
		}
		if running {
			lines = append(lines, fmt.Sprintf("`%s` (:%d): ✅ running", inst.Label, inst.Port))
		} else {
			lines = append(lines, fmt.Sprintf("`%s` (:%d): ❌ stopped", inst.Label, inst.Port))
		}
	}

	if b.deps.GetEngineVersion != nil {
		if v := b.deps.GetEngineVersion(); v != "" {
			lines = append(lines, fmt.Sprintf("Engine: `v%s`", v))
		} else {
			lines = append(lines, "Engine: not installed")
		}
	}

	enabled := 0
	total := 0
	if secrets, err := b.deps.Secrets.List(ctx); err == nil {
		total = len(secrets)
		for _, s := range secrets {
			if s.Enabled {
				enabled++
			}
		}
	}
	lines = append(lines, fmt.Sprintf("Secrets: %d/%d enabled", enabled, total))

	return strings.Join(lines, "\n")
}

// cmdTraffic shows detailed traffic breakdown.
func (b *Bot) cmdTraffic(ctx context.Context) string {
	global, err := b.deps.Traffic.GetGlobal(ctx)
	if err != nil {
		log.Warnf("cmdTraffic: get global traffic: %v", err)
		return "📊 Traffic data unavailable."
	}

	var lines []string
	lines = append(lines, "📊 *Traffic Report*")
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Global: ↓%s ↑%s (total %s)",
		formatBytes(global.BytesIn), formatBytes(global.BytesOut),
		formatBytes(global.BytesIn+global.BytesOut)))

	users, err := b.deps.Traffic.ListUserTraffic(ctx)
	if err == nil && len(users) > 0 {
		lines = append(lines, "")
		lines = append(lines, "*Per user:*")
		for _, u := range users {
			lines = append(lines, fmt.Sprintf("  `%s`: ↓%s ↑%s", u.Label,
				formatBytes(u.BytesIn), formatBytes(u.BytesOut)))
		}
	}

	return strings.Join(lines, "\n")
}

// cmdUpdate shows version info.
func (b *Bot) cmdUpdate(ctx context.Context) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("📦 *PopuGate* `v%s`", model.Version))
	if model.Commit != "" && model.Commit != "unknown" {
		lines = append(lines, fmt.Sprintf("Commit: `%s`", model.Commit))
	}
	if b.deps.GetEngineVersion != nil {
		if v := b.deps.GetEngineVersion(); v != "" {
			lines = append(lines, fmt.Sprintf("Engine: `v%s`", v))
		}
	}
	lines = append(lines, "")
	lines = append(lines, "Use the web UI to check and apply updates.")
	return strings.Join(lines, "\n")
}

// cmdLimits shows per-user limits.
func (b *Bot) cmdLimits(ctx context.Context) string {
	secrets, err := b.deps.Secrets.List(ctx)
	if err != nil || len(secrets) == 0 {
		return "📋 No secrets configured."
	}

	var lines []string
	lines = append(lines, "📏 *User Limits*")
	for _, s := range secrets {
		conns := "∞"
		if s.MaxConns > 0 {
			conns = fmt.Sprintf("%d", s.MaxConns)
		}
		ips := "∞"
		if s.MaxIPs > 0 {
			ips = fmt.Sprintf("%d", s.MaxIPs)
		}
		quota := "∞"
		if s.QuotaBytes > 0 {
			quota = formatBytes(s.QuotaBytes)
			if s.TrafficIn+s.TrafficOut > 0 {
				pct := float64(s.TrafficIn+s.TrafficOut) * 100 / float64(s.QuotaBytes)
				quota = fmt.Sprintf("%s (%.0f%% used)", formatBytes(s.QuotaBytes), pct)
			}
		}
		expiry := "never"
		if s.ExpiresAt != "" && s.ExpiresAt != "0" {
			expiry = s.ExpiresAt
		}
		lines = append(lines, fmt.Sprintf("`%s`: conns=%s ips=%s quota=%s expires=%s",
			s.Label, conns, ips, quota, expiry))
	}

	return strings.Join(lines, "\n")
}

// cmdSetLimit sets limits for a user: /setlimit <label> <conns> <ips> <quota> [expires]
func (b *Bot) cmdSetLimit(ctx context.Context, text string) string {
	parts := strings.Fields(text)
	if len(parts) < 5 {
		return "Usage: /setlimit <label> <max_conns> <max_ips> <quota_mb> [expires YYYY-MM-DD]"
	}
	label := parts[1]

	sec, err := b.deps.Secrets.GetByLabel(ctx, label)
	if err != nil || sec == nil {
		return fmt.Sprintf("Secret `%s` not found.", label)
	}

	var conns int
	if n, _ := fmt.Sscanf(parts[2], "%d", &conns); n != 1 {
		return "Invalid max_conns value. Must be a number."
	}

	var ips int
	if n, _ := fmt.Sscanf(parts[3], "%d", &ips); n != 1 {
		return "Invalid max_ips value. Must be a number."
	}

	var quotaMB int64
	if n, _ := fmt.Sscanf(parts[4], "%d", &quotaMB); n != 1 {
		return "Invalid quota_mb value. Must be a number."
	}
	quotaBytes := quotaMB * 1024 * 1024

	sec.MaxConns = conns
	sec.MaxIPs = ips
	sec.QuotaBytes = quotaBytes

	if len(parts) >= 6 {
		sec.ExpiresAt = parts[5]
	}

	if err := b.deps.Secrets.Update(ctx, sec); err != nil {
		return fmt.Sprintf("Failed: %s", err.Error())
	}

	return fmt.Sprintf("✅ Limits for `%s` updated: conns=%d ips=%d quota=%dMB",
		label, conns, ips, quotaMB)
}

// cmdUpstreams lists configured upstreams.
func (b *Bot) cmdUpstreams(ctx context.Context) string {
	upstreams, err := b.deps.Upstreams.List(ctx)
	if err != nil || len(upstreams) == 0 {
		return "🔀 No upstreams configured."
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("🔀 *Upstreams (%d)*", len(upstreams)))
	for _, u := range upstreams {
		status := "✅"
		if !u.Enabled {
			status = "❌"
		}
		addr := u.Address
		if addr == "" {
			addr = "direct"
		}
		lines = append(lines, fmt.Sprintf("%s `%s` (%s) weight=%d `%s`",
			status, u.Name, u.Type, u.Weight, addr))
	}
	return strings.Join(lines, "\n")
}

// cmdTasks shows scheduled tasks status.
func (b *Bot) cmdTasks(ctx context.Context) string {
	if b.deps.GetSchedulerTasks == nil {
		return "⚠ Scheduler status not available."
	}

	lines := b.deps.GetSchedulerTasks(ctx)
	if len(lines) == 0 {
		return "📋 No scheduled tasks."
	}

	return strings.Join(lines, "\n")
}

// cmdHelp shows a bot description and all available commands.
func (b *Bot) cmdHelp() string {
	label := b.label
	if label == "" {
		label = "PopuGate"
	}
	return strings.Join([]string{
		fmt.Sprintf("📖 *Bot* — `%s`", mdSafe(label)),
		"",
		"Telegram MTProto proxy manager. Use the commands below to control your proxy, manage secrets, and monitor traffic.",
		"",
		"*Management:*",
		"/status — Proxy status & connections",
		"/health — Health check (Docker, ports, metrics)",
		"/restart — Restart proxy",
		"/start <label> — Start instance",
		"/stop <label> — Stop instance",
		"",
		"*Secrets:*",
		"/secrets — List secrets",
		"/link [label] — Proxy links + QR",
		"/add <label> — Add secret",
		"/remove <label> — Remove secret",
		"/rotate <label> — Rotate secret key",
		"/enable <label> — Enable secret",
		"/disable <label> — Disable secret",
		"",
		"*Limits & Traffic:*",
		"/limits — Show all user limits",
		"/setlimit <label> <conns> <ips> <quota_mb> [date] — Set limits",
		"/traffic — Traffic report",
		"",
		"*System:*",
		"/upstreams — List upstreams",
		"/tasks — Scheduled tasks status",
		"/update — Version info",
		"/help — This message",
	}, "\n")
}
