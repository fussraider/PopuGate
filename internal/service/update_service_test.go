package service

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
)

func TestIsDockerEnvironment(t *testing.T) {
	orig := os.Getenv("POPUGATE_DEPLOYMENT")
	t.Cleanup(func() { os.Setenv("POPUGATE_DEPLOYMENT", orig) })

	t.Run("env set to docker", func(t *testing.T) {
		os.Setenv("POPUGATE_DEPLOYMENT", "docker")
		if !IsDockerEnvironment() {
			t.Error("expected true when POPUGATE_DEPLOYMENT=docker")
		}
	})

	t.Run("env not set", func(t *testing.T) {
		os.Setenv("POPUGATE_DEPLOYMENT", "")
		IsDockerEnvironment()
	})
}

func TestSelfContainerName(t *testing.T) {
	svc := &UpdateService{}

	t.Run("HOSTNAME env takes priority", func(t *testing.T) {
		orig := os.Getenv("HOSTNAME")
		t.Cleanup(func() { os.Setenv("HOSTNAME", orig) })
		os.Setenv("HOSTNAME", "my-container")
		if got := svc.selfContainerName(); got != "my-container" {
			t.Errorf("got %q, want %q", got, "my-container")
		}
	})

	t.Run("fallback when HOSTNAME empty", func(t *testing.T) {
		orig := os.Getenv("HOSTNAME")
		t.Cleanup(func() { os.Setenv("HOSTNAME", orig) })
		os.Setenv("HOSTNAME", "")
		got := svc.selfContainerName()
		if got == "" {
			t.Error("expected non-empty container name")
		}
	})
}

func TestWebContainerName(t *testing.T) {
	orig := os.Getenv("POPUGATE_WEB_CONTAINER")
	t.Cleanup(func() { os.Setenv("POPUGATE_WEB_CONTAINER", orig) })

	t.Run("env override", func(t *testing.T) {
		os.Setenv("POPUGATE_WEB_CONTAINER", "custom-web")
		if got := webContainerName("popugate-backend"); got != "custom-web" {
			t.Errorf("got %q, want %q", got, "custom-web")
		}
	})

	t.Run("strip -backend suffix", func(t *testing.T) {
		os.Setenv("POPUGATE_WEB_CONTAINER", "")
		if got := webContainerName("popugate-backend"); got != "popugate-web" {
			t.Errorf("got %q, want %q", got, "popugate-web")
		}
	})

	t.Run("no suffix", func(t *testing.T) {
		os.Setenv("POPUGATE_WEB_CONTAINER", "")
		if got := webContainerName("popugate"); got != "popugate-web" {
			t.Errorf("got %q, want %q", got, "popugate-web")
		}
	})

	t.Run("custom name without suffix", func(t *testing.T) {
		os.Setenv("POPUGATE_WEB_CONTAINER", "")
		if got := webContainerName("myapp"); got != "myapp-web" {
			t.Errorf("got %q, want %q", got, "myapp-web")
		}
	})
}

func TestWebRootDir(t *testing.T) {
	orig := os.Getenv("POPUGATE_WEB_DIR")
	t.Cleanup(func() { os.Setenv("POPUGATE_WEB_DIR", orig) })

	t.Run("env override", func(t *testing.T) {
		os.Setenv("POPUGATE_WEB_DIR", "/var/www/html")
		if got := webRootDir(); got != "/var/www/html" {
			t.Errorf("got %q, want %q", got, "/var/www/html")
		}
	})

	t.Run("default from InstallDir", func(t *testing.T) {
		os.Setenv("POPUGATE_WEB_DIR", "")
		got := webRootDir()
		if !strings.HasSuffix(got, filepath.Join("web", "dist")) {
			t.Errorf("expected path ending in web/dist, got %q", got)
		}
	})
}

func makeInspect(config *container.Config, hostConfig *container.HostConfig, mounts []container.MountPoint) container.InspectResponse {
	return container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			HostConfig: hostConfig,
		},
		Config: config,
		Mounts: mounts,
	}
}

