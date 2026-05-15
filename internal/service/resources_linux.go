//go:build linux

package service

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/fussraider/PopuGate/internal/model"
)

func readSysinfo(r *model.SystemResources) {
	// Use /proc/meminfo for memory — MemAvailable correctly excludes reclaimable cache/buffers.
	readMeminfo(r)

	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err != nil {
		return
	}
	if r.MemoryTotal == 0 {
		r.MemoryTotal = info.Totalram * uint64(info.Unit)
	}
	if r.MemoryUsed == 0 {
		r.MemoryUsed = (info.Totalram - info.Freeram) * uint64(info.Unit)
	}
	r.Uptime = uint64(info.Uptime)
	// Sysinfo loads are scaled by 65536
	r.Load1 = float64(info.Loads[0]) / 65536.0
	r.Load5 = float64(info.Loads[1]) / 65536.0
	r.Load15 = float64(info.Loads[2]) / 65536.0
}

func readMeminfo(r *model.SystemResources) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return
	}
	var total, available uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			total = val * 1024 // kB to bytes
		case "MemAvailable:":
			available = val * 1024
		}
	}
	if total > 0 {
		r.MemoryTotal = total
		r.MemoryUsed = total - available
	}
}

func readDiskUsage(r *model.SystemResources, path string) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return
	}
	r.DiskTotal = stat.Blocks * uint64(stat.Bsize)
	r.DiskUsed = (stat.Blocks - stat.Bfree) * uint64(stat.Bsize)
}

func parseProcStat() (*procStat, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return nil, err
	}
	line := strings.Split(string(data), "\n")[0]
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return nil, fmt.Errorf("unexpected /proc/stat format")
	}
	var total, idle uint64
	for i := 1; i < len(fields); i++ {
		v, _ := strconv.ParseUint(fields[i], 10, 64)
		total += v
		if i == 4 { // idle is the 4th value (index 4)
			idle = v
		}
	}
	return &procStat{idle: idle, total: total}, nil
}
