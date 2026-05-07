package store

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsSafeFilename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		safe  bool
	}{
		{"simple filename", "backup.tar.gz", true},
		{"with dash", "popugate-20260101-120000.tar.gz", true},
		{"with underscore", "my_backup.tar.gz", true},
		{"with spaces", "my backup.tar.gz", true},
		{"parent traversal", "../../etc/passwd", false},
		{"single dot dot", "..", false},
		{"path with dir", "subdir/file.tar.gz", false},
		{"absolute path", "/etc/passwd", false},
		{"hidden traversal", ".hidden", true},
		{"dot dot in middle", "backup..tar.gz", false},
		{"empty string", "", false}, // filepath.Base("") == "." which doesn't match ""
		{"just slash", "/", true},   // filepath.Base("/") == "/" and "/" != ".." — edge case but safe
		{" traversal at end", "backup.tar.gz/..", false},
		{"traversal at start", "../backup.tar.gz", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSafeFilename(tt.input)
			if got != tt.safe {
				t.Errorf("isSafeFilename(%q) = %v, want %v", tt.input, got, tt.safe)
			}
		})
	}
}

func TestBackupStore_Restore_RejectsPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)

	traversalAttempts := []string{
		"../../etc/passwd",
		"../../../tmp/evil.tar.gz",
		"../secret.key",
		"/etc/passwd",
		"subdir/../../etc/passwd",
	}

	for _, attempt := range traversalAttempts {
		t.Run(attempt, func(t *testing.T) {
			err := s.Restore(context.Background(), attempt)
			if err == nil {
				t.Errorf("expected error for filename %q, got nil", attempt)
			}
			if !strings.Contains(err.Error(), "invalid filename") {
				t.Errorf("expected 'invalid filename' error, got: %v", err)
			}
		})
	}
}

func TestBackupStore_Delete_RejectsPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)

	traversalAttempts := []string{
		"../../etc/passwd",
		"../important_file",
		"/tmp/evil",
	}

	for _, attempt := range traversalAttempts {
		t.Run(attempt, func(t *testing.T) {
			err := s.Delete(context.Background(), attempt)
			if err == nil {
				t.Errorf("expected error for filename %q, got nil", attempt)
			}
			if !strings.Contains(err.Error(), "invalid filename") {
				t.Errorf("expected 'invalid filename' error, got: %v", err)
			}
		})
	}
}

func TestBackupStore_Restore_ValidFilename(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)

	// Create a valid backup file for the test
	backupsDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a minimal valid tar.gz
	backupPath := filepath.Join(backupsDir, "test-backup.tar.gz")
	if err := createMinimalTarGz(t, backupPath, map[string]string{
		"testfile.txt": "hello world",
	}); err != nil {
		t.Fatal(err)
	}

	// Restore with valid filename should proceed (won't fail on filename check)
	err := s.Restore(context.Background(), "test-backup.tar.gz")
	// It may fail on extraction if baseDir doesn't have the expected structure,
	// but it should NOT fail with "invalid filename"
	if err != nil && strings.Contains(err.Error(), "invalid filename") {
		t.Errorf("valid filename rejected: %v", err)
	}
}

func TestBackupStore_ExtractTarGz_RejectsTarSlip(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)
	backupsDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a malicious tar.gz with path traversal entries
	maliciousEntries := map[string]string{
		"../../evil.txt":               "pwned",
		"../../../tmp/pwned.txt":       "pwned",
		"subdir/../../etc/cron.d/back": "pwned",
	}

	for entryPath, content := range maliciousEntries {
		t.Run(entryPath, func(t *testing.T) {
			h := sha1.Sum([]byte(entryPath))
			backupPath := filepath.Join(backupsDir, fmt.Sprintf("malicious-%x.tar.gz", h[:4]))
			if err := createMinimalTarGz(t, backupPath, map[string]string{
				entryPath: content,
			}); err != nil {
				t.Fatal(err)
			}

			err := s.Restore(context.Background(), filepath.Base(backupPath))
			if err == nil {
				t.Error("expected error for tar-slip entry, got nil")
				// Cleanup if somehow file was written
				_ = os.RemoveAll(filepath.Join(tmpDir, "evil.txt"))
			}
			if err != nil && !strings.Contains(err.Error(), "refusing to extract") {
				t.Errorf("expected 'refusing to extract' error, got: %v", err)
			}

			// Verify the file was NOT written outside baseDir
			targetPath := filepath.Join(tmpDir, "evil.txt")
			if _, err := os.Stat(targetPath); err == nil {
				t.Error("file was written outside baseDir!")
				_ = os.Remove(targetPath)
			}
		})
	}
}

