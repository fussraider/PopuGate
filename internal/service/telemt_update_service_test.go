package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/internal/testutil"
)

func TestTelemtUpdateService_GetStatus_NoLatestCached(t *testing.T) {
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	telemtCfg := NewDBTelemtConfig(settingsStore)
	telemtCfg.SetCacheTTL(0)

	svc := NewTelemtUpdateService(settingsStore, nil, nil, telemtCfg)

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.UpdateAvailable {
		t.Error("should not report update available when no latest cached")
	}
	if status.Latest != nil {
		t.Error("expected nil latest when nothing cached")
	}
}

func TestTelemtUpdateService_GetStatus_UpdateAvailable(t *testing.T) {
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)

	settingsStore.Save(context.Background(), map[string]string{
		"telemt_latest_version": "99.0.0",
		"telemt_latest_commit":  "abcdef",
	})

	telemtCfg := NewDBTelemtConfig(settingsStore)
	telemtCfg.SetCacheTTL(0)

	svc := NewTelemtUpdateService(settingsStore, nil, nil, telemtCfg)

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.UpdateAvailable {
		t.Error("should report update available")
	}
	if status.Latest == nil {
		t.Fatal("expected non-nil latest")
	}
	if status.Latest.Version != "99.0.0" {
		t.Errorf("latest version = %q, want 99.0.0", status.Latest.Version)
	}
	if status.Latest.Commit != "abcdef" {
		t.Errorf("latest commit = %q, want abcdef", status.Latest.Commit)
	}
}

func TestTelemtUpdateService_GetStatus_UpToDate(t *testing.T) {
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)

	// Set current version to match latest
	settingsStore.Save(context.Background(), map[string]string{
		"telemt_version":        "3.3.39",
		"telemt_commit":         "bc69153",
		"telemt_latest_version": "3.3.39",
		"telemt_latest_commit":  "bc69153",
	})

	telemtCfg := NewDBTelemtConfig(settingsStore)
	telemtCfg.SetCacheTTL(0)

	svc := NewTelemtUpdateService(settingsStore, nil, nil, telemtCfg)

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.UpdateAvailable {
		t.Error("should not report update available when versions match")
	}
}

func TestTelemtUpdateService_CacheRelease(t *testing.T) {
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	telemtCfg := NewDBTelemtConfig(settingsStore)
	telemtCfg.SetCacheTTL(0)

	svc := NewTelemtUpdateService(settingsStore, nil, nil, telemtCfg)

	info := &TelemtReleaseInfo{
		Version: "4.0.0",
		Commit:  "deadbeef",
	}
	svc.cacheRelease(context.Background(), info)

	// Verify cached values in DB
	v, _ := settingsStore.Get(context.Background(), "telemt_latest_version")
	if v != "4.0.0" {
		t.Errorf("cached version = %q, want 4.0.0", v)
	}
	c, _ := settingsStore.Get(context.Background(), "telemt_latest_commit")
	if c != "deadbeef" {
		t.Errorf("cached commit = %q, want deadbeef", c)
	}
	checked, _ := settingsStore.Get(context.Background(), "telemt_latest_checked")
	if checked == "" {
		t.Error("expected non-empty checked timestamp")
	}
}

func TestTelemtUpdateService_GetStatus_LastChecked(t *testing.T) {
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)

	settingsStore.Save(context.Background(), map[string]string{
		"telemt_latest_checked": "1700000000",
		"telemt_latest_version": "5.0.0",
	})

	telemtCfg := NewDBTelemtConfig(settingsStore)
	telemtCfg.SetCacheTTL(0)

	svc := NewTelemtUpdateService(settingsStore, nil, nil, telemtCfg)

	status, _ := svc.GetStatus(context.Background())
	if status.LastChecked != "1700000000" {
		t.Errorf("last_checked = %q, want 1700000000", status.LastChecked)
	}
}

func TestNewTelemtUpdateService(t *testing.T) {
	svc := NewTelemtUpdateService(nil, nil, nil, nil)
	if svc == nil {
		t.Fatal("expected non-nil TelemtUpdateService")
	}
}

func TestTelemtUpdateService_GetStatus_UpdatingFlag(t *testing.T) {
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)

	settingsStore.Save(context.Background(), map[string]string{
		"telemt_updating":       "true",
		"telemt_updating_to":    "4.0.0-abcd",
		"telemt_latest_version": "4.0.0",
	})

	telemtCfg := NewDBTelemtConfig(settingsStore)
	telemtCfg.SetCacheTTL(0)

	svc := NewTelemtUpdateService(settingsStore, nil, nil, telemtCfg)

	status, err := svc.GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Updating {
		t.Error("expected updating=true")
	}
	if status.UpdatingTo != "4.0.0-abcd" {
		t.Errorf("updating_to = %q, want 4.0.0-abcd", status.UpdatingTo)
	}
}

func TestTelemtUpdateService_GetStatus_NotUpdating(t *testing.T) {
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)

	telemtCfg := NewDBTelemtConfig(settingsStore)
	telemtCfg.SetCacheTTL(0)

	svc := NewTelemtUpdateService(settingsStore, nil, nil, telemtCfg)

	status, _ := svc.GetStatus(context.Background())
	if status.Updating {
		t.Error("expected updating=false by default")
	}
}

func TestTelemtUpdateService_GetReleases_Empty(t *testing.T) {
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	telemtCfg := NewDBTelemtConfig(settingsStore)

	svc := NewTelemtUpdateService(settingsStore, nil, nil, telemtCfg)

	releases, err := svc.GetReleases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if releases != nil {
		t.Errorf("expected nil releases when nothing cached, got %v", releases)
	}
}

func TestTelemtUpdateService_GetReleases_Cached(t *testing.T) {
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	telemtCfg := NewDBTelemtConfig(settingsStore)

	cached := []TelemtReleaseListItem{
		{Version: "3.3.38", Commit: "aabbccd", TagName: "v3.3.38", Prerelease: false},
		{Version: "3.3.39", Commit: "bc69153", TagName: "v3.3.39", Prerelease: false},
	}
	data, _ := json.Marshal(cached)
	settingsStore.Save(context.Background(), map[string]string{
		"telemt_releases_cache": string(data),
	})

	svc := NewTelemtUpdateService(settingsStore, nil, nil, telemtCfg)

	releases, err := svc.GetReleases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(releases))
	}
	if releases[0].Version != "3.3.38" {
		t.Errorf("first release version = %q, want 3.3.38", releases[0].Version)
	}
	if releases[1].TagName != "v3.3.39" {
		t.Errorf("second release tag = %q, want v3.3.39", releases[1].TagName)
	}
}

func TestTelemtUpdateService_GetReleases_InvalidJSON(t *testing.T) {
	db := testutil.OpenTestDB(t)
	settingsStore := store.NewSettingsStore(db)
	telemtCfg := NewDBTelemtConfig(settingsStore)

	settingsStore.Save(context.Background(), map[string]string{
		"telemt_releases_cache": "not-valid-json",
	})

	svc := NewTelemtUpdateService(settingsStore, nil, nil, telemtCfg)

	releases, err := svc.GetReleases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if releases != nil {
		t.Errorf("expected nil for invalid JSON, got %v", releases)
	}
}

func TestShortSHA(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"bc69153deadbeef12345678", "bc69153"},
		{"bc69153", "bc69153"},
		{"abc", "abc"},
		{"", ""},
	}
	for _, tt := range tests {
		got := shortSHA(tt.input)
		if got != tt.want {
			t.Errorf("shortSHA(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
