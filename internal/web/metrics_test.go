package web

import (
	"errors"
	"testing"
	"time"
)

func TestMetricsCollectorPrefersRSS(t *testing.T) {
	goMemoryRead := false
	collector := metricsCollector{
		readRSS: func() (uint64, error) {
			return 123, nil
		},
		readCPUTime: func() (time.Duration, error) {
			return 0, errors.New("CPU unavailable")
		},
		readGoMemory: func() uint64 {
			goMemoryRead = true
			return 456
		},
	}

	metrics := collector.sample()
	if metrics.MemoryBytes != 123 || metrics.MemorySource != "rss" {
		t.Fatalf("memory = %d (%q), want 123 (rss)", metrics.MemoryBytes, metrics.MemorySource)
	}
	if goMemoryRead {
		t.Fatal("Go memory read despite available RSS")
	}
}

func TestMetricsCollectorFallsBackToGoMemory(t *testing.T) {
	collector := metricsCollector{
		readRSS: func() (uint64, error) {
			return 0, errors.New("RSS unavailable")
		},
		readCPUTime: func() (time.Duration, error) {
			return 0, errors.New("CPU unavailable")
		},
		readGoMemory: func() uint64 {
			return 456
		},
	}

	metrics := collector.sample()
	if metrics.MemoryBytes != 456 || metrics.MemorySource != "go" {
		t.Fatalf("memory = %d (%q), want 456 (go)", metrics.MemoryBytes, metrics.MemorySource)
	}
}

func TestMetricsCollectorOmitsUnavailableCPU(t *testing.T) {
	collector := metricsCollector{
		readRSS: func() (uint64, error) {
			return 123, nil
		},
		readCPUTime: func() (time.Duration, error) {
			return 0, errors.New("CPU unavailable")
		},
	}

	if metrics := collector.sample(); metrics.CPUUsage != nil {
		t.Fatalf("CPU usage = %v, want nil", *metrics.CPUUsage)
	}
}
