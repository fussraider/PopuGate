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
	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err != nil {
		return
	}
	r.MemoryTotal = info.Totalram * uint64(info.Unit)
	r.MemoryUsed = (info.Totalram - info.Freeram) * uint64(info.Unit)
	r.Uptime = uint64(info.Uptime)
	// Sysinfo loads are scaled by 65536
	r.Load1 = float64(info.Loads[0]) / 65536.0
	r.Load5 = float64(info.Loads[1]) / 65536.0
	r.Load15 = float64(info.Loads[2]) / 65536.0
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
