package service

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/docker/docker/api/types/container"
	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/logger"
)

const (
	githubReleasesAPI = "https://api.github.com/repos/%s/releases/latest"
	webImageRef       = "ghcr.io/fussraider/popugate-web:latest"
	webDistAssetName  = "popugate-web-dist.tar.gz"
)

// UpdateService handles checking and applying self-updates.
type UpdateService struct {
	mu        sync.Mutex
	dockerCli *dockerutil.DockerClient
	isDocker  bool
}

// NewUpdateService creates a new UpdateService.
func NewUpdateService(dockerCli *dockerutil.DockerClient) *UpdateService {
	return &UpdateService{
		dockerCli: dockerCli,
		isDocker:  IsDockerEnvironment(),
	}
}

// IsDockerEnvironment detects if the process is running inside a Docker container.
func IsDockerEnvironment() bool {
	if os.Getenv("POPUGATE_DEPLOYMENT") == "docker" {
		return true
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	return false
}

// UpdateStatus holds the result of an update check.
type UpdateStatus struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
	HTMLURL         string `json:"url,omitempty"`
	Mode            string `json:"mode"` // "docker" or "binary"
}

// UpdateResult holds the outcome of an apply operation.
type UpdateResult struct {
	PreviousVersion  string `json:"previous_version"`
	NewVersion       string `json:"new_version"`
	BinaryPath       string `json:"binary_path,omitempty"`
	BackupPath       string `json:"backup_path,omitempty"`
	ImagePulled      string `json:"image_pulled,omitempty"`
	WebImagePulled   string `json:"web_image_pulled,omitempty"`
	ContainerName    string `json:"container_name,omitempty"`
	WebContainerName string `json:"web_container_name,omitempty"`
	WebDistPath      string `json:"web_dist_path,omitempty"`
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
	Digest             string `json:"digest,omitempty"`
}

