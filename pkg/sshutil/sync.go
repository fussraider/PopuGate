package sshutil

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// SyncResult holds the result of a sync operation.
type SyncResult struct {
	Host       string `json:"host"`
	FilesSent  int    `json:"files_sent"`
	FilesDel   int    `json:"files_deleted"`
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// SyncConfig holds configuration for a sync operation.
type SyncConfig struct {
	Host           string
	Port           int
	User           string
	PrivateKeyPath string
	SourceDir      string
	Exclude        []string
	DeleteExtra    bool
	KnownHostsPath string // path to known_hosts file (required)
}

// Sync performs an SFTP sync from source to remote.
func Sync(ctx context.Context, cfg SyncConfig) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{Host: cfg.Host}

	// Connect via SSH
	sshClient, err := DialSSH(cfg)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	defer sshClient.Close()

	// Open SFTP session
	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	defer sftpClient.Close()

	// Walk local directory and sync files
	excludeSet := make(map[string]bool)
	for _, e := range cfg.Exclude {
		excludeSet[e] = true
	}

	err = filepath.Walk(cfg.SourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(cfg.SourceDir, path)
		if err != nil {
			return err
		}

		// Check exclusions
		for exc := range excludeSet {
			if strings.Contains(relPath, exc) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if info.IsDir() {
			// Create remote directory (use forward slashes for SFTP)
			remotePath := "/" + relPath
			_ = sftpClient.Mkdir(remotePath)
			return nil
		}

		// Compare and upload if needed
		if needsUpload(sftpClient, path, relPath, info) {
			if err := uploadFile(sftpClient, path, relPath); err != nil {
				return fmt.Errorf("upload %s: %w", relPath, err)
			}
			result.FilesSent++
		}

		return nil
	})

	if err != nil {
		result.Error = err.Error()
	}

	// Delete extra files on remote
	if cfg.DeleteExtra && result.FilesSent > 0 {
		deleted, _ := deleteExtraFiles(sftpClient, cfg.SourceDir, excludeSet)
		result.FilesDel = deleted
	}

	result.DurationMs = time.Since(start).Milliseconds()
	return result, nil
}

// RestartRemote runs docker restart on the remote host via SSH.
func RestartRemote(ctx context.Context, cfg SyncConfig, containerName string) error {
	if !isSafeContainerName(containerName) {
		return fmt.Errorf("invalid container name: %s", containerName)
	}

	sshClient, err := DialSSH(cfg)
	if err != nil {
		return err
	}
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	return session.Run(fmt.Sprintf("docker restart %s", containerName))
}

// TestSSH tests SSH connectivity to a host.
func TestSSH(ctx context.Context, cfg SyncConfig) error {
	sshClient, err := DialSSH(cfg)
	if err != nil {
		return err
	}
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	return session.Run("echo ok")
}

func DialSSH(cfg SyncConfig) (*ssh.Client, error) {
	keyBytes, err := os.ReadFile(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	hostKeyCallback, err := hostKeyCallbackFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("host key setup: %w", err)
	}

	config := &ssh.ClientConfig{
		User: cfg.User,
		Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		Config: ssh.Config{
			KeyExchanges: []string{"curve25519-sha256"},
		},
		Timeout:         10 * time.Second,
		HostKeyCallback: hostKeyCallback,
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	return ssh.Dial("tcp", addr, config)
}

// hostKeyCallbackFor returns a HostKeyCallback based on the SyncConfig.
// KnownHostsPath is required. If the file exists, it verifies against it.
// If the file doesn't exist, it auto-accepts and saves the host key on first connection (TOFU).
// If KnownHostsPath is empty, returns an error — insecure host key verification is not allowed.
func hostKeyCallbackFor(cfg SyncConfig) (ssh.HostKeyCallback, error) {
	if cfg.KnownHostsPath == "" {
		return nil, fmt.Errorf("KnownHostsPath is required: SSH host key verification cannot be disabled")
	}

	// If known_hosts file exists, use strict verification
	if _, err := os.Stat(cfg.KnownHostsPath); err == nil {
		cb, err := knownhosts.New(cfg.KnownHostsPath)
		if err != nil {
			return nil, fmt.Errorf("parse known_hosts: %w", err)
		}
		return cb, nil
	}

	// First connection: auto-accept and save host key (Trust On First Use)
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		// Save the host key for future connections
		if err := saveHostKey(cfg.KnownHostsPath, hostname, key); err != nil {
			return fmt.Errorf("save host key: %w", err)
		}
		return nil
	}, nil
}

// saveHostKey appends a host key to the known_hosts file.
func saveHostKey(path, hostname string, key ssh.PublicKey) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	line := knownhosts.Line([]string{hostname}, key)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "%s\n", line)
	return err
}

func needsUpload(sftpClient *sftp.Client, localPath, relPath string, info os.FileInfo) bool {
	remotePath := "/" + relPath
	remoteInfo, err := sftpClient.Stat(remotePath)
	if err != nil {
		return true // file doesn't exist
	}
	// Upload if local is newer or different size
	localMod := info.ModTime().Unix()
	remoteMod := remoteInfo.ModTime().Unix()
	return localMod > remoteMod || info.Size() != remoteInfo.Size()
}

func uploadFile(sftpClient *sftp.Client, localPath, relPath string) error {
	localFile, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer localFile.Close()

	remotePath := "/" + relPath
	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return err
	}
	defer remoteFile.Close()

	_, err = io.Copy(remoteFile, localFile)
	return err
}

func deleteExtraFiles(sftpClient *sftp.Client, sourceDir string, excludeSet map[string]bool) (int, error) {
	// Walk remote and remove files not present locally
	var deleted int
	walker := sftpClient.Walk(sourceDir)
	for walker.Step() {
		if err := walker.Err(); err != nil {
			continue
		}
		path := walker.Path()
		relPath := strings.TrimPrefix(path, "/")

		if relPath == "" {
			continue
		}

		if isExcluded(relPath, excludeSet) {
			continue
		}

		localPath := sourceDir + string(os.PathSeparator) + relPath
		if _, err := os.Stat(localPath); os.IsNotExist(err) {
			_ = sftpClient.Remove(path)
			deleted++
		}
	}
	return deleted, nil
}

// isExcluded checks whether relPath matches any exclusion pattern.
func isExcluded(relPath string, excludeSet map[string]bool) bool {
	for exc := range excludeSet {
		if strings.Contains(relPath, exc) {
			return true
		}
	}
	return false
}

// isSafeContainerName validates that a container name only contains safe characters.
func isSafeContainerName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
