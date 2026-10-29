//go:build !windows

package runcontainerd_test

import (
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"code.cloudfoundry.org/guardian/rundmc/runcontainerd"
	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
	cgroup1stats "github.com/containerd/cgroups/v3/cgroup1/stats"
	cgroup2stats "github.com/containerd/cgroups/v3/cgroup2/stats"
	"github.com/containerd/typeurl/v2"
)

// fakeMetricsGetter implements runcontainerd.ContainerMetricsGetter for tests.
type fakeMetricsGetter struct {
	metrics   typeurl.Any
	createdAt time.Time
	err       error
	calledID  string
}

func (f *fakeMetricsGetter) TaskMetrics(log lager.Logger, containerID string) (typeurl.Any, time.Time, error) {
	f.calledID = containerID
	return f.metrics, f.createdAt, f.err
}

// marshalAny wraps typeurl.MarshalAny with a test-fatal on error.
func marshalAny(t *testing.T, v interface{}) typeurl.Any {
	t.Helper()
	a, err := typeurl.MarshalAny(v)
	if err != nil {
		t.Fatalf("typeurl.MarshalAny: %v", err)
	}
	return a
}

// fixedCreatedAt is a stable past timestamp for deterministic Age assertions.
var fixedCreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// ---- V2 tests ----

func TestStats_V2_Populated(t *testing.T) {
	// Fully populated cgroup-v2 metrics with non-trivial values.
	// v2 CPU is in microseconds; Garden expects nanoseconds.
	v2 := &cgroup2stats.Metrics{
		CPU: &cgroup2stats.CPUStat{
			UsageUsec:  123,
			UserUsec:   100,
			SystemUsec: 23,
		},
		Memory: &cgroup2stats.MemoryStat{
			Anon:         200,
			File:         100,
			ActiveAnon:   80,
			InactiveAnon: 120,
			ActiveFile:   60,
			InactiveFile: 40,
			Unevictable:  10,
			FileMapped:   50,
			Pgfault:      5000,
			Pgmajfault:   3,
			SwapUsage:    15,
			UsageLimit:   1073741824, // 1GiB
		},
		Pids: &cgroup2stats.PidsStat{
			Current: 42,
			Limit:   1000,
		},
	}

	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v2),
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "test-container")
	if err != nil {
		t.Fatalf("Stats returned error: %v", err)
	}

	// Verify container ID was forwarded.
	if getter.calledID != "test-container" {
		t.Errorf("getter received container ID %q, want %q", getter.calledID, "test-container")
	}

	// CPU: microseconds → nanoseconds
	if got.CPU.Usage != 123000 {
		t.Errorf("CPU.Usage = %d, want 123000 (123µs → ns)", got.CPU.Usage)
	}
	if got.CPU.User != 100000 {
		t.Errorf("CPU.User = %d, want 100000", got.CPU.User)
	}
	if got.CPU.System != 23000 {
		t.Errorf("CPU.System = %d, want 23000", got.CPU.System)
	}

	// Memory: direct field mappings
	if got.Memory.Anon != 200 {
		t.Errorf("Memory.Anon = %d, want 200", got.Memory.Anon)
	}
	if got.Memory.File != 100 {
		t.Errorf("Memory.File = %d, want 100", got.Memory.File)
	}
	if got.Memory.Rss != 200 {
		t.Errorf("Memory.Rss = %d, want 200 (aliased from Anon)", got.Memory.Rss)
	}
	if got.Memory.Cache != 100 {
		t.Errorf("Memory.Cache = %d, want 100 (aliased from File)", got.Memory.Cache)
	}
	if got.Memory.ActiveAnon != 80 {
		t.Errorf("Memory.ActiveAnon = %d, want 80", got.Memory.ActiveAnon)
	}
	if got.Memory.InactiveFile != 40 {
		t.Errorf("Memory.InactiveFile = %d, want 40", got.Memory.InactiveFile)
	}
	if got.Memory.Unevictable != 10 {
		t.Errorf("Memory.Unevictable = %d, want 10", got.Memory.Unevictable)
	}
	if got.Memory.MappedFile != 50 {
		t.Errorf("Memory.MappedFile = %d, want 50 (from FileMapped)", got.Memory.MappedFile)
	}
	if got.Memory.Pgfault != 5000 {
		t.Errorf("Memory.Pgfault = %d, want 5000", got.Memory.Pgfault)
	}
	if got.Memory.Pgmajfault != 3 {
		t.Errorf("Memory.Pgmajfault = %d, want 3", got.Memory.Pgmajfault)
	}
	if got.Memory.Swap != 15 {
		t.Errorf("Memory.Swap = %d, want 15", got.Memory.Swap)
	}
	if got.Memory.HierarchicalMemoryLimit != 1073741824 {
		t.Errorf("Memory.HierarchicalMemoryLimit = %d, want 1073741824", got.Memory.HierarchicalMemoryLimit)
	}

	// TotalUsageTowardLimit = File + Anon + SwapCached(0) - InactiveFile
	// = 100 + 200 + 0 - 40 = 260
	if got.Memory.TotalUsageTowardLimit != 260 {
		t.Errorf("Memory.TotalUsageTowardLimit = %d, want 260 (100+200+0-40)", got.Memory.TotalUsageTowardLimit)
	}

	// Total* aliased fields
	if got.Memory.TotalRss != 200 {
		t.Errorf("Memory.TotalRss = %d, want 200", got.Memory.TotalRss)
	}
	if got.Memory.TotalCache != 100 {
		t.Errorf("Memory.TotalCache = %d, want 100", got.Memory.TotalCache)
	}
	if got.Memory.TotalSwap != 15 {
		t.Errorf("Memory.TotalSwap = %d, want 15", got.Memory.TotalSwap)
	}

	// PIDs
	if got.Pid.Current != 42 {
		t.Errorf("Pid.Current = %d, want 42", got.Pid.Current)
	}
	if got.Pid.Max != 1000 {
		t.Errorf("Pid.Max = %d, want 1000", got.Pid.Max)
	}

	// Age: should be positive since fixedCreatedAt is in the past
	if got.Age <= 0 {
		t.Errorf("Age = %v, want > 0 for a past creation time", got.Age)
	}
}

