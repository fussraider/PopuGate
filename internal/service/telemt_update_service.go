package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/logger"
)

const telemtGitHubRepo = "telemt/telemt"

const (
	// updateTimeout bounds the whole background update (pull/build can stall
	// on a dead network or hung docker daemon).
	updateTimeout = 35 * time.Minute
	// restartTimeout bounds the proxy restart after a successful build.
	restartTimeout = 10 * time.Minute
)

// engineBuilder builds the telemt engine image (implemented by DockerService).
type engineBuilder interface {
	BuildEngine(ctx context.Context, force bool, trigger string) (*BuildResult, error)
	BuildRunning() bool
}

// proxyRestarter restarts the proxy container (implemented by ContainerService).
type proxyRestarter interface {
	Restart(ctx context.Context) error
}

// TelemtUpdateService handles checking and applying telemt engine updates.
type TelemtUpdateService struct {
	settings     *store.SettingsStore
	dockerSvc    engineBuilder
	containerSvc proxyRestarter
	telemtCfg    *DBTelemtConfig
	notify       atomic.Value

	mu          sync.RWMutex
	subscribers map[chan *TelemtUpdateStatus]struct{}

	applyMu sync.Mutex
	cancel  context.CancelFunc
}

// TelemtReleaseInfo holds information about a remote telemt release.
type TelemtReleaseInfo struct {
	Version     string `json:"version"`
	Commit      string `json:"commit,omitempty"`
	TagName     string `json:"tag_name,omitempty"`
	HTMLURL     string `json:"html_url,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
}

// TelemtReleaseListItem is a trimmed release entry for the UI list.
type TelemtReleaseListItem struct {
	Version     string `json:"version"`
	Commit      string `json:"commit"`
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
	Prerelease  bool   `json:"prerelease"`
}

// TelemtUpdateStatus is returned to the UI.
type TelemtUpdateStatus struct {
	Current         string             `json:"current"`
	Latest          *TelemtReleaseInfo `json:"latest,omitempty"`
	UpdateAvailable bool               `json:"update_available"`
	LastChecked     string             `json:"last_checked,omitempty"`
	Updating        bool               `json:"updating"`
	UpdatingTo      string             `json:"updating_to,omitempty"`
	LastError       string             `json:"last_error,omitempty"`
}

// NewTelemtUpdateService creates a new TelemtUpdateService.
func NewTelemtUpdateService(
	settings *store.SettingsStore,
	dockerSvc *DockerService,
	containerSvc *ContainerService,
	telemtCfg *DBTelemtConfig,
) *TelemtUpdateService {
	s := &TelemtUpdateService{
		settings:    settings,
		telemtCfg:   telemtCfg,
		subscribers: make(map[chan *TelemtUpdateStatus]struct{}),
	}
	// Assign via nil checks so a nil *DockerService/*ContainerService does not
	// become a non-nil interface wrapping a nil pointer.
	if dockerSvc != nil {
		s.dockerSvc = dockerSvc
	}
	if containerSvc != nil {
		s.containerSvc = containerSvc
	}
	return s
}

// Subscribe returns a channel that receives TelemtUpdateStatus updates.
func (s *TelemtUpdateService) Subscribe() (chan *TelemtUpdateStatus, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan *TelemtUpdateStatus, 1)
	s.subscribers[ch] = struct{}{}

	return ch, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.subscribers, ch)
		close(ch)
	}
}

func (s *TelemtUpdateService) broadcast(status *TelemtUpdateStatus) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for ch := range s.subscribers {
		select {
		case ch <- status:
		default:
			// Buffer full, skip this subscriber
		}
	}
}

// SetNotify sets the notification callback.
func (s *TelemtUpdateService) SetNotify(fn NotifyFunc) { s.notify.Store(fn) }

// ResetStaleUpdate clears a stale "updating" flag left from a crash/restart.
// Should be called once at server startup.
func (s *TelemtUpdateService) ResetStaleUpdate(ctx context.Context) {
	updatingFlag, _ := s.settings.Get(ctx, "telemt_updating")
	if updatingFlag == "true" {
		logger.WithScope("telemt-update").Warnf("clearing stale telemt update flag (server restarted mid-update)")
		_ = s.settings.Save(ctx, map[string]string{
			"telemt_updating":    "false",
			"telemt_updating_to": "",
		})
		if status, err := s.GetStatus(ctx); err == nil {
			s.broadcast(status)
		}
	}
}

// CheckRemote queries the GitHub API for the latest telemt release, fetches the
// full releases list, and caches everything.
func (s *TelemtUpdateService) CheckRemote(ctx context.Context) (*TelemtReleaseInfo, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	// Fetch latest release
	latestURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", telemtGitHubRepo)
	req, err := http.NewRequestWithContext(ctx, "GET", latestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("check telemt release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release struct {
		TagName         string `json:"tag_name"`
		HTMLURL         string `json:"html_url"`
		TargetCommitish string `json:"target_commitish"`
		PublishedAt     string `json:"published_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}

	version := strings.TrimPrefix(release.TagName, "v")
	info := &TelemtReleaseInfo{
		Version:     version,
		TagName:     release.TagName,
		Commit:      shortSHA(release.TargetCommitish),
		HTMLURL:     release.HTMLURL,
		PublishedAt: release.PublishedAt,
	}

	s.cacheRelease(ctx, info)

	// Fetch and cache the full releases list (best-effort, don't fail the whole call)
	s.fetchAndCacheReleases(ctx, client)

	return info, nil
}

