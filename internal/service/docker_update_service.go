package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/logger"
)

const mobyGitHubRepo = "moby/moby"

// DockerUpdateStatus holds the result of a Docker update check.
type DockerUpdateStatus struct {
	CurrentVersion     string `json:"current_version"`
	LatestVersion      string `json:"latest_version"`
	UpdateAvailable    bool   `json:"update_available"`
	LiveRestoreEnabled bool   `json:"live_restore_enabled"`
	ChangelogURL       string `json:"changelog_url,omitempty"`
	LastChecked        string `json:"last_checked,omitempty"`
	Updating           bool   `json:"updating"`
}

// DockerUpdateService handles host Docker daemon updates and active state restoration.
type DockerUpdateService struct {
	settings     *store.SettingsStore
	dockerCli    *dockerutil.DockerClient
	containerSvc *ContainerService
	notify       atomic.Value

	mu      sync.Mutex
	applyMu sync.Mutex

	isInstanceRunningFn func(ctx context.Context, name string) (bool, error)
}

// NewDockerUpdateService creates a new DockerUpdateService.
func NewDockerUpdateService(
	settings *store.SettingsStore,
	dockerCli *dockerutil.DockerClient,
	containerSvc *ContainerService,
) *DockerUpdateService {
	svc := &DockerUpdateService{
		settings:     settings,
		dockerCli:    dockerCli,
		containerSvc: containerSvc,
	}
	svc.isInstanceRunningFn = func(ctx context.Context, name string) (bool, error) {
		if svc.dockerCli == nil {
			return false, nil
		}
		return svc.dockerCli.IsInstanceRunning(ctx, name)
	}
	return svc
}

// SetNotify sets the notification callback.
func (s *DockerUpdateService) SetNotify(fn NotifyFunc) { s.notify.Store(fn) }

func (s *DockerUpdateService) getNotify() NotifyFunc {
	if v := s.notify.Load(); v != nil {
		return v.(NotifyFunc)
	}
	return nil
}

// GetStatus returns the current vs. latest Docker release info for the UI.
func (s *DockerUpdateService) GetStatus(ctx context.Context) (*DockerUpdateStatus, error) {
	currentVersion := ""
	liveRestore := false

	if s.dockerCli != nil && s.dockerCli.IsInstalled(ctx) {
		info, err := s.dockerCli.Info(ctx)
		if err == nil {
			currentVersion = info.ServerVersion
			liveRestore = info.LiveRestoreEnabled
		}
	}

	latestVersion, _ := s.settings.Get(ctx, "docker_latest_version")
	lastChecked, _ := s.settings.Get(ctx, "docker_latest_checked")
	changelogURL, _ := s.settings.Get(ctx, "docker_changelog_url")
	updatingFlag, _ := s.settings.Get(ctx, "docker_updating")

	status := &DockerUpdateStatus{
		CurrentVersion:     currentVersion,
		LiveRestoreEnabled: liveRestore,
		LastChecked:        lastChecked,
		Updating:           updatingFlag == "true",
	}

	if latestVersion != "" {
		status.LatestVersion = latestVersion
		status.ChangelogURL = changelogURL
		status.UpdateAvailable = isVersionNewer(currentVersion, latestVersion)
	}

	return status, nil
}

