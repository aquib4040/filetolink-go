package bot

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type ResourceStats struct {
	AppRSSBytes    int64
	DynoLimitBytes int64
	RAMPercent     float64
	HeapAllocBytes uint64
	HeapSysBytes   uint64
	CPUPercent     float64
}

type CPUTracker struct {
	mu           sync.RWMutex
	lastTotalCPU float64
	lastSample   time.Time
	cpuPercent   float64
}

var cpuTrackerInstance = &CPUTracker{
	lastSample: time.Now(),
}

func init() {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err == nil {
		cpuTrackerInstance.lastTotalCPU = float64(usage.Utime.Sec) + float64(usage.Utime.Usec)/1e6 +
			float64(usage.Stime.Sec) + float64(usage.Stime.Usec)/1e6
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			cpuTrackerInstance.sample()
		}
	}()
}

func (t *CPUTracker) sample() {
	t.mu.Lock()
	defer t.mu.Unlock()

	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return
	}

	totalCPU := float64(usage.Utime.Sec) + float64(usage.Utime.Usec)/1e6 +
		float64(usage.Stime.Sec) + float64(usage.Stime.Usec)/1e6
	now := time.Now()
	dt := now.Sub(t.lastSample).Seconds()
	if dt > 0.5 {
		deltaCPU := totalCPU - t.lastTotalCPU
		pct := (deltaCPU / dt) * 100
		if pct < 0 {
			pct = 0
		}
		t.cpuPercent = pct
		t.lastTotalCPU = totalCPU
		t.lastSample = now
	}
}

func (t *CPUTracker) GetCPUPercent() float64 {
	t.sample()
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.cpuPercent
}

// getContainerMemoryLimit calculates the memory limit for this dyno/container.
// It avoids reading the shared host machine's physical memory (e.g. 64 GB) by checking
// container cgroups or defaulting to the Heroku 2X dyno quota (1024 MB).
func getContainerMemoryLimit() int64 {
	if v := os.Getenv("MEMORY_LIMIT_MB"); v != "" {
		if mb, err := strconv.ParseInt(v, 10, 64); err == nil && mb > 0 {
			return mb * 1024 * 1024
		}
	}
	if v := os.Getenv("DYNO_RAM_MB"); v != "" {
		if mb, err := strconv.ParseInt(v, 10, 64); err == nil && mb > 0 {
			return mb * 1024 * 1024
		}
	}

	// Check cgroup v2
	if data, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		s := strings.TrimSpace(string(data))
		if s != "max" && s != "" {
			if val, err := strconv.ParseInt(s, 10, 64); err == nil && val > 0 && val < (32*1024*1024*1024) {
				return val
			}
		}
	}

	// Check cgroup v1
	if data, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
		s := strings.TrimSpace(string(data))
		if val, err := strconv.ParseInt(s, 10, 64); err == nil && val > 0 && val < (32*1024*1024*1024) {
			return val
		}
	}

	// Default to Heroku 2X dyno quota (1024 MB)
	return 1024 * 1024 * 1024
}

// getProcessRSS retrieves the exact resident set size (physical RAM) consumed by this app.
func getProcessRSS() int64 {
	data, err := os.ReadFile("/proc/self/statm")
	if err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 2 {
			if pages, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				return pages * int64(os.Getpagesize())
			}
		}
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int64(m.Sys)
}

// GetAppResourceStats returns the app's real container memory and CPU metrics.
func GetAppResourceStats() ResourceStats {
	limit := getContainerMemoryLimit()
	rss := getProcessRSS()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	ramPct := 0.0
	if limit > 0 {
		ramPct = (float64(rss) / float64(limit)) * 100
	}

	return ResourceStats{
		AppRSSBytes:    rss,
		DynoLimitBytes: limit,
		RAMPercent:     ramPct,
		HeapAllocBytes: m.Alloc,
		HeapSysBytes:   m.Sys,
		CPUPercent:     cpuTrackerInstance.GetCPUPercent(),
	}
}
