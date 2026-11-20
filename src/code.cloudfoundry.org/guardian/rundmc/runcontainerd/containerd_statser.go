//go:build !windows

package runcontainerd

import (
	"fmt"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/gardener"
	"code.cloudfoundry.org/lager/v3"
	cgroup1stats "github.com/containerd/cgroups/v3/cgroup1/stats"
	cgroup2stats "github.com/containerd/cgroups/v3/cgroup2/stats"
	"github.com/containerd/typeurl/v2"
)

// ContainerMetricsGetter abstracts the containerd task metrics retrieval.
// Returns the raw protobuf Any from task.Metrics() so the caller can handle
// both cgroupv1 and cgroupv2 formats.
type ContainerMetricsGetter interface {
	TaskMetrics(log lager.Logger, containerID string) (typeurl.Any, time.Time, error)
}

// ContainerdStatser implements the Statser interface using containerd task
// metrics API instead of shelling out to runc. This is required for non-runc
// runtimes (e.g. gVisor/runsc) where "runc events --stats" does not work
// because runc has no knowledge of the container.
type ContainerdStatser struct {
	metricsGetter ContainerMetricsGetter
}

func NewContainerdStatser(metricsGetter ContainerMetricsGetter) *ContainerdStatser {
	return &ContainerdStatser{metricsGetter: metricsGetter}
}

func (s *ContainerdStatser) Stats(log lager.Logger, id string) (gardener.StatsContainerMetrics, error) {
	log = log.Session("containerd-statser", lager.Data{"id": id})

	rawMetrics, createdAt, err := s.metricsGetter.TaskMetrics(log, id)
	if err != nil {
		return gardener.StatsContainerMetrics{}, fmt.Errorf("containerd task metrics: %w", err)
	}

	if rawMetrics == nil {
		return gardener.StatsContainerMetrics{}, fmt.Errorf("containerd task metrics: nil metrics payload")
	}

	if typeurl.Is(rawMetrics, &cgroup2stats.Metrics{}) {
		metrics, err := unmarshalV2Metrics(rawMetrics)
		if err != nil {
			return gardener.StatsContainerMetrics{}, fmt.Errorf("containerd task metrics: decode v2 metrics: %w", err)
		}
		return statsFromV2(metrics, createdAt), nil
	}
	if typeurl.Is(rawMetrics, &cgroup1stats.Metrics{}) {
		metrics, err := unmarshalV1Metrics(rawMetrics)
		if err != nil {
			return gardener.StatsContainerMetrics{}, fmt.Errorf("containerd task metrics: decode v1 metrics: %w", err)
		}
		return statsFromV1(metrics, createdAt), nil
	}

	return gardener.StatsContainerMetrics{}, fmt.Errorf("containerd task metrics: unsupported metrics type")
}

