// Package limits implements introspecting a process' environment for configured
// limits and quotas.
package limits

import (
	"os"
	"runtime"
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
