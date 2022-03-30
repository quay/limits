package limits

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func memoryPSI(ctx context.Context, threshold time.Duration) (<-chan struct{}, error) {
	if !v2.Enabled {
		return nil, errNeedV2
	}
	if v2.ReadOnly {
		// can't set a trigger, fall back to reading the average
		return memoryPSIAvg(ctx, threshold)
	}
	// 10 seconds is hard-coded so that "threshold" means the same thing if
	// looking at the average or the trigger.
	//
	// Also need to check this error and fall back because it's possible to have
	// writes denied via SELinux even if the cgroup2 fs is mounted rw.
	out, err := memoryPSIEpoll(ctx, threshold, 10*time.Second)
	switch {
	case errors.Is(err, nil):
	case errors.Is(err, os.ErrPermission):
		return memoryPSIAvg(ctx, threshold)
	default:
		return nil, err
	}
	return out, nil
}

func memoryPSIAvg(ctx context.Context, threshold time.Duration) (<-chan struct{}, error) {
	const (
		which = 0
		avg10 = 1
	)
	fn := filepath.Join(v2.Root, v2.Path, `memory.pressure`)
	trip := threshold.Seconds() / 10.0
	out := make(chan struct{}, 1)
	go func() {
		defer close(out)
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			b, err := os.ReadFile(fn)
			if err != nil {
				// ???
				continue
			}
			var avg float64
			for _, b := range bytes.SplitN(b, []byte("\n"), 2) {
				fs := strings.Fields(string(b))
				if fs[which] == "some" {
					avg, _ = strconv.ParseFloat(fs[avg10][6:], 64)
					break
				}
			}
			if avg >= trip {
				select {
				case out <- struct{}{}:
				default:
				}
			}
		}
	}()
	return out, nil
}

func memoryPSIEpoll(ctx context.Context, threshold, period time.Duration) (<-chan struct{}, error) {
	f, err := os.OpenFile(filepath.Join(v2.Root, v2.Path, `memory.pressure`), os.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	if _, err := fmt.Fprintf(f, "some %d %d\x00", threshold.Microseconds(), period.Microseconds()); err != nil {
		return nil, err
	}

	// Do the syscalls under a label so it's easier to see if they're going
	// wrong, in theory.
	var epollfd int
	pprof.Do(ctx, pprof.Labels("memory_psi", "epoll_create"), func(_ context.Context) {
		epollfd, err = syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
		if err != nil {
			return
		}
		fd := f.Fd()
		ev := syscall.EpollEvent{
			Fd:     int32(fd),
			Events: syscall.EPOLLPRI,
		}
		err = syscall.EpollCtl(epollfd, syscall.EPOLL_CTL_ADD, int(fd), &ev)
	})
	if err != nil {
		return nil, err
	}

	out := make(chan struct{}, 1)
	go pprof.Do(ctx, pprof.Labels("memory_psi", "epoll_wait"), func(ctx context.Context) {
		defer f.Close()
		defer close(out)
		// Epoll APIs should all be thread-safe, but this goroutine is going
		// to be spending most of its time in epoll_wait, so lock it down
		// anyway.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer syscall.Close(epollfd)
		events := make([]syscall.EpollEvent, 1) // I guess we only really care if it fires at all?
		for {
			n, err := syscall.EpollWait(epollfd, events, -1)
			if err != nil {
				// Can we get EINTR here? All the other error conditions
				// should be impossible.
				return
			}
			var fired, exit bool
			for _, ev := range events[:n] {
				fired = fired || (ev.Events&syscall.EPOLLPRI) != 0
				exit = exit || (ev.Events&syscall.EPOLLERR) != 0
			}
			switch {
			case fired && exit: // OK
				fallthrough
			case fired && !exit: // OK
				select {
				case <-ctx.Done():
					return
				case out <- struct{}{}:
				default:
					// Dropped our event.
				}
			case !fired && exit: // POLLERR
				return
			case !fired && !exit: // nothing happened
				select {
				case <-ctx.Done():
					return
				default:
					// Back to the epoll_wait.
				}
			}
		}
	})
	return out, nil
}
