package sshutil

import (
	"strings"
	"testing"
)

func TestIsSafeContainerName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		isValid bool
	}{
		// Valid names
		{"simple lowercase", "popugate", true},
		{"with numbers", "app123", true},
		{"with dashes", "my-container", true},
		{"with underscores", "my_container", true},
		{"mixed case", "MyContainer", true},
		{"all allowed chars", "Aa1_-", true},

		// Invalid names
		{"empty string", "", false},
		{"semicolon injection", "foo; rm -rf /", false},
		{"pipe injection", "foo | cat /etc/passwd", false},
		{"backticks injection", "foo`whoami`", false},
		{"dollar injection", "foo$(whoami)", false},
		{"ampersand injection", "foo && echo pwned", false},
		{"newline injection", "foo\nbar", false},
		{"space in name", "my container", false},
		{"dot in name", "my.container", false},
		{"slash in name", "foo/bar", false},
		{"path traversal", "../foo", false},
		{"absolute path", "/usr/bin/foo", false},
		{"shell redirect", "foo > /tmp/out", false},
		{"double dash", "--flag", true}, // technically safe per regex, just dashes and letters
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSafeContainerName(tt.input)
			if got != tt.isValid {
				t.Errorf("isSafeContainerName(%q) = %v, want %v", tt.input, got, tt.isValid)
			}
		})
	}
}

func TestIsExcluded(t *testing.T) {
	excludeSet := map[string]bool{
		".ssh": true,
		".git": true,
		".db":  true,
	}

	tests := []struct {
		name     string
		relPath  string
		excluded bool
	}{
		{"exact match", ".ssh", true},
		{"contains pattern", "config/.ssh/id_rsa", true},
		{"git directory", ".git/HEAD", true},
		{"db file", "data/settings.db", true},
		{"no match", "mtproxy/config.toml", false},
		{"partial no match", "ssh_config.txt", false},
		{"empty path", "", false},
		{"different extension", "settings.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExcluded(tt.relPath, excludeSet)
			if got != tt.excluded {
				t.Errorf("isExcluded(%q) = %v, want %v", tt.relPath, got, tt.excluded)
			}
		})
	}
}

func TestIsExcluded_EmptySet(t *testing.T) {
	excludeSet := map[string]bool{}
	if isExcluded("any/path", excludeSet) {
		t.Error("empty exclude set should not exclude anything")
	}
}

func TestIsExcluded_MultiplePatternsAllMatch(t *testing.T) {
	excludeSet := map[string]bool{
		"secret": true,
		"key":    true,
	}
	// path contains both patterns — should still be excluded
	if !isExcluded("secret_key.txt", excludeSet) {
		t.Error("path matching any pattern should be excluded")
	}
}

func TestIsExcluded_RegressionOldContinueBug(t *testing.T) {
	// This test specifically catches the old bug where `continue` inside
	// the for-range loop continued the inner loop instead of the outer walker.
	// With the fix, isExcluded returns true/false and the caller skips correctly.

	excludeSet := map[string]bool{
		".ssh": true,
		".git": true,
	}

	// These should be excluded (the old bug would NOT exclude them)
	mustExclude := []string{
		".ssh/id_rsa",
		".ssh/authorized_keys",
		".git/objects/pack/foo",
	}
	for _, p := range mustExclude {
		if !isExcluded(p, excludeSet) {
			t.Errorf("isExcluded(%q) should be true — old continue bug regression", p)
		}
	}

	// These should NOT be excluded
	mustNotExclude := []string{
		"mtproxy/config.toml",
		"settings.db",
		"geoblock/zone_ru.cidr",
	}
	for _, p := range mustNotExclude {
		if isExcluded(p, excludeSet) {
			t.Errorf("isExcluded(%q) should be false", p)
		}
	}
}

func TestRestartRemote_RejectsInvalidContainerName(t *testing.T) {
	// We can't test the full SSH path without a real SSH server,
	// but we can verify the name validation is called before any SSH connection.
	// Since RestartRemote calls DialSSH (which fails without a real server),
	// an invalid name should fail BEFORE DialSSH is attempted.

	invalidNames := []string{
		"foo; rm -rf /",
		"",
		"foo$(whoami)",
		"foo`id`",
		"foo && echo pwned",
		"foo/bar",
		"../foo",
	}

	for _, name := range invalidNames {
		err := RestartRemote(nil, SyncConfig{}, name)
		if err == nil {
			t.Errorf("expected error for container name %q, got nil", name)
		}
		if !strings.Contains(err.Error(), "invalid container name") {
			t.Errorf("expected 'invalid container name' error for %q, got: %v", name, err)
		}
	}
}

func TestRestartRemote_AcceptsValidContainerName(t *testing.T) {
	// Valid names should pass the check and fail only on SSH connection (no real server)
	err := RestartRemote(nil, SyncConfig{}, "popugate")
	// Should NOT fail with "invalid container name"
	if err != nil && strings.Contains(err.Error(), "invalid container name") {
		t.Errorf("valid name 'popugate' rejected: %v", err)
	}
	// It will fail on SSH connection, which is expected
}
