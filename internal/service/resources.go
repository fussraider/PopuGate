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

// procStat holds parsed CPU usage fields.
// When direct is true, the usage percentage is already computed (e.g. on macOS
// via `top -l 2`) and stored in usagePct; the idle/total fields are unused.
type procStat struct {
	idle     uint64
	total    uint64
	direct   bool    // true when usagePct is already computed
	usagePct float64 // valid only when direct == true
}

func readCPUUsage() float64 {
	s1, err := parseProcStat()
	if err != nil {
		return 0
	}

	// When the platform (e.g. macOS) already provides a ready-made percentage,
	// return it directly without a second sample.
	if s1.direct {
		return s1.usagePct
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
