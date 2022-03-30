// Package limits implements introspecting a process' environment for configured
// limits and quotas.
package limits

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"
)

// CPU reports the unit-less time quota the process has per period.
func CPU() (quota, period int64, err error) {
	switch runtime.GOOS {
	case `linux`:
		return cpuCgroup(os.DirFS("/"))
	default:
	}
	return -1, -1, nil
}

// Memory reports the "high" and "max" memory limits.
//
// These may also be known as the "soft" and "hard" limits.
func Memory() (high, max int64, err error) {
	switch runtime.GOOS {
	case `linux`:
		return memoryCgroup(os.DirFS("/"))
	default:
	}
	return -1, -1, nil
}

type EventKind uint

const (
	_ EventKind = iota
	MemoryEvents
)

func Events(ctx context.Context, kind EventKind, threshold time.Duration) (<-chan struct{}, error) {
	switch runtime.GOOS {
	case `linux`:
		switch kind {
		case MemoryEvents:
			return memoryPSI(ctx, threshold)
		default:
			return nil, fmt.Errorf("limits: unknown event kind: %v", kind)
		}
	default:
	}
	out := make(chan struct{})
	close(out)
	return out, nil
}
