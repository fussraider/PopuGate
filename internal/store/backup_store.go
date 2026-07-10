package store

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/fussraider/PopuGate/internal/model"
)

// Backup represents a backup file record.
type Backup struct {
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	CreatedAt string `json:"created_at"`
	Checksum  string `json:"checksum,omitempty"`
	Encrypted bool   `json:"encrypted"`
}

// Manifest represents the backup manifest metadata.
type Manifest struct {
	FormatVersion int      `json:"format_version"`
	AppVersion    string   `json:"app_version"`
	AppCommit     string   `json:"app_commit"`
	SchemaVersion int      `json:"schema_version"`
	CreatedAt     string   `json:"created_at"`
	Encryption    string   `json:"encryption"` // "aes-gcm" or "none"
	Tables        []string `json:"tables,omitempty"`
}

// BackupStore handles backup management (listing, creation, deletion).
// It works with the filesystem in the configured backups directory.
type BackupStore struct {
	baseDir       string // base directory for application data
	backupsDir    string // full path to backups directory
	dbPath        string // path to SQLite database
	encryptionKey []byte // optional AES-GCM encryption key
}

// NewBackupStore creates a new BackupStore.
// encryptionKey is optional (nil = no encryption).
func NewBackupStore(baseDir string, encryptionKey ...[]byte) *BackupStore {
	dir := filepath.Join(baseDir, "backups")
	_ = os.MkdirAll(dir, 0755)
	s := &BackupStore{
		baseDir:    baseDir,
		backupsDir: dir,
		dbPath:     filepath.Join(baseDir, "settings.db"),
	}
	if len(encryptionKey) > 0 && len(encryptionKey[0]) > 0 {
		s.encryptionKey = encryptionKey[0]
	}
	return s
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
			Encrypted: s.encryptionKey != nil,
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

	filename := fmt.Sprintf("popugate-%s.tar.gz", time.Now().UTC().Format("20060102-150405.000"))
	// Ensure uniqueness if file already exists
	if _, err := os.Stat(filepath.Join(s.backupsDir, filename)); err == nil {
		suffix := make([]byte, 2)
		rand.Read(suffix)
		filename = fmt.Sprintf("popugate-%s-%x.tar.gz", time.Now().UTC().Format("20060102-150405"), suffix)
	}
	outputPath := filepath.Join(s.backupsDir, filename)

	// Files/dirs to include (relative to baseDir)
	includes := []string{
		"settings.db",
		"mtproxy",
		"geoblock",
		".ssh",
		".telemt_version",
	}

	// Create manifest
	manifest := Manifest{
		FormatVersion: 1,
		AppVersion:    model.Version,
		AppCommit:     model.Commit,
		SchemaVersion: s.getCurrentSchemaVersion(),
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Encryption:    "none",
		Tables:        s.getTableList(),
	}

	if s.encryptionKey != nil {
		manifest.Encryption = "aes-gcm"
	}

	if err := s.createTarGz(outputPath, includes, &manifest); err != nil {
		return Backup{}, err
	}

	// Calculate checksum and write sidecar for restore verification
	checksum, cerr := s.calculateFileChecksum(outputPath)
	if cerr == nil {
		_ = os.WriteFile(outputPath+".sha256", []byte(checksum), 0644)
	} else {
		checksum = ""
	}

	info, err := os.Stat(outputPath)
	if err != nil {
		return Backup{}, fmt.Errorf("stat backup file: %w", err)
	}
	return Backup{
		Filename:  filename,
		Size:      info.Size(),
		CreatedAt: info.ModTime().UTC().Format(time.RFC3339),
		Checksum:  checksum,
		Encrypted: s.encryptionKey != nil,
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

	// Verify checksum if sidecar exists
	if expected, err := os.ReadFile(path + ".sha256"); err == nil && len(expected) > 0 {
		actual, err := s.calculateFileChecksum(path)
		if err != nil {
			return fmt.Errorf("checksum verification failed: cannot read backup: %w", err)
		}
		if actual != strings.TrimSpace(string(expected)) {
			return fmt.Errorf("checksum mismatch: backup may be corrupted")
		}
	}

	// Check manifest and version compatibility
	manifest, err := s.readManifestFromBackup(path)
	if err != nil {
		// Old backup without manifest - allow with warning
		// This maintains backward compatibility
	} else {
		// Check schema version compatibility
		currentSchema := s.getCurrentSchemaVersion()
		if manifest.SchemaVersion > currentSchema {
			return fmt.Errorf("backup schema version %d is newer than current %d - cannot restore", manifest.SchemaVersion, currentSchema)
		}
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
	_ = os.Remove(path + ".sha256") // clean sidecar
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
			_ = os.Remove(filepath.Join(s.backupsDir, e.Name()+".sha256"))
			deleted++
		}
	}
	return deleted, nil
}

