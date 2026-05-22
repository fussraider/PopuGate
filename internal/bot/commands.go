package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

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
func (b *Bot) cmdWelcome() cmdResp {
	return b.cmdHelp()
}

// cmdStatus shows proxy status per instance, engine version, uptime, and traffic.
func (b *Bot) cmdStatus(ctx context.Context) cmdResp {
	settings, _ := b.deps.Settings.Load(ctx)
	label := b.label
	if label == "" {
		label = settings.TelegramServerLabel
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("🔧 *Status* — `%s`", mdSafe(label)))
	lines = append(lines, "")

	lines = append(lines, b.instanceStatusLines(ctx)...)

	trafficIn, trafficOut := b.globalTraffic(ctx)
	secrets, _ := b.deps.Secrets.List(ctx)
	enabled := countEnabled(secrets)

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Secrets: %d/%d enabled", enabled, len(secrets)))
	lines = append(lines, fmt.Sprintf("Traffic: ↓%s ↑%s", formatBytes(trafficIn), formatBytes(trafficOut)))
	lines = append(lines, b.engineInfoLines(ctx)...)
	lines = append(lines, fmt.Sprintf("PopuGate: `%s`", model.VersionTag()))

	return replyKB(strings.Join(lines, "\n"), b.dashboardKB(ctx))
}

func (b *Bot) instanceStatusLines(ctx context.Context) []string {
	instances, err := b.deps.Instances.List(ctx)
	if err != nil || len(instances) == 0 {
		return []string{"No instances configured."}
	}

	var lines []string
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
	if !anyRunning {
		lines = append(lines, "", "⚠ No instances are running")
	}
	return lines
}

func (b *Bot) globalTraffic(ctx context.Context) (int64, int64) {
	global, err := b.deps.Traffic.GetGlobal(ctx)
	if err != nil {
		return 0, 0
	}
	return global.BytesIn, global.BytesOut
}

func countEnabled(secrets []model.Secret) int {
	n := 0
	for _, s := range secrets {
		if s.Enabled {
			n++
		}
	}
	return n
}

func (b *Bot) engineInfoLines(ctx context.Context) []string {
	var lines []string
	if b.deps.GetEngineVersion != nil {
		if v := b.deps.GetEngineVersion(); v != "" {
			lines = append(lines, fmt.Sprintf("Engine: `v%s`", v))
		} else {
			lines = append(lines, "Engine: not installed")
		}
	}
	if b.deps.GetUptime != nil {
		if uptime := b.deps.GetUptime(ctx); uptime != "" {
			lines = append(lines, fmt.Sprintf("Uptime: `%s`", uptime))
		}
	}
	return lines
}

// cmdSecrets lists all secrets with per-user stats and limits.
func (b *Bot) cmdSecrets(ctx context.Context) cmdResp {
	secrets, err := b.deps.Secrets.List(ctx)
	if err != nil || len(secrets) == 0 {
		return reply("📋 No secrets configured.")
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
		limits := b.formatSecretLimits(&s)
		lines = append(lines, fmt.Sprintf("%s `%s`%s", status, s.Label, traffic))
		if limits != "" {
			lines = append(lines, fmt.Sprintf("    %s", limits))
		}
	}
	return reply(strings.Join(lines, "\n"))
}

// cmdLink shows proxy links for a specific secret or all enabled secrets.
func (b *Bot) resolvePublicIP(ctx context.Context) string {
	settings, _ := b.deps.Settings.Load(ctx)
	publicIP := ""
	if b.deps.GetPublicIP != nil {
		publicIP = b.deps.GetPublicIP(ctx)
	}
	if publicIP == "" {
		publicIP = settings.CustomIP
	}
	return publicIP
}

