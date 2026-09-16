package web

// Process sampling adapted from px0, licensed under MIT. See vendor/PX0-LICENSE.txt.

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type processMetrics struct {
	MemoryBytes  uint64   `json:"memoryBytes"`
	MemorySource string   `json:"memorySource"`
	CPUUsage     *float64 `json:"cpuUsage"`
	Goroutines   int      `json:"goroutines"`
}

type metricsCollector struct {
	mu           sync.Mutex
	lastSample   time.Time
	lastCPUTime  time.Duration
	lastUsage    float64
	readRSS      func() (uint64, error)
	readCPUTime  func() (time.Duration, error)
	readGoMemory func() uint64
}

func (c *metricsCollector) sample() processMetrics {
	memory, memoryErr := c.processRSS()
	memorySource := "rss"
	if memoryErr != nil {
		memory = c.goMemory()
		memorySource = "go"
	}
	cpu, cpuErr := c.sampleCPU()
	var cpuUsage *float64
	if cpuErr == nil {
		cpuUsage = &cpu
	}
	return processMetrics{
		MemoryBytes:  memory,
		MemorySource: memorySource,
		CPUUsage:     cpuUsage,
		Goroutines:   runtime.NumGoroutine(),
	}
}

func (c *metricsCollector) processRSS() (uint64, error) {
	if c.readRSS != nil {
		return c.readRSS()
	}
	return readProcessRSS()
}

func (c *metricsCollector) goMemory() uint64 {
	if c.readGoMemory != nil {
		return c.readGoMemory()
	}
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.Sys
}

func readProcessRSS() (uint64, error) {
	data, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, fmt.Errorf("resident pages unavailable")
	}
	pages, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, err
	}
	return pages * uint64(os.Getpagesize()), nil
}

func (c *metricsCollector) sampleCPU() (float64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	cpuTime, err := c.processCPUTime()
	if err != nil {
		return c.lastUsage, err
	}
	if c.lastSample.IsZero() {
		c.lastSample = now
		c.lastCPUTime = cpuTime
		return 0, nil
	}

	wallDelta := now.Sub(c.lastSample)
	if wallDelta < 200*time.Millisecond {
		return c.lastUsage, nil
	}
	cpuDelta := cpuTime - c.lastCPUTime
	usage := float64(cpuDelta) / float64(wallDelta) * 100
	if usage < 0 {
		usage = 0
	}
	c.lastSample = now
	c.lastCPUTime = cpuTime
	c.lastUsage = usage
	return usage, nil
}

func (c *metricsCollector) processCPUTime() (time.Duration, error) {
	if c.readCPUTime != nil {
		return c.readCPUTime()
	}
	return readProcessCPUTime()
}

func readProcessCPUTime() (time.Duration, error) {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0, err
	}
	endName := strings.LastIndexByte(string(data), ')')
	if endName < 0 || len(data) <= endName+2 {
		return 0, fmt.Errorf("process stat unavailable")
	}
	fields := strings.Fields(string(data[endName+2:]))
	if len(fields) < 13 {
		return 0, fmt.Errorf("process CPU fields unavailable")
	}
	userTicks, userErr := strconv.ParseInt(fields[11], 10, 64)
	systemTicks, systemErr := strconv.ParseInt(fields[12], 10, 64)
	if userErr != nil {
		return 0, userErr
	}
	if systemErr != nil {
		return 0, systemErr
	}
	const clockTicksPerSecond = 100
	return time.Duration(userTicks+systemTicks) * time.Second / clockTicksPerSecond, nil
}
