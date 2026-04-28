package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
	"github.com/fussraider/PopuGate/internal/store"
	"github.com/fussraider/PopuGate/pkg/logger"
	"github.com/fussraider/PopuGate/pkg/sshutil"
)

// ReplicationService handles master/slave sync.
type ReplicationService struct {
	settings *store.SettingsStore
	slaves   *store.SlaveStore
}

// NewReplicationService creates a new ReplicationService.
func NewReplicationService(settings *store.SettingsStore, slaves *store.SlaveStore) *ReplicationService {
	return &ReplicationService{settings: settings, slaves: slaves}
}

// SyncAll syncs to all enabled slaves (with lock file to prevent concurrent runs).
func (s *ReplicationService) SyncAll(ctx context.Context) []sshutil.SyncResult {
	syncLockPath := filepath.Join(model.InstallDir, "sync.lock")
	// Acquire lock to prevent concurrent sync
	lockFile, err := os.OpenFile(syncLockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err == nil {
		defer lockFile.Close()
		if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			return []sshutil.SyncResult{{Error: "another sync already running"}}
		}
		defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	}

	settings, _ := s.settings.Load(ctx)
	slaves, _ := s.slaves.List(ctx)

	var results []sshutil.SyncResult
	for _, sl := range slaves {
		if !sl.Enabled {
			continue
		}

		result := s.syncSlave(ctx, settings, sl)
		results = append(results, result)

		// Update slave status
		status := "ok"
		if result.Error != "" {
			status = "error"
		}
		if err := s.slaves.UpdateStatus(ctx, sl.Host, status, time.Now().Unix()); err != nil {
			logger.WithScope("replication").Warnf("update status for %s: %v", sl.Host, err)
		}

		// Restart remote container if files changed
		if result.FilesSent > 0 && settings.ReplicationRestartOnChange {
			cfg := s.buildSyncConfig(settings, sl)
			if err := sshutil.RestartRemote(ctx, cfg, model.ContainerName); err != nil {
				logger.WithScope("replication").Warnf("restart remote %s: %v", sl.Host, err)
			}
		}
	}

	return results
}

// SyncSlave syncs to a specific slave.
func (s *ReplicationService) SyncSlave(ctx context.Context, host string) (*sshutil.SyncResult, error) {
	settings, _ := s.settings.Load(ctx)
	slave, err := s.slaves.GetByHost(ctx, host)
	if err != nil {
		return nil, err
	}
	if slave == nil {
		return nil, fmt.Errorf("slave %s not found", host)
	}

	result := s.syncSlave(ctx, settings, *slave)

	status := "ok"
	if result.Error != "" {
		status = "error"
	}
	if err := s.slaves.UpdateStatus(ctx, host, status, time.Now().Unix()); err != nil {
		logger.WithScope("replication").Warnf("update status for %s: %v", host, err)
	}

	return &result, nil
}

// TestSSH tests connectivity to a slave.
func (s *ReplicationService) TestSSH(ctx context.Context, host string) (*model.SlaveTestResult, error) {
	settings, _ := s.settings.Load(ctx)
	slave, err := s.slaves.GetByHost(ctx, host)
	if err != nil {
		return nil, err
	}
	if slave == nil {
		return nil, fmt.Errorf("slave %s not found", host)
	}

	cfg := s.buildSyncConfig(settings, *slave)
	result := &model.SlaveTestResult{Host: host}

	if err := sshutil.TestSSH(ctx, cfg); err != nil {
		result.Error = err.Error()
		return result, nil
	}
	result.SSHOK = true

	// Try to get docker status via SSH
	sshClient, err := sshutil.DialSSH(cfg)
	if err == nil {
		session, err := sshClient.NewSession()
		if err == nil {
			var buf bytes.Buffer
			session.Stdout = &buf
			if err := session.Run(fmt.Sprintf("docker ps --filter name=%s --format '{{.Status}}'", shellescape(model.ContainerName))); err == nil {
				result.DockerStatus = strings.TrimSpace(buf.String())
			}
			session.Close()
		}
		sshClient.Close()
	}

	return result, nil
}

// GenerateSSHKey generates an ed25519 key pair for replication.
func (s *ReplicationService) GenerateSSHKey(ctx context.Context) (string, error) {
	settings, _ := s.settings.Load(ctx)
	return sshutil.GenerateEd25519Key(settings.SSHKeyPath())
}

// GetSSHPublicKey reads the existing public key from disk.
func (s *ReplicationService) GetSSHPublicKey(ctx context.Context) (string, error) {
	settings, _ := s.settings.Load(ctx)
	return sshutil.ReadPublicKey(settings.SSHKeyPath())
}

func (s *ReplicationService) syncSlave(ctx context.Context, settings *model.Settings, sl model.Slave) sshutil.SyncResult {
	cfg := s.buildSyncConfig(settings, sl)
	result, err := sshutil.Sync(ctx, cfg)
	if err != nil || result == nil {
		return sshutil.SyncResult{Host: sl.Host, Error: fmt.Sprintf("sync failed: %v", err)}
	}
	return *result
}

func (s *ReplicationService) buildSyncConfig(settings *model.Settings, sl model.Slave) sshutil.SyncConfig {
	return sshutil.SyncConfig{
		Host:           sl.Host,
		Port:           sl.Port,
		User:           settings.ReplicationSSHUser,
		PrivateKeyPath: settings.SSHKeyPath(),
		SourceDir:      model.InstallDir,
		Exclude:        strings.Split(settings.ReplicationExclude, ","),
		DeleteExtra:    settings.ReplicationDeleteExtra,
		KnownHostsPath: filepath.Join(model.InstallDir, ".ssh", "known_hosts"),
	}
}

// shellescape wraps a string in single quotes for safe shell interpolation.
func shellescape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