func (b *Bot) sendQRForLink(ctx context.Context, link model.ProxyLink, secretLabel string) {
	if b.deps.GenerateQR == nil {
		return
	}
	qrPNG, err := b.deps.GenerateQR(ctx, link.WebLink)
	if err != nil {
		return
	}
	caption := fmt.Sprintf("🔗 %s — %s :%d — %s", secretLabel, link.InstanceLabel, link.InstancePort, link.Domain)
	if sendErr := b.SendPhoto(ctx, qrPNG, caption); sendErr != nil {
		log.Errorf("QR photo send error: %v", sendErr)
	}
}

func (b *Bot) cmdLinkSingle(ctx context.Context, sec *model.Secret, instances []model.Instance, publicIP string) cmdResp {
	var lines []string
	lines = append(lines, fmt.Sprintf("🔗 *Links* — `%s`:", sec.Label))

	links := service.BuildLinksForSecret(sec, instances, publicIP)

	var rows [][]InlineKeyboardButton
	for _, link := range links {
		lines = append(lines, fmt.Sprintf("`%s` :%d — `%s`", link.InstanceLabel, link.InstancePort, link.Domain))
		lines = append(lines, fmt.Sprintf("`%s`", link.TGLink))
		lines = append(lines, link.WebLink)
		b.sendQRForLink(ctx, link, sec.Label)

		if link.TGLink != "" {
			rows = append(rows, []InlineKeyboardButton{{
				Text: fmt.Sprintf("🔗 %s :%d", link.InstanceLabel, link.InstancePort),
				URL:  link.WebLink,
			}})
		}
	}

	if len(lines) <= 1 {
		return reply(fmt.Sprintf("No accessible instances for secret `%s`.", sec.Label))
	}

	if wu := b.webURL(ctx); wu != "" {
		rows = append(rows, []InlineKeyboardButton{{Text: "Dashboard", URL: wu}})
	}

	return replyKB(strings.Join(lines, "\n"), rows)
}

func (b *Bot) cmdLinkAll(ctx context.Context, instances []model.Instance, publicIP string) cmdResp {
	secrets, _ := b.deps.Secrets.List(ctx)
	var allLines []string
	var rows [][]InlineKeyboardButton
	for _, s := range secrets {
		if !s.Enabled {
			continue
		}
		links := service.BuildLinksForSecret(&s, instances, publicIP)
		for _, link := range links {
			allLines = append(allLines, fmt.Sprintf("🔗 `%s` `%s` :%d `%s`: `%s`", s.Label, link.InstanceLabel, link.InstancePort, link.Domain, link.TGLink))
			b.sendQRForLink(ctx, link, s.Label)

			if link.TGLink != "" {
				rows = append(rows, []InlineKeyboardButton{{
					Text: fmt.Sprintf("🔗 %s :%d (%s)", link.InstanceLabel, link.InstancePort, s.Label),
					URL:  link.WebLink,
				}})
			}
		}
	}
	if len(allLines) == 0 {
		return reply("No enabled secrets or instances.")
	}

	if wu := b.webURL(ctx); wu != "" {
		rows = append(rows, []InlineKeyboardButton{{Text: "Dashboard", URL: wu}})
	}

	return replyKB(strings.Join(allLines, "\n"), rows)
}

// cmdLink shows proxy links for a specific secret or all enabled secrets.
func (b *Bot) cmdLink(ctx context.Context, text string) cmdResp {
	publicIP := b.resolvePublicIP(ctx)
	instances, _ := b.deps.Instances.List(ctx)

	label := b.args(text)
	if label != "" {
		sec, err := b.deps.Secrets.GetByLabel(ctx, label)
		if err != nil || sec == nil {
			return reply(fmt.Sprintf("Secret `%s` not found.", label))
		}
		return b.cmdLinkSingle(ctx, sec, instances, publicIP)
	}

	return b.cmdLinkAll(ctx, instances, publicIP)
}

