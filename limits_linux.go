// Linux
//
// On linux systems, the limits are implemented by inspecting the process'
// cgroup. Both version 1 and version 2 are supported, version 2 being
// preferred. See also cgroups(7),
// https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v1/cgroups.html, and
// https://www.kernel.org/doc/html/latest/admin-guide/cgroup-v2.html for
// additional information.
//
// CPU v1
//
// The "quota" and "period" values are "cpu.cfs_quota_us" and
// "cpu.cfs_period_us".
//
// CPU v2
//
// The "quota" and "period" values are parsed out of the "cpu.max" file.
//
// Memory v1
//
// The "high" and "max" values are "memory.soft_limit_in_bytes" and
// "memory.limit_in_bytes".
//
// Memory v2
//
// The "high" and "max" values are "memory.high" and "memory.max".
package limits

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"strings"
)

var (
	errNoQuota = errors.New("no quota")
	errNeedV2  = errors.New("cgroupv2 needed")

	v2 struct {
		Root     string
		Path     string
		Enabled  bool
		ReadOnly bool
	}
)

func init() {
	// interrogate the running process at startup.
	b, err := os.ReadFile(`/proc/mounts`)
	if err != nil {
		return
	}
	s := bufio.NewScanner(bytes.NewReader(b))
	const (
		kind  = 0
		mount = 1
		opts  = 3
	)
	for s.Scan() {
		fs := strings.Fields(s.Text())
		if fs[kind] != "cgroup2" {
			continue
		}
		v2.Enabled = true
		if v2.Root != "" && strings.HasPrefix(v2.Root, "/sys/fs") {
			// If the cgroup2 fs is mounted multiple times, prefer the one in
			// /sys/fs, which is the "canonical" location.
			continue
		}
		v2.Root = fs[mount]
		for _, o := range strings.Split(fs[opts], ",") {
			if o == "ro" {
				v2.ReadOnly = true
				break
			}
		}
	}
	if !v2.Enabled {
		return
	}
	b, err = os.ReadFile(`/proc/self/cgroup`)
	if err != nil {
		return
	}
	v2.Path = string(b[3 : len(b)-1])
}

type (
	cgroupv1Func func(fs.FS, string, string) (int64, int64, error)
	cgroupv2Func func(fs.FS, string) (int64, int64, error)
)

func cgroupLookup(sys fs.FS, v2 cgroupv2Func, v1 cgroupv1Func, v1Name string) (a, b int64, err error) {
	cg, err := fs.ReadFile(sys, "proc/self/cgroup")
	if err != nil {
		return 0, 0, err
	}
	s := bufio.NewScanner(bytes.NewReader(cg))
	s.Split(bufio.ScanLines)
Lines:
	for s.Scan() {
		sl := bytes.SplitN(s.Bytes(), []byte(":"), 3)
		hid, ctls, pb := sl[0], sl[1], sl[2]
		switch {
		case bytes.Equal(hid, []byte("0")) &&
			len(ctls) == 0:
			a, b, err = v2(sys, string(pb))
		default:
			found := false
			for _, b := range bytes.Split(ctls, []byte(",")) {
				if bytes.Equal(b, []byte(v1Name)) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
			a, b, err = v1(sys, string(ctls), string(pb))
		}
		switch {
		case errors.Is(err, nil):
			break Lines
		case errors.Is(err, errNoQuota):
			return -1, -1, nil
		default:
			return 0, 0, err
		}
	}
	if err := s.Err(); err != nil {
		return 0, 0, err
	}
	return a, b, nil
}
