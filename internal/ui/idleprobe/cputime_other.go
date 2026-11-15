//go:build !unix && !windows

package idleprobe

import "time"

// ProcessCPU is unknown here.
func ProcessCPU() time.Duration { return 0 }
