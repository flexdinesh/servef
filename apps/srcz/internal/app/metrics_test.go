package app

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestMetricsCollectorPrefersRSS(t *testing.T) {
	goMemoryRead := false
	collector := Metrics{
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

	metrics := collector.Sample()
	if metrics.MemoryBytes != 123 || metrics.MemorySource != "rss" {
		t.Fatalf("memory = %d (%q), want 123 (rss)", metrics.MemoryBytes, metrics.MemorySource)
	}
	if goMemoryRead {
		t.Fatal("Go memory read despite available RSS")
	}
}

func TestMetricsCollectorFallsBackToGoMemory(t *testing.T) {
	collector := Metrics{
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

	metrics := collector.Sample()
	if metrics.MemoryBytes != 456 || metrics.MemorySource != "go" {
		t.Fatalf("memory = %d (%q), want 456 (go)", metrics.MemoryBytes, metrics.MemorySource)
	}
}

func TestMetricsCollectorOmitsUnavailableCPU(t *testing.T) {
	collector := Metrics{
		readRSS: func() (uint64, error) {
			return 123, nil
		},
		readCPUTime: func() (time.Duration, error) {
			return 0, errors.New("CPU unavailable")
		},
	}

	if metrics := collector.Sample(); metrics.CPUUsage != nil {
		t.Fatalf("CPU usage = %v, want nil", *metrics.CPUUsage)
	}
}

func TestMetricsEncodeUnavailableCPUAsNull(t *testing.T) {
	collector := Metrics{
		readRSS:      func() (uint64, error) { return 0, errors.New("RSS unavailable") },
		readCPUTime:  func() (time.Duration, error) { return 0, errors.New("CPU unavailable") },
		readGoMemory: func() uint64 { return 456 },
	}
	encoded, err := json.Marshal(collector.Sample())
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["cpuUsage"]) != "null" || string(fields["memorySource"]) != `"go"` || string(fields["memoryBytes"]) != "456" {
		t.Fatalf("metrics = %s", encoded)
	}
	for _, legacy := range []string{"rssBytes", "supported"} {
		if _, ok := fields[legacy]; ok {
			t.Fatalf("legacy field %q present", legacy)
		}
	}
}
