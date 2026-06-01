package netutil

import (
	"strings"
	"testing"
)

func TestValidateIPSetName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "mtpmax_ru", false},
		{"valid with hyphens", "my-set-123", false},
		{"valid with underscores", "my_set_456", false},
		{"empty", "", true},
		{"too long", strings.Repeat("a", 129), true},
		{"max length ok", strings.Repeat("a", 128), false},
		{"with spaces", "my set", true},
		{"with semicolon", "set;rm -rf", true},
		{"with pipe", "set|evil", true},
		{"with dollar", "set$var", true},
		{"with backtick", "set`cmd`", true},
		{"path traversal", "../etc/passwd", true},
		{"with newline", "set\nname", true},
		{"with special chars", "set&(evil)", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPSetName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateIPSetName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid 80", "80", false},
		{"valid 443", "443", false},
		{"valid 1", "1", false},
		{"valid 65535", "65535", false},
		{"zero", "0", true},
		{"negative", "-1", true},
		{"too high", "65536", true},
		{"very large", "99999", true},
		{"not a number", "abc", true},
		{"empty", "", true},
		{"with spaces", "80  ", true},
		{"float", "80.5", true},
		{"with semicolon", "80;echo", true},
		{"path traversal", "../etc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePort(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateAction(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"DROP ok", "DROP", false},
		{"ACCEPT ok", "ACCEPT", false},
		{"REJECT ok", "REJECT", false},
		{"lowercase drop", "drop", true},
		{"empty", "", true},
		{"arbitrary string", "ACCEPT;rm -rf /", true},
		{"LOG", "LOG", true},
		{"RETURN", "RETURN", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAction(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAction(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestSetNameForCountry(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"RU", "mtpmax_ru"},
		{"us", "mtpmax_us"},
		{"De", "mtpmax_de"},
		{"CN", "mtpmax_cn"},
	}
	for _, tt := range tests {
		got := SetNameForCountry(tt.input)
		if got != tt.want {
			t.Errorf("SetNameForCountry(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSetTCPMSSRule_InvalidPort(t *testing.T) {
	m := NewIptablesManager()
	tests := []struct {
		name string
		port int
		mss  int
	}{
		{"zero port", 0, 88},
		{"negative port", -1, 88},
		{"too high port", 70000, 88},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := m.SetTCPMSSRule(tt.port, tt.mss); err == nil {
				t.Errorf("expected error for port=%d", tt.port)
			}
		})
	}
}

func TestSetTCPMSSRule_InvalidMSS(t *testing.T) {
	m := NewIptablesManager()
	tests := []struct {
		name string
		mss  int
	}{
		{"zero", 0},
		{"negative", -1},
		{"too high", 1461},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := m.SetTCPMSSRule(443, tt.mss); err == nil {
				t.Errorf("expected error for mss=%d", tt.mss)
			}
		})
	}
}

func TestPortRedirect_InvalidPorts(t *testing.T) {
	m := NewIptablesManager()
	invalidPorts := []int{0, -1, 70000}

	for _, p := range invalidPorts {
		t.Run("AddPortRedirect invalid primary", func(t *testing.T) {
			if err := m.AddPortRedirect(p, 10443); err == nil {
				t.Errorf("expected error for primary port %d", p)
			}
		})
		t.Run("AddPortRedirect invalid temp", func(t *testing.T) {
			if err := m.AddPortRedirect(443, p); err == nil {
				t.Errorf("expected error for temp port %d", p)
			}
		})
		t.Run("RemovePortRedirect invalid primary", func(t *testing.T) {
			if err := m.RemovePortRedirect(p, 10443); err == nil {
				t.Errorf("expected error for primary port %d", p)
			}
		})
		t.Run("RemovePortRedirect invalid temp", func(t *testing.T) {
			if err := m.RemovePortRedirect(443, p); err == nil {
				t.Errorf("expected error for temp port %d", p)
			}
		})
		t.Run("HasPortRedirect invalid primary", func(t *testing.T) {
			if _, err := m.HasPortRedirect(p, 10443); err == nil {
				t.Errorf("expected error for primary port %d", p)
			}
		})
		t.Run("HasPortRedirect invalid temp", func(t *testing.T) {
			if _, err := m.HasPortRedirect(443, p); err == nil {
				t.Errorf("expected error for temp port %d", p)
			}
		})
	}
}

