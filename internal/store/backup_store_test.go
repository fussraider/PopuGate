package store

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"fmt"
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