func TestStats_V2_InactiveFileLargerThanTotal(t *testing.T) {
	// When InactiveFile > File+Anon+SwapCached, TotalUsageTowardLimit
	// must be clamped to 0 to avoid unsigned underflow.
	v2 := &cgroup2stats.Metrics{
		Memory: &cgroup2stats.MemoryStat{
			File:         100,
			Anon:         200,
			InactiveFile: 301, // 301 > 100+200+0=300
		},
	}

	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v2),
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "underflow-test")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if got.Memory.TotalUsageTowardLimit != 0 {
		t.Errorf("TotalUsageTowardLimit = %d, want 0 (underflow guard)", got.Memory.TotalUsageTowardLimit)
	}
}

func TestStats_V2_InactiveFileEqualsTotal(t *testing.T) {
	// Equality boundary: 300 - 300 = 0, not underflow.
	v2 := &cgroup2stats.Metrics{
		Memory: &cgroup2stats.MemoryStat{
			File:         100,
			Anon:         200,
			InactiveFile: 300, // exactly equal
		},
	}

	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v2),
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "boundary-test")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if got.Memory.TotalUsageTowardLimit != 0 {
		t.Errorf("TotalUsageTowardLimit = %d, want 0 (exact boundary)", got.Memory.TotalUsageTowardLimit)
	}
}

func TestStats_V2_NilMemory(t *testing.T) {
	v2 := &cgroup2stats.Metrics{
		CPU: &cgroup2stats.CPUStat{UsageUsec: 50},
	}

	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v2),
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "no-mem")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if got.CPU.Usage != 50000 {
		t.Errorf("CPU.Usage = %d, want 50000", got.CPU.Usage)
	}
	// Memory fields should all be zero
	zeroMem := got.Memory
	if zeroMem.Anon != 0 || zeroMem.File != 0 || zeroMem.TotalUsageTowardLimit != 0 {
		t.Errorf("expected zero memory, got Anon=%d File=%d Total=%d",
			zeroMem.Anon, zeroMem.File, zeroMem.TotalUsageTowardLimit)
	}
}

