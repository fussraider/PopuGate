package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
	"github.com/fussraider/PopuGate/pkg/logger"
)

// DockerService handles Docker installation and engine image management.
type DockerService struct {
	docker    *dockerutil.DockerClient
	telemtCfg TelemtConfigProvider
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
func (s *DockerService) BuildEngine(ctx context.Context, force bool) (*BuildResult, error) {
	version := fmt.Sprintf("%s-%s", s.telemtCfg.TelemtVersion(), s.telemtCfg.TelemtCommit())
	taggedImage := model.DockerImageBase + ":" + version
	latestImage := model.DockerImageBase + ":latest"
	registryVersion := model.RegistryImage + ":" + version
	registryLatest := model.RegistryImage + ":latest"

	// Check if already exists (unless forced)
	if !force {
		has, err := s.docker.HasImage(ctx, taggedImage)
		if err == nil && has {
			return &BuildResult{Method: "cached", Version: version, Message: "image already exists"}, nil
		}
	}

	// Strategy 1: Pull exact version from registry
	result, err := s.pullAndTag(ctx, registryVersion, taggedImage, latestImage, version)
	if err == nil {
		result.Method = "registry"
		return result, nil
	}

	// Strategy 2: Pull :latest from registry (unless force=source)
	if !force {
		result, err = s.pullAndTag(ctx, registryLatest, taggedImage, latestImage, version)
		if err == nil {
			result.Method = "latest"
			result.Message = "pulled latest (exact version not in registry)"
			return result, nil
		}
	}

	// Strategy 3: Build from source
	if err := s.buildFromSource(ctx, version, taggedImage, latestImage); err != nil {
		return nil, fmt.Errorf("all build strategies failed: %w", err)
	}

	s.writeVersionFile(version)
	return &BuildResult{Method: "source", Version: version, Message: "built from source"}, nil
}

func (s *DockerService) pullAndTag(ctx context.Context, pullRef, taggedImage, latestImage, version string) (*BuildResult, error) {
	reader, err := s.docker.PullImage(ctx, pullRef)
	if err != nil {
		return nil, fmt.Errorf("pull %s: %w", pullRef, err)
	}
	if _, err := io.ReadAll(reader); err != nil {
		reader.Close()
		return nil, fmt.Errorf("read pull output: %w", err)
	}
	reader.Close()

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

func (s *DockerService) buildFromSource(ctx context.Context, version, taggedImage, latestImage string) error {
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
	defer os.RemoveAll(buildDir)

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
RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates && \
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
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

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
	images, err := s.docker.ListImages(context.Background(), "popugate-telemt")
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

// isSafeGitURL validates that a git URL only contains safe characters.
func isSafeGitURL(url string) bool {
	if url == "" {
		return false
	}
	for _, r := range url {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == ':' || r == '/' || r == '.' || r == '-' || r == '_' || r == '@' ||
			r == '~' || r == '+' || r == '?') {
			return false
		}
	}
	return !strings.Contains(url, `"`)
}

// IsSafeGitRef validates that a git ref (commit/branch/tag) only contains safe characters.
func IsSafeGitRef(ref string) bool {
	if ref == "" {
		return false
	}
	for _, r := range ref {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '-' || r == '_' || r == '/') {
			return false
		}
	}
	return true
}
