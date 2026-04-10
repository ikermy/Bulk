package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ikermy/Bulk/internal/di"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/ports"
)

type fakeBatchRepo struct {
	batches []*domain.Batch
	total   int
}

func (f *fakeBatchRepo) Create(ctx context.Context, b *domain.Batch) error { return nil }
func (f *fakeBatchRepo) GetByID(ctx context.Context, id string) (*domain.Batch, error) {
	return nil, nil
}
func (f *fakeBatchRepo) Update(ctx context.Context, b *domain.Batch) error { return nil }
func (f *fakeBatchRepo) List(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) {
	return f.batches, f.total, nil
}

func (f *fakeBatchRepo) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) {
	return &ports.AdminStatsWithQueues{}, nil
}

func TestHandleListBatches_ReturnsDataAndMeta(t *testing.T) {
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	// set query params via request URL
	req := httptest.NewRequest("GET", "/batches?page=1&limit=2", nil)
	c.Request = req

	now := time.Now()
	b1 := &domain.Batch{ID: "b1", Status: "completed", TotalRows: 10, CompletedCount: 10, FailedCount: 0, CreatedAt: now}
	b2 := &domain.Batch{ID: "b2", Status: "processing", TotalRows: 5, CompletedCount: 2, FailedCount: 1, CreatedAt: now}
	repo := &fakeBatchRepo{batches: []*domain.Batch{b1, b2}, total: 2}
	deps := &di.Deps{BatchRepo: repo}

	HandleListBatches(c, deps)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rw.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	meta, ok := resp["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta missing or invalid")
	}
	if int(meta["total"].(float64)) != 2 {
		t.Fatalf("expected total 2")
	}
	data, ok := resp["data"].([]any)
	if !ok || len(data) != 2 {
		t.Fatalf("expected 2 data items, got %v", resp["data"])
	}
}