func TestStats_V2_NilCPU(t *testing.T) {
	v2 := &cgroup2stats.Metrics{
		Memory: &cgroup2stats.MemoryStat{Anon: 100, File: 50},
	}

	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v2),
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "no-cpu")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if got.CPU.Usage != 0 || got.CPU.User != 0 || got.CPU.System != 0 {
		t.Errorf("expected zero CPU, got Usage=%d User=%d System=%d",
			got.CPU.Usage, got.CPU.User, got.CPU.System)
	}
	if got.Memory.Anon != 100 {
		t.Errorf("Memory.Anon = %d, want 100", got.Memory.Anon)
	}
}

func TestStats_V2_NilPids(t *testing.T) {
	v2 := &cgroup2stats.Metrics{
		CPU: &cgroup2stats.CPUStat{UsageUsec: 1},
	}

	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v2),
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "no-pids")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if got.Pid.Current != 0 || got.Pid.Max != 0 {
		t.Errorf("expected zero PIDs, got Current=%d Max=%d", got.Pid.Current, got.Pid.Max)
	}
}

func TestStats_V2_AllZero(t *testing.T) {
	// Valid all-zero counters must remain valid (not treated as error).
	v2 := &cgroup2stats.Metrics{
		CPU:    &cgroup2stats.CPUStat{},
		Memory: &cgroup2stats.MemoryStat{},
		Pids:   &cgroup2stats.PidsStat{},
	}

	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v2),
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "all-zero")
	if err != nil {
		t.Fatalf("all-zero v2 metrics should not error: %v", err)
	}
	if got.CPU.Usage != 0 || got.Memory.Anon != 0 || got.Pid.Current != 0 {
		t.Errorf("expected all zeros, got CPU.Usage=%d Anon=%d Pid.Current=%d",
			got.CPU.Usage, got.Memory.Anon, got.Pid.Current)
	}
}

// ---- V1 tests ----