// CheckRemote queries the GitHub API for the latest moby/moby release and caches it.
func (s *DockerUpdateService) CheckRemote(ctx context.Context) (*DockerUpdateStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	client := &http.Client{Timeout: 15 * time.Second}
	latestURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", mobyGitHubRepo)

	req, err := http.NewRequestWithContext(ctx, "GET", latestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("check Docker release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}

	// moby/moby tags are like docker-v29.5.2 or v27.0.3, trim docker-v or v prefix
	version := release.TagName
	version = strings.TrimPrefix(version, "docker-v")
	version = strings.TrimPrefix(version, "v")

	nowUnix := fmt.Sprintf("%d", time.Now().Unix())
	_ = s.settings.Save(ctx, map[string]string{
		"docker_latest_version": version,
		"docker_changelog_url":  release.HTMLURL,
		"docker_latest_checked": nowUnix,
	})

	return s.GetStatus(ctx)
}

// isVersionNewer compares installed current version with latest available semver versions.
func isVersionNewer(current, latest string) bool {
	if current == "" || latest == "" || current == latest {
		return false
	}
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	currBase, currPre := splitPreRelease(current)
	lateBase, latePre := splitPreRelease(latest)

	cmp := compareNumericParts(strings.Split(currBase, "."), strings.Split(lateBase, "."))
	if cmp != 0 {
		return cmp > 0
	}

	return comparePreReleases(currPre, latePre)
}

func splitPreRelease(version string) (string, string) {
	if idx := strings.Index(version, "-"); idx >= 0 {
		return version[:idx], version[idx+1:]
	}
	return version, ""
}

func compareNumericParts(currParts, lateParts []string) int {
	for i := 0; i < len(currParts) || i < len(lateParts); i++ {
		var currVal, lateVal int
		if i < len(currParts) {
			_, _ = fmt.Sscanf(currParts[i], "%d", &currVal)
		}
		if i < len(lateParts) {
			_, _ = fmt.Sscanf(lateParts[i], "%d", &lateVal)
		}
		if lateVal > currVal {
			return 1
		}
		if currVal > lateVal {
			return -1
		}
	}
	return 0
}

func comparePreReleases(currPre, latePre string) bool {
	if currPre != "" && latePre == "" {
		return true
	}
	if currPre == "" && latePre != "" {
		return false
	}
	return latePre > currPre
}

// SnapshotItem represents a single instance running snapshot unit.
type SnapshotItem struct {
	InstanceID    int64  `json:"instance_id"`
	ContainerName string `json:"container_name"`
}

// CreateStateSnapshot captures all currently running proxy containers.
func (s *DockerUpdateService) CreateStateSnapshot(ctx context.Context) error {
	insts, err := s.containerSvc.instances.List(ctx)
	if err != nil {
		return fmt.Errorf("list instances: %w", err)
	}

	var runningSnapshots []SnapshotItem
	for _, inst := range insts {
		running, err := s.isInstanceRunningFn(ctx, inst.ContainerName())
		if err == nil && running {
			runningSnapshots = append(runningSnapshots, SnapshotItem{
				InstanceID:    inst.ID,
				ContainerName: inst.ContainerName(),
			})
		}
	}

	snapJSON, err := json.Marshal(runningSnapshots)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	err = s.settings.Save(ctx, map[string]string{
		"docker_update_snapshot": string(snapJSON),
	})
	if err != nil {
		return fmt.Errorf("save snapshot settings: %w", err)
	}

	return nil
}

// RestoreFromSnapshot restores active proxy containers to their pre-update state.
func (s *DockerUpdateService) RestoreFromSnapshot(ctx context.Context) error {
	log := logger.WithScope("docker-restore")

	snapStr, err := s.settings.Get(ctx, "docker_update_snapshot")
	if err != nil || snapStr == "" {
		log.Infof("no active Docker update snapshot found to restore")
		return nil
	}

	var snapshot []SnapshotItem
	if err := json.Unmarshal([]byte(snapStr), &snapshot); err != nil {
		return fmt.Errorf("unmarshal snapshot: %w", err)
	}

	if len(snapshot) == 0 {
		log.Infof("snapshot is empty; no instances to restore")
		_ = s.settings.Save(ctx, map[string]string{
			"docker_update_snapshot": "",
		})
		return nil
	}

	log.Infof("restoring %d proxy instances from snapshot...", len(snapshot))

	var restoreErrors []string
	for _, item := range snapshot {
		running, err := s.isInstanceRunningFn(ctx, item.ContainerName)
		if err == nil && running {
			log.Infof("instance %s is already running", item.ContainerName)
			continue
		}

		log.Infof("starting proxy instance ID %d (name: %s)...", item.InstanceID, item.ContainerName)
		err = s.containerSvc.StartInstance(ctx, item.InstanceID)
		if err != nil {
			log.Warnf("failed to restore instance ID %d: %v", item.InstanceID, err)
			restoreErrors = append(restoreErrors, fmt.Sprintf("instance ID %d: %v", item.InstanceID, err))
		} else {
			log.Infof("successfully restored instance %s", item.ContainerName)
		}
	}

	_ = s.settings.Save(ctx, map[string]string{
		"docker_update_snapshot": "",
	})

	if len(restoreErrors) > 0 {
		return fmt.Errorf("restoration failures: %s", strings.Join(restoreErrors, "; "))
	}

	return nil
}

// Apply triggers the host Docker Engine package update asynchronously.
func (s *DockerUpdateService) Apply(ctx context.Context) error {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()

	log := logger.WithScope("docker-update")

	status, err := s.GetStatus(ctx)
	if err != nil {
		return fmt.Errorf("get update status: %w", err)
	}
	if status.Updating {
		return fmt.Errorf("docker update is already in progress")
	}

	if !status.UpdateAvailable {
		return fmt.Errorf("already up to date (version %s)", status.CurrentVersion)
	}

	log.Infof("starting Docker Engine update from version %s to %s...", status.CurrentVersion, status.LatestVersion)

	_ = s.settings.Save(ctx, map[string]string{
		"docker_updating":     "true",
		"docker_update_error": "",
	})

	notify := s.getNotify()
	if notify != nil {
		notify(context.Background(), "🔄 *%s* Starting host Docker Engine update from version %s to %s...", status.CurrentVersion, status.LatestVersion)
	}

	if err := s.CreateStateSnapshot(ctx); err != nil {
		log.Errorf("failed to create proxy snapshot: %v", err)
	}

	go s.runUpdateLoop(status.CurrentVersion, status.LatestVersion)

	return nil
}

func (s *DockerUpdateService) runUpdateLoop(currentVer, targetVer string) {
	defer func() {
		if r := recover(); r != nil {
			logger.WithScope("docker-update").Errorf("recovered panic in background update: %v", r)
		}
	}()

	log := logger.WithScope("docker-update")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	notify := s.getNotify()

	cmdName, cmdArgs, err := s.resolvePackageManager()
	if err != nil {
		log.Errorf("%v", err)
		s.completeUpdateWithError(ctx, err.Error(), notify, currentVer, targetVer)
		return
	}

	outputLog, err := s.runUpgradeCommand(ctx, cmdName, cmdArgs)
	if err != nil {
		errStr := fmt.Sprintf("%v\nOutput:\n%s", err, outputLog)
		s.completeUpdateWithError(ctx, errStr, notify, currentVer, targetVer)
		return
	}

	log.Infof("Docker Engine package update completed successfully!")
	log.Infof("waiting for Docker daemon socket to be healthy...")

	if !s.waitForDockerDaemon(ctx) {
		errStr := "Docker daemon did not become healthy after update within 60 seconds"
		s.completeUpdateWithError(ctx, errStr, notify, currentVer, targetVer)
		return
	}

	log.Infof("Docker daemon is healthy. Restoring active proxies...")

	restoreErr := s.RestoreFromSnapshot(ctx)
	if restoreErr != nil {
		log.Warnf("some proxies failed to restore: %v", restoreErr)
	}

	_ = s.settings.Save(ctx, map[string]string{
		"docker_updating":     "false",
		"docker_update_error": "",
	})

	if notify != nil {
		notify(context.Background(), "✅ *%s* Docker Engine successfully updated to version %s! All proxies restored.", targetVer)
	}
}

func (s *DockerUpdateService) resolvePackageManager() (string, []string, error) {
	if _, err := exec.LookPath("apt-get"); err == nil {
		return "apt-get", []string{"install", "-y", "--only-upgrade", "docker-ce", "docker-ce-cli", "containerd.io"}, nil
	}
	if _, err := exec.LookPath("yum"); err == nil {
		return "yum", []string{"update", "-y", "docker-ce", "docker-ce-cli", "containerd.io"}, nil
	}
	return "", nil, fmt.Errorf("no supported package manager found (apt-get or yum required)")
}

func (s *DockerUpdateService) runUpgradeCommand(ctx context.Context, cmdName string, cmdArgs []string) (string, error) {
	log := logger.WithScope("docker-update")
	if cmdName == "apt-get" {
		updateCmd := exec.CommandContext(ctx, "apt-get", "update")
		if out, err := updateCmd.CombinedOutput(); err != nil {
			log.Warnf("apt-get update failed: %v: %s", err, string(out))
		}
	}

	log.Infof("running package upgrade: %s %s", cmdName, strings.Join(cmdArgs, " "))
	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	cmd.Env = append(cmd.Env, "DEBIAN_FRONTEND=noninteractive")

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start update command: %w", err)
	}

	var outputLog strings.Builder
	scanner := bufio.NewScanner(stdoutPipe)
	for scanner.Scan() {
		line := scanner.Text()
		log.Infof("[docker-host-upgrade] %s", line)
		outputLog.WriteString(line + "\n")
	}

	if err := cmd.Wait(); err != nil {
		return outputLog.String(), fmt.Errorf("update command failed: %w", err)
	}

	return outputLog.String(), nil
}

func (s *DockerUpdateService) waitForDockerDaemon(ctx context.Context) bool {
	for i := 0; i < 30; i++ {
		time.Sleep(2 * time.Second)
		if s.dockerCli != nil && s.dockerCli.IsInstalled(ctx) {
			return true
		}
	}
	return false
}

func (s *DockerUpdateService) completeUpdateWithError(ctx context.Context, errStr string, notify NotifyFunc, currentVer, targetVer string) {
	log := logger.WithScope("docker-update")
	log.Errorf("Docker update failed: %s", errStr)

	_ = s.settings.Save(ctx, map[string]string{
		"docker_updating":     "false",
		"docker_update_error": errStr,
	})

	_ = s.RestoreFromSnapshot(ctx)

	if notify != nil {
		notify(context.Background(), "❌ *%s* Host Docker Engine update failed: %s", targetVer, errStr)
	}
}

// HandleStartupRecovery is invoked at server boot to recover proxies if the server restarted mid-update.
func (s *DockerUpdateService) HandleStartupRecovery(ctx context.Context) {
	log := logger.WithScope("docker-recovery")

	updatingFlag, _ := s.settings.Get(ctx, "docker_updating")
	snapStr, _ := s.settings.Get(ctx, "docker_update_snapshot")

	if updatingFlag != "true" && snapStr == "" {
		return
	}

	log.Warnf("server restarted mid-update or snapshot exists — initiating background restoration loop")

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("recovered panic in startup recovery: %v", r)
			}
		}()

		recoveryCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		log.Infof("waiting for Docker daemon socket to be healthy...")
		daemonHealthy := false
		for i := 0; i < 60; i++ {
			select {
			case <-recoveryCtx.Done():
				log.Errorf("recovery timeout reached waiting for Docker socket")
				return
			default:
				if s.dockerCli != nil && s.dockerCli.IsInstalled(recoveryCtx) {
					daemonHealthy = true
					break
				}
				time.Sleep(3 * time.Second)
			}
		}

		if !daemonHealthy {
			log.Errorf("Docker daemon did not become healthy; skipping restoration")
			return
		}

		log.Infof("Docker socket is online. Commencing proxy states recovery...")

		notify := s.getNotify()
		err := s.RestoreFromSnapshot(recoveryCtx)

		_ = s.settings.Save(recoveryCtx, map[string]string{
			"docker_updating": "false",
		})

		if err != nil {
			log.Errorf("startup restoration failed: %v", err)
			if notify != nil {
				notify(context.Background(), "⚠️ *%s* Server restarted after Docker update, but some proxies failed to restore: %v", err)
			}
		} else {
			log.Infof("startup restoration completed successfully; all proxy containers are active")
			if notify != nil {
				notify(context.Background(), "🟢 *%s* Server successfully restarted after Docker update and restored all active proxies!", "Host Recovery")
			}
		}
	}()
}
