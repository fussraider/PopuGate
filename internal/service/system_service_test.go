package service

import (
	"strings"
	"testing"
)

func TestParseSystemctlProperty(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ActiveState=active\n", "active"},
		{"ActiveState=inactive\n", "inactive"},
		{"MainPID=1234\n", "1234"},
		{"MainPID=0\n", "0"},
		{"KEY=\n", ""},
		{"", ""},
		{"NOEQUALSSIGN\n", ""},
		{"KEY=VALUE=WITH=EQUALS\n", "VALUE=WITH=EQUALS"},
		{"  KEY  =  value  \n", "  value"},
	}

	for _, tt := range tests {
		got := parseSystemctlProperty(tt.input)
		if got != tt.want {
			t.Errorf("parseSystemctlProperty(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		s    string
		subs []string
		want bool
	}{
		{"ubuntu 22.04", []string{"ubuntu", "debian"}, true},
		{"debian bullseye", []string{"ubuntu"}, false},
		{"", []string{"anything"}, false},
		{"centos 9", []string{"centos", "rhel"}, true},
		{"alpine linux", []string{"alpine"}, true},
		{"fedora 40", []string{"ubuntu", "debian"}, false},
	}

	for _, tt := range tests {
		got := containsAny(tt.s, tt.subs...)
		if got != tt.want {
			t.Errorf("containsAny(%q, %v) = %v, want %v", tt.s, tt.subs, got, tt.want)
		}
	}
}

func TestDetectOS(t *testing.T) {
	os := DetectOS()
	if os == nil {
		t.Fatal("DetectOS returned nil")
	}
	if os.Arch == "" {
		t.Error("Arch should not be empty")
	}
	if os.Family == "" {
		t.Error("Family should not be empty")
	}
}

func TestShellescape(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "'simple'"},
		{"", "''"},
		{"it's", "'it'\\''s'"},
		{"hello world", "'hello world'"},
		{"popugate", "'popugate'"},
	}

	for _, tt := range tests {
		got := shellescape(tt.input)
		if got != tt.want {
			t.Errorf("shellescape(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseCIDRs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"single CIDR", "10.0.0.0/8\n", 1},
		{"multiple", "10.0.0.0/8\n172.16.0.0/12\n", 2},
		{"with comments", "# comment\n10.0.0.0/8\n", 1},
		{"with empty lines", "\n10.0.0.0/8\n\n172.16.0.0/12\n", 2},
		{"empty", "", 0},
		{"only comments", "# comment\n# another\n", 0},
		{"trailing whitespace", "10.0.0.0/8  \n  172.16.0.0/12  \n", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCIDRs(tt.input)
			if len(got) != tt.want {
				t.Errorf("parseCIDRs returned %d entries, want %d: %v", len(got), tt.want, got)
			}
		})
	}
}

func TestIsSystemdInstalled(t *testing.T) {
	// On macOS (test env), systemd should not be installed
	installed := IsSystemdInstalled()
	if installed {
		// Could be true on Linux CI, but on macOS it should be false
		if strings.HasPrefix(DetectOS().Family, "macos") {
			t.Error("IsSystemdInstalled should be false on macOS")
		}
	}
}