// GetStatus returns the current vs. latest release info for the UI.
func (s *TelemtUpdateService) GetStatus(ctx context.Context) (*TelemtUpdateStatus, error) {
	current := fmt.Sprintf("%s-%s", s.telemtCfg.TelemtVersion(), s.telemtCfg.TelemtCommit())

	latestVersion, _ := s.settings.Get(ctx, "telemt_latest_version")
	latestCommit, _ := s.settings.Get(ctx, "telemt_latest_commit")
	latestURL, _ := s.settings.Get(ctx, "telemt_latest_url")
	lastChecked, _ := s.settings.Get(ctx, "telemt_latest_checked")

	updatingFlag, _ := s.settings.Get(ctx, "telemt_updating")
	updatingTo, _ := s.settings.Get(ctx, "telemt_updating_to")
	lastError, _ := s.settings.Get(ctx, "telemt_update_error")

	status := &TelemtUpdateStatus{
		Current:     current,
		LastChecked: lastChecked,
		Updating:    updatingFlag == "true",
		UpdatingTo:  updatingTo,
		LastError:   lastError,
	}

	if latestVersion != "" {
		htmlURL := latestURL
		if htmlURL == "" {
			htmlURL = fmt.Sprintf("https://github.com/%s/releases/tag/v%s", telemtGitHubRepo, latestVersion)
		}
		status.Latest = &TelemtReleaseInfo{
			Version: latestVersion,
			Commit:  latestCommit,
			HTMLURL: htmlURL,
		}
		status.UpdateAvailable = latestVersion != s.telemtCfg.TelemtVersion() ||
			(latestCommit != "" && latestCommit != s.telemtCfg.TelemtCommit())
	}

	return status, nil
}

// Apply performs the engine update: save version -> build image -> restart proxy.
// The proxy is NOT stopped during the build. Runs asynchronously.
func (s *TelemtUpdateService) Apply(ctx context.Context, version, commit string) error {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()

	if !IsSafeGitRef(version) {
		return fmt.Errorf("invalid version: rejected by safety check")
	}
	if commit != "" && !IsSafeGitRef(commit) {
		return fmt.Errorf("invalid commit: rejected by safety check")
	}
	if s.dockerSvc == nil {
		return fmt.Errorf("docker service is not configured")
	}

	status, err := s.GetStatus(ctx)
	if err != nil {
		return err
	}
	if status.Updating {
		return fmt.Errorf("update already in progress")
	}
	if s.dockerSvc.BuildRunning() {
		return ErrBuildInProgress
	}

	updatingTo := fmt.Sprintf("%s-%s", version, commit)
	// Persist the guard flag with a background context: the request context may
	// be cancelled by a client disconnect right after the POST, and a silently
	// failed save would let a second Apply start a concurrent update.
	if err := s.settings.Save(context.Background(), map[string]string{
		"telemt_updating":     "true",
		"telemt_updating_to":  updatingTo,
		"telemt_update_error": "",
	}); err != nil {
		return fmt.Errorf("save update state: %w", err)
	}

	if status, err := s.GetStatus(ctx); err == nil {
		s.broadcast(status)
	}

	s.notifyUpdate(ctx, "⏳ *%s* Updating telemt engine to %s...", updatingTo)

	s.mu.Lock()
	// The overall update is bounded (pull/build can stall on a dead network or
	// hung docker daemon); the deadline restores the old handler's 35m limit.
	bgCtx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	s.cancel = cancel
	s.mu.Unlock()

	go s.runUpdateBackground(bgCtx, version, commit, updatingTo)

	return nil
}

