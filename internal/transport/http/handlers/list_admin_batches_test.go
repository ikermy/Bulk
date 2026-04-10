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

// fake repo reusing structure from list_batches_test
type fakeAdminBatchRepo struct {
    batches []*domain.Batch
    total   int
}

func (f *fakeAdminBatchRepo) Create(ctx context.Context, b *domain.Batch) error { return nil }
func (f *fakeAdminBatchRepo) GetByID(ctx context.Context, id string) (*domain.Batch, error) { return nil, nil }
func (f *fakeAdminBatchRepo) Update(ctx context.Context, b *domain.Batch) error { return nil }
func (f *fakeAdminBatchRepo) List(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) { return f.batches, f.total, nil }

// Implement required interface methods for AdminStats to satisfy tests; implementation not used in this test
func (f *fakeAdminBatchRepo) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) { return &ports.AdminStatsWithQueues{}, nil }

func TestHandleAdminListBatches_ReturnsTZShape(t *testing.T) {
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    req := httptest.NewRequest("GET", "/admin/batches?page=1&perPage=2", nil)
    c.Request = req

    now := time.Now().UTC()
    b1 := &domain.Batch{ID: "batch-1", UserID: "user-1", Status: "processing", TotalRows: 10, CompletedCount: 4, FailedCount: 1, CreatedAt: now}
    b2 := &domain.Batch{ID: "batch-2", UserID: "user-2", Status: "ready", TotalRows: 5, CompletedCount: 5, FailedCount: 0, CreatedAt: now}
    repo := &fakeAdminBatchRepo{batches: []*domain.Batch{b1, b2}, total: 2}
    deps := &di.Deps{BatchRepo: repo}

    HandleAdminListBatches(c, deps)
    if rw.Code != http.StatusOK {
        t.Fatalf("expected 200 got %d", rw.Code)
    }
    var resp map[string]any
    if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
        t.Fatalf("json decode: %v", err)
    }
    batches, ok := resp["batches"].([]any)
    if !ok || len(batches) != 2 {
        t.Fatalf("expected 2 batches, got %v", resp["batches"])
    }
    pagination, ok := resp["pagination"].(map[string]any)
    if !ok {
        t.Fatalf("pagination missing or invalid")
    }
    if int(pagination["perPage"].(float64)) != 2 {
        t.Fatalf("expected perPage 2, got %v", pagination["perPage"])
    }
}