// GetPath returns the full path to a backup file.
func (s *BackupStore) GetPath(filename string) string {
	return filepath.Join(s.backupsDir, filename)
}

// EncryptionEnabled reports whether backup encryption is configured.
func (s *BackupStore) EncryptionEnabled() bool {
	return s.encryptionKey != nil
}

// Internal tar.gz helpers

func (s *BackupStore) createTarGz(outputPath string, includes []string, manifest *Manifest) (retErr error) {
	tmpPath := outputPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
		if retErr != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	if err := s.writeManifest(tw, manifest); err != nil {
		return err
	}
	for _, rel := range includes {
		if err := s.addToArchive(tw, rel); err != nil {
			return err
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("close tar writer: %w", err)
	}
	if err := gw.Close(); err != nil {
		return fmt.Errorf("close gzip writer: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	return s.finalizeArchive(tmpPath, outputPath)
}

func (s *BackupStore) writeManifest(tw *tar.Writer, manifest *Manifest) error {
	if manifest == nil {
		return nil
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := tw.WriteHeader(&tar.Header{
		Name:    "manifest.json",
		Mode:    0644,
		Size:    int64(len(manifestData)),
		ModTime: time.Now(),
	}); err != nil {
		return err
	}
	_, err = tw.Write(manifestData)
	return err
}

func (s *BackupStore) addToArchive(tw *tar.Writer, rel string) error {
	path := filepath.Join(s.baseDir, rel)
	if _, err := os.Stat(path); err != nil {
		return nil // skip missing files
	}
	if rel == "settings.db" {
		return s.addDBToTar(tw, path)
	}
	return s.addTreeToTar(tw, path)
}

func (s *BackupStore) addTreeToTar(tw *tar.Writer, root string) error {
	return filepath.Walk(root, func(file string, info os.FileInfo, err error) error {
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
		_, err = io.Copy(tw, fileIn)
		_ = fileIn.Close()
		return err
	})
}

func (s *BackupStore) finalizeArchive(tmpPath, outputPath string) error {
	if s.encryptionKey != nil {
		if err := s.encryptFile(tmpPath, outputPath); err != nil {
			return err
		}
		_ = os.Remove(tmpPath)
		return nil
	}
	return os.Rename(tmpPath, outputPath)
}

// addDBToTar creates a consistent database snapshot using VACUUM INTO.
func (s *BackupStore) addDBToTar(tw *tar.Writer, dbPath string) error {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return s.addFileToTar(tw, dbPath, "settings.db")
	}
	defer func() { _ = db.Close() }()

	tmpDB, err := os.CreateTemp("", "backup-*.db")
	if err != nil {
		return s.addFileToTar(tw, dbPath, "settings.db")
	}
	tmpPath := tmpDB.Name()
	_ = tmpDB.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	// Sanitize path for VACUUM INTO — escape single quotes
	safePath := strings.ReplaceAll(tmpPath, "'", "''")
	_, err = db.Exec(fmt.Sprintf("VACUUM INTO '%s'", safePath))
	if err != nil {
		return s.addFileToTar(tw, dbPath, "settings.db")
	}

	return s.addFileToTar(tw, tmpPath, "settings.db")
}

// addFileToTar adds a single file to the tar archive.
func (s *BackupStore) addFileToTar(tw *tar.Writer, filePath, tarName string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	header.Name = tarName

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	_, err = io.Copy(tw, file)
	return err
}

// encryptFile encrypts a file using AES-256-GCM (whole-file, single nonce).
// Format: [12-byte nonce][4-byte big-endian ciphertext length][ciphertext+GCM tag]
func (s *BackupStore) encryptFile(inputPath, outputPath string) error {
	plaintext, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read plaintext: %w", err)
	}

	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	nonce := make([]byte, aesgcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}

	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)

	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer func() { _ = outFile.Close() }()

	if _, err := outFile.Write(nonce); err != nil {
		return err
	}

	// Write length prefix so decrypt knows exact ciphertext size
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(ciphertext)))
	if _, err := outFile.Write(lenBuf); err != nil {
		return err
	}

	if _, err := outFile.Write(ciphertext); err != nil {
		return err
	}

	return nil
}

