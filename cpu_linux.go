package limits

import (
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"strconv"
)

func cpuCgroup(sys fs.FS) (quota, period int64, err error) {
	return cgroupLookup(sys, cgroupv2CPU, cgroupv1CPU, "cpu")
}

func cgroupv1CPU(sys fs.FS, ctls, path string) (quota, period int64, err error) {
	prefix := filepath.Join("sys/fs/cgroup", ctls, path)
	// Check for the existence of the named cgroup. If it doesn't exist,
	// look at the root of the controller. The named group not existing
	// probably means the process is in a container and is having remounting
	// tricks done. If, for some reason this is actually the root cgroup,
	// it'll be unlimited and fall back to the default.
	if _, err := fs.Stat(sys, prefix); errors.Is(err, fs.ErrNotExist) {
		prefix = filepath.Join("sys/fs/cgroup", ctls)
	}

	b, err := fs.ReadFile(sys, filepath.Join(prefix, "cpu.cfs_quota_us"))
	if err != nil {
		return 0, 0, err
	}
	quota, err = strconv.ParseInt(string(bytes.TrimSpace(b)), 10, 64)
	if err != nil {
		return 0, 0, err
	}
	if quota == -1 {
		return 0, 0, errNoQuota
	}
	b, err = fs.ReadFile(sys, filepath.Join(prefix, "cpu.cfs_period_us"))
	if err != nil {
		return 0, 0, err
	}
	period, err = strconv.ParseInt(string(bytes.TrimSpace(b)), 10, 64)
	if err != nil {
		return 0, 0, err
	}
	return quota, period, nil
}

func cgroupv2CPU(sys fs.FS, path string) (quota, period int64, err error) {
	n := filepath.Join("sys/fs/cgroup", path, "cpu.max")
	b, err := fs.ReadFile(sys, n)
	if err != nil {
		return 0, 0, err
	}
	l := bytes.Fields(b)
	qt, per := string(l[0]), string(l[1])
	if qt == "max" {
		return 0, 0, errNoQuota
	}
	quota, err = strconv.ParseInt(qt, 10, 64)
	if err != nil {
		return 0, 0, err
	}
	period, err = strconv.ParseInt(per, 10, 64)
	if err != nil {
		return 0, 0, err
	}
	return quota, period, nil
}
