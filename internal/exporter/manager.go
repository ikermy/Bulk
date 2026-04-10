package exporter

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/ikermy/Bulk/internal/logging"
	"github.com/ikermy/Bulk/internal/storage"
	svc "github.com/ikermy/Bulk/internal/usecase/bulk"
)

// ExportStatus Simple in-memory exporter queue and manager
type ExportStatus string

const (
	StatusPending ExportStatus = "pending"
	StatusRunning ExportStatus = "running"
	StatusDone    ExportStatus = "done"
	StatusFailed  ExportStatus = "failed"
)

type ExportResult struct {
	ID        string
	Status    ExportStatus
	StorageID string
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Manager struct {
	mu      sync.Mutex
	queue   chan string
	results map[string]*ExportResult
	storage *storage.FileClient
	svc     *svc.Service
	Logger  logging.Logger
}

// ManagerAPI defines minimal methods used by other packages
type ManagerAPI interface {
	Enqueue(id string) error
	Get(id string) (*ExportResult, bool)
}

func NewManager(buf int, storageClient *storage.FileClient, service *svc.Service) *Manager {
	m := &Manager{
		queue:   make(chan string, buf),
		results: make(map[string]*ExportResult),
		storage: storageClient,
		svc:     service,
	}
	go m.worker()
	return m
}

func (m *Manager) Enqueue(id string) error {
	m.mu.Lock()
	if _, ok := m.results[id]; ok {
		m.mu.Unlock()
		return errors.New("export already exists")
	}
	res := &ExportResult{ID: id, Status: StatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	m.results[id] = res
	m.mu.Unlock()
	select {
	case m.queue <- id:
		return nil
	default:
		// queue full
		return errors.New("export queue full")
	}
}

func (m *Manager) Get(id string) (*ExportResult, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.results[id]
	return r, ok
}

// worker processes queued exports; here we simulate generation and save to storage
func (m *Manager) worker() {
	for id := range m.queue {
		if m.Logger != nil {
			m.Logger.Info("export_worker_start", "exportId", id)
		}
		m.mu.Lock()
		r := m.results[id]
		if r == nil {
			m.mu.Unlock()
			continue
		}
		r.Status = StatusRunning
		r.UpdatedAt = time.Now()
		m.mu.Unlock()

		// call service to generate real XLSX stream
		var storageID string
		var err error
		if m.svc != nil {
			rc, gerr := m.svc.ExportResultsXLS(context.Background(), id, 0)
			if gerr != nil {
				err = gerr
			} else if rc != nil {
				if m.storage != nil {
					storageID, err = m.storage.Save("exports/"+id+".xlsx", rc)
				}
				_ = rc.Close()
			}
		} else {
			// fallback to dummy content
			data := []byte("export for batch " + id)
			if m.storage != nil {
				storageID, err = m.storage.Save("exports/"+id+".xlsx", io.NopCloser(strings.NewReader(string(data))))
			}
		}

		m.mu.Lock()
		if err != nil {
			r.Status = StatusFailed
			r.Error = err.Error()
			if m.Logger != nil {
				m.Logger.Error("export_failed", "exportId", id, "error", err)
			}
		} else {
			r.Status = StatusDone
			r.StorageID = storageID
			if m.Logger != nil {
				m.Logger.Info("export_completed", "exportId", id, "storageId", storageID)
			}
		}
		r.UpdatedAt = time.Now()
		m.mu.Unlock()
	}
}
