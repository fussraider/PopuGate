package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/logger"
)

// ErrBuildInProgress is returned when an engine build is already running.
var ErrBuildInProgress = errors.New("engine build already in progress")

// ErrNoBuildRunning is returned by CancelBuild when nothing is being built.
var ErrNoBuildRunning = errors.New("no engine build in progress")

// DockerService handles Docker installation and engine image management.
type DockerService struct {
	docker    *dockerutil.DockerClient
	telemtCfg TelemtConfigProvider

	buildMu     sync.Mutex
	buildCancel context.CancelFunc
}

// NewDockerService creates a new DockerService.
func NewDockerService(docker *dockerutil.DockerClient, telemtCfg TelemtConfigProvider) *DockerService {
	if telemtCfg == nil {
		telemtCfg = &defaultTelemtConfig{}
	}
	return &DockerService{docker: docker, telemtCfg: telemtCfg}
}

// BuildResult holds the outcome of a build attempt.
type BuildResult struct {
	Method  string `json:"method"` // "registry", "latest", "source", "cached"
	Version string `json:"version"`
	Message string `json:"message"`
}

// BuildEngine implements the three-tier image build strategy:
// 1. Pull exact version from registry
// 2. Pull :latest from registry
// 3. Build from source
//
// trigger is a human-readable description of what initiated the build
// (manual build/pull, force rebuild, engine update); it opens the build log.
func (s *DockerService) BuildEngine(ctx context.Context, force bool, trigger string) (*BuildResult, error) {
	ctx, done, err := s.beginBuild(ctx)
	if err != nil {
		return nil, err
	}
	defer done()

	writeLog, logWriter, closeLog := newBuildLog()
	defer closeLog()

	version := fmt.Sprintf("%s-%s", s.telemtCfg.TelemtVersion(), s.telemtCfg.TelemtCommit())
	taggedImage := model.DockerImageBase + ":" + version
	latestImage := model.DockerImageBase + ":latest"
	registryVersion := model.RegistryImage + ":" + version
	registryLatest := model.RegistryImage + ":latest"

	if trigger == "" {
		trigger = "engine build"
	}
	writeLog("=== %s ===", trigger)
	writeLog("Starting Telemt Engine Build for version %s", version)

	// Check if already exists (unless forced)
	if !force {
		has, err := s.docker.HasImage(ctx, taggedImage)
		if err == nil && has {
			writeLog("Image %s already exists in cache, skipping build", taggedImage)
			return &BuildResult{Method: "cached", Version: version, Message: "image already exists"}, nil
		}
	}

	// Strategy 1: Pull exact version from registry
	writeLog("Strategy 1: Pulling exact version %s from registry...", registryVersion)
	result, err := s.pullAndTag(ctx, registryVersion, taggedImage, latestImage, version, logWriter)
	if err == nil {
		result.Method = "registry"
		writeLog("Strategy 1 Succeeded: exact version pulled and tagged")
		return result, nil
	}
	if ctx.Err() != nil {
		writeLog("Build cancelled by user")
		return nil, ctx.Err()
	}
	writeLog("Strategy 1 Failed: %v", err)

	// Strategy 2: Pull :latest from registry (unless force=source)
	if !force {
		writeLog("Strategy 2: Pulling latest version %s from registry...", registryLatest)
		result, err = s.pullAndTag(ctx, registryLatest, taggedImage, latestImage, version, logWriter)
		if err == nil {
			result.Method = "latest"
			result.Message = "pulled latest (exact version not in registry)"
			writeLog("Strategy 2 Succeeded: latest pulled and tagged")
			return result, nil
		}
		if ctx.Err() != nil {
			writeLog("Build cancelled by user")
			return nil, ctx.Err()
		}
		writeLog("Strategy 2 Failed: %v", err)
	}

	// Strategy 3: Build from source
	writeLog("Strategy 3: Building from source...")
	if err := s.buildFromSource(ctx, version, taggedImage, latestImage, logWriter); err != nil {
		if ctx.Err() != nil {
			writeLog("Build cancelled by user")
			return nil, ctx.Err()
		}
		writeLog("Strategy 3 Failed: %v", err)
		return nil, fmt.Errorf("all build strategies failed: %w", err)
	}

	s.writeVersionFile(version)
	writeLog("Strategy 3 Succeeded: built from source and tagged version %s", version)
	return &BuildResult{Method: "source", Version: version, Message: "built from source"}, nil
}

