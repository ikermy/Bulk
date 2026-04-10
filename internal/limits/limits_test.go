package limits

import (
	"testing"
)

func TestGetSetUpdate(t *testing.T) {
	// snapshot
	prev := Get()
	defer Set(prev)

	// set new values
	Set(Limits{MaxFileSize: 1024, MaxRowsPerBatch: 10, MaxConcurrentBatches: 2, MaxBatchesPerHour: 3})
	v := Get()
	if v.MaxFileSize != 1024 || v.MaxRowsPerBatch != 10 || v.MaxConcurrentBatches != 2 || v.MaxBatchesPerHour != 3 {
		t.Fatalf("set/get mismatch: %+v", v)
	}

	// UpdateIfPositive should only update positive fields
	UpdateIfPositive(Limits{MaxFileSize: 2048, MaxRowsPerBatch: 0, MaxConcurrentBatches: 5})
	v2 := Get()
	if v2.MaxFileSize != 2048 || v2.MaxRowsPerBatch != 10 || v2.MaxConcurrentBatches != 5 {
		t.Fatalf("unexpected values after UpdateIfPositive: %+v", v2)
	}
}
