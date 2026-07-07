package promutil

import (
	"math"
	"testing"
)

const samplePromText = `# HELP telemt_connections_current Current connections
# TYPE telemt_connections_current gauge
telemt_connections_current 42
telemt_connections_total 100
telemt_connections_bad_total 5
telemt_uptime_seconds 3600.5
telemt_upstream_connect_attempt_total 200
telemt_upstream_connect_success_total 180
telemt_upstream_connect_fail_total 20
telemt_me_writers_active_current 3
telemt_me_writers_warm_current 7
# HELP telemt_user_octets_from_client Per-user bytes from client
# TYPE telemt_user_octets_from_client counter
telemt_user_octets_from_client{user="user1"} 1024
telemt_user_octets_to_client{user="user1"} 2048
telemt_user_connections_current{user="user1"} 5
telemt_user_unique_ips_current{user="user1"} 3
telemt_user_octets_from_client{user="user2"} 4096
telemt_user_octets_to_client{user="user2"} 8192
telemt_connections_bad_by_class_total{class="unknown_tls_sni"} 12
telemt_connections_bad_by_class_total{class="timeout"} 3
telemt_handshake_failures_by_class_total{class="timeout"} 7
`

func TestExtractTelemtMetrics_ClassLabeled(t *testing.T) {
	metrics, err := ParseMetrics(samplePromText)
	if err != nil {
		t.Fatalf("ParseMetrics() error: %v", err)
	}
	lm := ExtractTelemtMetrics(metrics)
	if lm.BadByClass["unknown_tls_sni"] != 12 {
		t.Errorf("BadByClass[unknown_tls_sni] = %f, want 12", lm.BadByClass["unknown_tls_sni"])
	}
	if lm.BadByClass["timeout"] != 3 {
		t.Errorf("BadByClass[timeout] = %f, want 3", lm.BadByClass["timeout"])
	}
	if lm.HandshakeFailByClass["timeout"] != 7 {
		t.Errorf("HandshakeFailByClass[timeout] = %f, want 7", lm.HandshakeFailByClass["timeout"])
	}
	if len(lm.BadByClass) != 2 {
		t.Errorf("BadByClass len = %d, want 2", len(lm.BadByClass))
	}
}

func TestParseMetrics(t *testing.T) {
	metrics, err := ParseMetrics(samplePromText)
	if err != nil {
		t.Fatalf("ParseMetrics() returned error: %v", err)
	}
	if len(metrics) == 0 {
		t.Fatal("ParseMetrics() returned empty metrics for valid input")
	}

	// Verify comment and blank lines are skipped
	for _, m := range metrics {
		if m.Name == "" {
			t.Fatal("ParseMetrics() returned metric with empty name")
		}
	}

	// Find telemt_connections_current
	found := false
	for _, m := range metrics {
		if m.Name == "telemt_connections_current" {
			found = true
			if m.Value != 42 {
				t.Errorf("telemt_connections_current = %f, want 42", m.Value)
			}
			if len(m.Labels) != 0 {
				t.Errorf("telemt_connections_current should have no labels, got %v", m.Labels)
			}
		}
	}
	if !found {
		t.Error("ParseMetrics() did not find telemt_connections_current")
	}
}

func TestParseMetricsWithLabels(t *testing.T) {
	metrics, err := ParseMetrics(samplePromText)
	if err != nil {
		t.Fatalf("ParseMetrics() returned error: %v", err)
	}

	// Find a labeled metric
	found := false
	for _, m := range metrics {
		if m.Name == "telemt_user_octets_from_client" && m.Labels["user"] == "user1" {
			found = true
			if m.Value != 1024 {
				t.Errorf("telemt_user_octets_from_client{user1} = %f, want 1024", m.Value)
			}
		}
	}
	if !found {
		t.Error("ParseMetrics() did not find telemt_user_octets_from_client{user=\"user1\"}")
	}
}

func TestParseMetricsEmpty(t *testing.T) {
	metrics, err := ParseMetrics("")
	if err != nil {
		t.Fatalf("ParseMetrics(\"\") returned error: %v", err)
	}
	if len(metrics) != 0 {
		t.Errorf("ParseMetrics(\"\") returned %d metrics, want 0", len(metrics))
	}
}

func TestParseMetricsCommentsOnly(t *testing.T) {
	input := `# HELP some_metric A description
# TYPE some_metric gauge
`
	metrics, err := ParseMetrics(input)
	if err != nil {
		t.Fatalf("ParseMetrics() returned error: %v", err)
	}
	if len(metrics) != 0 {
		t.Errorf("ParseMetrics() with only comments returned %d metrics, want 0", len(metrics))
	}
}

func TestParseMetricsFloatValues(t *testing.T) {
	input := `telemt_uptime_seconds 3600.5
telemt_some_value 1.5e3
`
	metrics, err := ParseMetrics(input)
	if err != nil {
		t.Fatalf("ParseMetrics() returned error: %v", err)
	}
	if len(metrics) != 2 {
		t.Fatalf("ParseMetrics() returned %d metrics, want 2", len(metrics))
	}

	if metrics[0].Value != 3600.5 {
		t.Errorf("metrics[0].Value = %f, want 3600.5", metrics[0].Value)
	}
	if metrics[1].Value != 1500 {
		t.Errorf("metrics[1].Value = %f, want 1500", metrics[1].Value)
	}
}