// Check queries the GitHub releases API for the latest version.
func (s *UpdateService) Check(ctx context.Context) (*UpdateStatus, error) {
	release, err := s.fetchRelease(ctx)
	if err != nil {
		return nil, err
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(model.Version, "v")
	mode := "binary"
	if s.isDocker {
		mode = "docker"
	}

	return &UpdateStatus{
		Current:         current,
		Latest:          latest,
		UpdateAvailable: latest != current && latest != "",
		HTMLURL:         release.HTMLURL,
		Mode:            mode,
	}, nil
}

// Apply downloads and installs the update.
// In binary mode: downloads binary + web dist from GitHub releases.
// In Docker mode: pulls new images from GHCR.
func (s *UpdateService) Apply(ctx context.Context) (*UpdateResult, error) {
	if s.isDocker {
		return s.ApplyDocker(ctx)
	}
	return s.ApplyBinary(ctx)
}

// ApplyBinary downloads the latest release binary and replaces the running binary.
// Also downloads and extracts the web dist archive if available.
// The caller should trigger RestartSelf after sending the HTTP response.
func (s *UpdateService) ApplyBinary(ctx context.Context) (*UpdateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log := logger.WithScope("update")

	status, err := s.Check(ctx)
	if err != nil {
		return nil, fmt.Errorf("check: %w", err)
	}
	if !status.UpdateAvailable {
		return nil, fmt.Errorf("already up to date (v%s)", model.Version)
	}

	release, err := s.fetchRelease(ctx)
	if err != nil {
		return nil, err
	}

	asset := findAsset(release.Assets, fmt.Sprintf("popugate-%s-%s", runtime.GOOS, runtime.GOARCH))
	if asset == nil {
		return nil, fmt.Errorf("no binary found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	checksumsAsset := findAsset(release.Assets, "checksums.txt")
	var expectedSHA256 string
	if checksumsAsset != nil {
		hash, err := s.fetchChecksum(ctx, checksumsAsset.BrowserDownloadURL, asset.Name)
		if err != nil {
			return nil, fmt.Errorf("fetch checksums: %w", err)
		}
		expectedSHA256 = hash
	}

	exePath, err := resolveExePath()
	if err != nil {
		return nil, err
	}

	tmpFile, err := s.downloadAsset(ctx, asset.BrowserDownloadURL, asset.Size, filepath.Dir(exePath), expectedSHA256)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer os.Remove(tmpFile)

	if err := os.Chmod(tmpFile, 0755); err != nil {
		return nil, fmt.Errorf("chmod: %w", err)
	}

	backupPath, err := replaceBinary(tmpFile, exePath)
	if err != nil {
		return nil, err
	}

	result := &UpdateResult{
		PreviousVersion: model.Version,
		NewVersion:      status.Latest,
		BinaryPath:      exePath,
		BackupPath:      backupPath,
	}

	s.updateWebDist(ctx, release, checksumsAsset, log, result)

	return result, nil
}

func findAsset(assets []githubReleaseAsset, name string) *githubReleaseAsset {
	for i := range assets {
		if assets[i].Name == name {
			return &assets[i]
		}
	}
	return nil
}

func resolveExePath() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return "", fmt.Errorf("eval symlinks: %w", err)
	}
	return exePath, nil
}

func replaceBinary(tmpFile, exePath string) (string, error) {
	backupPath := exePath + ".bak"
	if err := os.Rename(exePath, backupPath); err != nil {
		return "", fmt.Errorf("backup current binary: %w", err)
	}
	if err := os.Rename(tmpFile, exePath); err != nil {
		_ = os.Rename(backupPath, exePath)
		return "", fmt.Errorf("replace binary: %w", err)
	}
	return backupPath, nil
}

func (s *UpdateService) updateWebDist(ctx context.Context, release *githubRelease, checksumsAsset *githubReleaseAsset, log *logger.Logger, result *UpdateResult) {
	webAsset := findAsset(release.Assets, webDistAssetName)
	if webAsset == nil {
		return
	}
	destDir := webRootDir()
	var webChecksum string
	if checksumsAsset != nil {
		if h, err := s.fetchChecksum(ctx, checksumsAsset.BrowserDownloadURL, webDistAssetName); err == nil {
			webChecksum = h
		}
	}
	tmpWeb, err := s.downloadWebArchive(ctx, webAsset.BrowserDownloadURL, webAsset.Size, webChecksum)
	if err != nil {
		log.Warnf("web dist download failed: %v", err)
		return
	}
	defer os.Remove(tmpWeb)
	if err := extractWebDist(tmpWeb, destDir); err != nil {
		log.Warnf("web dist extraction failed: %v", err)
		return
	}
	log.Infof("web dist updated: %s", destDir)
	result.WebDistPath = destDir
}

// ApplyDocker pulls the new backend and web images.
func (s *UpdateService) ApplyDocker(ctx context.Context) (*UpdateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log := logger.WithScope("update")

	status, err := s.Check(ctx)
	if err != nil {
		return nil, fmt.Errorf("check: %w", err)
	}
	if !status.UpdateAvailable {
		return nil, fmt.Errorf("already up to date (v%s)", model.Version)
	}

	newImage := "ghcr.io/fussraider/popugate:latest"
	log.Infof("pulling Docker image: %s", newImage)

	reader, err := s.dockerCli.PullImage(ctx, newImage)
	if err != nil {
		return nil, fmt.Errorf("pull image %s: %w", newImage, err)
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		reader.Close()
		return nil, fmt.Errorf("read pull response for %s: %w", newImage, err)
	}
	if err := reader.Close(); err != nil {
		return nil, fmt.Errorf("close pull stream for %s: %w", newImage, err)
	}
	log.Infof("image pulled successfully: %s", newImage)

	var webPulled string
	webReader, err := s.dockerCli.PullImage(ctx, webImageRef)
	if err != nil {
		log.Warnf("failed to pull web image %s: %v", webImageRef, err)
	} else {
		if _, copyErr := io.Copy(io.Discard, webReader); copyErr != nil {
			log.Warnf("failed to read web pull response: %v", copyErr)
		}
		if closeErr := webReader.Close(); closeErr != nil {
			log.Warnf("failed to close web pull stream: %v", closeErr)
		}
		webPulled = webImageRef
		log.Infof("web image pulled successfully: %s", webImageRef)
	}

	backendName := s.selfContainerName()
	return &UpdateResult{
		PreviousVersion:  model.Version,
		NewVersion:       status.Latest,
		ImagePulled:      newImage,
		WebImagePulled:   webPulled,
		ContainerName:    backendName,
		WebContainerName: webContainerName(backendName),
	}, nil
}

// RestartSelf restarts the service.
// In binary mode: uses systemd.
// In Docker mode: spawns a sidecar container to recreate the container(s).
func (s *UpdateService) RestartSelf(newImage string) error {
	if s.isDocker {
		return s.RestartSelfDocker(newImage)
	}
	if IsSystemdInstalled() {
		return RestartService()
	}
	return fmt.Errorf("non-systemd restart not supported; please restart manually")
}

// composeInfo holds docker-compose project metadata extracted from container labels.
type composeInfo struct {
	project     string
	service     string
	configFiles string
	workingDir  string
}

// RestartSelfDocker creates a sidecar container to recreate the current
// container (and web container) with new images.
func (s *UpdateService) RestartSelfDocker(newImage string) error {
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		return fmt.Errorf("docker.sock not available at /var/run/docker.sock: %w", err)
	}

	ctx := context.Background()
	log := logger.WithScope("update")
	containerName := s.selfContainerName()

	inspect, err := s.dockerCli.ContainerInspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("inspect container %s: %w", containerName, err)
	}

	currentImage := inspect.Config.Image
	sidecarName := containerName + "-updater"

	_ = s.dockerCli.Cli().ContainerRemove(ctx, sidecarName, container.RemoveOptions{Force: true})

	sidecarConfig := &container.Config{
		Image: currentImage,
	}
	sidecarHostConfig := &container.HostConfig{
		Binds:      []string{"/var/run/docker.sock:/var/run/docker.sock"},
		AutoRemove: true,
	}

	webName := webContainerName(containerName)
	webInspect, webErr := s.dockerCli.ContainerInspect(ctx, webName)
	if webErr != nil {
		log.Warnf("could not inspect web container %s: %v (will skip web recreation)", webName, webErr)
	}

	if ci := getComposeInfo(inspect); ci != nil {
		webService := ""
		if webErr == nil {
			webService = "popugate-web"
		}
		log.Infof("detected docker-compose project %q — using compose-aware update", ci.project)
		sidecarConfig.Cmd = []string{"sh", "-c", s.buildComposeRecreateScript(ci, currentImage, newImage, webService)}
		sidecarHostConfig.Binds = append(sidecarHostConfig.Binds,
			fmt.Sprintf("%s:%s:ro", ci.workingDir, ci.workingDir),
		)
		for _, f := range strings.Split(ci.configFiles, ",") {
			f = strings.TrimSpace(f)
			if f != "" && f != ci.workingDir {
				sidecarHostConfig.Binds = append(sidecarHostConfig.Binds,
					fmt.Sprintf("%s:%s:ro", f, f),
				)
			}
		}
	} else if webErr == nil {
		log.Infof("standalone mode: recreating backend %s and web %s", containerName, webName)
		sidecarConfig.Cmd = []string{"sh", "-c", s.buildDualRecreateScript(containerName, inspect, newImage, webInspect, webName, webImageRef)}
	} else {
		sidecarConfig.Cmd = []string{"sh", "-c", s.buildRecreateScript(containerName, inspect, newImage)}
	}

	log.Infof("spawning updater sidecar to recreate container %s with image %s", containerName, newImage)

	resp, err := s.dockerCli.Cli().ContainerCreate(ctx, sidecarConfig, sidecarHostConfig, nil, nil, sidecarName)
	if err != nil {
		return fmt.Errorf("create updater sidecar: %w", err)
	}

	if err := s.dockerCli.Cli().ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start updater sidecar: %w", err)
	}

	log.Infof("updater sidecar started (ID: %s)", resp.ID[:12])

	// Stream sidecar logs so failures are visible in the main container's output
	go s.streamSidecarLogs(ctx, resp.ID)

	return nil
}

