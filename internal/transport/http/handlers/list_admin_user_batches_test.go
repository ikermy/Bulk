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

type fakeAdminUserBatchRepo struct{
    batches []*domain.Batch
    total int
}
func (f *fakeAdminUserBatchRepo) Create(ctx context.Context, b *domain.Batch) error { return nil }
func (f *fakeAdminUserBatchRepo) GetByID(ctx context.Context, id string) (*domain.Batch, error) { return nil, nil }
func (f *fakeAdminUserBatchRepo) Update(ctx context.Context, b *domain.Batch) error { return nil }
func (f *fakeAdminUserBatchRepo) List(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) { return f.batches, f.total, nil }
func (f *fakeAdminUserBatchRepo) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) { return &ports.AdminStatsWithQueues{}, nil }

func TestHandleAdminListBatches_ByUserPathParam(t *testing.T) {
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    // simulate path /admin/users/user-123/batches
    req := httptest.NewRequest("GET", "/admin/users/user-123/batches?page=1&perPage=2", nil)
    c.Request = req
    c.Params = append(c.Params, gin.Param{Key: "userId", Value: "user-123"})

    now := time.Now().UTC()
    b1 := &domain.Batch{ID: "batch-1", UserID: "user-123", Status: "processing", TotalRows: 10, CompletedCount: 4, FailedCount: 1, CreatedAt: now}
    repo := &fakeAdminUserBatchRepo{batches: []*domain.Batch{b1}, total: 1}
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
    if !ok || len(batches) != 1 {
        t.Fatalf("expected 1 batch, got %v", resp["batches"])
    }
    // ensure returned batch contains userId == user-123
    batch0 := batches[0].(map[string]any)
    if batch0["userId"] != "user-123" {
        t.Fatalf("expected userId user-123, got %v", batch0["userId"])
    }
}

