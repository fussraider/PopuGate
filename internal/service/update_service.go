package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
)

const githubReleasesAPI = "https://api.github.com/repos/%s/releases/latest"

// UpdateService handles checking and applying self-updates.
type UpdateService struct {
	mu sync.Mutex
}

// NewUpdateService creates a new UpdateService.
func NewUpdateService() *UpdateService {
	return &UpdateService{}
}

// UpdateStatus holds the result of an update check.
type UpdateStatus struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
	HTMLURL         string `json:"url,omitempty"`
}

// UpdateResult holds the outcome of an apply operation.
type UpdateResult struct {
	PreviousVersion string `json:"previous_version"`
	NewVersion      string `json:"new_version"`
	BinaryPath      string `json:"binary_path"`
	BackupPath      string `json:"backup_path"`
}

// githubRelease represents the relevant fields from the GitHub releases API.
type githubRelease struct {
	TagName string               `json:"tag_name"`
	HTMLURL string               `json:"html_url"`
	Assets  []githubReleaseAsset `json:"assets"`
}

// githubReleaseAsset represents a single release asset.
type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Check queries the GitHub releases API for the latest version.
func (s *UpdateService) Check(ctx context.Context) (*UpdateStatus, error) {
	release, err := s.fetchRelease(ctx)
	if err != nil {
		return nil, err
	}

	latest := strings.TrimPrefix(release.TagName, "v")

	return &UpdateStatus{
		Current:         model.Version,
		Latest:          latest,
		UpdateAvailable: latest != model.Version && latest != "",
		HTMLURL:         release.HTMLURL,
	}, nil
}

// Apply downloads the latest release binary and replaces the running binary.
// The caller should trigger RestartSelf after sending the HTTP response.
func (s *UpdateService) Apply(ctx context.Context) (*UpdateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Check for update
	status, err := s.Check(ctx)
	if err != nil {
		return nil, fmt.Errorf("check: %w", err)
	}
	if !status.UpdateAvailable {
		return nil, fmt.Errorf("already up to date (v%s)", model.Version)
	}

	// 2. Fetch release info
	release, err := s.fetchRelease(ctx)
	if err != nil {
		return nil, err
	}

	// 3. Find matching asset
	assetName := fmt.Sprintf("popugate-%s-%s", runtime.GOOS, runtime.GOARCH)
	var asset *githubReleaseAsset
	for i := range release.Assets {
		if release.Assets[i].Name == assetName {
			asset = &release.Assets[i]
			break
		}
	}
	if asset == nil {
		return nil, fmt.Errorf("no binary found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	// 4. Resolve current binary path
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return nil, fmt.Errorf("eval symlinks: %w", err)
	}

	// 5. Download to temp file in same directory (same filesystem for atomic rename)
	tmpFile, err := s.downloadAsset(ctx, asset.BrowserDownloadURL, asset.Size, filepath.Dir(exePath))
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer os.Remove(tmpFile)

	// 6. Make executable
	if err := os.Chmod(tmpFile, 0755); err != nil {
		return nil, fmt.Errorf("chmod: %w", err)
	}

	// 7. Backup current binary
	backupPath := exePath + ".bak"
	if err := os.Rename(exePath, backupPath); err != nil {
		return nil, fmt.Errorf("backup current binary: %w", err)
	}

	// 8. Move new binary into place
	if err := os.Rename(tmpFile, exePath); err != nil {
		// Rollback
		_ = os.Rename(backupPath, exePath)
		return nil, fmt.Errorf("replace binary: %w", err)
	}

	return &UpdateResult{
		PreviousVersion: model.Version,
		NewVersion:      status.Latest,
		BinaryPath:      exePath,
		BackupPath:      backupPath,
	}, nil
}

// RestartSelf restarts the popugate service via systemd.
func (s *UpdateService) RestartSelf() error {
	if IsSystemdInstalled() {
		return RestartService()
	}
	return fmt.Errorf("non-systemd restart not supported; please restart manually")
}

// Rollback restores the backup binary.
func (s *UpdateService) Rollback(backupPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	exePath, _ = filepath.EvalSymlinks(exePath)
	return os.Rename(backupPath, exePath)
}

// fetchRelease gets the latest release metadata from GitHub.
func (s *UpdateService) fetchRelease(ctx context.Context) (*githubRelease, error) {
	url := fmt.Sprintf(githubReleasesAPI, model.GitHubRepo)
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("check update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	return &release, nil
}

// downloadAsset downloads a release asset to a temp file in targetDir.
func (s *UpdateService) downloadAsset(ctx context.Context, url string, expectedSize int64, targetDir string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Minute}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download returned %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp(targetDir, ".popugate-update-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	written, err := io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("write download: %w", err)
	}

	// Size validation
	if expectedSize > 0 && written != expectedSize {
		os.Remove(tmpPath)
		return "", fmt.Errorf("size mismatch: expected %d, got %d", expectedSize, written)
	}

	// Minimum sanity check (real binary must be > 1MB)
	if written < 1<<20 {
		os.Remove(tmpPath)
		return "", fmt.Errorf("downloaded file too small (%d bytes), likely corrupted", written)
	}

	return tmpPath, nil
}
