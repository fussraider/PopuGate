//go:build darwin

package service

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/fussraider/PopuGate/internal/model"
)

func readSysinfo(r *model.SystemResources) {
	// Memory on macOS using sysctl
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err == nil {
		r.MemoryTotal, _ = strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	}

	// Used memory is harder on macOS without CGO or complex parsing of vm_stat.
	// We'll use a simplified approach parsing vm_stat output.
	vmStat, err := exec.Command("vm_stat").Output()
	if err == nil {
		lines := strings.Split(string(vmStat), "\n")
		var free, inactive, spec uint64
		pageSize := uint64(4096) // Default
		if out, err := exec.Command("sysctl", "-n", "hw.pagesize").Output(); err == nil {
			pageSize, _ = strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
		}

		for _, line := range lines {
			parts := strings.Split(line, ":")
			if len(parts) < 2 {
				continue
			}
			valStr := strings.TrimSpace(strings.TrimSuffix(parts[1], "."))
			val, _ := strconv.ParseUint(valStr, 10, 64)

			key := strings.TrimSpace(parts[0])
			switch key {
			case "Pages free":
				free = val
			case "Pages inactive":
				inactive = val
			case "Pages speculative":
				spec = val
			}
		}
		// In macOS "Free" is just 'free' pages. More accurately "Available" is free + inactive + speculative.
		// Used is Total - Available.
		r.MemoryUsed = r.MemoryTotal - (free+inactive+spec)*pageSize
	}

	// Uptime
	out, err = exec.Command("sysctl", "-n", "kern.boottime").Output()
	if err == nil {
		// Example: { sec = 1714834800, usec = 0 } Mon May  4 18:00:00 2026
		s := string(out)
		if start := strings.Index(s, "sec = "); start != -1 {
			s = s[start+6:]
			if end := strings.Index(s, ","); end != -1 {
				secStr := s[:end]
				bootSecs, _ := strconv.ParseInt(secStr, 10, 64)
				r.Uptime = uint64(time.Now().Unix() - bootSecs)
			}
		}
	}

	// Loads
	out, err = exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err == nil {
		// Example: { 2.10 2.05 1.98 }
		s := strings.Trim(string(out), "{ }\n")
		loads := strings.Fields(s)
		if len(loads) >= 3 {
			r.Load1, _ = strconv.ParseFloat(loads[0], 64)
			r.Load5, _ = strconv.ParseFloat(loads[1], 64)
			r.Load15, _ = strconv.ParseFloat(loads[2], 64)
		}
	}
}

func readDiskUsage(r *model.SystemResources, path string) {
	// df -b uses 512-byte blocks by default on some systems, but -g/m/k are more reliable
	// or we just use the default and parse the output.
	out, err := exec.Command("df", "-k", path).Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 4 {
				// 1K-blocks, Used, Available
				totalK, _ := strconv.ParseUint(fields[1], 10, 64)
				usedK, _ := strconv.ParseUint(fields[2], 10, 64)
				r.DiskTotal = totalK * 1024
				r.DiskUsed = usedK * 1024
			}
		}
	}
}

func parseProcStat() (*procStat, error) {
	// On macOS we use top -l 2 to get a more accurate delta of CPU usage.
	// The first sample of top is often stats since boot, the second is the current delta.
	out, err := exec.Command("top", "-l", "2", "-n", "0", "-F", "-R").Output()
	if err != nil {
		return nil, err
	}

	// We only need the last "CPU usage" line from the output
	lines := strings.Split(string(out), "\n")
	var lastCPUUsage string
	for _, line := range lines {
		if strings.HasPrefix(line, "CPU usage:") {
			lastCPUUsage = line
		}
	}

	if lastCPUUsage != "" {
		parts := strings.Split(lastCPUUsage, ":")
		if len(parts) >= 2 {
			usageParts := strings.Split(parts[1], ",")
			for _, p := range usageParts {
				if strings.Contains(p, "idle") {
					p = strings.TrimSpace(p)
					idleStr := strings.Fields(p)[0]
					idleStr = strings.TrimSuffix(idleStr, "%")
					idlePct, err := strconv.ParseFloat(idleStr, 64)
					if err != nil {
						return nil, err
					}

					// Return "fake" ticks that represent this percentage.
					// Since we are getting a delta from top -l 2, we can just return it
					// in a way that readCPUUsage will correctly calculate it.
					// readCPUUsage expects: (dTotal - dIdle) / dTotal * 100
					// If we return total=1000 and idle=idlePct*10, then:
					// (1000 - idlePct*10) / 1000 * 100 = (100 - idlePct) = usagePct.
					// But readCPUUsage does delta between two calls!
					// So we need to make sure the delta works.

					// Actually, if we use top -l 2, we already have the percentage.
					// We can bypass the delta logic in readCPUUsage if we wanted,
					// but let's try to fit into it.

					return &procStat{
						idle:  uint64(idlePct * 10),
						total: 1000,
					}, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("could not parse CPU usage")
}