func (s *TelemtUpdateService) runUpdateBackground(ctx context.Context, version, commit, updatingTo string) {
	log := logger.WithScope("telemt-update")

	prevVersion, _ := s.settings.Get(context.Background(), "telemt_version")
	prevCommit, _ := s.settings.Get(context.Background(), "telemt_commit")

	defer func() {
		s.mu.Lock()
		s.cancel = nil
		s.mu.Unlock()

		cleanupCtx := context.Background()
		_ = s.settings.Save(cleanupCtx, map[string]string{
			"telemt_updating":    "false",
			"telemt_updating_to": "",
		})
		if status, err := s.GetStatus(cleanupCtx); err == nil {
			s.broadcast(status)
		}
	}()

	if err := s.settings.Save(ctx, map[string]string{
		"telemt_version": version,
		"telemt_commit":  commit,
	}); err != nil {
		if ctx.Err() == context.Canceled {
			s.notifyUpdate(context.Background(), "⏹️ *%s* Telemt engine update to %s cancelled by user", updatingTo)
			return
		}
		log.Errorf("save version error: %v", err)
		s.recordUpdateError(fmt.Sprintf("save error: %v", err))
		s.notifyUpdate(context.Background(), "❌ *%s* Telemt engine update to %s failed: save error\n%s", updatingTo, err)
		return
	}

	s.telemtCfg.InvalidateCache()

	log.Infof("telemt version set to %s, building image...", updatingTo)

	result, err := s.dockerSvc.BuildEngine(ctx, true, fmt.Sprintf("Engine update to %s", updatingTo))
	if err != nil {
		log.Errorf("build failed, reverting version: %v", err)
		revertCtx := context.Background()
		_ = s.settings.Save(revertCtx, map[string]string{
			"telemt_version": prevVersion,
			"telemt_commit":  prevCommit,
		})
		s.telemtCfg.InvalidateCache()

		switch {
		case ctx.Err() == context.Canceled || errors.Is(err, context.Canceled):
			s.notifyUpdate(revertCtx, "⏹️ *%s* Telemt engine update to %s cancelled by user", updatingTo)
		case ctx.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded):
			s.recordUpdateError(fmt.Sprintf("update to %s timed out after %s", updatingTo, updateTimeout))
			s.notifyUpdate(revertCtx, "❌ *%s* Telemt engine update to %s failed: timed out\n%s", updatingTo, err)
		case errors.Is(err, ErrBuildInProgress):
			s.recordUpdateError("update aborted: another engine build was in progress")
			s.notifyUpdate(revertCtx, "⚠️ *%s* Telemt engine update to %s aborted: %s", updatingTo, err)
		default:
			s.recordUpdateError(fmt.Sprintf("build error: %v", err))
			s.notifyUpdate(revertCtx, "❌ *%s* Telemt engine update to %s failed: build error\n%s", updatingTo, err)
		}
		return
	}

	log.Infof("engine image built successfully: [%s] %s", result.Method, result.Version)

	if s.containerSvc != nil {
		// Point of no return: the new image is built and tagged. Restart runs on
		// its own bounded context so a user cancel (or the update deadline firing
		// mid-restart) cannot abort it halfway and leave the container stopped.
		restartCtx, cancelRestart := context.WithTimeout(context.Background(), restartTimeout)
		err := s.containerSvc.Restart(restartCtx)
		cancelRestart()
		if err != nil {
			log.Warnf("proxy restart failed (image is ready): %v", err)
			s.recordUpdateError(fmt.Sprintf("build succeeded but proxy restart failed: %v", err))
			s.notifyUpdate(context.Background(), "❌ *%s* Telemt engine update to %s failed: restart error\n%s", updatingTo, err)
			return
		}
		log.Infof("proxy restarted with new engine")
	}

	s.notifyUpdate(context.Background(), "✅ *%s* Telemt engine updated to %s", updatingTo)
}

