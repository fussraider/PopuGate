package model

// TrafficSnapshot represents a point-in-time traffic reading.
type TrafficSnapshot struct {
	BytesIn  int64 `json:"bytes_in"`
	BytesOut int64 `json:"bytes_out"`
	SnapIn   int64 `json:"snap_in"`
	SnapOut  int64 `json:"snap_out"`
}

// UserTraffic holds per-user traffic stats.
type UserTraffic struct {
	Label    string `json:"label"`
	BytesIn  int64  `json:"bytes_in"`
	BytesOut int64  `json:"bytes_out"`
}

// LiveMetrics holds parsed Prometheus metrics from the telemt engine.
type LiveMetrics struct {
	UptimeSeconds        float64                     `json:"uptime_seconds"`
	ConnsCurrent         float64                     `json:"conns_current"`
	ConnsTotal           float64                     `json:"conns_total"`
	ConnsBadTotal        float64                     `json:"conns_bad_total"`
	UpstreamAttemptTotal float64                     `json:"upstream_attempt_total"`
	UpstreamSuccessTotal float64                     `json:"upstream_success_total"`
	UpstreamFailTotal    float64                     `json:"upstream_fail_total"`
	MEWritersActive      float64                     `json:"me_writers_active"`
	MEWritersWarm        float64                     `json:"me_writers_warm"`
	UserMetrics          map[string]*UserLiveMetrics `json:"users"`
}

// UserLiveMetrics holds per-user live Prometheus metrics.
type UserLiveMetrics struct {
	OctetsFromClient float64 `json:"octets_from_client"`
	OctetsToClient   float64 `json:"octets_to_client"`
	Connections      float64 `json:"connections"`
	UniqueIPs        float64 `json:"unique_ips"`
}

// GlobalTraffic holds cumulative global traffic.
type GlobalTraffic struct {
	TotalIn  int64 `json:"total_in"`
	TotalOut int64 `json:"total_out"`
}

// TrafficReport combines global and per-user traffic.
type TrafficReport struct {
	Global GlobalTraffic `json:"global"`
	Users  []UserTraffic `json:"users"`
}

// TrafficHistoryRecord is a single timestamped traffic snapshot.
type TrafficHistoryRecord struct {
	Timestamp   int64 `json:"timestamp"`
	BytesIn     int64 `json:"bytes_in"`
	BytesOut    int64 `json:"bytes_out"`
	Connections int64 `json:"connections"`
}