func (s *UpdateService) streamSidecarLogs(ctx context.Context, containerID string) {
	log := logger.WithScope("updater")
	opts := container.LogsOptions{
		Follow:     true,
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "0",
	}
	reader, err := s.dockerCli.Cli().ContainerLogs(ctx, containerID, opts)
	if err != nil {
		log.Warnf("sidecar logs: %v", err)
		return
	}
	defer reader.Close()

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 8 {
			line = line[8:]
		}
		line = strings.TrimSpace(line)
		if line != "" {
			log.Infof("[sidecar] %s", line)
		}
	}
}

// getComposeInfo extracts docker-compose project metadata from container labels.
func getComposeInfo(inspect container.InspectResponse) *composeInfo {
	if inspect.Config == nil || inspect.Config.Labels == nil {
		return nil
	}
	labels := inspect.Config.Labels
	project, ok := labels["com.docker.compose.project"]
	if !ok || project == "" {
		return nil
	}
	service := labels["com.docker.compose.service"]
	configFiles := labels["com.docker.compose.project.config_files"]
	workingDir := labels["com.docker.compose.project.working_dir"]
	if configFiles == "" || workingDir == "" {
		return nil
	}
	return &composeInfo{
		project:     project,
		service:     service,
		configFiles: configFiles,
		workingDir:  workingDir,
	}
}