// decryptFile decrypts a file encrypted by encryptFile.
func (s *BackupStore) decryptFile(inputPath, outputPath string) error {
	inFile, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer func() { _ = inFile.Close() }()

	nonce := make([]byte, 12) // GCM standard nonce size
	if _, err := io.ReadFull(inFile, nonce); err != nil {
		return fmt.Errorf("read nonce: %w", err)
	}

	var ctLen uint32
	if err := binary.Read(inFile, binary.BigEndian, &ctLen); err != nil {
		return fmt.Errorf("read ciphertext length: %w", err)
	}

	const maxCiphertextSize = 2 * 1024 * 1024 * 1024 // 2 GB
	if ctLen > maxCiphertextSize {
		return fmt.Errorf("ciphertext too large: %d bytes", ctLen)
	}

	ciphertext := make([]byte, ctLen)
	if _, err := io.ReadFull(inFile, ciphertext); err != nil {
		return fmt.Errorf("read ciphertext: %w", err)
	}

	block, err := aes.NewCipher(s.encryptionKey)
	if err != nil {
		return err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("decrypt: %w", err)
	}

	if err := os.WriteFile(outputPath, plaintext, 0644); err != nil {
		return err
	}

	return nil
}

func (s *BackupStore) extractTarGz(path string) error {
	f, cleanup, err := s.openArchive(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
		if cleanup != nil {
			cleanup()
		}
	}()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Name == "manifest.json" {
			continue
		}
		if err := s.extractEntry(tr, header); err != nil {
			return err
		}
	}
	return nil
}

func (s *BackupStore) openArchive(path string) (*os.File, func(), error) {
	if s.encryptionKey == nil {
		f, err := os.Open(path)
		return f, nil, err
	}
	tmpPath := path + ".decrypted"
	if err := s.decryptFile(path, tmpPath); err != nil {
		return nil, nil, fmt.Errorf("decrypt backup: %w", err)
	}
	cleanup := func() { _ = os.Remove(tmpPath) }
	f, err := os.Open(tmpPath)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return f, cleanup, nil
}

func (s *BackupStore) extractEntry(tr *tar.Reader, header *tar.Header) error {
	// Reject non-local entry names (absolute, or containing "..") up front, then
	// re-check the joined path stays under baseDir. Defends against archive path
	// traversal ("zip slip") on restore.
	if !filepath.IsLocal(header.Name) {
		return fmt.Errorf("refusing to extract non-local path: %s", header.Name)
	}
	target := filepath.Join(s.baseDir, header.Name)
	if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(s.baseDir)+string(os.PathSeparator)) {
		return fmt.Errorf("refusing to extract path outside base dir: %s", header.Name)
	}
	switch header.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, 0755)
	case tar.TypeReg:
		return s.extractFileEntry(tr, target)
	}
	return nil
}

func (s *BackupStore) extractFileEntry(tr *tar.Reader, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	tmpTarget := target + ".tmp"
	fout, err := os.OpenFile(tmpTarget, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(fout, tr); err != nil {
		_ = fout.Close()
		_ = os.Remove(tmpTarget)
		return err
	}
	if err := fout.Close(); err != nil {
		_ = os.Remove(tmpTarget)
		return fmt.Errorf("close extracted file: %w", err)
	}
	if err := os.Rename(tmpTarget, target); err != nil {
		_ = os.Remove(tmpTarget)
		return err
	}
	return nil
}

// readManifestFromBackup reads the manifest from a backup file.
func (s *BackupStore) readManifestFromBackup(path string) (*Manifest, error) {
	// Open and decrypt if needed
	var reader io.Reader
	if s.encryptionKey != nil {
		tmpPath := path + ".read"
		if err := s.decryptFile(path, tmpPath); err != nil {
			return nil, err
		}
		defer func() { _ = os.Remove(tmpPath) }()
		f, err := os.Open(tmpPath)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		reader = f
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		reader = f
	}

	gr, err := gzip.NewReader(reader)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if header.Name == "manifest.json" {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
			var manifest Manifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				return nil, err
			}
			return &manifest, nil
		}
	}

	return nil, errors.New("manifest not found")
}

// getCurrentSchemaVersion returns the current database schema version.
func (s *BackupStore) getCurrentSchemaVersion() int {
	// Read from schema_version table
	db, err := sql.Open("sqlite", s.dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return 9 // Default to latest known version
	}
	defer func() { _ = db.Close() }()

	var version int
	err = db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version)
	if err != nil {
		return 9
	}
	return version
}

// getTableList returns the list of tables in the database.
func (s *BackupStore) getTableList() []string {
	tables := []string{
		"settings", "secrets", "upstreams", "instances", "slaves",
		"traffic_global", "traffic_user", "geoblock_cache",
		"quota_alerts", "expiry_alerts", "token_blocklist",
		"scheduler_task_overrides", "scheduler_history", "traffic_history",
		"audit_log", "secret_templates",
	}
	return tables
}

// calculateFileChecksum calculates SHA256 checksum of a file.
func (s *BackupStore) calculateFileChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