// newBuildLog truncates and opens the engine build log file. writeLog appends
// a line to it (when available) and mirrors it to the application log;
// logWriter is the raw file writer for streaming subprocess output.
func newBuildLog() (writeLog func(format string, args ...any), logWriter io.Writer, closeLog func()) {
	if err := os.MkdirAll(model.InstallDir, 0755); err != nil {
		logger.WithScope("docker").Warnf("create install dir: %v", err)
	}

	closeLog = func() {}
	logFile, err := os.OpenFile(model.TelemtBuildLogPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		logger.WithScope("docker").Warnf("failed to open build log file: %v", err)
	} else {
		logWriter = logFile
		closeLog = func() { _ = logFile.Close() }
	}

	writeLog = func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		if logWriter != nil {
			_, _ = io.WriteString(logWriter, msg+"\n")
		}
		logger.WithScope("docker").Infof("%s", msg)
	}
	return writeLog, logWriter, closeLog
}

// beginBuild registers a cancellable build. Only one engine build may run at a
// time regardless of who initiated it (manual build or engine update).
func (s *DockerService) beginBuild(ctx context.Context) (context.Context, func(), error) {
	s.buildMu.Lock()
	defer s.buildMu.Unlock()
	if s.buildCancel != nil {
		return nil, nil, ErrBuildInProgress
	}
	ctx, cancel := context.WithCancel(ctx)
	s.buildCancel = cancel
	done := func() {
		s.buildMu.Lock()
		s.buildCancel = nil
		s.buildMu.Unlock()
		cancel()
	}
	return ctx, done, nil
}

// CancelBuild cancels the currently running engine build, if any. The build
// registration is kept until the build actually exits, so repeated calls are
// idempotent while the build is winding down.
func (s *DockerService) CancelBuild() error {
	s.buildMu.Lock()
	defer s.buildMu.Unlock()
	if s.buildCancel == nil {
		return ErrNoBuildRunning
	}
	s.buildCancel()
	return nil
}

// BuildRunning reports whether an engine build is currently in progress.
func (s *DockerService) BuildRunning() bool {
	s.buildMu.Lock()
	defer s.buildMu.Unlock()
	return s.buildCancel != nil
}

func (s *DockerService) pullAndTag(ctx context.Context, pullRef, taggedImage, latestImage, version string, logWriter io.Writer) (*BuildResult, error) {
	reader, err := s.docker.PullImage(ctx, pullRef)
	if err != nil {
		return nil, fmt.Errorf("pull %s: %w", pullRef, err)
	}
	defer func() { _ = reader.Close() }()

	if logWriter != nil {
		_, _ = io.WriteString(logWriter, fmt.Sprintf("Pulling %s...\n", pullRef))
		if _, err := io.Copy(logWriter, reader); err != nil {
			return nil, fmt.Errorf("read pull output: %w", err)
		}
	} else {
		if _, err := io.ReadAll(reader); err != nil {
			return nil, fmt.Errorf("read pull output: %w", err)
		}
	}

	// Tag as local version
	if err := s.dockerTag(ctx, pullRef, taggedImage); err != nil {
		return nil, fmt.Errorf("tag %s: %w", taggedImage, err)
	}
	// Tag as :latest (best-effort)
	if err := s.dockerTag(ctx, taggedImage, latestImage); err != nil {
		logger.WithScope("docker").Warnf("tag %s as latest: %v", taggedImage, err)
	}

	s.writeVersionFile(version)
	return &BuildResult{Version: version, Message: "pulled from registry"}, nil
}