// buildComposeRecreateScript generates a sidecar script that uses docker compose
// to recreate the backend (and optionally web) service.
func (s *UpdateService) buildComposeRecreateScript(ci *composeInfo, currentImage, newImage, webService string) string {
	var configFileArgs string
	for _, f := range strings.Split(ci.configFiles, ",") {
		f = strings.TrimSpace(f)
		if f != "" {
			configFileArgs += fmt.Sprintf(" -f %s", shellescape(f))
		}
	}

	var script strings.Builder
	fmt.Fprintf(&script, "set -e\n")
	fmt.Fprintf(&script, "echo '[popugate-updater] Waiting for HTTP response to be delivered...'\n")
	fmt.Fprintf(&script, "sleep 3\n")
	fmt.Fprintf(&script, "echo '[popugate-updater] Pulling backend image...'\n")
	fmt.Fprintf(&script, "docker pull %s\n", shellescape(newImage))
	fmt.Fprintf(&script, "docker tag %s %s\n", shellescape(newImage), shellescape(currentImage))

	if webService != "" {
		fmt.Fprintf(&script, "echo '[popugate-updater] Pulling web image...'\n")
		fmt.Fprintf(&script, "docker pull %s\n", shellescape(webImageRef))
	}

	services := shellescape(ci.service)
	if webService != "" {
		services += " " + shellescape(webService)
	}

	fmt.Fprintf(&script, "echo '[popugate-updater] Recreating via docker compose (project=%s)...'\n", ci.project)
	fmt.Fprintf(&script, "cd %s\n", shellescape(ci.workingDir))
	fmt.Fprintf(&script, "docker compose -p %s%s up -d --force-recreate --no-deps %s\n",
		shellescape(ci.project), configFileArgs, services)
	fmt.Fprintf(&script, "echo '[popugate-updater] Cleaning up old images...'\n")
	fmt.Fprintf(&script, "docker image prune -f\n")
	fmt.Fprintf(&script, "echo '[popugate-updater] Update complete. Sidecar exiting.'\n")
	return script.String()
}

