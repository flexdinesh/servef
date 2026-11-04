package contract

// MemorySource names what MemoryBytes measures: resident memory, or the Go
// runtime's memory where resident memory is unavailable.
type MemorySource string

const (
	MemoryRSS MemorySource = "rss"
	MemoryGo  MemorySource = "go"
)

// ProcessMetrics is GET /api/metrics: the servef process's memory, CPU
// usage (percent of one core since the last sample; null when unavailable),
// and goroutines.
type ProcessMetrics struct {
	MemoryBytes  uint64       `json:"memoryBytes"`
	MemorySource MemorySource `json:"memorySource"`
	CPUUsage     *float64     `json:"cpuUsage"`
	Goroutines   int          `json:"goroutines"`
}
