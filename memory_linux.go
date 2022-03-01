package limits

import (
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"strconv"
)

func memoryCgroup(sys fs.FS) (high, max int64, err error) {
	return cgroupLookup(sys, cgroupv2Memory, cgroupv1Memory, "memory")
}

func cgroupv1Memory(sys fs.FS, ctls, path string) (high, max int64, err error) {
	prefix := filepath.Join("sys/fs/cgroup", ctls, path)
	if _, err := fs.Stat(sys, prefix); errors.Is(err, fs.ErrNotExist) {
		prefix = filepath.Join("sys/fs/cgroup", ctls)
	}
	high, max = -1, -1
	for _, f := range []struct {
		Val *int64
		Suf string
	}{
		{Suf: "soft_limit_in_bytes", Val: &high},
		{Suf: "limit_in_bytes", Val: &max},
	} {
		n := filepath.Join(prefix, "memory."+f.Suf)
		b, err := fs.ReadFile(sys, n)
		if err != nil {
			return 0, 0, err
		}
		if sb := string(bytes.TrimSpace(b)); sb != "max" {
			*f.Val, err = strconv.ParseInt(sb, 10, 64)
			if err != nil {
				return 0, 0, err
			}
		}
	}

	if high == -1 && max == -1 {
		return 0, 0, errNoQuota
	}
	return high, max, nil
}

func cgroupv2Memory(sys fs.FS, path string) (high, max int64, err error) {
	high, max = -1, -1
	for _, f := range []struct {
		Val *int64
		Suf string
	}{
		{Suf: "high", Val: &high},
		{Suf: "max", Val: &max},
	} {
		n := filepath.Join("sys/fs/cgroup", path, "memory."+f.Suf)
		b, err := fs.ReadFile(sys, n)
		if err != nil {
			return 0, 0, err
		}
		if sb := string(bytes.TrimSpace(b)); sb != "max" {
			*f.Val, err = strconv.ParseInt(sb, 10, 64)
			if err != nil {
				return 0, 0, err
			}
		}
	}

	if high == -1 && max == -1 {
		return 0, 0, errNoQuota
	}
	return high, max, nil
}
