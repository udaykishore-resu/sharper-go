// Package memlimit is Technique 79: give the GC the container's
// memory limit as a soft target, with headroom for memory the Go
// runtime does not count (cgo, mmap'd files, kernel buffers).
package memlimit

import (
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// CgroupMemoryMax is the cgroup v2 limit file inside a container.
const CgroupMemoryMax = "/sys/fs/cgroup/memory.max"

// FromCgroup reads a cgroup v2 memory.max file and returns ratio
// of it. ok is false when there is no limit ("max") or no file.
func FromCgroup(path string, ratio float64) (limit int64, ok bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	s := strings.TrimSpace(string(b))
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 { // "max" means unlimited
		return 0, false
	}
	return int64(float64(n) * ratio), true
}

// Apply sets the soft limit to 90% of memory.max at startup. An
// explicit GOMEMLIMIT wins: the runtime already read it. It
// returns the limit in effect, for the startup log.
func Apply(path string) int64 {
	if _, set := os.LookupEnv("GOMEMLIMIT"); !set {
		if lim, ok := FromCgroup(path, 0.9); ok {
			debug.SetMemoryLimit(lim)
		}
	}
	return debug.SetMemoryLimit(-1) // read without changing
}
