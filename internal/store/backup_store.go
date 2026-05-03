package store

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Backup represents a backup file record.
type Backup struct {
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
}

// BackupStore handles backup management (listing, creation, deletion).
// It works with the filesystem in the configured backups directory.
type BackupStore struct {
	baseDir    string // base directory for application data
	backupsDir string // full path to backups directory
}

// NewBackupStore creates a new BackupStore.
func NewBackupStore(baseDir string) *BackupStore {
	dir := filepath.Join(baseDir, "backups")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &BackupStore{baseDir: baseDir, backupsDir: dir}
	}
	return &BackupStore{
		baseDir:    baseDir,
		backupsDir: dir,
	}
}

// List returns all backup files, newest first.
func (s *BackupStore) List(ctx context.Context) ([]Backup, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	entries, err := os.ReadDir(s.backupsDir)
	if err != nil {
		return nil, fmt.Errorf("read backups dir: %w", err)
	}

	var backups []Backup
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tar.gz") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		backups = append(backups, Backup{
			Filename:  e.Name(),
			Size:      info.Size(),
			CreatedAt: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt > backups[j].CreatedAt
	})

	if backups == nil {
		backups = []Backup{}
	}
	return backups, nil
}

// Create generates a new backup of the core application data.
func (s *BackupStore) Create(ctx context.Context) (Backup, error) {
	select {
	case <-ctx.Done():
		return Backup{}, ctx.Err()
	default:
	}

	filename := fmt.Sprintf("popugate-%s.tar.gz", time.Now().UTC().Format("20060102-150405"))
	outputPath := filepath.Join(s.backupsDir, filename)

	// Files/dirs to include (relative to baseDir)
	includes := []string{
		"settings.db",
		"mtproxy",
		"geoblock",
		".ssh",
	}

	if err := s.createTarGz(outputPath, includes); err != nil {
		return Backup{}, err
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		return Backup{}, fmt.Errorf("stat backup file: %w", err)
	}
	return Backup{
		Filename:  filename,
		Size:      info.Size(),
		CreatedAt: info.ModTime().UTC().Format(time.RFC3339),
	}, nil
}

// Restore restores data from the specified backup file.
func (s *BackupStore) Restore(ctx context.Context, filename string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if !isSafeFilename(filename) {
		return fmt.Errorf("invalid filename: %s", filename)
	}
	path := filepath.Join(s.backupsDir, filename)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("backup file not found: %s", filename)
	}
	return s.extractTarGz(path)
}

// Delete removes a backup file.
func (s *BackupStore) Delete(ctx context.Context, filename string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if !isSafeFilename(filename) {
		return fmt.Errorf("invalid filename: %s", filename)
	}
	path := filepath.Join(s.backupsDir, filename)
	return os.Remove(path)
}

// isSafeFilename validates that filename does not contain path traversal sequences.
func isSafeFilename(name string) bool {
	return name == filepath.Base(name) && !strings.Contains(name, "..")
}

// CleanOld removes backup files older than maxAge. Returns the number of deleted files.
func (s *BackupStore) CleanOld(ctx context.Context, maxAge time.Duration) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	entries, err := os.ReadDir(s.backupsDir)
	if err != nil {
		return 0, fmt.Errorf("read backups dir: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	deleted := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tar.gz") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(s.backupsDir, e.Name())); err != nil {
				return deleted, fmt.Errorf("remove %s: %w", e.Name(), err)
			}
			deleted++
		}
	}
	return deleted, nil
}

// GetPath returns the full path to a backup file.
func (s *BackupStore) GetPath(filename string) string {
	return filepath.Join(s.backupsDir, filename)
}

// Internal tar.gz helpers

func (s *BackupStore) createTarGz(outputPath string, includes []string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, rel := range includes {
		path := filepath.Join(s.baseDir, rel)
		if _, err := os.Stat(path); err != nil {
			continue // skip missing files
		}

		err = filepath.Walk(path, func(file string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}

			relPath, err := filepath.Rel(s.baseDir, file)
			if err != nil {
				return err
			}
			header.Name = relPath

			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			fileIn, err := os.Open(file)
			if err != nil {
				return err
			}
			// Close file immediately after copy, not deferred — defer in a
			// Walk callback keeps all file handles open until the walk ends.
			_, err = io.Copy(tw, fileIn)
			fileIn.Close()
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *BackupStore) extractTarGz(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(s.baseDir, header.Name)

		// Prevent tar-slip: ensure extracted path stays within baseDir
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(s.baseDir)+string(os.PathSeparator)) {
			return fmt.Errorf("refusing to extract path outside base dir: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			fout, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(fout, tr); err != nil {
				fout.Close()
				return err
			}
			fout.Close()
		}
	}
	return nil
}
