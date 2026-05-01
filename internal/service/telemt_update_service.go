package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/logger"
)

const telemtGitHubRepo = "telemt/telemt"

// TelemtUpdateService handles checking and applying telemt engine updates.
type TelemtUpdateService struct {
	settings     *store.SettingsStore
	dockerSvc    *DockerService
	containerSvc *ContainerService
	telemtCfg    *DBTelemtConfig
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
}

// NewTelemtUpdateService creates a new TelemtUpdateService.
func NewTelemtUpdateService(
	settings *store.SettingsStore,
	dockerSvc *DockerService,
	containerSvc *ContainerService,
	telemtCfg *DBTelemtConfig,
) *TelemtUpdateService {
	return &TelemtUpdateService{
		settings:     settings,
		dockerSvc:    dockerSvc,
		containerSvc: containerSvc,
		telemtCfg:    telemtCfg,
	}
}

// ResetStaleUpdate clears a stale "updating" flag left from a crash/restart.
// Should be called once at server startup.
func (s *TelemtUpdateService) ResetStaleUpdate(ctx context.Context) {
	updatingFlag, _ := s.settings.Get(ctx, "telemt_updating")
	if updatingFlag == "true" {
		logger.WithScope("telemt-update").Warnf("clearing stale telemt update flag (server restarted mid-update)")
		s.settings.Save(ctx, map[string]string{
			"telemt_updating":    "false",
			"telemt_updating_to": "",
		})
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
	defer resp.Body.Close()

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
	lastChecked, _ := s.settings.Get(ctx, "telemt_latest_checked")

	updatingFlag, _ := s.settings.Get(ctx, "telemt_updating")
	updatingTo, _ := s.settings.Get(ctx, "telemt_updating_to")

	status := &TelemtUpdateStatus{
		Current:     current,
		LastChecked: lastChecked,
		Updating:    updatingFlag == "true",
		UpdatingTo:  updatingTo,
	}

	if latestVersion != "" {
		status.Latest = &TelemtReleaseInfo{
			Version: latestVersion,
			Commit:  latestCommit,
		}
		status.UpdateAvailable = latestVersion != s.telemtCfg.TelemtVersion() ||
			(latestCommit != "" && latestCommit != s.telemtCfg.TelemtCommit())
	}

	return status, nil
}

// Apply performs the engine update: save version -> build image -> restart proxy.
// The proxy is NOT stopped during the build.
func (s *TelemtUpdateService) Apply(ctx context.Context, version, commit string) error {
	log := logger.WithScope("telemt-update")

	if !IsSafeGitRef(version) {
		return fmt.Errorf("invalid version: rejected by safety check")
	}
	if commit != "" && !IsSafeGitRef(commit) {
		return fmt.Errorf("invalid commit: rejected by safety check")
	}

	updatingTo := fmt.Sprintf("%s-%s", version, commit)
	s.settings.Save(ctx, map[string]string{
		"telemt_updating":    "true",
		"telemt_updating_to": updatingTo,
	})

	prevVersion, _ := s.settings.Get(ctx, "telemt_version")
	prevCommit, _ := s.settings.Get(ctx, "telemt_commit")

	defer func() {
		s.settings.Save(ctx, map[string]string{
			"telemt_updating":    "false",
			"telemt_updating_to": "",
		})
	}()

	if err := s.settings.Save(ctx, map[string]string{
		"telemt_version": version,
		"telemt_commit":  commit,
	}); err != nil {
		return fmt.Errorf("save telemt version: %w", err)
	}

	s.telemtCfg.InvalidateCache()

	log.Infof("telemt version set to %s, building image...", updatingTo)

	result, err := s.dockerSvc.BuildEngine(ctx, true)
	if err != nil {
		log.Errorf("build failed, reverting version: %v", err)
		s.settings.Save(ctx, map[string]string{
			"telemt_version": prevVersion,
			"telemt_commit":  prevCommit,
		})
		s.telemtCfg.InvalidateCache()
		return fmt.Errorf("build engine: %w", err)
	}

	log.Infof("engine image built successfully: [%s] %s", result.Method, result.Version)

	if s.containerSvc != nil {
		if err := s.containerSvc.Restart(ctx); err != nil {
			log.Warnf("proxy restart failed (image is ready): %v", err)
			return fmt.Errorf("build succeeded but restart failed: %w", err)
		}
		log.Infof("proxy restarted with new engine")
	}

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
	defer resp.Body.Close()

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
	s.settings.Save(ctx, map[string]string{
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
		"telemt_latest_checked": fmt.Sprintf("%d", time.Now().Unix()),
	}
	if err := s.settings.Save(ctx, updates); err != nil {
		logger.WithScope("telemt-update").Warnf("cache release: %v", err)
	}
}
