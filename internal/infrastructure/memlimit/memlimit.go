// Package memlimit gives the Go runtime a soft memory limit below the
// container's, so the garbage collector frees memory before the kernel kills
// the process.
//
// Without a limit the collector lets the heap grow to about twice the memory
// still in use before it runs, so a service whose live memory is bounded well
// under its container limit can still pass that limit with garbage it has not
// yet collected.
package memlimit

import (
	"errors"
	"io/fs"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

// Fraction is the share of the container's memory limit given to the Go
// runtime. The rest is left for memory the runtime does not account for
// (mapped binary, kernel buffers) and for the time the collector needs to
// catch up when live memory approaches the limit.
const Fraction = 0.8

// CgroupRoot is where the container's cgroup filesystem is mounted.
const CgroupRoot = "/sys/fs/cgroup"

// Source says where the limit in force came from.
type Source string

const (
	// FromEnv is a limit set by the GOMEMLIMIT environment variable, which the
	// runtime applies itself and which always wins.
	FromEnv Source = "GOMEMLIMIT"
	// FromCgroup is a limit derived from the container's memory limit.
	FromCgroup Source = "cgroup"
	// None means no limit was found, and the runtime keeps having none.
	None Source = "none"
)

// unlimitedV1 is the smallest value cgroup v1 reports for "no limit"; it
// writes the largest page-aligned int64 rather than a word.
const unlimitedV1 = int64(1) << 62

// Configure sets the runtime's soft memory limit to Fraction of the container
// memory limit found in cgroup, unless GOMEMLIMIT already sets one. It returns
// the limit in force (math.MaxInt64 when there is none) and its source.
func Configure(cgroup fs.FS, getenv func(string) string) (int64, Source) {
	if strings.TrimSpace(getenv("GOMEMLIMIT")) != "" {
		return debug.SetMemoryLimit(-1), FromEnv
	}
	container, ok := containerLimit(cgroup)
	if !ok {
		return debug.SetMemoryLimit(-1), None
	}
	limit := int64(float64(container) * Fraction)
	debug.SetMemoryLimit(limit)
	return limit, FromCgroup
}

// ConfigureFromHost applies Configure to this process's own cgroup and
// environment.
func ConfigureFromHost() (int64, Source) {
	return Configure(os.DirFS(CgroupRoot), os.Getenv)
}

// containerLimit reads the memory limit of a cgroup v2 or v1 hierarchy. A
// missing file, an unparsable value or an unlimited one reports no limit.
func containerLimit(cgroup fs.FS) (int64, bool) {
	for _, name := range []string{"memory.max", "memory/memory.limit_in_bytes"} {
		raw, err := fs.ReadFile(cgroup, name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return 0, false
		}
		value := strings.TrimSpace(string(raw))
		if value == "max" {
			return 0, false
		}
		limit, err := strconv.ParseInt(value, 10, 64)
		if err != nil || limit <= 0 || limit >= unlimitedV1 {
			return 0, false
		}
		return limit, true
	}
	return 0, false
}
