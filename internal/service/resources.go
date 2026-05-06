package service

import (
	"os"
	"path/filepath"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
)

// GetResources returns current system resource usage.
func GetResources() *model.SystemResources {
	r := &model.SystemResources{}
	readSysinfo(r)
	readDiskUsage(r, dataDir())
	r.CPUUsage = readCPUUsage()
	return r
}

func dataDir() string {
	if d := os.Getenv("POPUGATE_DATA_DIR"); d != "" {
		return d
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}

// procStat holds parsed /proc/stat CPU fields.
type procStat struct {
	idle  uint64
	total uint64
}

func readCPUUsage() float64 {
	s1, err := parseProcStat()
	if err != nil {
		return 0
	}

	// On macOS, parseProcStat might already return the current usage via top -l 2.
	// In that case, we don't need the second sample and delta.
	// We check if total is 1000, which is our "fake" indicator for macOS.
	if s1.total == 1000 {
		return float64(1000-s1.idle) / 10
	}

	time.Sleep(time.Second)
	s2, err := parseProcStat()
	if err != nil {
		return 0
	}
	dIdle := s2.idle - s1.idle
	dTotal := s2.total - s1.total
	if dTotal == 0 {
		return 0
	}
	return float64(dTotal-dIdle) / float64(dTotal) * 100
}
