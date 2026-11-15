//go:build unix

package idleprobe

import (
	"syscall"
	"time"
)

// ProcessCPU is the CPU time the whole process has used (user and
// system, every thread).
func ProcessCPU() time.Duration {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}
