package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
	"github.com/fussraider/PopuGate/pkg/dockerutil"
)

func TestIsVersionNewer(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"24.0.7", "26.1.4", true},
		{"26.1.4", "26.1.4", false},
		{"26.1.4", "24.0.7", false},
		{"26.1.4", "26.1.5", true},
		{"26.1.10", "26.1.9", false},
		{"26.1.9", "26.1.10", true},
		{"26.2", "27.0.1", true},
		{"27.0.0-rc1", "27.0.0", true}, // basic check
		{"", "26.1.4", false},
		{"24.0.7", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.current+"_vs_"+tt.latest, func(t *testing.T) {
			if got := isVersionNewer(tt.current, tt.latest); got != tt.want {
				t.Errorf("isVersionNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

type snapshotTestSetup struct {
	settingsStore *store.SettingsStore
	instanceStore *store.InstanceStore
	inst1         model.Instance
	inst2         model.Instance
	containerSvc  *ContainerService
	svc           *DockerUpdateService
}

func setupSnapshotTest(t *testing.T) snapshotTestSetup {
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	instanceStore := store.NewInstanceStore(db)
	ctx := context.Background()

	// Create test secrets, upstreams, traffic stores
	secretStore := store.NewSecretStore(db)
	upstreamStore := store.NewUpstreamStore(db)
	trafficStore := store.NewTrafficStore(db)

	// Create test secret so instance has matching secrets
	secret := model.Secret{
		Label:     "testsecret",
		SecretKey: "00000000000000000000000000000000",
		Enabled:   true,
	}
	if err := secretStore.Create(ctx, &secret); err != nil {
		t.Fatal(err)
	}

	// Create test instances in the DB
	inst1 := model.Instance{
		Port:      1001,
		TLSDomain: "domain1.com",
		Enabled:   true,
	}
	inst2 := model.Instance{
		Port:      1002,
		TLSDomain: "domain2.com",
		Enabled:   true,
	}

	if err := instanceStore.Create(ctx, &inst1); err != nil {
		t.Fatal(err)
	}
	if err := instanceStore.Create(ctx, &inst2); err != nil {
		t.Fatal(err)
	}

	// Setup ContainerService and DockerUpdateService
	containerSvc := NewContainerService(t.TempDir(), nil, secretStore, upstreamStore, instanceStore, trafficStore, settingsStore, nil)
	svc := NewDockerUpdateService(settingsStore, nil, containerSvc)

	return snapshotTestSetup{
		settingsStore: settingsStore,
		instanceStore: instanceStore,
		inst1:         inst1,
		inst2:         inst2,
		containerSvc:  containerSvc,
		svc:           svc,
	}
}

func TestDockerUpdateService_SnapshotAndRestore_CreateSnapshot(t *testing.T) {
	setup := setupSnapshotTest(t)
	ctx := context.Background()

	// Mock the isInstanceRunningFn hook so only inst1 (port 1001) is running
	setup.svc.isInstanceRunningFn = func(ctx context.Context, name string) (bool, error) {
		if name == setup.inst1.ContainerName() {
			return true, nil
		}
		return false, nil
	}

	// 1. Create snapshot
	if err := setup.svc.CreateStateSnapshot(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify settings snapshot is set correctly
	snapStr, err := setup.settingsStore.Get(ctx, "docker_update_snapshot")
	if err != nil {
		t.Fatal(err)
	}
	if snapStr == "" {
		t.Fatal("expected non-empty snapshot string in settings")
	}

	var snapshot []SnapshotItem
	if err := json.Unmarshal([]byte(snapStr), &snapshot); err != nil {
		t.Fatal(err)
	}

	if len(snapshot) != 1 {
		t.Fatalf("expected snapshot size 1, got %d", len(snapshot))
	}
	if snapshot[0].InstanceID != setup.inst1.ID {
		t.Errorf("expected snapshot instance ID %d, got %d", setup.inst1.ID, snapshot[0].InstanceID)
	}
}

func TestDockerUpdateService_SnapshotAndRestore_SuccessfulRestoration(t *testing.T) {
	setup := setupSnapshotTest(t)
	ctx := context.Background()

	setup.svc.isInstanceRunningFn = func(ctx context.Context, name string) (bool, error) {
		if name == setup.inst1.ContainerName() {
			return true, nil
		}
		return false, nil
	}

	if err := setup.svc.CreateStateSnapshot(ctx); err != nil {
		t.Fatal(err)
	}

	// 2. Test Restoration
	// Already running case: should return nil (no error)
	err := setup.svc.RestoreFromSnapshot(ctx)
	if err != nil {
		t.Errorf("expected no error when restoring already-running container, got: %v", err)
	}

	// Verify that the snapshot string is cleared in settings even on successful skip
	snapStrCleared, err := setup.settingsStore.Get(ctx, "docker_update_snapshot")
	if err != nil {
		t.Fatal(err)
	}
	if snapStrCleared != "" {
		t.Errorf("expected snapshot setting to be cleared, got %q", snapStrCleared)
	}
}

func TestDockerUpdateService_SnapshotAndRestore_FailureRestoration(t *testing.T) {
	setup := setupSnapshotTest(t)
	ctx := context.Background()

	setup.svc.isInstanceRunningFn = func(ctx context.Context, name string) (bool, error) {
		if name == setup.inst1.ContainerName() {
			return true, nil
		}
		return false, nil
	}

	// 3. Test failure to restore when not running
	// Recreate snapshot
	if err := setup.svc.CreateStateSnapshot(ctx); err != nil {
		t.Fatal(err)
	}

	// Mock as NOT running so it tries to start
	setup.svc.isInstanceRunningFn = func(ctx context.Context, name string) (bool, error) {
		return false, nil
	}

	// It will attempt to start and fail because containerSvc has nil Docker client and resources,
	// returning a restoration failures error, but clearing the lock.
	err := setup.svc.RestoreFromSnapshot(ctx)
	if err == nil {
		t.Error("expected restoration failure because StartInstance fails with uninitialized stores/docker in ContainerService")
	}

	// Verify that the snapshot string is cleared in settings
	snapStrCleared2, err := setup.settingsStore.Get(ctx, "docker_update_snapshot")
	if err != nil {
		t.Fatal(err)
	}
	if snapStrCleared2 != "" {
		t.Errorf("expected snapshot setting to be cleared after failure, got %q", snapStrCleared2)
	}
}

func TestDockerUpdateService_HandleStartupRecovery(t *testing.T) {
	// Spin up a mock Docker daemon server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/info") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ID":"test","ServerVersion":"26.1.4","LiveRestoreEnabled":true}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Direct Docker SDK to use our mock server via DOCKER_HOST env var
	host := strings.Replace(server.URL, "http://", "tcp://", 1)
	t.Setenv("DOCKER_HOST", host)
	t.Setenv("DOCKER_API_VERSION", "1.45")

	// Setup Database & Stores
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	instanceStore := store.NewInstanceStore(db)
	secretStore := store.NewSecretStore(db)
	upstreamStore := store.NewUpstreamStore(db)
	trafficStore := store.NewTrafficStore(db)
	ctx := context.Background()

	// Seed instance so startup recovery has an active instance to restore
	secret := model.Secret{
		Label:     "testsecret",
		SecretKey: "00000000000000000000000000000000",
		Enabled:   true,
	}
	if err := secretStore.Create(ctx, &secret); err != nil {
		t.Fatal(err)
	}

	inst := model.Instance{
		Port:      1001,
		TLSDomain: "domain.com",
		Enabled:   true,
	}
	if err := instanceStore.Create(ctx, &inst); err != nil {
		t.Fatal(err)
	}

	// Prepare container snapshot configuration inside DB
	snapshot := []SnapshotItem{
		{
			InstanceID:    inst.ID,
			ContainerName: inst.ContainerName(),
		},
	}
	snapJSON, _ := json.Marshal(snapshot)

	err := settingsStore.Save(ctx, map[string]string{
		"docker_updating":        "true",
		"docker_update_snapshot": string(snapJSON),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Initialize mock DockerClient and services
	dockerCli, err := dockerutil.NewDockerClient()
	if err != nil {
		t.Fatalf("failed to create DockerClient: %v", err)
	}

	containerSvc := NewContainerService(t.TempDir(), dockerCli, secretStore, upstreamStore, instanceStore, trafficStore, settingsStore, nil)
	svc := NewDockerUpdateService(settingsStore, dockerCli, containerSvc)

	// Mock the isInstanceRunningFn so it returns false, forcing restoration to execute
	svc.isInstanceRunningFn = func(ctx context.Context, name string) (bool, error) {
		return false, nil
	}

	// Configure notify callback tracking
	var notifiedMsg string
	svc.SetNotify(func(ctx context.Context, format string, args ...any) {
		notifiedMsg = fmt.Sprintf(format, args...)
	})

	// Invoke startup recovery handler (spins up background routine)
	svc.HandleStartupRecovery(ctx)

	// Poll database for "docker_updating" to be set to "false" by the recovery routine
	var finalUpdating string
	for i := 0; i < 50; i++ {
		finalUpdating, _ = settingsStore.Get(ctx, "docker_updating")
		if finalUpdating == "false" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if finalUpdating != "false" {
		t.Fatalf("expected docker_updating to be cleared to false, got %q", finalUpdating)
	}

	// Verify snapshot is also cleared
	snapStr, _ := settingsStore.Get(ctx, "docker_update_snapshot")
	if snapStr != "" {
		t.Errorf("expected docker_update_snapshot to be cleared, got %q", snapStr)
	}

	// Check if notification callback was fired
	if notifiedMsg == "" {
		t.Error("expected notification callback to have been fired during recovery")
	} else if !strings.Contains(notifiedMsg, "Server restarted after Docker update, but some proxies failed to restore") {
		// Since containerSvc.StartInstance fails due to lack of real container runtime in temp dir, it should notify failure
		t.Errorf("expected failure notification, got: %q", notifiedMsg)
	}
}
