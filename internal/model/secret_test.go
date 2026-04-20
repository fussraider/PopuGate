package model

import (
	"strings"
	"testing"
	"time"
)

func TestValidateLabel(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		wantErr bool
	}{
		{"simple alphanumeric", "user1", false},
		{"underscore", "test_user", false},
		{"hyphen", "a-b", false},
		{"mixed", "user_1-test", false},
		{"all letters", "abcde", false},
		{"all digits", "12345", false},
		{"single char", "a", false},
		{"32 chars", strings.Repeat("a", 32), false},
		{"empty string", "", true},
		{"spaces", "user with spaces", true},
		{"33 chars", strings.Repeat("a", 33), true},
		{"special char @", "user@domain", true},
		{"special char !", "user!", true},
		{"special char .", "user.name", true},
		{"tab character", "user\tname", true},
		{"newline", "user\nname", true},
		{"unicode", "пользователь", true},
		{"leading space", " user", true},
		{"trailing space", "user ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLabel(tt.label)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateLabel(%q) error = %v, wantErr %v", tt.label, err, tt.wantErr)
			}
		})
	}
}

func TestSecretIsActive(t *testing.T) {
	tests := []struct {
		name      string
		enabled   bool
		expiresAt string
		want      bool
	}{
		{"enabled, no expiry", true, "", true},
		{"enabled, zero expiry", true, "0", true},
		{"disabled, no expiry", false, "", false},
		{"disabled, zero expiry", false, "0", false},
		{"enabled, future RFC3339", true, time.Now().Add(24 * time.Hour).Format(time.RFC3339), true},
		{"enabled, future date-only", true, time.Now().AddDate(0, 0, 1).Format("2006-01-02"), true},
		{"enabled, past RFC3339", true, time.Now().Add(-24 * time.Hour).Format(time.RFC3339), false},
		{"enabled, past date-only", true, time.Now().AddDate(0, 0, -1).Format("2006-01-02"), false},
		{"enabled, unparseable expiry", true, "not-a-date", true},
		{"disabled, future expiry", false, time.Now().Add(24 * time.Hour).Format(time.RFC3339), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Secret{
				Enabled:   tt.enabled,
				ExpiresAt: tt.expiresAt,
			}
			got := s.IsActive()
			if got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecretIsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt string
		want      bool
	}{
		{"no expiry", "", false},
		{"zero expiry", "0", false},
		{"past RFC3339", time.Now().Add(-1 * time.Hour).Format(time.RFC3339), true},
		{"past date-only", time.Now().AddDate(0, 0, -1).Format("2006-01-02"), true},
		{"future RFC3339", time.Now().Add(1 * time.Hour).Format(time.RFC3339), false},
		{"future date-only", time.Now().AddDate(0, 0, 1).Format("2006-01-02"), false},
		{"unparseable", "garbage", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Secret{ExpiresAt: tt.expiresAt}
			got := s.IsExpired()
			if got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecretExpiryWarning(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt string
		within    time.Duration
		want      bool
	}{
		{
			"no expiry",
			"",
			24 * time.Hour,
			false,
		},
		{
			"zero expiry",
			"0",
			24 * time.Hour,
			false,
		},
		{
			"expires within 24h, threshold 48h",
			time.Now().Add(12 * time.Hour).Format(time.RFC3339),
			48 * time.Hour,
			true,
		},
		{
			"expires within 24h, threshold 6h",
			time.Now().Add(12 * time.Hour).Format(time.RFC3339),
			6 * time.Hour,
			false,
		},
		{
			"already expired",
			time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
			48 * time.Hour,
			false,
		},
		{
			"far future, large threshold",
			time.Now().AddDate(0, 0, 30).Format(time.RFC3339),
			48 * time.Hour,
			false,
		},
		{
			"unparseable",
			"garbage",
			24 * time.Hour,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Secret{ExpiresAt: tt.expiresAt}
			got := s.ExpiryWarning(tt.within)
			if got != tt.want {
				t.Errorf("ExpiryWarning() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecretQuotaPercent(t *testing.T) {
	tests := []struct {
		name       string
		quotaBytes int64
		trafficIn  int64
		trafficOut int64
		want       float64
	}{
		{"no quota", 0, 500, 500, 0},
		{"negative quota", -1, 500, 500, 0},
		{"50 percent", 1000, 250, 250, 50},
		{"80 percent", 1000, 400, 400, 80},
		{"100 percent", 1000, 500, 500, 100},
		{"over 100 percent", 1000, 800, 800, 160},
		{"zero traffic", 1000, 0, 0, 0},
		{"only inbound", 1000, 500, 0, 50},
		{"only outbound", 1000, 0, 500, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Secret{
				QuotaBytes: tt.quotaBytes,
				TrafficIn:  tt.trafficIn,
				TrafficOut: tt.trafficOut,
			}
			got := s.QuotaPercent()
			if got != tt.want {
				t.Errorf("QuotaPercent() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestSecretQuotaExceeded(t *testing.T) {
	tests := []struct {
		name       string
		quotaBytes int64
		trafficIn  int64
		trafficOut int64
		want       bool
	}{
		{"no quota", 0, 500, 500, false},
		{"negative quota", -1, 500, 500, false},
		{"traffic equals quota", 1000, 500, 500, true},
		{"traffic exceeds quota", 1000, 600, 600, true},
		{"traffic below quota", 1000, 400, 400, false},
		{"zero traffic", 1000, 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Secret{
				QuotaBytes: tt.quotaBytes,
				TrafficIn:  tt.trafficIn,
				TrafficOut: tt.trafficOut,
			}
			got := s.QuotaExceeded()
			if got != tt.want {
				t.Errorf("QuotaExceeded() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSecretQuotaWarning(t *testing.T) {
	tests := []struct {
		name       string
		quotaBytes int64
		trafficIn  int64
		trafficOut int64
		want       bool
	}{
		{"no quota", 0, 500, 500, false},
		{"at 80 percent", 1000, 400, 400, true},
		{"at 79 percent", 1000, 395, 395, false},
		{"at 100 percent", 1000, 500, 500, true},
		{"over 100 percent", 1000, 600, 600, true},
		{"at 50 percent", 1000, 250, 250, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Secret{
				QuotaBytes: tt.quotaBytes,
				TrafficIn:  tt.trafficIn,
				TrafficOut: tt.trafficOut,
			}
			got := s.QuotaWarning()
			if got != tt.want {
				t.Errorf("QuotaWarning() = %v, want %v", got, tt.want)
			}
		})
	}
}
