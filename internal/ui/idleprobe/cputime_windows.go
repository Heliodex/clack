//go:build windows

package idleprobe

import (
	"time"

	"golang.org/x/sys/windows"
)

// ProcessCPU is the CPU time the whole process has used (user and
// kernel, every thread).
func ProcessCPU() time.Duration {
	var c, e, k, u windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &c, &e, &k, &u); err != nil {
		return 0
	}
	// FILETIME counts 100 ns intervals
	ft := func(f windows.Filetime) int64 { return int64(f.HighDateTime)<<32 | int64(f.LowDateTime) }
	return time.Duration((ft(k) + ft(u)) * 100)
}
