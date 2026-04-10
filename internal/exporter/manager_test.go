package exporter

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestManager_EnqueueAndProcess(t *testing.T) {
	m := NewManager(2, nil, nil)
	id := "batch1"
	if err := m.Enqueue(id); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	// wait for worker to process
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r, ok := m.Get(id); ok {
			if r.Status == StatusDone || r.Status == StatusFailed {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("export did not complete in time")
}

func TestManager_EnqueueDuplicate(t *testing.T) {
	m := NewManager(4, nil, nil)
	id := "dup1"
	if err := m.Enqueue(id); err != nil {
		t.Fatalf("first enqueue failed: %v", err)
	}
	err := m.Enqueue(id)
	if err == nil {
		t.Fatalf("expected error for duplicate enqueue, got nil")
	}
	if err.Error() != "export already exists" {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestManager_QueueFull(t *testing.T) {
	// buffer=0 means queue cannot hold any items (channel capacity 0)
	m := &Manager{
		queue:   make(chan string, 0),
		results: make(map[string]*ExportResult),
	}
	// stop the auto-started worker by not starting one — just test the channel full path
	err := m.Enqueue("any")
	if err == nil {
		t.Fatalf("expected 'export queue full', got nil")
	}
	if err.Error() != "export queue full" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManager_Get_NotFound(t *testing.T) {
	m := NewManager(2, nil, nil)
	r, ok := m.Get("nonexistent")
	if ok {
		t.Fatalf("expected not found, got result: %+v", r)
	}
	if r != nil {
		t.Fatalf("expected nil result, got %+v", r)
	}
}

// mockLogger satisfies logging.Logger to exercise Logger paths
// NOTE: logging.Logger = *zap.SugaredLogger — use a real nop logger.

func TestManager_WithLogger(t *testing.T) {
	log := zap.NewNop().Sugar()
	m := NewManager(4, nil, nil)
	m.Logger = log

	id := "logged1"
	if err := m.Enqueue(id); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r, ok := m.Get(id); ok && (r.Status == StatusDone || r.Status == StatusFailed) {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("export with logger did not complete in time")
}