func TestStats_V1_Populated(t *testing.T) {
	// Fully populated cgroup-v1 metrics. CPU is already in nanoseconds.
	v1 := &cgroup1stats.Metrics{
		CPU: &cgroup1stats.CPUStat{
			Usage: &cgroup1stats.CPUUsage{
				Total:  999000000, // nanoseconds, preserved as-is
				User:   800000000,
				Kernel: 199000000,
			},
		},
		Memory: &cgroup1stats.MemoryStat{
			Cache:                   500,
			RSS:                     300,
			MappedFile:              100,
			PgFault:                 2000,
			PgMajFault:              7,
			ActiveAnon:              150,
			InactiveAnon:            150,
			ActiveFile:              250,
			InactiveFile:            250,
			Unevictable:             20,
			HierarchicalMemoryLimit: 2147483648, // 2GiB
			TotalCache:              550,
			TotalRSS:                350,
			TotalMappedFile:         120,
			TotalPgFault:            2100,
			TotalPgMajFault:         8,
			TotalActiveAnon:         160,
			TotalInactiveAnon:       160,
			TotalActiveFile:         260,
			TotalInactiveFile:       260,
			TotalUnevictable:        25,
			Swap: &cgroup1stats.MemoryEntry{
				Usage: 42,
			},
			Usage: &cgroup1stats.MemoryEntry{
				Usage: 99999, // aggregate usage: the v1 fallback
			},
		},
		Pids: &cgroup1stats.PidsStat{
			Current: 8,
			Limit:   500,
		},
	}

	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v1),
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "v1-container")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	// CPU: nanoseconds preserved
	if got.CPU.Usage != 999000000 {
		t.Errorf("CPU.Usage = %d, want 999000000 (nanoseconds preserved)", got.CPU.Usage)
	}
	if got.CPU.User != 800000000 {
		t.Errorf("CPU.User = %d, want 800000000", got.CPU.User)
	}
	if got.CPU.System != 199000000 {
		t.Errorf("CPU.System = %d, want 199000000", got.CPU.System)
	}

	// Memory: direct mappings
	if got.Memory.Cache != 500 {
		t.Errorf("Memory.Cache = %d, want 500", got.Memory.Cache)
	}
	if got.Memory.Rss != 300 {
		t.Errorf("Memory.Rss = %d, want 300", got.Memory.Rss)
	}
	if got.Memory.HierarchicalMemoryLimit != 2147483648 {
		t.Errorf("Memory.HierarchicalMemoryLimit = %d, want 2147483648", got.Memory.HierarchicalMemoryLimit)
	}

	// v1 fallback: TotalUsageTowardLimit comes from Usage.Usage
	if got.Memory.TotalUsageTowardLimit != 99999 {
		t.Errorf("Memory.TotalUsageTowardLimit = %d, want 99999 (v1 aggregate fallback)", got.Memory.TotalUsageTowardLimit)
	}

	// v1 approximations: Anon=TotalRSS, File=TotalCache
	if got.Memory.Anon != 350 {
		t.Errorf("Memory.Anon = %d, want 350 (approximated from TotalRSS)", got.Memory.Anon)
	}
	if got.Memory.File != 550 {
		t.Errorf("Memory.File = %d, want 550 (approximated from TotalCache)", got.Memory.File)
	}

	// Swap from dedicated entry
	if got.Memory.Swap != 42 {
		t.Errorf("Memory.Swap = %d, want 42", got.Memory.Swap)
	}
	if got.Memory.TotalSwap != 42 {
		t.Errorf("Memory.TotalSwap = %d, want 42", got.Memory.TotalSwap)
	}

	// Total* fields
	if got.Memory.TotalRss != 350 {
		t.Errorf("Memory.TotalRss = %d, want 350", got.Memory.TotalRss)
	}
	if got.Memory.TotalCache != 550 {
		t.Errorf("Memory.TotalCache = %d, want 550", got.Memory.TotalCache)
	}
	if got.Memory.TotalActiveAnon != 160 {
		t.Errorf("Memory.TotalActiveAnon = %d, want 160", got.Memory.TotalActiveAnon)
	}

	// PIDs
	if got.Pid.Current != 8 {
		t.Errorf("Pid.Current = %d, want 8", got.Pid.Current)
	}
	if got.Pid.Max != 500 {
		t.Errorf("Pid.Max = %d, want 500", got.Pid.Max)
	}

	// Age
	if got.Age <= 0 {
		t.Errorf("Age = %v, want > 0", got.Age)
	}
}

func TestStats_V1_AggregateUsageFallback(t *testing.T) {
	// REGRESSION: v1 TotalUsageTowardLimit must use Usage.Usage even when
	// all detailed memory totals are zero. This catches the bug where
	// runsc shim returns v1 with zeroed detailed counters but populated
	// aggregate usage.
	v1 := &cgroup1stats.Metrics{
		Memory: &cgroup1stats.MemoryStat{
			// All detailed fields zero (as runsc sometimes does)
			Usage: &cgroup1stats.MemoryEntry{
				Usage: 87654321, // this MUST appear in TotalUsageTowardLimit
			},
		},
	}

	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v1),
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "aggregate-fallback")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if got.Memory.TotalUsageTowardLimit != 87654321 {
		t.Errorf("TotalUsageTowardLimit = %d, want 87654321 (aggregate usage fallback)", got.Memory.TotalUsageTowardLimit)
	}
	// Detailed fields should be zero
	if got.Memory.Cache != 0 || got.Memory.Rss != 0 {
		t.Errorf("expected zero detailed counters, got Cache=%d Rss=%d", got.Memory.Cache, got.Memory.Rss)
	}
}

func TestStats_V1_NilSwap(t *testing.T) {
	v1 := &cgroup1stats.Metrics{
		Memory: &cgroup1stats.MemoryStat{
			Cache: 100,
			RSS:   50,
			// Swap is nil
		},
	}

	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v1),
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "nil-swap")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if got.Memory.Swap != 0 {
		t.Errorf("Memory.Swap = %d, want 0 (nil swap entry)", got.Memory.Swap)
	}
}

