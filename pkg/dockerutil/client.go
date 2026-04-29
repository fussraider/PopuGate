package dockerutil

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/client"
)

// DockerClient wraps the Docker Engine SDK.
type DockerClient struct {
	cli          *client.Client
	hostPath     string
	hostPathOnce sync.Once
}

// NewDockerClient creates a new Docker client from environment.
func NewDockerClient() (*DockerClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &DockerClient{cli: cli}, nil
}

// Cli returns the underlying Docker SDK client for advanced operations.
func (d *DockerClient) Cli() *client.Client {
	return d.cli
}

// Close releases Docker client resources.
func (d *DockerClient) Close() error {
	if d.cli != nil {
		return d.cli.Close()
	}
	return nil
}

// IsInstalled checks if Docker is available.
func (d *DockerClient) IsInstalled(ctx context.Context) bool {
	_, err := d.cli.Info(ctx)
	return err == nil
}

// Info returns Docker system info.
func (d *DockerClient) Info(ctx context.Context) (system.Info, error) {
	return d.cli.Info(ctx)
}

// ContainerName is the default container name.
const ContainerName = "popugate"

// IsRunning checks if the proxy container is running.
func (d *DockerClient) IsRunning(ctx context.Context) (bool, error) {
	filter := filters.NewArgs()
	filter.Add("name", ContainerName)
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{Filters: filter})
	if err != nil {
		return false, err
	}
	for _, c := range containers {
		for _, name := range c.Names {
			if name == "/"+ContainerName {
				return c.State == "running", nil
			}
		}
	}
	return false, nil
}

// ContainerInspect returns detailed container info.
func (d *DockerClient) ContainerInspect(ctx context.Context, name string) (types.ContainerJSON, error) {
	if name == "" {
		name = ContainerName
	}
	return d.cli.ContainerInspect(ctx, name)
}

// StopContainer stops the proxy container.
func (d *DockerClient) StopContainer(ctx context.Context, timeout int) error {
	return d.cli.ContainerStop(ctx, ContainerName, container.StopOptions{Timeout: &timeout})
}

// RemoveContainer force-removes the container.
func (d *DockerClient) RemoveContainer(ctx context.Context) error {
	return d.cli.ContainerRemove(ctx, ContainerName, container.RemoveOptions{Force: true})
}

// RunContainer creates and starts the proxy container.
func (d *DockerClient) RunContainer(ctx context.Context, opts RunOptions) (string, error) {
	_ = d.RemoveContainer(ctx)

	config := &container.Config{
		Image: opts.Image,
		Cmd:   []string{"/etc/telemt.toml"},
	}

	ulimitHard := int64(65535)
	hostConfig := &container.HostConfig{
		NetworkMode: "host",
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
		LogConfig: container.LogConfig{
			Config: map[string]string{
				"max-size": "10m",
				"max-file": "3",
			},
		},
		Binds: []string{
			d.ResolveHostPath(opts.ConfigPath) + ":/etc/telemt.toml:ro",
		},
		Resources: container.Resources{
			Ulimits: []*container.Ulimit{
				{Name: "nofile", Hard: ulimitHard, Soft: ulimitHard},
			},
		},
	}

	if opts.CPUs != "" {
		hostConfig.NanoCPUs = parseCPUs(opts.CPUs)
	}
	if opts.Memory != "" {
		memBytes := parseMemory(opts.Memory)
		hostConfig.Memory = memBytes
		hostConfig.MemorySwap = memBytes
	}

	resp, err := d.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, ContainerName)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}

	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("start container: %w", err)
	}

	return resp.ID, nil
}

// KillSignal sends a signal to the container (for hot-reload).
func (d *DockerClient) KillSignal(ctx context.Context, signal string) error {
	return d.cli.ContainerKill(ctx, ContainerName, signal)
}

// KillSignalInstance sends a signal to a named instance container.
func (d *DockerClient) KillSignalInstance(ctx context.Context, name, signal string) error {
	return d.cli.ContainerKill(ctx, name, signal)
}

// UpdateRestartPolicy changes the container restart policy.
func (d *DockerClient) UpdateRestartPolicy(ctx context.Context, policy container.RestartPolicy) error {
	_, err := d.cli.ContainerUpdate(ctx, ContainerName, container.UpdateConfig{
		RestartPolicy: policy,
	})
	return err
}

// Logs returns container logs as a readcloser.
func (d *DockerClient) Logs(ctx context.Context, tail string, follow bool) (io.ReadCloser, error) {
	return d.cli.ContainerLogs(ctx, ContainerName, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       tail,
	})
}

// PullImage pulls a Docker image from a registry.
func (d *DockerClient) PullImage(ctx context.Context, ref string) (io.ReadCloser, error) {
	return d.cli.ImagePull(ctx, ref, image.PullOptions{})
}

// HasImage checks if an image exists locally.
func (d *DockerClient) HasImage(ctx context.Context, ref string) (bool, error) {
	filter := filters.NewArgs()
	filter.Add("reference", ref)
	images, err := d.cli.ImageList(ctx, image.ListOptions{Filters: filter})
	if err != nil {
		return false, err
	}
	return len(images) > 0, nil
}