// cmdAdd adds a new secret with the given label.
func (b *Bot) cmdAdd(ctx context.Context, text string) cmdResp {
	label := b.args(text)
	if label == "" {
		return reply("Usage: /add <label>")
	}
	if err := model.ValidateLabel(label); err != nil {
		return reply(fmt.Sprintf("Invalid label: %s", err.Error()))
	}

	existing, _ := b.deps.Secrets.GetByLabel(ctx, label)
	if existing != nil {
		return reply(fmt.Sprintf("Secret `%s` already exists.", label))
	}

	secretKey, err := telemt.GenerateSecret()
	if err != nil {
		return reply(fmt.Sprintf("Failed to generate secret: %s", err.Error()))
	}

	sec := &model.Secret{
		Label:     label,
		SecretKey: secretKey,
		Enabled:   true,
	}
	if err := b.deps.Secrets.Create(ctx, sec); err != nil {
		return reply(fmt.Sprintf("Failed to create secret: %s", err.Error()))
	}

	return reply(fmt.Sprintf("✅ Secret `%s` added. Use /restart to apply.", label))
}

// cmdRemove removes a secret by label.
func (b *Bot) cmdRemove(ctx context.Context, text string) cmdResp {
	label := b.args(text)
	if label == "" {
		return reply("Usage: /remove <label>")
	}

	existing, _ := b.deps.Secrets.GetByLabel(ctx, label)
	if existing == nil {
		return reply(fmt.Sprintf("Secret `%s` not found.", label))
	}

	if err := b.deps.Secrets.Delete(ctx, label); err != nil {
		return reply(fmt.Sprintf("Failed to remove: %s", err.Error()))
	}

	return reply(fmt.Sprintf("🗑 Secret `%s` removed. Use /restart to apply.", label))
}

// cmdRotate rotates a secret's key.
func (b *Bot) cmdRotate(ctx context.Context, text string) cmdResp {
	label := b.args(text)
	if label == "" {
		return reply("Usage: /rotate <label>")
	}

	existing, err := b.deps.Secrets.GetByLabel(ctx, label)
	if err != nil || existing == nil {
		return reply(fmt.Sprintf("Secret `%s` not found.", label))
	}

	newKey, err := telemt.GenerateSecret()
	if err != nil {
		return reply(fmt.Sprintf("Failed to generate key: %s", err.Error()))
	}

	existing.SecretKey = newKey
	if err := b.deps.Secrets.Update(ctx, existing); err != nil {
		return reply(fmt.Sprintf("Failed to rotate: %s", err.Error()))
	}

	return reply(fmt.Sprintf("🔄 Secret `%s` rotated. Use /restart to apply.", label))
}

// cmdRestart restarts the proxy.
func (b *Bot) cmdRestart(ctx context.Context) cmdResp {
	if b.deps.RestartProxy == nil {
		return reply("⚠ Restart not available.")
	}
	if err := b.deps.RestartProxy(ctx); err != nil {
		return reply(fmt.Sprintf("❌ Restart failed: %s", err.Error()))
	}
	return reply("🔄 Proxy restarted.")
}

// cmdStartInstance starts a specific instance by label.
func (b *Bot) cmdStartInstance(ctx context.Context, text string) cmdResp {
	label := b.args(text)
	if label == "" {
		return reply("Usage: /start <label>")
	}

	instances, err := b.deps.Instances.List(ctx)
	if err != nil || len(instances) == 0 {
		return reply("No instances found.")
	}

	for _, inst := range instances {
		if inst.Label == label || inst.Label == strings.TrimSpace(label) {
			if b.deps.StartInstance == nil {
				return reply("⚠ Start not available.")
			}
			if err := b.deps.StartInstance(ctx, inst.ID); err != nil {
				return reply(fmt.Sprintf("❌ Failed to start `%s`: %s", inst.Label, err.Error()))
			}
			return reply(fmt.Sprintf("▶ Instance `%s` (:%d) started.", inst.Label, inst.Port))
		}
	}
	return reply(fmt.Sprintf("Instance `%s` not found.", label))
}