func TestBackupStore_ExtractTarGz_AcceptsValidEntries(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)
	backupsDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a legitimate tar.gz with entries inside baseDir
	backupPath := filepath.Join(backupsDir, "legit.tar.gz")
	if err := createMinimalTarGz(t, backupPath, map[string]string{
		"mtproxy/config.txt":    "some config",
		"settings.db":           "fake db content",
		"geoblock/zone_ru.cidr": "1.0.0.0/24",
	}); err != nil {
		t.Fatal(err)
	}

	err := s.Restore(context.Background(), "legit.tar.gz")
	if err != nil {
		t.Errorf("legitimate backup restore failed: %v", err)
	}

	// Verify files were extracted
	for _, f := range []string{"mtproxy/config.txt", "settings.db", "geoblock/zone_ru.cidr"} {
		p := filepath.Join(tmpDir, f)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected file %s to exist after restore: %v", f, err)
		}
	}
}

func TestBackupStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)
	backupsDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(backupsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Empty directory
	backups, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List on empty dir: %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("expected empty list, got %d items", len(backups))
	}

	// Create a backup file
	if err := createMinimalTarGz(t, filepath.Join(backupsDir, "test-001.tar.gz"), map[string]string{
		"dummy.txt": "content",
	}); err != nil {
		t.Fatal(err)
	}

	backups, err = s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(backups))
	}
	if backups[0].Filename != "test-001.tar.gz" {
		t.Errorf("expected filename test-001.tar.gz, got %s", backups[0].Filename)
	}
	if backups[0].Size <= 0 {
		t.Errorf("expected positive size, got %d", backups[0].Size)
	}

	// Non .tar.gz files should be ignored
	if err := os.WriteFile(filepath.Join(backupsDir, "readme.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatal(err)
	}
	backups, _ = s.List(context.Background())
	if len(backups) != 1 {
		t.Errorf("non-tar.gz files should be ignored, got %d items", len(backups))
	}
}

// createMinimalTarGz creates a tar.gz file with the given files (name -> content).
func createMinimalTarGz(t *testing.T, outputPath string, files map[string]string) error {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for name, content := range files {
		// Ensure parent directories have entries
		parts := strings.Split(filepath.Dir(name), "/")
		for i := 1; i <= len(parts); i++ {
			dirPath := strings.Join(parts[:i], "/")
			if dirPath == "." || dirPath == "" {
				continue
			}
			if err := tw.WriteHeader(&tar.Header{
				Name:     dirPath + "/",
				Mode:     0755,
				Typeflag: tar.TypeDir,
			}); err != nil {
				return err
			}
		}

		if err := tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0644,
			Size: int64(len(content)),
		}); err != nil {
			return err
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return err
		}
	}

	return nil
}

// --- Regression tests for ST-04 (context.Context support) ---

func TestBackupStore_List_RespectsContext(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := s.List(ctx)
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}

func TestBackupStore_Create_RespectsContext(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Create(ctx)
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}

func TestBackupStore_Restore_RespectsContext(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.Restore(ctx, "test.tar.gz")
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}

func TestBackupStore_Delete_RespectsContext(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.Delete(ctx, "test.tar.gz")
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}

func TestBackupStore_CreateAndRestore_Roundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)

	// Create some files to back up
	os.MkdirAll(filepath.Join(tmpDir, "mtproxy"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "settings.db"), []byte("fake db"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "mtproxy", "config.toml"), []byte("key=value"), 0644)

	ctx := context.Background()
	backup, err := s.Create(ctx)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if backup.Filename == "" {
		t.Error("expected non-empty filename")
	}
	if backup.Size <= 0 {
		t.Error("expected positive size")
	}

	// Verify the backup file exists
	if _, err := os.Stat(filepath.Join(tmpDir, "backups", backup.Filename)); os.IsNotExist(err) {
		t.Error("backup file not created")
	}

	// List should show the backup
	backups, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(backups))
	}
	if backups[0].Filename != backup.Filename {
		t.Errorf("expected filename %s, got %s", backup.Filename, backups[0].Filename)
	}

	// Delete the original files
	os.Remove(filepath.Join(tmpDir, "settings.db"))
	os.Remove(filepath.Join(tmpDir, "mtproxy", "config.toml"))

	// Restore should bring them back
	if err := s.Restore(ctx, backup.Filename); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "settings.db"))
	if err != nil {
		t.Fatalf("settings.db not restored: %v", err)
	}
	if string(data) != "fake db" {
		t.Errorf("restored content mismatch: got %q", data)
	}
}