func TestBuildRecreateScriptInner(t *testing.T) {
	svc := &UpdateService{}

	t.Run("includes entrypoint", func(t *testing.T) {
		inspect := makeInspect(
			&container.Config{
				Image:      "ghcr.io/fussraider/popugate:v1.0.0",
				Entrypoint: []string{"/usr/local/bin/docker-entrypoint.sh"},
				Cmd:        []string{"server", "--port", "8090"},
			},
			&container.HostConfig{
				NetworkMode:   "host",
				RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
			},
			nil,
		)

		script := svc.buildRecreateScriptInner("popugate", inspect, "ghcr.io/fussraider/popugate:latest")
		if !strings.Contains(script, "--entrypoint '/usr/local/bin/docker-entrypoint.sh'") {
			t.Errorf("script missing entrypoint, got:\n%s", script)
		}
		if !strings.Contains(script, "docker stop") {
			t.Error("script missing docker stop")
		}
		if !strings.Contains(script, "docker create") {
			t.Error("script missing docker create")
		}
	})

	t.Run("preserves named volumes", func(t *testing.T) {
		inspect := makeInspect(
			&container.Config{Image: "img", Cmd: []string{"server"}},
			&container.HostConfig{},
			[]container.MountPoint{
				{Type: mount.TypeVolume, Name: "data", Destination: "/data", RW: true},
			},
		)
		script := svc.buildRecreateScriptInner("popugate", inspect, "img:latest")
		if !strings.Contains(script, "-v 'data':'/data'") {
			t.Errorf("script missing named volume, got:\n%s", script)
		}
	})

	t.Run("preserves labels", func(t *testing.T) {
		inspect := makeInspect(
			&container.Config{
				Image: "img", Cmd: []string{"server"},
				Labels: map[string]string{"app": "test"},
			},
			&container.HostConfig{},
			nil,
		)
		script := svc.buildRecreateScriptInner("popugate", inspect, "img:latest")
		if !strings.Contains(script, "-l 'app=test'") {
			t.Errorf("script missing label, got:\n%s", script)
		}
	})

	t.Run("fills empty HostIP", func(t *testing.T) {
		pm := nat.PortMap{}
		pm[nat.Port("8090/tcp")] = []nat.PortBinding{{HostIP: "", HostPort: "8090"}}
		inspect := makeInspect(
			&container.Config{Image: "img", Cmd: []string{"server"}},
			&container.HostConfig{PortBindings: pm},
			nil,
		)
		script := svc.buildRecreateScriptInner("popugate", inspect, "img:latest")
		if !strings.Contains(script, "-p '0.0.0.0':'8090/tcp':'8090'") {
			t.Errorf("script missing port, got:\n%s", script)
		}
	})

	t.Run("preserves log config and memory", func(t *testing.T) {
		inspect := makeInspect(
			&container.Config{Image: "img", Cmd: []string{"server"}},
			&container.HostConfig{
				LogConfig: container.LogConfig{Config: map[string]string{"max-size": "10m"}},
				Resources: container.Resources{Memory: 536870912},
			},
			nil,
		)
		script := svc.buildRecreateScriptInner("popugate", inspect, "img:latest")
		if !strings.Contains(script, "--log-opt 'max-size=10m'") {
			t.Errorf("script missing log-opt, got:\n%s", script)
		}
		if !strings.Contains(script, "--memory=") {
			t.Errorf("script missing memory, got:\n%s", script)
		}
	})

	t.Run("no entrypoint when empty", func(t *testing.T) {
		inspect := makeInspect(
			&container.Config{Image: "img", Cmd: []string{"server"}},
			&container.HostConfig{},
			nil,
		)
		script := svc.buildRecreateScriptInner("popugate", inspect, "img:latest")
		if strings.Contains(script, "--entrypoint") {
			t.Errorf("should not contain --entrypoint, got:\n%s", script)
		}
	})

	t.Run("omits restart when policy is no", func(t *testing.T) {
		inspect := makeInspect(
			&container.Config{Image: "img", Cmd: []string{"server"}},
			&container.HostConfig{RestartPolicy: container.RestartPolicy{Name: "no"}},
			nil,
		)
		script := svc.buildRecreateScriptInner("popugate", inspect, "img:latest")
		if strings.Contains(script, "--restart") {
			t.Errorf("should not contain --restart, got:\n%s", script)
		}
	})

	t.Run("escapes container name", func(t *testing.T) {
		inspect := makeInspect(
			&container.Config{Image: "img", Cmd: []string{"server"}},
			&container.HostConfig{},
			nil,
		)
		script := svc.buildRecreateScriptInner("my-container", inspect, "img:latest")
		if !strings.Contains(script, "docker stop -t 10 'my-container'") {
			t.Errorf("script missing escaped name, got:\n%s", script)
		}
	})
}

func TestBuildRecreateScript(t *testing.T) {
	svc := &UpdateService{}
	inspect := makeInspect(
		&container.Config{Image: "img", Cmd: []string{"server"}},
		&container.HostConfig{},
		nil,
	)

	script := svc.buildRecreateScript("popugate", inspect, "img:latest")
	if !strings.Contains(script, "set -e\n") {
		t.Error("missing set -e")
	}
	if !strings.Contains(script, "sleep 3\n") {
		t.Error("missing sleep 3")
	}
	if !strings.Contains(script, "docker image prune -f") {
		t.Error("missing image prune")
	}
	if strings.Count(script, "set -e\n") != 1 {
		t.Error("set -e should appear exactly once")
	}
}