func TestParseMetricsNaN(t *testing.T) {
	input := "telemt_test NaN\n"
	metrics, err := ParseMetrics(input)
	if err != nil {
		t.Fatalf("ParseMetrics() returned error: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("ParseMetrics() returned %d metrics, want 1", len(metrics))
	}
	if !math.IsNaN(metrics[0].Value) {
		t.Errorf("metrics[0].Value = %f, want NaN", metrics[0].Value)
	}
}

func TestParseMetricsNegativeValue(t *testing.T) {
	input := "telemt_test -42.5\n"
	metrics, err := ParseMetrics(input)
	if err != nil {
		t.Fatalf("ParseMetrics() returned error: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("ParseMetrics() returned %d metrics, want 1", len(metrics))
	}
	if metrics[0].Value != -42.5 {
		t.Errorf("metrics[0].Value = %f, want -42.5", metrics[0].Value)
	}
}

func TestExtractTelemtMetrics(t *testing.T) {
	metrics, err := ParseMetrics(samplePromText)
	if err != nil {
		t.Fatalf("ParseMetrics() returned error: %v", err)
	}

	lm := ExtractTelemtMetrics(metrics)
	if lm == nil {
		t.Fatal("ExtractTelemtMetrics() returned nil")
	}

	// Check scalar metrics
	if lm.ConnsCurrent != 42 {
		t.Errorf("ConnsCurrent = %f, want 42", lm.ConnsCurrent)
	}
	if lm.ConnsTotal != 100 {
		t.Errorf("ConnsTotal = %f, want 100", lm.ConnsTotal)
	}
	if lm.ConnsBadTotal != 5 {
		t.Errorf("ConnsBadTotal = %f, want 5", lm.ConnsBadTotal)
	}
	if lm.UptimeSeconds != 3600.5 {
		t.Errorf("UptimeSeconds = %f, want 3600.5", lm.UptimeSeconds)
	}
	if lm.UpstreamAttemptTotal != 200 {
		t.Errorf("UpstreamAttemptTotal = %f, want 200", lm.UpstreamAttemptTotal)
	}
	if lm.UpstreamSuccessTotal != 180 {
		t.Errorf("UpstreamSuccessTotal = %f, want 180", lm.UpstreamSuccessTotal)
	}
	if lm.UpstreamFailTotal != 20 {
		t.Errorf("UpstreamFailTotal = %f, want 20", lm.UpstreamFailTotal)
	}
	if lm.MEWritersActive != 3 {
		t.Errorf("MEWritersActive = %f, want 3", lm.MEWritersActive)
	}
	if lm.MEWritersWarm != 7 {
		t.Errorf("MEWritersWarm = %f, want 7", lm.MEWritersWarm)
	}
}

func TestExtractTelemtMetricsUserMetrics(t *testing.T) {
	metrics, err := ParseMetrics(samplePromText)
	if err != nil {
		t.Fatalf("ParseMetrics() returned error: %v", err)
	}

	lm := ExtractTelemtMetrics(metrics)

	// Check user metrics
	if len(lm.UserMetrics) != 2 {
		t.Fatalf("len(UserMetrics) = %d, want 2", len(lm.UserMetrics))
	}

	u1, ok := lm.UserMetrics["user1"]
	if !ok {
		t.Fatal("user1 not found in UserMetrics")
	}
	if u1.OctetsFromClient != 1024 {
		t.Errorf("user1.OctetsFromClient = %f, want 1024", u1.OctetsFromClient)
	}
	if u1.OctetsToClient != 2048 {
		t.Errorf("user1.OctetsToClient = %f, want 2048", u1.OctetsToClient)
	}
	if u1.Connections != 5 {
		t.Errorf("user1.Connections = %f, want 5", u1.Connections)
	}
	if u1.UniqueIPs != 3 {
		t.Errorf("user1.UniqueIPs = %f, want 3", u1.UniqueIPs)
	}

	u2, ok := lm.UserMetrics["user2"]
	if !ok {
		t.Fatal("user2 not found in UserMetrics")
	}
	if u2.OctetsFromClient != 4096 {
		t.Errorf("user2.OctetsFromClient = %f, want 4096", u2.OctetsFromClient)
	}
	if u2.OctetsToClient != 8192 {
		t.Errorf("user2.OctetsToClient = %f, want 8192", u2.OctetsToClient)
	}
	// user2 has no connections or unique_ips metrics; should be zero
	if u2.Connections != 0 {
		t.Errorf("user2.Connections = %f, want 0", u2.Connections)
	}
	if u2.UniqueIPs != 0 {
		t.Errorf("user2.UniqueIPs = %f, want 0", u2.UniqueIPs)
	}
}

func TestExtractTelemtMetricsEmpty(t *testing.T) {
	lm := ExtractTelemtMetrics(nil)
	if lm == nil {
		t.Fatal("ExtractTelemtMetrics(nil) returned nil")
	}
	if lm.UserMetrics == nil {
		t.Fatal("ExtractTelemtMetrics(nil) returned nil UserMetrics map")
	}
	if lm.ConnsCurrent != 0 {
		t.Errorf("ConnsCurrent = %f, want 0 for empty input", lm.ConnsCurrent)
	}
}

func TestExtractTelemtMetricsUnknownMetricsIgnored(t *testing.T) {
	input := `some_other_metric 99
telemt_unknown_name 42
telemt_connections_current 10
`
	metrics, _ := ParseMetrics(input)
	lm := ExtractTelemtMetrics(metrics)

	if lm.ConnsCurrent != 10 {
		t.Errorf("ConnsCurrent = %f, want 10", lm.ConnsCurrent)
	}
	// Unknown metrics should not cause panics or be stored
	if len(lm.UserMetrics) != 0 {
		t.Errorf("UserMetrics should be empty, got %d entries", len(lm.UserMetrics))
	}
}
