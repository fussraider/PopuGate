package service

import "testing"

func TestIsValidCountryCode(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		{"us", true},
		{"US", true},
		{" Us ", true},
		{"ru", true},
		{"de", true},
		{"gb", true},
		{"cn", true},
		{"xx", false},
		{"a", false},
		{"abc", false},
		{"12", false},
		{"", false},
		{"../../", false},
		{"r", false},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := IsValidCountryCode(tt.code); got != tt.want {
				t.Errorf("IsValidCountryCode(%q) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}
