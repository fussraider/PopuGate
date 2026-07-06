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
	metricRe = regexp.MustCompile(`^(\w+)(?:\{([^}]*)\})?\s+([\d.eE+-]+|[+-]?Inf|NaN)`)
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

	scalarFields := map[string]*float64{
		"telemt_uptime_seconds":                 &lm.UptimeSeconds,
		"telemt_connections_current":            &lm.ConnsCurrent,
		"telemt_connections_total":              &lm.ConnsTotal,
		"telemt_connections_bad_total":          &lm.ConnsBadTotal,
		"telemt_upstream_connect_attempt_total": &lm.UpstreamAttemptTotal,
		"telemt_upstream_connect_success_total": &lm.UpstreamSuccessTotal,
		"telemt_upstream_connect_fail_total":    &lm.UpstreamFailTotal,
		"telemt_me_writers_active_current":      &lm.MEWritersActive,
		"telemt_me_writers_warm_current":        &lm.MEWritersWarm,
	}

	userFields := map[string]func(*model.UserLiveMetrics, float64){
		"telemt_user_octets_from_client":  func(u *model.UserLiveMetrics, v float64) { u.OctetsFromClient = v },
		"telemt_user_octets_to_client":    func(u *model.UserLiveMetrics, v float64) { u.OctetsToClient = v },
		"telemt_user_connections_current": func(u *model.UserLiveMetrics, v float64) { u.Connections = v },
		"telemt_user_unique_ips_current":  func(u *model.UserLiveMetrics, v float64) { u.UniqueIPs = v },
	}

	for _, m := range metrics {
		if ptr, ok := scalarFields[m.Name]; ok {
			*ptr = m.Value
			continue
		}
		if fn, ok := userFields[m.Name]; ok {
			if user, ok := m.Labels["user"]; ok {
				ensureUser(lm, user)
				fn(lm.UserMetrics[user], m.Value)
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
