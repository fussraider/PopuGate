package service

import "testing"

func TestIsSafeGitURL(t *testing.T) {
	tests := []struct {
		url  string
		safe bool
	}{
		{"https://github.com/user/repo.git", true},
		{"git@github.com:user/repo.git", true},
		{"https://github.com/user/repo", true},
		{"", false},
		{`https://github.com/user/repo"; rm -rf /`, false},
		{`https://evil.com$(whoami)`, false},
		{"https://github.com/user/repo;echo pwned", false},
		{"https://github.com/user/repo|cat", false},
		{`https://github.com/user/repo'`, false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := isSafeGitURL(tt.url)
			if got != tt.safe {
				t.Errorf("isSafeGitURL(%q) = %v, want %v", tt.url, got, tt.safe)
			}
		})
	}
}

func TestIsSafeGitRef(t *testing.T) {
	tests := []struct {
		ref  string
		safe bool
	}{
		{"abc123def456", true},
		{"main", true},
		{"v3.3.39", true},
		{"refs/heads/main", true},
		{"", false},
		{"abc; rm -rf /", false},
		{"$(whoami)", false},
		{"`id`", false},
		{"abc|def", false},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got := isSafeGitRef(tt.ref)
			if got != tt.safe {
				t.Errorf("isSafeGitRef(%q) = %v, want %v", tt.ref, got, tt.safe)
			}
		})
	}
}
