package limits

import "sync"

// Limits holds runtime-configurable limits for bulk processing.
// Default values match ТЗ §11.2
type Limits struct {
	MaxFileSize           int // bytes
	MaxRowsPerBatch       int
	MaxConcurrentBatches  int
	MaxBatchesPerHour     int
}

var (
	mu sync.RWMutex
	val = Limits{
		MaxFileSize:          10 * 1024 * 1024,
		MaxRowsPerBatch:      1000,
		MaxConcurrentBatches: 5,
		MaxBatchesPerHour:    10,
	}
)

func Get() Limits {
	mu.RLock()
	defer mu.RUnlock()
	return val
}

func Set(l Limits) {
	mu.Lock()
	val = l
	mu.Unlock()
}

func UpdateIfPositive(l Limits) {
	mu.Lock()
	if l.MaxFileSize > 0 {
		val.MaxFileSize = l.MaxFileSize
	}
	if l.MaxRowsPerBatch > 0 {
		val.MaxRowsPerBatch = l.MaxRowsPerBatch
	}
	if l.MaxConcurrentBatches > 0 {
		val.MaxConcurrentBatches = l.MaxConcurrentBatches
	}
	if l.MaxBatchesPerHour > 0 {
		val.MaxBatchesPerHour = l.MaxBatchesPerHour
	}
	mu.Unlock()
}