func unmarshalV2Metrics(data typeurl.Any) (*cgroup2stats.Metrics, error) {
	var m cgroup2stats.Metrics
	if err := typeurl.UnmarshalTo(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func unmarshalV1Metrics(data typeurl.Any) (*cgroup1stats.Metrics, error) {
	var m cgroup1stats.Metrics
	if err := typeurl.UnmarshalTo(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func statsFromV2(metrics *cgroup2stats.Metrics, createdAt time.Time) gardener.StatsContainerMetrics {
	var memoryStat garden.ContainerMemoryStat
	if metrics.Memory != nil {
		memoryStat = garden.ContainerMemoryStat{
			Anon:                    metrics.Memory.Anon,
			File:                    metrics.Memory.File,
			ActiveAnon:              metrics.Memory.ActiveAnon,
			InactiveAnon:            metrics.Memory.InactiveAnon,
			ActiveFile:              metrics.Memory.ActiveFile,
			InactiveFile:            metrics.Memory.InactiveFile,
			Unevictable:             metrics.Memory.Unevictable,
			MappedFile:              metrics.Memory.FileMapped,
			Pgfault:                 metrics.Memory.Pgfault,
			Pgmajfault:              metrics.Memory.Pgmajfault,
			SwapCached:              0, // cgroupsv2 does not expose swapcached separately
			HierarchicalMemoryLimit: metrics.Memory.UsageLimit,
			TotalActiveAnon:         metrics.Memory.ActiveAnon,
			TotalActiveFile:         metrics.Memory.ActiveFile,
			TotalInactiveAnon:       metrics.Memory.InactiveAnon,
			TotalInactiveFile:       metrics.Memory.InactiveFile,
			TotalUnevictable:        metrics.Memory.Unevictable,
			TotalMappedFile:         metrics.Memory.FileMapped,
			TotalPgfault:            metrics.Memory.Pgfault,
			TotalPgmajfault:         metrics.Memory.Pgmajfault,
			Rss:                     metrics.Memory.Anon,
			TotalRss:                metrics.Memory.Anon,
			Cache:                   metrics.Memory.File,
			TotalCache:              metrics.Memory.File,
			Swap:                    metrics.Memory.SwapUsage,
			TotalSwap:               metrics.Memory.SwapUsage,
		}
		totalMemoryUsage := memoryStat.File + memoryStat.Anon + memoryStat.SwapCached
		if memoryStat.InactiveFile > totalMemoryUsage {
			memoryStat.TotalUsageTowardLimit = 0
		} else {
			memoryStat.TotalUsageTowardLimit = totalMemoryUsage - memoryStat.InactiveFile
		}
	}

	var cpuStat garden.ContainerCPUStat
	if metrics.CPU != nil {
		// cgroupsv2 reports CPU in microseconds; garden expects nanoseconds
		cpuStat = garden.ContainerCPUStat{
			Usage:  metrics.CPU.UsageUsec * 1000,
			User:   metrics.CPU.UserUsec * 1000,
			System: metrics.CPU.SystemUsec * 1000,
		}
	}

	var pidStat garden.ContainerPidStat
	if metrics.Pids != nil {
		pidStat = garden.ContainerPidStat{
			Current: metrics.Pids.Current,
			Max:     metrics.Pids.Limit,
		}
	}

	return gardener.StatsContainerMetrics{
		CPU:    cpuStat,
		Memory: memoryStat,
		Pid:    pidStat,
		Age:    time.Since(createdAt),
	}
}

func statsFromV1(metrics *cgroup1stats.Metrics, createdAt time.Time) gardener.StatsContainerMetrics {
	var memoryStat garden.ContainerMemoryStat
	if metrics.Memory != nil {
		m := metrics.Memory
		memoryStat = garden.ContainerMemoryStat{
			Cache:                   m.Cache,
			Rss:                     m.RSS,
			MappedFile:              m.MappedFile,
			Pgfault:                 m.PgFault,
			Pgmajfault:              m.PgMajFault,
			ActiveAnon:              m.ActiveAnon,
			InactiveAnon:            m.InactiveAnon,
			ActiveFile:              m.ActiveFile,
			InactiveFile:            m.InactiveFile,
			Unevictable:             m.Unevictable,
			HierarchicalMemoryLimit: m.HierarchicalMemoryLimit,
			TotalCache:              m.TotalCache,
			TotalRss:                m.TotalRSS,
			TotalMappedFile:         m.TotalMappedFile,
			TotalPgfault:            m.TotalPgFault,
			TotalPgmajfault:         m.TotalPgMajFault,
			TotalActiveAnon:         m.TotalActiveAnon,
			TotalInactiveAnon:       m.TotalInactiveAnon,
			TotalActiveFile:         m.TotalActiveFile,
			TotalInactiveFile:       m.TotalInactiveFile,
			TotalUnevictable:        m.TotalUnevictable,
			// Anon and File not in v1 directly; approximate from totals
			Anon: m.TotalRSS,
			File: m.TotalCache,
		}
		if m.Swap != nil {
			memoryStat.Swap = m.Swap.Usage
			memoryStat.TotalSwap = m.Swap.Usage
		}
		if m.Usage != nil {
			memoryStat.TotalUsageTowardLimit = m.Usage.Usage
		}
	}

	var cpuStat garden.ContainerCPUStat
	if metrics.CPU != nil && metrics.CPU.Usage != nil {
		// cgroupsv1 CPU is already in nanoseconds
		cpuStat = garden.ContainerCPUStat{
			Usage:  metrics.CPU.Usage.Total,
			User:   metrics.CPU.Usage.User,
			System: metrics.CPU.Usage.Kernel,
		}
	}

	var pidStat garden.ContainerPidStat
	if metrics.Pids != nil {
		pidStat = garden.ContainerPidStat{
			Current: metrics.Pids.Current,
			Max:     metrics.Pids.Limit,
		}
	}

	return gardener.StatsContainerMetrics{
		CPU:    cpuStat,
		Memory: memoryStat,
		Pid:    pidStat,
		Age:    time.Since(createdAt),
	}
}