// recordUpdateError persists the last update failure so GetStatus can expose
// it to the UI even after the updating flag clears.
func (s *TelemtUpdateService) recordUpdateError(msg string) {
	if err := s.settings.Save(context.Background(), map[string]string{"telemt_update_error": msg}); err != nil {
		logger.WithScope("telemt-update").Warnf("failed to record update error: %v", err)
	}
}

// Cancel cancels the running update process if one is active. The cancel
// function is kept registered until the background goroutine actually exits,
// so repeated calls while the update is winding down are idempotent.
func (s *TelemtUpdateService) Cancel(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel == nil {
		return fmt.Errorf("no update in progress to cancel")
	}

	s.cancel()
	return nil
}

// GetReleases returns the cached releases list from the DB.
func (s *TelemtUpdateService) GetReleases(ctx context.Context) ([]TelemtReleaseListItem, error) {
	data, err := s.settings.Get(ctx, "telemt_releases_cache")
	if err != nil || data == "" {
		return nil, nil
	}
	var releases []TelemtReleaseListItem
	if err := json.Unmarshal([]byte(data), &releases); err != nil {
		return nil, nil
	}
	return releases, nil
}

func (s *TelemtUpdateService) fetchAndCacheReleases(ctx context.Context, client *http.Client) {
	listURL := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=30", telemtGitHubRepo)
	req, err := http.NewRequestWithContext(ctx, "GET", listURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		logger.WithScope("telemt-update").Warnf("fetch releases list: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		logger.WithScope("telemt-update").Warnf("fetch releases list: status %d", resp.StatusCode)
		return
	}

	var ghReleases []struct {
		TagName         string `json:"tag_name"`
		TargetCommitish string `json:"target_commitish"`
		HTMLURL         string `json:"html_url"`
		PublishedAt     string `json:"published_at"`
		Prerelease      bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ghReleases); err != nil {
		logger.WithScope("telemt-update").Warnf("decode releases: %v", err)
		return
	}

	items := make([]TelemtReleaseListItem, 0, len(ghReleases))
	for _, r := range ghReleases {
		if r.Prerelease {
			continue
		}
		items = append(items, TelemtReleaseListItem{
			Version:     strings.TrimPrefix(r.TagName, "v"),
			Commit:      shortSHA(r.TargetCommitish),
			TagName:     r.TagName,
			HTMLURL:     r.HTMLURL,
			PublishedAt: r.PublishedAt,
		})
	}

	data, err := json.Marshal(items)
	if err != nil {
		return
	}
	_ = s.settings.Save(ctx, map[string]string{
		"telemt_releases_cache": string(data),
	})
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func (s *TelemtUpdateService) cacheRelease(ctx context.Context, info *TelemtReleaseInfo) {
	updates := map[string]string{
		"telemt_latest_version": info.Version,
		"telemt_latest_commit":  info.Commit,
		"telemt_latest_url":     info.HTMLURL,
		"telemt_latest_checked": fmt.Sprintf("%d", time.Now().Unix()),
	}
	if err := s.settings.Save(ctx, updates); err != nil {
		logger.WithScope("telemt-update").Warnf("cache release: %v", err)
	}
}

func (s *TelemtUpdateService) notifyUpdate(ctx context.Context, format string, args ...any) {
	if v := s.notify.Load(); v != nil {
		v.(NotifyFunc)(ctx, format, args...)
	}
}