// RemoveImage removes an image by ID.
func (d *DockerClient) RemoveImage(ctx context.Context, imageID string) error {
	_, err := d.cli.ImageRemove(ctx, imageID, image.RemoveOptions{})
	return err
}

// ListImages returns all images matching a filter.
func (d *DockerClient) ListImages(ctx context.Context, ref string) ([]image.Summary, error) {
	filter := filters.NewArgs()
	filter.Add("reference", ref)
	return d.cli.ImageList(ctx, image.ListOptions{Filters: filter})
}

// RunOptions for RunContainer.
type RunOptions struct {
	Image      string
	ConfigPath string
	CPUs       string
	Memory     string
}

// InstanceRunOptions for multi-port instances.
type InstanceRunOptions struct {
	RunOptions
	Name string
	Port int
}

// RunInstance creates and starts an instance container.
func (d *DockerClient) RunInstance(ctx context.Context, opts InstanceRunOptions) (string, error) {
	_ = d.cli.ContainerRemove(ctx, opts.Name, container.RemoveOptions{Force: true})

	config := &container.Config{
		Image: opts.Image,
		Cmd:   []string{"/etc/telemt.toml"},
	}

	ulimitHard := int64(65535)
	hostConfig := &container.HostConfig{
		NetworkMode: "host",
		RestartPolicy: container.RestartPolicy{
			Name: "unless-stopped",
		},
		LogConfig: container.LogConfig{
			Config: map[string]string{
				"max-size": "10m",
				"max-file": "3",
			},
		},
		Binds: []string{
			d.ResolveHostPath(opts.ConfigPath) + ":/etc/telemt.toml:ro",
		},
		Resources: container.Resources{
			Ulimits: []*container.Ulimit{
				{Name: "nofile", Hard: ulimitHard, Soft: ulimitHard},
			},
		},
	}

	resp, err := d.cli.ContainerCreate(ctx, config, hostConfig, nil, nil, opts.Name)
	if err != nil {
		return "", fmt.Errorf("create instance %s: %w", opts.Name, err)
	}

	if err := d.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("start instance %s: %w", opts.Name, err)
	}

	return resp.ID, nil
}

// StopInstance stops an instance container by name.
func (d *DockerClient) StopInstance(ctx context.Context, name string, timeout int) error {
	return d.cli.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeout})
}

// RemoveInstance removes an instance container by name.
func (d *DockerClient) RemoveInstance(ctx context.Context, name string) error {
	return d.cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
}

// IsInstanceRunning checks if an instance container is running.
func (d *DockerClient) IsInstanceRunning(ctx context.Context, name string) (bool, error) {
	filter := filters.NewArgs()
	filter.Add("name", name)
	containers, err := d.cli.ContainerList(ctx, container.ListOptions{Filters: filter})
	if err != nil {
		return false, err
	}
	for _, c := range containers {
		for _, n := range c.Names {
			if n == "/"+name {
				return c.State == "running", nil
			}
		}
	}
	return false, nil
}

// ResolveHostPath translates a container-local path to a host-relative path
// if running inside a container with the data volume mounted.
func (d *DockerClient) ResolveHostPath(path string) string {
	d.hostPathOnce.Do(func() {
		d.detectHostPath(context.Background())
	})
	if d.hostPath == "" {
		return path
	}

	dataDir := os.Getenv("POPUGATE_DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}

	if strings.HasPrefix(path, dataDir) {
		rel, err := filepath.Rel(dataDir, path)
		if err != nil {
			return path
		}
		return filepath.Join(d.hostPath, rel)
	}
	return path
}

func (d *DockerClient) detectHostPath(ctx context.Context) {
	hostname, _ := os.Hostname()
	if hostname == "" {
		return
	}

	containerInfo, err := d.cli.ContainerInspect(ctx, hostname)
	if err != nil {
		return
	}

	dataDir := os.Getenv("POPUGATE_DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}

	for _, m := range containerInfo.Mounts {
		if m.Destination == dataDir {
			d.hostPath = m.Source
			return
		}
	}
}

func parseCPUs(s string) int64 {
	if s == "" {
		return 0
	}
	var cpus float64
	fmt.Sscanf(s, "%f", &cpus)
	return int64(cpus * 1e9)
}

func parseMemory(s string) int64 {
	if s == "" {
		return 0
	}
	var amount int64
	var unit string
	fmt.Sscanf(s, "%d%s", &amount, &unit)
	switch unit {
	case "g", "G":
		return amount * 1024 * 1024 * 1024
	case "m", "M":
		return amount * 1024 * 1024
	case "k", "K":
		return amount * 1024
	case "b", "B", "":
		return amount
	}
	return 0
}

// RestartPolicyNo returns a "no" restart policy.
func RestartPolicyNo() container.RestartPolicy {
	return container.RestartPolicy{Name: "no"}
}

// EnsureDockerInstalled installs Docker if not present.
func EnsureDockerInstalled(ctx context.Context) error {
	if _, err := os.Stat("/usr/bin/docker"); err == nil {
		return nil
	}
	return fmt.Errorf("docker not installed; use the installation wizard")
}
