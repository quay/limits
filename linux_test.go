//go:build linux
// +build linux

package limits

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"
)

type cgTestcase struct {
	Func func(fs.FS) (int64, int64, error)
	In   fstest.MapFS
	Err  error
	Name string
	Want [2]int64
}

func (tc cgTestcase) Run(t *testing.T) {
	t.Run(tc.Name, func(t *testing.T) {
		a, b, err := tc.Func(tc.In)
		if err != tc.Err {
			t.Error(err)
		}
		if got, want := [2]int64{a, b}, tc.Want; tc.Err == nil && got != want {
			t.Errorf("got: %v, want: %v", got, want)
		}
	})
}

const cgmap = `11:pids:/user.slice/user-1000.slice/session-4.scope
10:cpuset:/
9:blkio:/user.slice
8:hugetlb:/
7:perf_event:/
6:devices:/user.slice
5:net_cls,net_prio:/
4:cpu,cpuacct:/user.slice
3:freezer:/
2:memory:/user.slice/user-1000.slice/session-4.scope
1:name=systemd:/user.slice/user-1000.slice/session-4.scope
0::/user.slice/user-1000.slice/session-4.scope
`

func TestCPU(t *testing.T) {
	t.Run("V1", func(t *testing.T) {
		tt := []cgTestcase{
			{
				Name: "NoLimit",
				Func: cpuCgroup,
				In: fstest.MapFS{
					"proc/self/cgroup": &fstest.MapFile{Data: []byte(cgmap)},
					"sys/fs/cgroup/cpu,cpuacct/user.slice/cpu.cfs_quota_us": &fstest.MapFile{
						Data: []byte("-1\n"),
					},
				},
				Want: [2]int64{-1, -1},
			},
			{
				Name: "Limit1",
				Func: cpuCgroup,
				In: fstest.MapFS{
					"proc/self/cgroup": &fstest.MapFile{Data: []byte(cgmap)},
					"sys/fs/cgroup/cpu,cpuacct/user.slice/cpu.cfs_quota_us": &fstest.MapFile{
						Data: []byte("100000\n"),
					},
					"sys/fs/cgroup/cpu,cpuacct/user.slice/cpu.cfs_period_us": &fstest.MapFile{
						Data: []byte("100000\n"),
					},
				},
				Want: [2]int64{100_000, 100_000},
			},
			{
				Name: "RootFallback",
				Func: cpuCgroup,
				In: fstest.MapFS{
					"proc/self/cgroup": &fstest.MapFile{Data: []byte(cgmap)},
					"sys/fs/cgroup/cpu,cpuacct/cpu.cfs_quota_us": &fstest.MapFile{
						Data: []byte("100000\n"),
					},
					"sys/fs/cgroup/cpu,cpuacct/cpu.cfs_period_us": &fstest.MapFile{
						Data: []byte("100000\n"),
					},
				},
				Want: [2]int64{100_000, 100_000},
			},
		}
		for _, tc := range tt {
			tc.Run(t)
		}
	})
	t.Run("V2", func(t *testing.T) {
		tt := []cgTestcase{
			{
				Name: "NoLimit",
				Func: cpuCgroup,
				In: fstest.MapFS{
					"proc/self/cgroup": &fstest.MapFile{
						Data: []byte("0::/\n"),
					},
					"sys/fs/cgroup/cpu.max": &fstest.MapFile{
						Data: []byte("max 100000\n"),
					},
				},
				Want: [2]int64{-1, -1},
			},
			{
				Name: "Limit4",
				Func: cpuCgroup,
				In: fstest.MapFS{
					"proc/self/cgroup": &fstest.MapFile{
						Data: []byte("0::/\n"),
					},
					"sys/fs/cgroup/cpu.max": &fstest.MapFile{
						Data: []byte("400000 100000\n"),
					},
				},
				Want: [2]int64{400_000, 100_000},
			},
		}
		for _, tc := range tt {
			tc.Run(t)
		}
	})
}

func TestMemory(t *testing.T) {
	t.Run("V1", func(t *testing.T) {
		tt := []cgTestcase{
			{
				Name: "NoLimit",
				Func: memoryCgroup,
				In: fstest.MapFS{
					"proc/self/cgroup": &fstest.MapFile{Data: []byte(cgmap)},
					"sys/fs/cgroup/memory/user.slice/user-1000.slice/session-4.scope/memory.soft_limit_in_bytes": &fstest.MapFile{
						Data: []byte("max\n"),
					},
					"sys/fs/cgroup/memory/user.slice/user-1000.slice/session-4.scope/memory.limit_in_bytes": &fstest.MapFile{
						Data: []byte("max\n"),
					},
				},
				Want: [2]int64{-1, -1},
			},
			{
				Name: "512MiB",
				Func: memoryCgroup,
				In: fstest.MapFS{
					"proc/self/cgroup": &fstest.MapFile{Data: []byte(cgmap)},
					"sys/fs/cgroup/memory/user.slice/user-1000.slice/session-4.scope/memory.soft_limit_in_bytes": &fstest.MapFile{
						Data: []byte("max\n"),
					},
					"sys/fs/cgroup/memory/user.slice/user-1000.slice/session-4.scope/memory.limit_in_bytes": &fstest.MapFile{
						Data: []byte("536870912\n"),
					},
				},
				Want: [2]int64{-1, 536_870_912},
			},
			{
				Name: "RootFallback",
				Func: memoryCgroup,
				In: fstest.MapFS{
					"proc/self/cgroup": &fstest.MapFile{Data: []byte(cgmap)},
					"sys/fs/cgroup/memory/memory.soft_limit_in_bytes": &fstest.MapFile{
						Data: []byte("max\n"),
					},
					"sys/fs/cgroup/memory/memory.limit_in_bytes": &fstest.MapFile{
						Data: []byte("536870912\n"),
					},
				},
				Want: [2]int64{-1, 536_870_912},
			},
		}
		for _, tc := range tt {
			tc.Run(t)
		}
	})
	t.Run("V2", func(t *testing.T) {
		tt := []cgTestcase{
			{
				Name: "NoLimit",
				Func: memoryCgroup,
				In: fstest.MapFS{
					"proc/self/cgroup":          &fstest.MapFile{Data: []byte("0::/\n")},
					"sys/fs/cgroup/memory.high": &fstest.MapFile{Data: []byte("max\n")},
					"sys/fs/cgroup/memory.max":  &fstest.MapFile{Data: []byte("max\n")},
				},
				Want: [2]int64{-1, -1},
			},
			{
				Name: "512MiB",
				Func: memoryCgroup,
				In: fstest.MapFS{
					"proc/self/cgroup":          &fstest.MapFile{Data: []byte("0::/\n")},
					"sys/fs/cgroup/memory.high": &fstest.MapFile{Data: []byte("max\n")},
					"sys/fs/cgroup/memory.max":  &fstest.MapFile{Data: []byte("536870912\n")},
				},
				Want: [2]int64{-1, 536_870_912},
			},
		}
		for _, tc := range tt {
			tc.Run(t)
		}
	})
}

func TestMemoryPSI(t *testing.T) {
	t.Skip("TODO: write some deterministic tests")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ev, err := memoryPSI(ctx, 150*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		var bs [][]byte
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			_ = bs
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			bs = append(bs, make([]byte, 4*1024*1024)) // allocate 4MiB chunks for a while
		}
	}()
	select {
	case <-ev:
		t.Logf("event fired")
	case <-ctx.Done():
		t.Error(ctx.Err())
	}
}