func TestStats_V1_NilUsage(t *testing.T) {
	// When Usage entry is nil, TotalUsageTowardLimit stays 0.
	v1 := &cgroup1stats.Metrics{
		Memory: &cgroup1stats.MemoryStat{
			Cache: 100,
			// Usage is nil
		},
	}

	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v1),
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "nil-usage")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if got.Memory.TotalUsageTowardLimit != 0 {
		t.Errorf("TotalUsageTowardLimit = %d, want 0 (nil Usage entry)", got.Memory.TotalUsageTowardLimit)
	}
}

func TestStats_V1_NilCPUUsage(t *testing.T) {
	v1 := &cgroup1stats.Metrics{
		CPU: &cgroup1stats.CPUStat{
			// Usage is nil
		},
	}

	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v1),
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "nil-cpu-usage")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if got.CPU.Usage != 0 || got.CPU.User != 0 || got.CPU.System != 0 {
		t.Errorf("expected zero CPU with nil Usage, got %+v", got.CPU)
	}
}

func TestStats_V1_NilMemory(t *testing.T) {
	v1 := &cgroup1stats.Metrics{
		CPU: &cgroup1stats.CPUStat{
			Usage: &cgroup1stats.CPUUsage{Total: 1000},
		},
	}

	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v1),
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "nil-mem")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if got.CPU.Usage != 1000 {
		t.Errorf("CPU.Usage = %d, want 1000", got.CPU.Usage)
	}
	if got.Memory.TotalUsageTowardLimit != 0 {
		t.Errorf("TotalUsageTowardLimit = %d, want 0", got.Memory.TotalUsageTowardLimit)
	}
}

// ---- Error and unsupported type tests ----

func TestStats_TaskMetricsError(t *testing.T) {
	cause := errors.New("connection lost")
	getter := &fakeMetricsGetter{err: cause}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	_, err := statser.Stats(log, "error-test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, cause) {
		t.Errorf("error should wrap cause; got %q, want errors.Is to match %q", err, cause)
	}
}

func TestStats_UnsupportedType(t *testing.T) {
	// A typeurl.Any with an unknown type URL that cannot unmarshal as v1 or v2.
	// We use a raw anyType implementation to simulate this.
	getter := &fakeMetricsGetter{
		metrics: &unknownAny{
			typeURL: "io.containerd.cgroups.v99.Metrics",
			value:   []byte("garbage"),
		},
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	_, err := statser.Stats(log, "unknown-type")
	if err == nil {
		t.Fatal("expected error for unsupported type, got nil")
	}
	// The error message should mention "unsupported"
	if got := err.Error(); !containsSubstring(got, "unsupported") {
		t.Errorf("error = %q, want it to contain 'unsupported'", got)
	}
}

func TestStats_MalformedV2Bytes(t *testing.T) {
	// Valid v2 type URL but corrupt protobuf bytes: should fall through to v1,
	// which also fails, yielding "unsupported metrics type".
	getter := &fakeMetricsGetter{
		metrics: &unknownAny{
			typeURL: "io.containerd.cgroups.v2.Metrics",
			value:   []byte{0xff, 0xfe, 0xfd},
		},
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	_, err := statser.Stats(log, "malformed")
	if err == nil {
		t.Fatal("expected error for malformed bytes, got nil")
	}
	if !containsSubstring(err.Error(), "decode v2 metrics") {
		t.Errorf("error = %q, want supported-type decode diagnostic", err)
	}
}

func TestStats_ZeroCreatedAt(t *testing.T) {
	// Zero creation time: Age will be very large but should not error.
	v2 := &cgroup2stats.Metrics{
		CPU: &cgroup2stats.CPUStat{UsageUsec: 1},
	}

	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v2),
		createdAt: time.Time{}, // zero
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "zero-time")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	// Age from zero time should be very large (years)
	if got.Age < 24*time.Hour {
		t.Errorf("Age = %v, expected >> 24h for zero creation time", got.Age)
	}
}