func TestBuildDualRecreateScript(t *testing.T) {
	svc := &UpdateService{}

	backend := makeInspect(
		&container.Config{Image: "ghcr.io/fussraider/popugate:v1.0.0", Cmd: []string{"server"}},
		&container.HostConfig{NetworkMode: "host"},
		nil,
	)
	web := makeInspect(
		&container.Config{Image: "ghcr.io/fussraider/popugate-web:v1.0.0", Cmd: []string{"/entrypoint.sh"}},
		&container.HostConfig{PortBindings: nat.PortMap{}},
		nil,
	)

	script := svc.buildDualRecreateScript("popugate-backend", backend, "ghcr.io/fussraider/popugate:latest", web, "popugate-web", "ghcr.io/fussraider/popugate-web:latest")

	if !strings.Contains(script, "Phase 1: Recreating backend popugate-backend") {
		t.Errorf("missing phase 1 header, got:\n%s", script)
	}
	if !strings.Contains(script, "Phase 2: Recreating web popugate-web") {
		t.Errorf("missing phase 2 header, got:\n%s", script)
	}
	if !strings.Contains(script, "Stopping old container: popugate-backend") {
		t.Errorf("missing backend stop, got:\n%s", script)
	}
	if !strings.Contains(script, "Stopping old container: popugate-web") {
		t.Errorf("missing web stop, got:\n%s", script)
	}
	if strings.Count(script, "set -e\n") != 1 {
		t.Errorf("set -e should appear exactly once, got:\n%s", script)
	}
	if strings.Count(script, "sleep 3\n") != 1 {
		t.Errorf("sleep 3 should appear exactly once, got:\n%s", script)
	}
	if strings.Count(script, "docker image prune -f") != 1 {
		t.Errorf("image prune should appear exactly once, got:\n%s", script)
	}
}

func TestGetComposeInfo(t *testing.T) {
	t.Run("detects compose project", func(t *testing.T) {
		inspect := makeInspect(
			&container.Config{
				Image: "img", Cmd: []string{"server"},
				Labels: map[string]string{
					"com.docker.compose.project":              "myproject",
					"com.docker.compose.service":              "popugate",
					"com.docker.compose.project.config_files": "/opt/app/docker-compose.yml",
					"com.docker.compose.project.working_dir":  "/opt/app",
				},
			},
			&container.HostConfig{},
			nil,
		)
		ci := getComposeInfo(inspect)
		if ci == nil {
			t.Fatal("expected compose info")
		}
		if ci.project != "myproject" {
			t.Errorf("project = %q, want %q", ci.project, "myproject")
		}
	})

	t.Run("returns nil without compose labels", func(t *testing.T) {
		inspect := makeInspect(
			&container.Config{Image: "img", Cmd: []string{"server"}},
			&container.HostConfig{},
			nil,
		)
		if ci := getComposeInfo(inspect); ci != nil {
			t.Errorf("expected nil, got %+v", ci)
		}
	})

	t.Run("returns nil with partial labels", func(t *testing.T) {
		inspect := makeInspect(
			&container.Config{
				Image: "img", Cmd: []string{"server"},
				Labels: map[string]string{"com.docker.compose.project": "myproject"},
			},
			&container.HostConfig{},
			nil,
		)
		if ci := getComposeInfo(inspect); ci != nil {
			t.Errorf("expected nil, got %+v", ci)
		}
	})

	t.Run("returns nil for nil config", func(t *testing.T) {
		inspect := container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{}}
		if ci := getComposeInfo(inspect); ci != nil {
			t.Errorf("expected nil, got %+v", ci)
		}
	})
}

