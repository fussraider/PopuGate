package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/pkg/telemt"
)

// cmdStatus shows proxy status, uptime, and quick traffic stats.
func (b *Bot) cmdStatus(ctx context.Context) string {
	settings, _ := b.deps.Settings.Load(ctx)
	label := b.label
	if label == "" {
		label = settings.TelegramServerLabel
	}

	running := false
	if b.deps.IsProxyRunning != nil {
		running = b.deps.IsProxyRunning(ctx)
	}

	status := "stopped"
	if running {
		status = "running"
	}

	uptime := "-"
	if b.deps.GetUptime != nil {
		if u := b.deps.GetUptime(ctx); u != "" {
			uptime = u
		}
	}

	// Global traffic
	var totalIn, totalOut int64
	global, err := b.deps.Traffic.GetGlobal(ctx)
	if err == nil {
		totalIn = global.BytesIn
		totalOut = global.BytesOut
	} else {
		log.Warnf("cmdStatus: get global traffic: %v", err)
	}

	// Secret count
	secrets, _ := b.deps.Secrets.List(ctx)
	enabled := 0
	for _, s := range secrets {
		if s.Enabled {
			enabled++
		}
	}

	return strings.Join([]string{
		fmt.Sprintf("🔧 *%s Status*", label),
		"",
		fmt.Sprintf("Proxy: %s", status),
		fmt.Sprintf("Uptime: %s", uptime),
		fmt.Sprintf("Port: %d", settings.ProxyPort),
		fmt.Sprintf("Secrets: %d/%d enabled", enabled, len(secrets)),
		fmt.Sprintf("Traffic: ↓%s ↑%s", formatBytes(totalIn), formatBytes(totalOut)),
	}, "\n")
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
// When GenerateQR callback is set, also sends QR code images via Telegram.
func (b *Bot) cmdLink(ctx context.Context, text string) string {
	settings, _ := b.deps.Settings.Load(ctx)
	publicIP := ""
	if b.deps.GetPublicIP != nil {
		publicIP = b.deps.GetPublicIP(ctx)
	}
	if publicIP == "" {
		publicIP = settings.CustomIP
	}

	label := b.args(text)
	if label != "" {
		sec, err := b.deps.Secrets.GetByLabel(ctx, label)
		if err != nil || sec == nil {
			return fmt.Sprintf("Secret `%s` not found.", label)
		}
		secret := telemt.BuildFakeTLSSecret(sec.SecretKey, settings.ProxyDomain, settings.MaskingEnabled)
		link := telemt.BuildProxyLink(publicIP, settings.ProxyPort, secret)
		webLink := telemt.BuildWebLink(publicIP, settings.ProxyPort, secret)
		response := fmt.Sprintf("🔗 *%s*\n`%s`\n%s", sec.Label, link, webLink)

		// Send QR code photo if available
		if b.deps.GenerateQR != nil {
			if qrPNG, err := b.deps.GenerateQR(ctx, webLink); err == nil {
				caption := fmt.Sprintf("🔗 %s — scan to connect", sec.Label)
				if sendErr := b.SendPhoto(ctx, qrPNG, caption); sendErr != nil {
					log.Errorf("QR photo send error: %v", sendErr)
				}
			}
		}

		return response
	}

	// Show all enabled secrets
	secrets, _ := b.deps.Secrets.List(ctx)
	var lines []string
	for _, s := range secrets {
		if !s.Enabled {
			continue
		}
		secret := telemt.BuildFakeTLSSecret(s.SecretKey, settings.ProxyDomain, settings.MaskingEnabled)
		link := telemt.BuildProxyLink(publicIP, settings.ProxyPort, secret)
		webLink := telemt.BuildWebLink(publicIP, settings.ProxyPort, secret)
		lines = append(lines, fmt.Sprintf("🔗 `%s`: `%s`", s.Label, link))

		// Send QR code photo for each enabled secret
		if b.deps.GenerateQR != nil {
			if qrPNG, err := b.deps.GenerateQR(ctx, webLink); err == nil {
				caption := fmt.Sprintf("🔗 %s — scan to connect", s.Label)
				if sendErr := b.SendPhoto(ctx, qrPNG, caption); sendErr != nil {
					log.Errorf("QR photo send error for %s: %v", s.Label, sendErr)
				}
			}
		}
	}
	if len(lines) == 0 {
		return "No enabled secrets."
	}
	return strings.Join(lines, "\n")
}

// cmdAdd adds a new secret with the given label.
func (b *Bot) cmdAdd(ctx context.Context, text string) string {
	label := b.args(text)
	if label == "" {
		return "Usage: /mp_add <label>"
	}
	if err := model.ValidateLabel(label); err != nil {
		return fmt.Sprintf("Invalid label: %s", err.Error())
	}

	// Check if already exists
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

	return fmt.Sprintf("✅ Secret `%s` added. Use /mp_reload or restart proxy to apply.", label)
}

// cmdRemove removes a secret by label.
func (b *Bot) cmdRemove(ctx context.Context, text string) string {
	label := b.args(text)
	if label == "" {
		return "Usage: /mp_remove <label>"
	}

	existing, _ := b.deps.Secrets.GetByLabel(ctx, label)
	if existing == nil {
		return fmt.Sprintf("Secret `%s` not found.", label)
	}

	if err := b.deps.Secrets.Delete(ctx, label); err != nil {
		return fmt.Sprintf("Failed to remove: %s", err.Error())
	}

	return fmt.Sprintf("🗑 Secret `%s` removed. Use /mp_reload or restart proxy to apply.", label)
}

// cmdRotate rotates a secret's key.
func (b *Bot) cmdRotate(ctx context.Context, text string) string {
	label := b.args(text)
	if label == "" {
		return "Usage: /mp_rotate <label>"
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

	return fmt.Sprintf("🔄 Secret `%s` rotated. Use /mp_reload or restart proxy to apply.", label)
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

// cmdEnable enables a secret.
func (b *Bot) cmdEnable(ctx context.Context, text string) string {
	label := b.args(text)
	if label == "" {
		return "Usage: /mp_enable <label>"
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

	return fmt.Sprintf("✅ Secret `%s` enabled. Use /mp_reload or restart proxy to apply.", label)
}

// cmdDisable disables a secret.
func (b *Bot) cmdDisable(ctx context.Context, text string) string {
	label := b.args(text)
	if label == "" {
		return "Usage: /mp_disable <label>"
	}

	sec, err := b.deps.Secrets.GetByLabel(ctx, label)
	if err != nil || sec == nil {
		return fmt.Sprintf("Secret `%s` not found.", label)
	}

	if !sec.Enabled {
		return fmt.Sprintf("Secret `%s` is already disabled.", label)
	}

	// Prevent disabling last active secret
	enabled, _ := b.deps.Secrets.CountEnabled(ctx)
	if enabled <= 1 {
		return "⚠ Cannot disable the last active secret."
	}

	sec.Enabled = false
	if err := b.deps.Secrets.Update(ctx, sec); err != nil {
		return fmt.Sprintf("Failed: %s", err.Error())
	}

	return fmt.Sprintf("❌ Secret `%s` disabled. Use /mp_reload or restart proxy to apply.", label)
}

// cmdHealth runs a quick health check.
func (b *Bot) cmdHealth(ctx context.Context) string {
	settings, _ := b.deps.Settings.Load(ctx)

	running := false
	if b.deps.IsProxyRunning != nil {
		running = b.deps.IsProxyRunning(ctx)
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("🏥 *%s Health*", b.label))

	if running {
		lines = append(lines, "Container: ✅ running")
	} else {
		lines = append(lines, "Container: ❌ stopped")
	}

	// Engine version
	if b.deps.GetEngineVersion != nil {
		if v := b.deps.GetEngineVersion(); v != "" {
			lines = append(lines, fmt.Sprintf("Engine: v%s", v))
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

	lines = append(lines, fmt.Sprintf("Port: %d", settings.ProxyPort))

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

// cmdUpdate checks for updates.
func (b *Bot) cmdUpdate(ctx context.Context) string {
	return fmt.Sprintf("📦 *PopuGate v%s*\nUse the web UI to check and apply updates.", model.Version)
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
		conns := "unlimited"
		if s.MaxConns > 0 {
			conns = fmt.Sprintf("%d", s.MaxConns)
		}
		ips := "unlimited"
		if s.MaxIPs > 0 {
			ips = fmt.Sprintf("%d", s.MaxIPs)
		}
		quota := "unlimited"
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

// cmdSetLimit sets limits for a user: /mp_setlimit <label> <conns> <ips> <quota> [expires]
func (b *Bot) cmdSetLimit(ctx context.Context, text string) string {
	parts := strings.Fields(text)
	if len(parts) < 5 {
		return "Usage: /mp_setlimit <label> <max_conns> <max_ips> <quota_mb> [expires YYYY-MM-DD]"
	}
	label := parts[1]
	if len(parts) < 5 {
		return "Usage: /mp_setlimit <label> <max_conns> <max_ips> <quota_mb> [expires YYYY-MM-DD]"
	}

	sec, err := b.deps.Secrets.GetByLabel(ctx, label)
	if err != nil || sec == nil {
		return fmt.Sprintf("Secret `%s` not found.", label)
	}

	// Parse conns
	var conns int
	fmt.Sscanf(parts[2], "%d", &conns)

	// Parse IPs
	var ips int
	fmt.Sscanf(parts[3], "%d", &ips)

	// Parse quota (in MB)
	var quotaMB int64
	fmt.Sscanf(parts[4], "%d", &quotaMB)
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
		lines = append(lines, fmt.Sprintf("%s `%s` (%s) weight=%d %s",
			status, u.Name, u.Type, u.Weight, addr))
	}
	return strings.Join(lines, "\n")
}

// cmdHelp shows all available commands.
func (b *Bot) cmdHelp() string {
	return strings.Join([]string{
		"📖 *PopuGate Bot Commands*",
		"",
		"/mp_status — Proxy status",
		"/mp_secrets — List secrets",
		"/mp_link [label] — Proxy links",
		"/mp_add <label> — Add secret",
		"/mp_remove <label> — Remove secret",
		"/mp_rotate <label> — Rotate secret",
		"/mp_restart — Restart proxy",
		"/mp_enable <label> — Enable secret",
		"/mp_disable <label> — Disable secret",
		"/mp_health — Health check",
		"/mp_traffic — Traffic report",
		"/mp_update — Version info",
		"/mp_limits — Show user limits",
		"/mp_setlimit <label> <conns> <ips> <quota_mb> [date] — Set limits",
		"/mp_upstreams — List upstreams",
		"/mp_help — This message",
	}, "\n")
}