// selfContainerName returns the name of the current PopuGate backend container.
func (s *UpdateService) selfContainerName() string {
	if name := os.Getenv("HOSTNAME"); name != "" {
		return name
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "popugate-backend"
}

// webContainerName returns the expected web container name derived from the backend name.
func webContainerName(backendName string) string {
	if env := os.Getenv("POPUGATE_WEB_CONTAINER"); env != "" {
		return env
	}
	base := strings.TrimSuffix(backendName, "-backend")
	if base == backendName {
		return backendName + "-web"
	}
	return base + "-web"
}

// webRootDir returns the directory where web static files are served from.
func webRootDir() string {
	if dir := os.Getenv("POPUGATE_WEB_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(model.InstallDir, "web", "dist")
}

// buildRecreateScript generates a shell script for recreating a single container.
func (s *UpdateService) buildRecreateScript(containerName string, inspect container.InspectResponse, newImage string) string {
	var script strings.Builder
	fmt.Fprintf(&script, "set -e\n")
	fmt.Fprintf(&script, "echo '[popugate-updater] Waiting for HTTP response to be delivered...'\n")
	fmt.Fprintf(&script, "sleep 3\n")
	script.WriteString(s.buildRecreateScriptInner(containerName, inspect, newImage))
	fmt.Fprintf(&script, "echo '[popugate-updater] Cleaning up old images...'\n")
	fmt.Fprintf(&script, "docker image prune -f\n")
	fmt.Fprintf(&script, "echo '[popugate-updater] Update complete. Sidecar exiting.'\n")
	return script.String()
}

// buildRecreateScriptInner generates stop/rm/create/start commands for one container.
func (s *UpdateService) buildRecreateScriptInner(containerName string, inspect container.InspectResponse, newImage string) string {
	var flags []string
	flags = append(flags, "-d")
	flags = append(flags, fmt.Sprintf("--name %s", shellescape(containerName)))

	if inspect.Config.Hostname != "" {
		flags = append(flags, fmt.Sprintf("--hostname %s", shellescape(inspect.Config.Hostname)))
	}
	if len(inspect.Config.Entrypoint) > 0 {
		var ep []string
		for _, e := range inspect.Config.Entrypoint {
			ep = append(ep, shellescape(e))
		}
		flags = append(flags, fmt.Sprintf("--entrypoint %s", strings.Join(ep, " ")))
	}
	if nm := string(inspect.HostConfig.NetworkMode); nm != "" {
		flags = append(flags, fmt.Sprintf("--network %s", shellescape(nm)))
	}
	if rp := string(inspect.HostConfig.RestartPolicy.Name); rp != "" && rp != "no" {
		flags = append(flags, fmt.Sprintf("--restart %s", shellescape(rp)))
	}
	for k, v := range inspect.Config.Labels {
		flags = append(flags, "-l", shellescape(fmt.Sprintf("%s=%s", k, v)))
	}
	for _, e := range inspect.Config.Env {
		flags = append(flags, "-e", shellescape(e))
	}
	for _, m := range inspect.Mounts {
		switch m.Type {
		case "bind":
			ro := ""
			if !m.RW {
				ro = ":ro"
			}
			flags = append(flags, "-v", fmt.Sprintf("%s:%s%s", shellescape(m.Source), shellescape(m.Destination), ro))
		case "volume":
			flags = append(flags, "-v", fmt.Sprintf("%s:%s", shellescape(m.Name), shellescape(m.Destination)))
		}
	}
	for hostPort, bindings := range inspect.HostConfig.PortBindings {
		for _, b := range bindings {
			hostIP := b.HostIP
			if hostIP == "" {
				hostIP = "0.0.0.0"
			}
			flags = append(flags, "-p", fmt.Sprintf("%s:%s:%s", shellescape(hostIP), shellescape(string(hostPort)), shellescape(b.HostPort)))
		}
	}
	for _, eh := range inspect.HostConfig.ExtraHosts {
		flags = append(flags, "--add-host", shellescape(eh))
	}
	if inspect.HostConfig.LogConfig.Config != nil {
		for k, v := range inspect.HostConfig.LogConfig.Config {
			flags = append(flags, "--log-opt", shellescape(fmt.Sprintf("%s=%s", k, v)))
		}
	}
	if inspect.HostConfig.Memory != 0 {
		flags = append(flags, fmt.Sprintf("--memory=%d", inspect.HostConfig.Memory))
	}
	if inspect.HostConfig.NanoCPUs != 0 {
		flags = append(flags, fmt.Sprintf("--cpus=%.2f", float64(inspect.HostConfig.NanoCPUs)/1e9))
	}
	if inspect.Config.StopSignal != "" {
		flags = append(flags, fmt.Sprintf("--stop-signal %s", shellescape(inspect.Config.StopSignal)))
	}

	var cmdParts []string
	for _, c := range inspect.Config.Cmd {
		cmdParts = append(cmdParts, shellescape(c))
	}

	argsStr := strings.Join(flags, " ")
	cmdStr := strings.Join(cmdParts, " ")

	var script strings.Builder
	fmt.Fprintf(&script, "echo '[popugate-updater] Stopping old container: %s'\n", containerName)
	fmt.Fprintf(&script, "docker stop -t 10 %s\n", shellescape(containerName))
	fmt.Fprintf(&script, "echo '[popugate-updater] Removing old container: %s'\n", containerName)
	fmt.Fprintf(&script, "docker rm %s\n", shellescape(containerName))
	fmt.Fprintf(&script, "echo '[popugate-updater] Creating new container with image: %s'\n", newImage)
	fmt.Fprintf(&script, "docker create %s %s %s\n", argsStr, shellescape(newImage), cmdStr)
	fmt.Fprintf(&script, "echo '[popugate-updater] Starting new container: %s'\n", containerName)
	fmt.Fprintf(&script, "docker start %s\n", shellescape(containerName))
	return script.String()
}

// buildDualRecreateScript generates a sidecar script that recreates both
// the backend and web containers.
func (s *UpdateService) buildDualRecreateScript(backendName string, backendInspect container.InspectResponse, backendImage string, webInspect container.InspectResponse, webName, webImage string) string {
	var script strings.Builder
	fmt.Fprintf(&script, "set -e\n")
	fmt.Fprintf(&script, "echo '[popugate-updater] Waiting for HTTP response to be delivered...'\n")
	fmt.Fprintf(&script, "sleep 3\n")

	fmt.Fprintf(&script, "echo '[popugate-updater] Phase 1: Recreating backend %s'\n", backendName)
	script.WriteString(s.buildRecreateScriptInner(backendName, backendInspect, backendImage))

	fmt.Fprintf(&script, "echo '[popugate-updater] Phase 2: Recreating web %s'\n", webName)
	script.WriteString(s.buildRecreateScriptInner(webName, webInspect, webImage))

	fmt.Fprintf(&script, "echo '[popugate-updater] Cleaning up old images...'\n")
	fmt.Fprintf(&script, "docker image prune -f\n")
	fmt.Fprintf(&script, "echo '[popugate-updater] Update complete. Sidecar exiting.'\n")
	return script.String()
}

// extractWebDist extracts a tar.gz archive into the target directory,
// replacing existing files.
func extractWebDist(archivePath, targetDir string) error {
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("create web dir: %w", err)
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}
		target := filepath.Join(targetDir, hdr.Name)

		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, os.FileMode(hdr.Mode))
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("create %s: %w", target, err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return fmt.Errorf("write %s: %w", target, err)
			}
			out.Close()
		}
	}
	return nil
}