func (s *DockerService) buildFromSource(ctx context.Context, version, taggedImage, latestImage string, logWriter io.Writer) error {
	repo := s.telemtCfg.TelemtRepo()
	commit := s.telemtCfg.TelemtCommit()

	if !isSafeGitURL(repo) {
		return fmt.Errorf("invalid TELEMT_REPO value: rejected by safety check")
	}
	if !IsSafeGitRef(commit) {
		return fmt.Errorf("invalid TELEMT_COMMIT value: rejected by safety check")
	}

	buildDir, err := os.MkdirTemp("", "popugate-build-*")
	if err != nil {
		return fmt.Errorf("create build dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(buildDir) }()

	dockerfile := fmt.Sprintf(`FROM rust:1-bookworm AS builder
ARG TELEMT_COMMIT
RUN apt-get update && apt-get install -y --no-install-recommends git && \
    rm -rf /var/lib/apt/lists/*
RUN git clone "%s" /build
WORKDIR /build
RUN git checkout "${TELEMT_COMMIT}"
ENV CARGO_PROFILE_RELEASE_LTO=thin CARGO_PROFILE_RELEASE_CODEGEN_UNITS=16 CARGO_PROFILE_RELEASE_DEBUG=false
RUN cargo build --release && \
    strip target/release/telemt 2>/dev/null || true && \
    cp target/release/telemt /telemt

FROM debian:bookworm-slim
# nftables + iptables ship unconditionally (small footprint) so the opt-in
# netfilter SYN limiter works when enabled; unused otherwise (no CAP_NET_ADMIN).
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates nftables iptables && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /telemt /usr/local/bin/telemt
RUN chmod +x /usr/local/bin/telemt
STOPSIGNAL SIGINT
ENTRYPOINT ["telemt"]
`, repo)

	if err := os.WriteFile(filepath.Join(buildDir, "Dockerfile"), []byte(dockerfile), 0644); err != nil {
		return fmt.Errorf("write Dockerfile: %w", err)
	}

	cmd := exec.CommandContext(ctx, "docker", "buildx", "build",
		"--build-arg", "TELEMT_COMMIT="+commit,
		"-t", taggedImage,
		buildDir,
		"--load",
	)
	cmd.Env = append(os.Environ(), "DOCKER_BUILDKIT=1")
	if logWriter != nil {
		_, _ = io.WriteString(logWriter, "Starting Docker build...\n")
		cmd.Stdout = io.MultiWriter(os.Stdout, logWriter)
		cmd.Stderr = io.MultiWriter(os.Stderr, logWriter)
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("source build failed: %w", err)
	}

	if err := s.dockerTag(ctx, taggedImage, latestImage); err != nil {
		logger.WithScope("docker").Warnf("tag %s as latest: %v", taggedImage, err)
	}
	return nil
}

func (s *DockerService) dockerTag(ctx context.Context, source, target string) error {
	cmd := exec.CommandContext(ctx, "docker", "tag", source, target)
	return cmd.Run()
}

func (s *DockerService) writeVersionFile(version string) {
	if err := os.MkdirAll(model.InstallDir, 0755); err != nil {
		logger.WithScope("docker").Warnf("create install dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(model.InstallDir, ".telemt_version"), []byte(version), 0644); err != nil {
		logger.WithScope("docker").Warnf("write version file: %v", err)
	}
}

// GetInstalledVersion returns the currently installed telemt version string.
func (s *DockerService) GetInstalledVersion() string {
	data, err := os.ReadFile(filepath.Join(model.InstallDir, ".telemt_version"))
	if err == nil {
		return strings.TrimSpace(string(data))
	}

	// Fallback to docker images if version file is missing
	listCtx, listCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer listCancel()
	images, err := s.docker.ListImages(listCtx, "popugate-telemt")
	if err != nil || len(images) == 0 {
		return ""
	}

	// Prefer tags that look like versions (contain digits and hyphens)
	for _, img := range images {
		for _, tag := range img.RepoTags {
			// tag is "popugate-telemt:3.3.39-bc69153"
			parts := strings.Split(tag, ":")
			if len(parts) == 2 && parts[1] != "latest" {
				return parts[1]
			}
		}
	}

	// Check if :latest exists
	for _, img := range images {
		if slices.Contains(img.RepoTags, "popugate-telemt:latest") {
			return "latest"
		}
	}

	return ""
}

func isSafeGitURLChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
		r == ':' || r == '/' || r == '.' || r == '-' || r == '_' || r == '@' ||
		r == '~' || r == '+' || r == '?'
}

// isSafeGitURL validates that a git URL only contains safe characters.
func isSafeGitURL(url string) bool {
	if url == "" || strings.Contains(url, `"`) {
		return false
	}
	for _, r := range url {
		if !isSafeGitURLChar(r) {
			return false
		}
	}
	return true
}

func isSafeGitRefChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
		r == '.' || r == '-' || r == '_' || r == '/'
}

// IsSafeGitRef validates that a git ref (commit/branch/tag) only contains safe characters.
func IsSafeGitRef(ref string) bool {
	if ref == "" {
		return false
	}
	for _, r := range ref {
		if !isSafeGitRefChar(r) {
			return false
		}
	}
	return true
}
