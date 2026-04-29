package containerd

import (
	"context"
	"fmt"
	"time"

	cgroup1stats "github.com/containerd/cgroups/v3/cgroup1/stats"
	cgroup2stats "github.com/containerd/cgroups/v3/cgroup2/stats"
	containerd "github.com/containerd/containerd/v2/client"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const metricsSampleInterval = 200 * time.Millisecond

const maxPracticalMemoryLimitBytes = uint64(1) << 60

type taskMetricsSample struct {
	timestamp   time.Time
	cpuUsageNS  uint64
	cpuUserNS   uint64
	cpuKernelNS uint64
	memoryUsage uint64
	memoryLimit uint64
}

func (s *DefaultService) GetContainerMetrics(ctx context.Context, containerID string) (ContainerMetrics, error) {
	task, ctx, err := s.getTask(ctx, containerID)
	if err != nil {
		return ContainerMetrics{}, err
	}

	first, err := sampleTaskMetrics(ctx, task)
	if err != nil {
		return ContainerMetrics{}, err
	}

	timer := time.NewTimer(metricsSampleInterval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ContainerMetrics{}, ctx.Err()
	case <-timer.C:
	}

	second, err := sampleTaskMetrics(ctx, task)
	if err != nil {
		return ContainerMetrics{}, err
	}

	return ContainerMetrics{
		SampledAt: second.timestamp,
		CPU:       buildCPUMetrics(first, second),
		Memory:    buildMemoryMetrics(second),
	}, nil
}

func sampleTaskMetrics(ctx context.Context, task containerd.Task) (taskMetricsSample, error) {
	metric, err := task.Metrics(ctx)
	if err != nil {
		return taskMetricsSample{}, err
	}
	if metric == nil || metric.Data == nil {
		return taskMetricsSample{}, ErrNotSupported
	}

	timestamp := time.Now()
	if ts := metric.GetTimestamp(); ts != nil {
		timestamp = ts.AsTime()
	}

	switch {
	case metric.Data.MessageIs(&cgroup1stats.Metrics{}):
		var stats cgroup1stats.Metrics
		if err := anypb.UnmarshalTo(metric.Data, &stats, proto.UnmarshalOptions{}); err != nil {
			return taskMetricsSample{}, fmt.Errorf("decode cgroup v1 metrics: %w", err)
		}
		return sampleFromCgroup1(timestamp, &stats), nil
	case metric.Data.MessageIs(&cgroup2stats.Metrics{}):
		var stats cgroup2stats.Metrics
		if err := anypb.UnmarshalTo(metric.Data, &stats, proto.UnmarshalOptions{}); err != nil {
			return taskMetricsSample{}, fmt.Errorf("decode cgroup v2 metrics: %w", err)
		}
		return sampleFromCgroup2(timestamp, &stats), nil
	default:
		msg, decodeErr := anypb.UnmarshalNew(metric.Data, proto.UnmarshalOptions{})
		if decodeErr != nil {
			return taskMetricsSample{}, fmt.Errorf("decode task metrics: %w", decodeErr)
		}
		return taskMetricsSample{}, fmt.Errorf("%w: unsupported task metrics type %T", ErrNotSupported, msg)
	}
}

func sampleFromCgroup1(timestamp time.Time, stats *cgroup1stats.Metrics) taskMetricsSample {
	sample := taskMetricsSample{timestamp: timestamp}
	if stats == nil {
		return sample
	}
	if cpu := stats.GetCPU(); cpu != nil {
		usage := cpu.GetUsage()
		sample.cpuUsageNS = usage.GetTotal()
		sample.cpuUserNS = usage.GetUser()
		sample.cpuKernelNS = usage.GetKernel()
	}
	if memory := stats.GetMemory(); memory != nil {
		entry := memory.GetUsage()
		sample.memoryUsage = entry.GetUsage()
		sample.memoryLimit = normalizeMemoryLimit(entry.GetLimit())
	}
	return sample
}

func sampleFromCgroup2(timestamp time.Time, stats *cgroup2stats.Metrics) taskMetricsSample {
	sample := taskMetricsSample{timestamp: timestamp}
	if stats == nil {
		return sample
	}
	if cpu := stats.GetCPU(); cpu != nil {
		sample.cpuUsageNS = cpu.GetUsageUsec() * 1_000
		sample.cpuUserNS = cpu.GetUserUsec() * 1_000
		sample.cpuKernelNS = cpu.GetSystemUsec() * 1_000
	}
	if memory := stats.GetMemory(); memory != nil {
		sample.memoryUsage = memory.GetUsage()
		sample.memoryLimit = normalizeMemoryLimit(memory.GetUsageLimit())
	}
	return sample
}

func buildCPUMetrics(first, second taskMetricsSample) *CPUMetrics {
	metrics := &CPUMetrics{
		UsageNanoseconds:  second.cpuUsageNS,
		UserNanoseconds:   second.cpuUserNS,
		KernelNanoseconds: second.cpuKernelNS,
	}

	elapsedNS := second.timestamp.Sub(first.timestamp).Nanoseconds()
	if elapsedNS <= 0 || second.cpuUsageNS < first.cpuUsageNS {
		return metrics
	}

	metrics.UsagePercent = (float64(second.cpuUsageNS-first.cpuUsageNS) / float64(elapsedNS)) * 100
	if metrics.UsagePercent < 0 {
		metrics.UsagePercent = 0
	}

	return metrics
}

func buildMemoryMetrics(sample taskMetricsSample) *MemoryMetrics {
	metrics := &MemoryMetrics{
		UsageBytes: sample.memoryUsage,
		LimitBytes: sample.memoryLimit,
	}
	if sample.memoryLimit > 0 {
		metrics.UsagePercent = (float64(sample.memoryUsage) / float64(sample.memoryLimit)) * 100
	}
	return metrics
}

func normalizeMemoryLimit(limit uint64) uint64 {
	if limit == 0 || limit > maxPracticalMemoryLimitBytes {
		return 0
	}
	return limit
}
