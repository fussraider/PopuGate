package model

// SystemResources holds current system resource usage.
type SystemResources struct {
	CPUUsage    float64 `json:"cpu_usage"`    // percent (0-100)
	MemoryUsed  uint64  `json:"memory_used"`  // bytes
	MemoryTotal uint64  `json:"memory_total"` // bytes
	DiskUsed    uint64  `json:"disk_used"`    // bytes
	DiskTotal   uint64  `json:"disk_total"`   // bytes
	Load1       float64 `json:"load1"`
	Load5       float64 `json:"load5"`
	Load15      float64 `json:"load15"`
	Uptime      uint64  `json:"uptime"` // seconds
}