func TestBackupStore_CleanOld_RemovesOldFiles(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)
	backupsDir := filepath.Join(tmpDir, "backups")

	// Create an old backup file
	oldFile := filepath.Join(backupsDir, "popugate-20200101-000000.tar.gz")
	os.WriteFile(oldFile, []byte("old"), 0644)
	os.Chtimes(oldFile, time.Now().Add(-48*time.Hour), time.Now().Add(-48*time.Hour))

	// Create a recent backup file
	recentFile := filepath.Join(backupsDir, "popugate-20260503-000000.tar.gz")
	os.WriteFile(recentFile, []byte("recent"), 0644)

	deleted, err := s.CleanOld(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatalf("CleanOld: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("old file should be deleted")
	}
	if _, err := os.Stat(recentFile); err != nil {
		t.Error("recent file should still exist")
	}
}

func TestBackupStore_CleanOld_NoOldFiles(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)
	backupsDir := filepath.Join(tmpDir, "backups")

	recentFile := filepath.Join(backupsDir, "popugate-20260503-000000.tar.gz")
	os.WriteFile(recentFile, []byte("recent"), 0644)

	deleted, err := s.CleanOld(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatalf("CleanOld: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted, got %d", deleted)
	}
}

// --- Tests for encryption, manifest, and VACUUM ---

func TestBackupStore_CreateWithEncryption(t *testing.T) {
	tmpDir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	s := NewBackupStore(tmpDir, key)

	// Create a dummy settings.db
	os.WriteFile(filepath.Join(tmpDir, "settings.db"), []byte("test db"), 0644)

	backup, err := s.Create(context.Background())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify backup was created
	if backup.Filename == "" {
		t.Error("expected non-empty filename")
	}

	// Verify the backup file exists and is encrypted
	backupPath := filepath.Join(tmpDir, "backups", backup.Filename)
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup file not created: %v", err)
	}

	// Verify the file is not a valid gzip (it's encrypted)
	f, err := os.Open(backupPath)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err == nil {
		gzReader.Close()
		t.Error("expected encrypted file to not be valid gzip")
	}
}

func TestBackupStore_ManifestInBackup(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)

	// Create a dummy settings.db
	os.WriteFile(filepath.Join(tmpDir, "settings.db"), []byte("test db"), 0644)

	backup, err := s.Create(context.Background())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Read the backup and verify manifest exists
	backupPath := filepath.Join(tmpDir, "backups", backup.Filename)
	f, err := os.Open(backupPath)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	manifestFound := false
	for {
		header, err := tarReader.Next()
		if err != nil {
			break
		}
		if header.Name == "manifest.json" {
			manifestFound = true
			// Read and parse manifest
			data, err := io.ReadAll(tarReader)
			if err != nil {
				t.Fatalf("read manifest: %v", err)
			}
			var manifest Manifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatalf("parse manifest: %v", err)
			}
			if manifest.FormatVersion != 1 {
				t.Errorf("expected format version 1, got %d", manifest.FormatVersion)
			}
			if manifest.Encryption != "none" {
				t.Errorf("expected encryption 'none', got %s", manifest.Encryption)
			}
			break
		}
	}

	if !manifestFound {
		t.Error("manifest.json not found in backup")
	}
}

func TestBackupStore_ChecksumInBackup(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)

	// Create a dummy settings.db
	os.WriteFile(filepath.Join(tmpDir, "settings.db"), []byte("test db"), 0644)

	backup, err := s.Create(context.Background())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify checksum is calculated
	if backup.Checksum == "" {
		t.Error("expected non-empty checksum")
	}

	// Verify checksum is SHA256 (64 hex chars)
	if len(backup.Checksum) != 64 {
		t.Errorf("expected 64-char checksum, got %d", len(backup.Checksum))
	}
}

func TestBackupStore_TelemtVersionInBackup(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)

	// Create .telemt_version file
	os.WriteFile(filepath.Join(tmpDir, ".telemt_version"), []byte("3.3.39"), 0644)

	backup, err := s.Create(context.Background())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Read the backup and verify .telemt_version exists
	backupPath := filepath.Join(tmpDir, "backups", backup.Filename)
	f, err := os.Open(backupPath)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	telemtVersionFound := false
	for {
		header, err := tarReader.Next()
		if err != nil {
			break
		}
		if header.Name == ".telemt_version" {
			telemtVersionFound = true
			break
		}
	}

	if !telemtVersionFound {
		t.Error(".telemt_version not found in backup")
	}
}