// downloadWebArchive downloads the web dist tar.gz to a temp file.
// Unlike downloadAsset, it doesn't enforce the 1MB minimum size check
// since web archives can vary in size.
func (s *UpdateService) downloadWebArchive(ctx context.Context, url string, expectedSize int64, expectedSHA256 string) (string, error) {
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

	tmpFile, err := os.CreateTemp("", ".popugate-web-update-*.tar.gz")
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

	if expectedSHA256 != "" {
		f, err := os.Open(tmpPath)
		if err != nil {
			os.Remove(tmpPath)
			return "", fmt.Errorf("open for hash: %w", err)
		}
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			os.Remove(tmpPath)
			return "", fmt.Errorf("hash read: %w", err)
		}
		f.Close()
		actualHash := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(actualHash, expectedSHA256) {
			os.Remove(tmpPath)
			return "", fmt.Errorf("SHA256 mismatch: expected %s, got %s", expectedSHA256, actualHash)
		}
	}

	if expectedSize > 0 && written != expectedSize {
		os.Remove(tmpPath)
		return "", fmt.Errorf("size mismatch: expected %d, got %d", expectedSize, written)
	}

	return tmpPath, nil
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

// downloadAsset downloads a release asset to a temp file in targetDir and verifies SHA256 if provided.
func (s *UpdateService) downloadAsset(ctx context.Context, url string, expectedSize int64, targetDir string, expectedSHA256 string) (string, error) {
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

	if expectedSHA256 != "" {
		f, err := os.Open(tmpPath)
		if err != nil {
			os.Remove(tmpPath)
			return "", fmt.Errorf("open for hash: %w", err)
		}
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			os.Remove(tmpPath)
			return "", fmt.Errorf("hash read: %w", err)
		}
		f.Close()
		actualHash := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(actualHash, expectedSHA256) {
			os.Remove(tmpPath)
			return "", fmt.Errorf("SHA256 mismatch: expected %s, got %s", expectedSHA256, actualHash)
		}
	}

	if expectedSize > 0 && written != expectedSize {
		os.Remove(tmpPath)
		return "", fmt.Errorf("size mismatch: expected %d, got %d", expectedSize, written)
	}

	if written < 1<<20 {
		os.Remove(tmpPath)
		return "", fmt.Errorf("downloaded file too small (%d bytes), likely corrupted", written)
	}

	return tmpPath, nil
}

// fetchChecksum downloads the checksums.txt and extracts the SHA256 hash for the given asset name.
func (s *UpdateService) fetchChecksum(ctx context.Context, url, assetName string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
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
		return "", fmt.Errorf("checksums download returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[1]) == assetName {
			return strings.TrimSpace(parts[0]), nil
		}
	}
	return "", fmt.Errorf("no checksum found for %s", assetName)
}