// cmdStopInstance stops a specific instance by label.
func (b *Bot) cmdStopInstance(ctx context.Context, text string) cmdResp {
	label := b.args(text)
	if label == "" {
		return reply("Usage: /stop <label>")
	}

	instances, err := b.deps.Instances.List(ctx)
	if err != nil || len(instances) == 0 {
		return reply("No instances found.")
	}

	for _, inst := range instances {
		if inst.Label == label || inst.Label == strings.TrimSpace(label) {
			if b.deps.StopInstance == nil {
				return reply("⚠ Stop not available.")
			}
			if err := b.deps.StopInstance(ctx, inst.ID); err != nil {
				return reply(fmt.Sprintf("❌ Failed to stop `%s`: %s", inst.Label, err.Error()))
			}
			return reply(fmt.Sprintf("⏹ Instance `%s` (:%d) stopped.", inst.Label, inst.Port))
		}
	}

	return reply(fmt.Sprintf("Instance `%s` not found.", label))
}

// cmdEnable enables a secret.
func (b *Bot) cmdEnable(ctx context.Context, text string) cmdResp {
	label := b.args(text)
	if label == "" {
		return reply("Usage: /enable <label>")
	}

	sec, err := b.deps.Secrets.GetByLabel(ctx, label)
	if err != nil || sec == nil {
		return reply(fmt.Sprintf("Secret `%s` not found.", label))
	}

	if sec.Enabled {
		return reply(fmt.Sprintf("Secret `%s` is already enabled.", label))
	}

	sec.Enabled = true
	if err := b.deps.Secrets.Update(ctx, sec); err != nil {
		return reply(fmt.Sprintf("Failed: %s", err.Error()))
	}

	return reply(fmt.Sprintf("✅ Secret `%s` enabled. Use /restart to apply.", label))
}

// cmdDisable disables a secret.
func (b *Bot) cmdDisable(ctx context.Context, text string) cmdResp {
	label := b.args(text)
	if label == "" {
		return reply("Usage: /disable <label>")
	}

	sec, err := b.deps.Secrets.GetByLabel(ctx, label)
	if err != nil || sec == nil {
		return reply(fmt.Sprintf("Secret `%s` not found.", label))
	}

	if !sec.Enabled {
		return reply(fmt.Sprintf("Secret `%s` is already disabled.", label))
	}

	enabled, _ := b.deps.Secrets.CountEnabled(ctx)
	if enabled <= 1 {
		return reply("⚠ Cannot disable the last active secret.")
	}

	sec.Enabled = false
	if err := b.deps.Secrets.Update(ctx, sec); err != nil {
		return reply(fmt.Sprintf("Failed: %s", err.Error()))
	}

	return reply(fmt.Sprintf("❌ Secret `%s` disabled. Use /restart to apply.", label))
}