func TestBackupStore_RestoreWithManifestVersionCheck(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)

	// Create a backup with a newer schema version (simulated)
	backupsDir := filepath.Join(tmpDir, "backups")
	os.MkdirAll(backupsDir, 0755)

	// Create a backup with manifest that has newer schema version
	backupPath := filepath.Join(backupsDir, "test.tar.gz")
	manifest := Manifest{
		FormatVersion: 1,
		AppVersion:    "1.0.0",
		AppCommit:     "abc123",
		SchemaVersion: 99, // Newer than current
		CreatedAt:     "2026-01-01T00:00:00Z",
		Encryption:    "none",
	}

	// Create minimal backup with manifest
	f, err := os.Create(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	manifestData, _ := json.Marshal(manifest)
	tw.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Size: int64(len(manifestData)),
	})
	tw.Write(manifestData)
	tw.Close()
	gw.Close()
	f.Close()

	// Restore should fail due to version mismatch
	err = s.Restore(context.Background(), "test.tar.gz")
	if err == nil {
		t.Error("expected error for newer schema version")
	}
	if !strings.Contains(err.Error(), "newer than current") {
		t.Errorf("expected version mismatch error, got: %v", err)
	}
}

func TestBackupStore_RestoreOldBackupWithoutManifest(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)

	// Create a backup without manifest (old format)
	backupsDir := filepath.Join(tmpDir, "backups")
	os.MkdirAll(backupsDir, 0755)

	backupPath := filepath.Join(backupsDir, "old.tar.gz")
	f, err := os.Create(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// Just add a simple file, no manifest
	tw.WriteHeader(&tar.Header{
		Name: "test.txt",
		Size: 4,
	})
	tw.Write([]byte("test"))
	tw.Close()
	gw.Close()
	f.Close()

	// Restore should succeed (backward compatibility)
	err = s.Restore(context.Background(), "old.tar.gz")
	if err != nil {
		t.Errorf("expected success for old backup without manifest, got: %v", err)
	}
}

func TestBackupStore_Restore_ChecksumMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)

	backupsDir := filepath.Join(tmpDir, "backups")
	os.MkdirAll(backupsDir, 0755)

	// Create a minimal tar.gz
	backupPath := filepath.Join(backupsDir, "corrupt.tar.gz")
	if err := createMinimalTarGz(t, backupPath, map[string]string{
		"test.txt": "hello",
	}); err != nil {
		t.Fatal(err)
	}

	// Write a wrong checksum sidecar
	os.WriteFile(backupPath+".sha256", []byte("0000000000000000000000000000000000000000000000000000000000000000"), 0644)

	// Restore should fail due to checksum mismatch
	err := s.Restore(context.Background(), "corrupt.tar.gz")
	if err == nil {
		t.Error("expected error for checksum mismatch")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected checksum mismatch error, got: %v", err)
	}
}

func TestBackupStore_Restore_ChecksumMatch(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewBackupStore(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "settings.db"), []byte("test"), 0644)

	// Create a proper backup (which writes .sha256 sidecar)
	backup, err := s.Create(context.Background())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify sidecar exists
	sidecar := filepath.Join(tmpDir, "backups", backup.Filename+".sha256")
	if _, err := os.Stat(sidecar); os.IsNotExist(err) {
		t.Fatal("sha256 sidecar not created")
	}

	// Restore should succeed (checksum matches)
	if err := s.Restore(context.Background(), backup.Filename); err != nil {
		t.Errorf("expected success with valid checksum, got: %v", err)
	}
}

func TestBackupStore_EncryptedCreateAndRestore_Roundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	s := NewBackupStore(tmpDir, key)

	// Create files to back up
	os.MkdirAll(filepath.Join(tmpDir, "mtproxy"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "settings.db"), []byte("encrypted db content"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "mtproxy", "config.toml"), []byte("secret=config"), 0644)

	ctx := context.Background()
	backup, err := s.Create(ctx)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if !backup.Encrypted {
		t.Error("expected backup to be encrypted")
	}
	if backup.Checksum == "" {
		t.Error("expected non-empty checksum for encrypted backup")
	}

	// Verify backup is not valid gzip (it's encrypted)
	backupPath := filepath.Join(tmpDir, "backups", backup.Filename)
	f, err := os.Open(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	_, gzErr := gzip.NewReader(f)
	f.Close()
	if gzErr == nil {
		t.Error("expected encrypted file to not be valid gzip")
	}

	// Delete original files
	os.Remove(filepath.Join(tmpDir, "settings.db"))
	os.Remove(filepath.Join(tmpDir, "mtproxy", "config.toml"))

	// Restore
	if err := s.Restore(ctx, backup.Filename); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Verify content
	data, err := os.ReadFile(filepath.Join(tmpDir, "settings.db"))
	if err != nil {
		t.Fatalf("settings.db not restored: %v", err)
	}
	if string(data) != "encrypted db content" {
		t.Errorf("restored content mismatch: got %q", data)
	}

	data, err = os.ReadFile(filepath.Join(tmpDir, "mtproxy", "config.toml"))
	if err != nil {
		t.Fatalf("config.toml not restored: %v", err)
	}
	if string(data) != "secret=config" {
		t.Errorf("restored content mismatch: got %q", data)
	}
}
