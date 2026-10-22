package memlimit

import (
	"math"
	"runtime/debug"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// keepRuntimeLimit restores the process's memory limit after a test changes it.
func keepRuntimeLimit(t *testing.T) {
	t.Helper()
	previous := debug.SetMemoryLimit(-1)
	t.Cleanup(func() { debug.SetMemoryLimit(previous) })
}

func noEnv(string) string { return "" }

func TestConfigure_SetsTheRuntimeLimitBelowTheContainerLimit(t *testing.T) {
	const podLimit = 512 << 20
	for name, cgroup := range map[string]fstest.MapFS{
		"cgroup v2": {"memory.max": {Data: []byte("536870912\n")}},
		"cgroup v1": {"memory/memory.limit_in_bytes": {Data: []byte("536870912\n")}},
	} {
		t.Run(name, func(t *testing.T) {
			keepRuntimeLimit(t)
			debug.SetMemoryLimit(math.MaxInt64)

			limit, source := Configure(cgroup, noEnv)

			require.Equal(t, FromCgroup, source)
			require.Equal(t, int64(429496729), limit, "80% of 512 MiB")
			require.Equal(t, limit, debug.SetMemoryLimit(-1), "the runtime collects before this limit")
			// The image budgets assume about 400 MiB for a 512 Mi pod.
			require.Less(t, limit, int64(podLimit))
			require.Greater(t, limit, int64(400<<20))
		})
	}
}

func TestConfigure_LeavesTheRuntimeUnlimitedWithoutAContainerLimit(t *testing.T) {
	for name, cgroup := range map[string]fstest.MapFS{
		"no cgroup":         {},
		"cgroup v2 max":     {"memory.max": {Data: []byte("max\n")}},
		"cgroup v1 maximum": {"memory/memory.limit_in_bytes": {Data: []byte("9223372036854771712\n")}},
		"unreadable value":  {"memory.max": {Data: []byte("lots\n")}},
	} {
		t.Run(name, func(t *testing.T) {
			keepRuntimeLimit(t)
			debug.SetMemoryLimit(math.MaxInt64)

			limit, source := Configure(cgroup, noEnv)

			require.Equal(t, None, source)
			require.Equal(t, int64(math.MaxInt64), limit)
			require.Equal(t, int64(math.MaxInt64), debug.SetMemoryLimit(-1))
		})
	}
}

func TestConfigure_LeavesAGOMEMLIMITInForce(t *testing.T) {
	keepRuntimeLimit(t)
	// The runtime has already applied GOMEMLIMIT when the process started.
	debug.SetMemoryLimit(300 << 20)
	cgroup := fstest.MapFS{"memory.max": {Data: []byte("536870912\n")}}

	limit, source := Configure(cgroup, func(name string) string {
		if name == "GOMEMLIMIT" {
			return "300MiB"
		}
		return ""
	})

	require.Equal(t, FromEnv, source)
	require.Equal(t, int64(300<<20), limit)
	require.Equal(t, int64(300<<20), debug.SetMemoryLimit(-1))
}