func TestBuildComposeRecreateScript(t *testing.T) {
	svc := &UpdateService{}

	t.Run("without web service", func(t *testing.T) {
		ci := &composeInfo{
			project: "myproject", service: "popugate-backend",
			configFiles: "/opt/app/docker-compose.yml", workingDir: "/opt/app",
		}
		script := svc.buildComposeRecreateScript(ci, "ghcr.io/fussraider/popugate:v1.0.0", "ghcr.io/fussraider/popugate:latest", "")
		if !strings.Contains(script, "docker pull 'ghcr.io/fussraider/popugate:latest'") {
			t.Errorf("missing backend pull, got:\n%s", script)
		}
		if !strings.Contains(script, "docker tag 'ghcr.io/fussraider/popugate:latest' 'ghcr.io/fussraider/popugate:v1.0.0'") {
			t.Errorf("missing tag, got:\n%s", script)
		}
		if !strings.Contains(script, "up -d --force-recreate --no-deps 'popugate-backend'\n") {
			t.Errorf("missing compose up (single service), got:\n%s", script)
		}
		if strings.Contains(script, "popugate-web") {
			t.Errorf("should not contain web service, got:\n%s", script)
		}
	})

	t.Run("with web service", func(t *testing.T) {
		ci := &composeInfo{
			project: "myproject", service: "popugate-backend",
			configFiles: "/opt/app/docker-compose.yml", workingDir: "/opt/app",
		}
		script := svc.buildComposeRecreateScript(ci, "ghcr.io/fussraider/popugate:v1.0.0", "ghcr.io/fussraider/popugate:latest", "popugate-web")
		if !strings.Contains(script, "Pulling web image") {
			t.Errorf("missing web pull header, got:\n%s", script)
		}
		if !strings.Contains(script, "docker pull 'ghcr.io/fussraider/popugate-web:latest'") {
			t.Errorf("missing web pull, got:\n%s", script)
		}
		if !strings.Contains(script, "'popugate-backend' 'popugate-web'") {
			t.Errorf("missing dual services in compose up, got:\n%s", script)
		}
	})

	t.Run("multiple compose files", func(t *testing.T) {
		ci := &composeInfo{
			project: "myproject", service: "popugate-backend",
			configFiles: "/opt/app/docker-compose.yml,/opt/app/docker-compose.override.yml",
			workingDir:  "/opt/app",
		}
		script := svc.buildComposeRecreateScript(ci, "img:v1", "img:latest", "popugate-web")
		if !strings.Contains(script, "-f '/opt/app/docker-compose.yml'") {
			t.Errorf("missing first -f, got:\n%s", script)
		}
		if !strings.Contains(script, "-f '/opt/app/docker-compose.override.yml'") {
			t.Errorf("missing second -f, got:\n%s", script)
		}
	})

	t.Run("does not use docker create/rm/start", func(t *testing.T) {
		ci := &composeInfo{
			project: "myproject", service: "popugate-backend",
			configFiles: "/opt/app/docker-compose.yml", workingDir: "/opt/app",
		}
		script := svc.buildComposeRecreateScript(ci, "img:v1", "img:latest", "popugate-web")
		for _, cmd := range []string{"docker create", "docker stop", "docker rm"} {
			if strings.Contains(script, cmd) {
				t.Errorf("compose script should not contain %q, got:\n%s", cmd, script)
			}
		}
	})
}

func TestBuildRecreateScriptNetworkSettings(t *testing.T) {
	svc := &UpdateService{}
	inspect := container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			HostConfig: &container.HostConfig{NetworkMode: "bridge"},
		},
		Config: &container.Config{Image: "img", Cmd: []string{"server"}},
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{"bridge": {}},
		},
	}
	script := svc.buildRecreateScript("popugate", inspect, "img:latest")
	if !strings.Contains(script, "--network 'bridge'") {
		t.Errorf("missing network mode, got:\n%s", script)
	}
}

func TestExtractWebDist(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "web.tar.gz")
	targetDir := filepath.Join(tmpDir, "dist")

	// Create a test tar.gz archive
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// Add a file
	hdr := &tar.Header{Name: "index.html", Mode: 0644, Size: int64(len("hello"))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	tw.Write([]byte("hello"))

	// Add a nested file
	hdr2 := &tar.Header{Name: "assets/app.js", Mode: 0644, Size: int64(len("js"))}
	if err := tw.WriteHeader(hdr2); err != nil {
		t.Fatal(err)
	}
	tw.Write([]byte("js"))

	tw.Close()
	gw.Close()
	f.Close()

	if err := extractWebDist(archivePath, targetDir); err != nil {
		t.Fatalf("extractWebDist failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(targetDir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("index.html = %q, want %q", data, "hello")
	}

	data2, err := os.ReadFile(filepath.Join(targetDir, "assets", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data2) != "js" {
		t.Errorf("assets/app.js = %q, want %q", data2, "js")
	}
}

func TestNewUpdateService(t *testing.T) {
	svc := NewUpdateService(nil)
	if svc == nil {
		t.Fatal("expected non-nil UpdateService")
	}
}

func TestApplyDispatchesByMode(t *testing.T) {
	orig := os.Getenv("POPUGATE_DEPLOYMENT")
	t.Cleanup(func() { os.Setenv("POPUGATE_DEPLOYMENT", orig) })

	t.Run("docker mode", func(t *testing.T) {
		os.Setenv("POPUGATE_DEPLOYMENT", "docker")
		svc := NewUpdateService(nil)
		if !svc.isDocker {
			t.Error("expected isDocker=true")
		}
	})

	t.Run("binary mode", func(t *testing.T) {
		os.Setenv("POPUGATE_DEPLOYMENT", "")
		svc := NewUpdateService(nil)
		if svc.isDocker {
			t.Error("expected isDocker=false")
		}
	})
}
