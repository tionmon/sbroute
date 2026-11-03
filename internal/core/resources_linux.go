package core

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func residentMemory() uint64 {
	b, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(b))
	if len(fields) < 2 {
		return 0
	}
	n, _ := strconv.ParseUint(fields[1], 10, 64)
	return n * uint64(os.Getpagesize())
}
func memoryLimit() int64 {
	var limit int64
	paths := []string{"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"}
	for _, controller := range memoryControllers() {
		paths = append(paths, controller.path)
	}
	for _, p := range paths {
		b, e := os.ReadFile(p)
		if e == nil {
			n, _ := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
			if n > 0 && n < (1<<60) && (limit == 0 || n < limit) {
				limit = n
			}
		}
	}
	b, _ := os.ReadFile("/proc/meminfo")
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			f := strings.Fields(line)
			if len(f) > 1 {
				n, _ := strconv.ParseInt(f[1], 10, 64)
				n *= 1024
				if limit == 0 || n < limit {
					limit = n
				}
			}
			break
		}
	}
	return limit
}

type memoryController struct {
	path string
	used string
}

func memoryControllers() []memoryController {
	b, _ := os.ReadFile("/proc/self/cgroup")
	result := []memoryController{}
	for _, line := range strings.Split(string(b), "\n") {
		p := strings.SplitN(line, ":", 3)
		if len(p) != 3 {
			continue
		}
		root, limit, used := "", "", ""
		if p[1] == "" {
			root = "/sys/fs/cgroup"
			limit = "memory.max"
			used = "memory.current"
		} else {
			for _, c := range strings.Split(p[1], ",") {
				if c == "memory" {
					root = "/sys/fs/cgroup/memory"
					limit = "memory.limit_in_bytes"
					used = "memory.usage_in_bytes"
				}
			}
		}
		if root == "" {
			continue
		}
		dir := filepath.Join(root, strings.TrimPrefix(filepath.Clean(p[2]), "/"))
		for strings.HasPrefix(dir, root) {
			result = append(result, memoryController{filepath.Join(dir, limit), filepath.Join(dir, used)})
			if dir == root {
				break
			}
			dir = filepath.Dir(dir)
		}
	}
	return result
}
func cgroupMemory() (used, limit uint64) {
	for _, c := range memoryControllers() {
		b, e := os.ReadFile(c.path)
		if e != nil {
			continue
		}
		n, _ := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
		if n == 0 || n >= 1<<60 {
			continue
		}
		if limit != 0 && n >= limit {
			continue
		}
		v, _ := os.ReadFile(c.used)
		u, _ := strconv.ParseUint(strings.TrimSpace(string(v)), 10, 64)
		limit = n
		used = u
	}
	return
}
func fileDescriptors() (count, max uint64) {
	f, e := os.Open("/proc/self/fd")
	if e == nil {
		defer f.Close()
		for {
			names, err := f.Readdirnames(256)
			count += uint64(len(names))
			if err != nil {
				break
			}
		}
	}
	var r unix.Rlimit
	if unix.Getrlimit(unix.RLIMIT_NOFILE, &r) == nil {
		max = r.Cur
	}
	return
}
func processCPUSeconds() float64 {
	var r unix.Rusage
	if unix.Getrusage(unix.RUSAGE_SELF, &r) != nil {
		return 0
	}
	return float64(r.Utime.Sec+r.Stime.Sec) + float64(r.Utime.Usec+r.Stime.Usec)/1e6
}
