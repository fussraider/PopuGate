//go:build !linux && !darwin

package service

import (
	"fmt"

	"github.com/fussraider/PopuGate/internal/model"
)

func readSysinfo(_ *model.SystemResources) {
	// Not available on non-Linux platforms
}

func readDiskUsage(_ *model.SystemResources, _ string) {
	// Not available on non-Linux platforms
}

func parseProcStat() (*procStat, error) {
	// /proc/stat not available on non-Linux platforms
	return nil, fmt.Errorf("not available")
}