// cmdTraffic shows detailed traffic breakdown.
func (b *Bot) cmdTraffic(ctx context.Context) cmdResp {
	global, err := b.deps.Traffic.GetGlobal(ctx)
	if err != nil {
		log.Warnf("cmdTraffic: get global traffic: %v", err)
		return reply("📊 Traffic data unavailable.")
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

	return replyKB(strings.Join(lines, "\n"), b.dashboardKB(ctx))
}

// cmdUpdate shows version info.
func (b *Bot) cmdUpdate(ctx context.Context) cmdResp {
	var lines []string
	lines = append(lines, fmt.Sprintf("📦 *PopuGate* `%s`", model.VersionTag()))
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
	return replyKB(strings.Join(lines, "\n"), b.pageKB(ctx, "Updates", "/updates"))
}

// cmdSetLimit sets limits for a user: /setlimit <label> <conns> <ips> <quota> [expires]
func (b *Bot) cmdSetLimit(ctx context.Context, text string) cmdResp {
	parts := strings.Fields(text)
	if len(parts) < 5 {
		return reply("Usage: /setlimit <label> <max_conns> <max_ips> <quota_mb> [expires YYYY-MM-DD]")
	}
	label := parts[1]

	sec, err := b.deps.Secrets.GetByLabel(ctx, label)
	if err != nil || sec == nil {
		return reply(fmt.Sprintf("Secret `%s` not found.", label))
	}

	var conns int
	if n, _ := fmt.Sscanf(parts[2], "%d", &conns); n != 1 {
		return reply("Invalid max_conns value. Must be a number.")
	}

	var ips int
	if n, _ := fmt.Sscanf(parts[3], "%d", &ips); n != 1 {
		return reply("Invalid max_ips value. Must be a number.")
	}

	var quotaMB int64
	if n, _ := fmt.Sscanf(parts[4], "%d", &quotaMB); n != 1 {
		return reply("Invalid quota_mb value. Must be a number.")
	}
	quotaBytes := quotaMB * 1024 * 1024

	sec.MaxConns = conns
	sec.MaxIPs = ips
	sec.QuotaBytes = quotaBytes

	if len(parts) >= 6 {
		sec.ExpiresAt = parts[5]
	}

	if err := b.deps.Secrets.Update(ctx, sec); err != nil {
		return reply(fmt.Sprintf("Failed: %s", err.Error()))
	}

	return reply(fmt.Sprintf("✅ Limits for `%s` updated: conns=%d ips=%d quota=%dMB",
		label, conns, ips, quotaMB))
}

// cmdUpstreams lists configured upstreams.
func (b *Bot) cmdUpstreams(ctx context.Context) cmdResp {
	upstreams, err := b.deps.Upstreams.List(ctx)
	if err != nil || len(upstreams) == 0 {
		return reply("🔀 No upstreams configured.")
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
	return replyKB(strings.Join(lines, "\n"), b.pageKB(ctx, "Upstreams", "/upstreams"))
}

// cmdTasks shows scheduled tasks status.
func (b *Bot) cmdTasks(ctx context.Context) cmdResp {
	if b.deps.GetSchedulerTasks == nil {
		return reply("⚠ Scheduler status not available.")
	}

	lines := b.deps.GetSchedulerTasks(ctx)
	if len(lines) == 0 {
		return reply("📋 No scheduled tasks.")
	}

	return replyKB(strings.Join(lines, "\n"), b.pageKB(ctx, "Scheduler", "/scheduler"))
}

// cmdHelp shows a bot description and all available commands.
func (b *Bot) cmdHelp() cmdResp {
	label := b.label
	if label == "" {
		label = "PopuGate"
	}
	text := strings.Join([]string{
		fmt.Sprintf("📖 *Bot* — `%s`", mdSafe(label)),
		"",
		"Telegram MTProto proxy manager. Use the commands below to control your proxy, manage secrets, and monitor traffic.",
		"",
		"*Management:*",
		"/status — Proxy status, engine, uptime",
		"/instances — List instances with config",
		"/restart — Restart proxy",
		"/start <label> — Start instance",
		"/stop <label> — Stop instance",
		"",
		"*Secrets:*",
		"/secrets — List secrets with limits",
		"/info <label> — Detailed secret card",
		"/link [label] — Proxy links + QR",
		"/add <label> — Add secret",
		"/remove <label> — Remove secret",
		"/rotate <label> — Rotate secret key",
		"/enable <label> — Enable secret",
		"/disable <label> — Disable secret",
		"/setlimit <label> <conns> <ips> <quota-MB> [date] — Set limits",
		"/resetquota <label> — Reset traffic counters",
		"",
		"*Traffic:*",
		"/traffic — Traffic report",
		"",
		"*System:*",
		"/geoblock — Geo-blocking status",
		"/replication — Replication status",
		"/backup [create] — Backup status or create",
		"/upstreams — List upstreams",
		"/tasks — Scheduled tasks status",
		"/update — Version info",
		"/help — This message",
	}, "\n")
	return reply(text)
}

// --- New commands ---

// formatSecretLimits returns a compact one-line summary of limits for a secret.
func (b *Bot) formatSecretLimits(s *model.Secret) string {
	var parts []string
	if s.MaxConns > 0 {
		parts = append(parts, fmt.Sprintf("conns=%d", s.MaxConns))
	}
	if s.MaxIPs > 0 {
		parts = append(parts, fmt.Sprintf("ips=%d", s.MaxIPs))
	}
	if s.QuotaBytes > 0 {
		pct := s.QuotaPercent()
		parts = append(parts, fmt.Sprintf("quota=%s (%.0f%%)", formatBytes(s.QuotaBytes), pct))
	}
	if s.ExpiresAt != "" && s.ExpiresAt != "0" {
		parts = append(parts, fmt.Sprintf("expires=%s", s.ExpiresAt))
	}
	return strings.Join(parts, " ")
}

// cmdInstances lists all instances with their configuration.
func (b *Bot) cmdInstances(ctx context.Context) cmdResp {
	instances, err := b.deps.Instances.List(ctx)
	if err != nil || len(instances) == 0 {
		return reply("📦 No instances configured.")
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("📦 *Instances (%d)*", len(instances)))
	for _, inst := range instances {
		status := "✅"
		if !inst.Enabled {
			status = "❌"
		}
		running := ""
		if inst.Enabled && b.deps.IsInstanceRunning != nil {
			if b.deps.IsInstanceRunning(ctx, inst.ContainerName()) {
				running = " 🟢"
			} else {
				running = " 🔴"
			}
		}
		domain := inst.TLSDomain
		if domain == "" {
			domain = "-"
		}
		lines = append(lines, fmt.Sprintf("%s `%s` :%d/%d — `%s`%s", status, inst.Label, inst.Port, inst.MetricsPort, domain, running))

		var details []string
		if inst.FakeTLS {
			details = append(details, "FakeTLS")
		}
		if inst.TLSFronting {
			details = append(details, "TLS-Front")
		}
		if inst.TCPMSSEnabled {
			details = append(details, fmt.Sprintf("MSS=%d", inst.TCPMSS))
		}
		if inst.MaskHost != "" {
			details = append(details, fmt.Sprintf("mask=`%s`:%d", inst.MaskHost, inst.MaskPort))
		}
		if len(details) > 0 {
			lines = append(lines, fmt.Sprintf("    %s", strings.Join(details, " ")))
		}
	}
	return replyKB(strings.Join(lines, "\n"), b.dashboardKB(ctx))
}

// cmdInfo shows a detailed card for a single secret.
func (b *Bot) cmdInfo(ctx context.Context, text string) cmdResp {
	label := b.args(text)
	if label == "" {
		return reply("Usage: /info <label>")
	}

	sec, err := b.deps.Secrets.GetByLabel(ctx, label)
	if err != nil || sec == nil {
		return reply(fmt.Sprintf("Secret `%s` not found.", label))
	}

	var lines []string
	status := "✅ Enabled"
	if !sec.Enabled {
		status = "❌ Disabled"
	}
	lines = append(lines, fmt.Sprintf("🔑 *Secret* — `%s` %s", sec.Label, status))

	// Key (masked)
	if len(sec.SecretKey) > 8 {
		lines = append(lines, fmt.Sprintf("Key: `%s...%s`", sec.SecretKey[:4], sec.SecretKey[len(sec.SecretKey)-4:]))
	} else {
		lines = append(lines, fmt.Sprintf("Key: `%s`", sec.SecretKey))
	}

	// Created
	if sec.CreatedAt > 0 {
		lines = append(lines, fmt.Sprintf("Created: `%s`", time.Unix(sec.CreatedAt, 0).Format("2006-01-02 15:04")))
	}

	// Limits
	var limitParts []string
	conns := "∞"
	if sec.MaxConns > 0 {
		conns = fmt.Sprintf("%d", sec.MaxConns)
	}
	limitParts = append(limitParts, fmt.Sprintf("conns=%s", conns))
	ips := "∞"
	if sec.MaxIPs > 0 {
		ips = fmt.Sprintf("%d", sec.MaxIPs)
	}
	limitParts = append(limitParts, fmt.Sprintf("ips=%s", ips))
	if sec.QuotaBytes > 0 {
		pct := sec.QuotaPercent()
		limitParts = append(limitParts, fmt.Sprintf("quota=%s (%.0f%%)", formatBytes(sec.QuotaBytes), pct))
	} else {
		limitParts = append(limitParts, "quota=∞")
	}
	if sec.ExpiresAt != "" && sec.ExpiresAt != "0" {
		limitParts = append(limitParts, fmt.Sprintf("expires=%s", sec.ExpiresAt))
	}
	lines = append(lines, fmt.Sprintf("Limits: %s", strings.Join(limitParts, " ")))

	// Traffic
	lines = append(lines, fmt.Sprintf("Traffic: ↓%s ↑%s (total %s)",
		formatBytes(sec.TrafficIn), formatBytes(sec.TrafficOut),
		formatBytes(sec.TrafficIn+sec.TrafficOut)))

	// Tags
	if tags := sec.GetTags(); len(tags) > 0 {
		lines = append(lines, fmt.Sprintf("Tags: `%s`", strings.Join(tags, "`, `")))
	}

	// Notes
	if sec.Notes != "" {
		lines = append(lines, fmt.Sprintf("Notes: %s", mdSafe(sec.Notes)))
	}

	return reply(strings.Join(lines, "\n"))
}

// cmdGeoblock shows geo-blocking status.
func (b *Bot) cmdGeoblock(ctx context.Context) cmdResp {
	settings, _ := b.deps.Settings.Load(ctx)
	if settings == nil {
		return reply("⚠ Settings unavailable.")
	}

	mode := settings.GeoblockMode
	if mode == "" {
		mode = "disabled"
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("🌍 *Geo-blocking* — mode: `%s`", mode))

	countries := settings.BlocklistCountries
	if countries == "" {
		lines = append(lines, "No countries configured.")
	} else {
		var codes []string
		for _, c := range strings.Split(countries, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				codes = append(codes, c)
			}
		}
		lines = append(lines, fmt.Sprintf("Countries (%d): `%s`", len(codes), strings.Join(codes, "`, `")))

		// Show cache status for each country
		if b.deps.Geoblock != nil {
			for _, code := range codes {
				_, downloadedAt, err := b.deps.Geoblock.GetCache(ctx, code)
				if err == nil && downloadedAt > 0 {
					age := time.Since(time.Unix(downloadedAt, 0))
					lines = append(lines, fmt.Sprintf("  `%s`: cached (%s ago)", code, formatDuration(age)))
				}
			}
		}
	}

	return replyKB(strings.Join(lines, "\n"), b.pageKB(ctx, "Geoblock", "/geoblock"))
}

// cmdReplication shows replication/slave status.
func (b *Bot) cmdReplication(ctx context.Context) cmdResp {
	settings, _ := b.deps.Settings.Load(ctx)
	if settings == nil {
		return reply("⚠ Settings unavailable.")
	}

	role := settings.ReplicationRole
	if role == "" {
		role = "standalone"
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("🔄 *Replication* — role: `%s`", role))

	if role == "standalone" {
		lines = append(lines, "")
		lines = append(lines, "Replication is not configured. Set up master or slave mode in the web UI.")
		return replyKB(strings.Join(lines, "\n"), b.pageKB(ctx, "Replication", "/replication"))
	}

	lines = append(lines, fmt.Sprintf("Sync interval: %ds", settings.ReplicationSyncInterval))
	lines = append(lines, fmt.Sprintf("SSH: `%s@:%d`", settings.ReplicationSSHUser, settings.ReplicationSSHPort))

	if role == "master" {
		slaves, err := b.deps.Slaves.List(ctx)
		if err != nil || len(slaves) == 0 {
			lines = append(lines, "")
			lines = append(lines, "No slaves configured.")
		} else {
			lines = append(lines, "")
			lines = append(lines, fmt.Sprintf("*Slaves (%d):*", len(slaves)))
			for _, sl := range slaves {
				status := "✅"
				if !sl.Enabled {
					status = "❌"
				}
				lastSync := "never"
				if sl.LastSync > 0 {
					lastSync = time.Unix(sl.LastSync, 0).Format("2006-01-02 15:04")
				}
				syncStatus := sl.Status
				if syncStatus == "" {
					syncStatus = "unknown"
				}
				lines = append(lines, fmt.Sprintf("%s `%s` `%s:%d` — %s (last: %s)",
					status, sl.Label, sl.Host, sl.Port, syncStatus, lastSync))
			}
		}
	}

	return replyKB(strings.Join(lines, "\n"), b.pageKB(ctx, "Replication", "/replication"))
}

// cmdBackup shows backup status or creates a new backup.
func (b *Bot) cmdBackup(ctx context.Context, text string) cmdResp {
	arg := b.args(text)

	// /backup create — trigger a new backup
	if arg == "create" {
		if b.deps.CreateBackup == nil {
			return reply("⚠ Backup creation not available.")
		}
		bk, err := b.deps.CreateBackup(ctx)
		if err != nil {
			return reply(fmt.Sprintf("❌ Backup failed: %s", err.Error()))
		}
		return reply(fmt.Sprintf("✅ Backup created: `%s` (%s)", bk.Filename, formatBytes(bk.Size)))
	}

	// /backup (no args) — show status
	if b.deps.Backups == nil {
		return reply("⚠ Backup store not available.")
	}

	backups, err := b.deps.Backups.List(ctx)
	if err != nil {
		return reply(fmt.Sprintf("⚠ Failed to list backups: %s", err.Error()))
	}

	var lines []string
	lines = append(lines, "💾 *Backups*")

	if len(backups) == 0 {
		lines = append(lines, "")
		lines = append(lines, "No backups found. Use `/backup create` to make one.")
	} else {
		settings, _ := b.deps.Settings.Load(ctx)
		retention := 7
		if settings != nil && settings.BackupRetentionDays > 0 {
			retention = settings.BackupRetentionDays
		}
		lines = append(lines, fmt.Sprintf("Retention: %d days", retention))
		lines = append(lines, "")

		// Show last 5 backups
		count := len(backups)
		if count > 5 {
			count = 5
		}
		for _, bk := range backups[:count] {
			enc := ""
			if bk.Encrypted {
				enc = " 🔒"
			}
			lines = append(lines, fmt.Sprintf("`%s` %s%s", bk.Filename, formatBytes(bk.Size), enc))
		}
		if len(backups) > 5 {
			lines = append(lines, fmt.Sprintf("... and %d more", len(backups)-5))
		}
		lines = append(lines, "")
		lines = append(lines, "Use `/backup create` to create a new backup.")
	}

	return replyKB(strings.Join(lines, "\n"), b.pageKB(ctx, "Backups", "/backups"))
}

// cmdResetQuota resets traffic counters for a specific user.
func (b *Bot) cmdResetQuota(ctx context.Context, text string) cmdResp {
	label := b.args(text)
	if label == "" {
		return reply("Usage: /resetquota <label>")
	}

	sec, err := b.deps.Secrets.GetByLabel(ctx, label)
	if err != nil || sec == nil {
		return reply(fmt.Sprintf("Secret `%s` not found.", label))
	}

	if b.deps.ResetTraffic == nil {
		// Fallback: just reset via traffic store directly
		return reply("⚠ Traffic reset not available.")
	}

	if err := b.deps.ResetTraffic(ctx, label); err != nil {
		return reply(fmt.Sprintf("❌ Failed to reset traffic for `%s`: %s", label, err.Error()))
	}

	return reply(fmt.Sprintf("✅ Traffic counters for `%s` reset to zero.", label))
}

// formatDuration returns a human-readable duration string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours > 0 {
		return fmt.Sprintf("%dd%dh", days, hours)
	}
	return fmt.Sprintf("%dd", days)
}
