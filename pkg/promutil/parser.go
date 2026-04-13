package promutil

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/fussraider/PopuGate/internal/model"
)

var (
	metricRe = regexp.MustCompile(`^(\w+)(?:\{([^}]*)\})?\s+([\d.eE+-]+|NaN)`)
	labelRe  = regexp.MustCompile(`(\w+)="([^"]*)"`)
)

// ParsedMetric holds a single parsed Prometheus metric line.
type ParsedMetric struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// ParseMetrics parses Prometheus text format into structured metrics.
func ParseMetrics(text string) ([]ParsedMetric, error) {
	var metrics []ParsedMetric
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := metricRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		val, err := strconv.ParseFloat(m[3], 64)
		if err != nil {
			continue
		}
		labels := make(map[string]string)
		if m[2] != "" {
			for _, lm := range labelRe.FindAllStringSubmatch(m[2], -1) {
				labels[lm[1]] = lm[2]
			}
		}
		metrics = append(metrics, ParsedMetric{
			Name:   m[1],
			Labels: labels,
			Value:  val,
		})
	}
	return metrics, nil
}

// ExtractTelemtMetrics extracts all relevant telemt metrics from parsed data.
func ExtractTelemtMetrics(metrics []ParsedMetric) *model.LiveMetrics {
	lm := &model.LiveMetrics{
		UserMetrics: make(map[string]*model.UserLiveMetrics),
	}

	for _, m := range metrics {
		switch m.Name {
		case "telemt_uptime_seconds":
			lm.UptimeSeconds = m.Value
		case "telemt_connections_current":
			lm.ConnsCurrent = m.Value
		case "telemt_connections_total":
			lm.ConnsTotal = m.Value
		case "telemt_connections_bad_total":
			lm.ConnsBadTotal = m.Value
		case "telemt_connections_me_current":
			lm.ConnsMECurrent = m.Value
		case "telemt_connections_direct_current":
			lm.ConnsDirectCurrent = m.Value
		case "telemt_upstream_connect_attempt_total":
			lm.UpstreamAttemptTotal = m.Value
		case "telemt_upstream_connect_success_total":
			lm.UpstreamSuccessTotal = m.Value
		case "telemt_upstream_connect_fail_total":
			lm.UpstreamFailTotal = m.Value
		case "telemt_me_writers_active":
			lm.MEWritersActive = m.Value
		case "telemt_me_writers_warm":
			lm.MEWritersWarm = m.Value

		// Per-user metrics
		case "telemt_user_octets_from_client":
			if user, ok := m.Labels["user"]; ok {
				ensureUser(lm, user)
				lm.UserMetrics[user].OctetsFromClient = m.Value
			}
		case "telemt_user_octets_to_client":
			if user, ok := m.Labels["user"]; ok {
				ensureUser(lm, user)
				lm.UserMetrics[user].OctetsToClient = m.Value
			}
		case "telemt_user_connections_current":
			if user, ok := m.Labels["user"]; ok {
				ensureUser(lm, user)
				lm.UserMetrics[user].Connections = m.Value
			}
		case "telemt_user_unique_ips_current":
			if user, ok := m.Labels["user"]; ok {
				ensureUser(lm, user)
				lm.UserMetrics[user].UniqueIPs = m.Value
			}
		}
	}

	return lm
}

// FetchAndParse fetches metrics from an HTTP endpoint and parses them.
func FetchAndParse(body io.Reader) (*model.LiveMetrics, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read metrics: %w", err)
	}
	metrics, err := ParseMetrics(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse metrics: %w", err)
	}
	return ExtractTelemtMetrics(metrics), nil
}

func ensureUser(lm *model.LiveMetrics, user string) {
	if _, ok := lm.UserMetrics[user]; !ok {
		lm.UserMetrics[user] = &model.UserLiveMetrics{}
	}
}