func TestStats_NilAny(t *testing.T) {
	// Gate 1: nil Any payload without error must not panic; must return
	// an actionable error. Before the nil guard was added, this panicked
	// in typeurl.UnmarshalTo(nil, ...) → nil.GetTypeUrl().
	getter := &fakeMetricsGetter{
		metrics:   nil, // nil payload, no error
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	_, err := statser.Stats(log, "nil-payload")
	if err == nil {
		t.Fatal("expected error for nil Any payload, got nil")
	}
	if got := err.Error(); !containsSubstring(got, "nil") {
		t.Errorf("error = %q, want it to mention 'nil'", got)
	}
}

func TestStats_MalformedV1PreservesCause(t *testing.T) {
	payload := marshalAny(t, &cgroup1stats.Metrics{})
	corrupt := &unknownAny{typeURL: payload.GetTypeUrl(), value: []byte{0xff}}
	statser := runcontainerd.NewContainerdStatser(&fakeMetricsGetter{metrics: corrupt})
	_, err := statser.Stats(lagertest.NewTestLogger("test"), "malformed-v1")
	if err == nil || !containsSubstring(err.Error(), "decode v1 metrics") || errors.Unwrap(err) == nil {
		t.Fatalf("error = %v, want wrapped v1 decode cause", err)
	}
}

func TestStats_AgePolicy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, payload := range []typeurl.Any{marshalAny(t, &cgroup1stats.Metrics{}), marshalAny(t, &cgroup2stats.Metrics{})} {
			for _, test := range []struct {
				createdAt time.Time
				want      time.Duration
			}{
				{createdAt: time.Now().Add(-73*time.Second - 29*time.Nanosecond), want: 73*time.Second + 29*time.Nanosecond},
				{createdAt: time.Now().Add(24 * time.Hour), want: -24 * time.Hour},
				{createdAt: time.Time{}, want: time.Duration(1<<63 - 1)},
			} {
				statser := runcontainerd.NewContainerdStatser(&fakeMetricsGetter{metrics: payload, createdAt: test.createdAt})
				got, err := statser.Stats(lagertest.NewTestLogger("test"), "age-policy")
				if err != nil || got.Age != test.want {
					t.Errorf("Age = %v, error = %v, want %v for %v", got.Age, err, test.want, test.createdAt)
				}
			}
		}
	})
}

func TestStats_FutureCreatedAt(t *testing.T) {
	// Gate 4: future creation timestamp policy. time.Since(futureTime) yields
	// a negative duration. Policy: Age is computed as-is; no clamping. The
	// caller (gardener.BulkMetrics) may discard or log anomalous values.
	// The statser itself must not error or clamp to zero silently.
	v2 := &cgroup2stats.Metrics{
		CPU: &cgroup2stats.CPUStat{UsageUsec: 1},
	}

	futureTime := time.Now().Add(24 * time.Hour)
	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v2),
		createdAt: futureTime,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "future-time")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	// Age should be negative (future creation time); statser must not clamp.
	if got.Age >= 0 {
		t.Errorf("Age = %v, want < 0 for a future creation time (no silent clamping)", got.Age)
	}
}

// ---- V1/V2 ordering test ----

func TestStats_V2_TriedBeforeV1(t *testing.T) {
	// When a payload is valid v2, it should be decoded as v2 (microseconds),
	// not accidentally as v1 (nanoseconds). We verify by checking the CPU
	// conversion factor.
	v2 := &cgroup2stats.Metrics{
		CPU: &cgroup2stats.CPUStat{UsageUsec: 1}, // 1µs → 1000ns
	}

	getter := &fakeMetricsGetter{
		metrics:   marshalAny(t, v2),
		createdAt: fixedCreatedAt,
	}
	statser := runcontainerd.NewContainerdStatser(getter)
	log := lagertest.NewTestLogger("test")

	got, err := statser.Stats(log, "v2-first")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	// If decoded as v2: 1 * 1000 = 1000
	// If decoded as v1: 1 (preserved)
	if got.CPU.Usage != 1000 {
		t.Errorf("CPU.Usage = %d, want 1000 (v2 microseconds→nanoseconds)", got.CPU.Usage)
	}
}

// ---- helpers ----

// unknownAny implements typeurl.Any with a custom type URL for testing
// unsupported/malformed payloads.
type unknownAny struct {
	typeURL string
	value   []byte
}

func (u *unknownAny) GetTypeUrl() string { return u.typeURL }
func (u *unknownAny) GetValue() []byte   { return u.value }

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
