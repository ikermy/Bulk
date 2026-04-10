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

// generic fake repo that invokes provided callback for List
type paramAssertRepo struct{
    onList func(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error)
}
func (r *paramAssertRepo) Create(ctx context.Context, b *domain.Batch) error { return nil }
func (r *paramAssertRepo) GetByID(ctx context.Context, id string) (*domain.Batch, error) { return nil, nil }
func (r *paramAssertRepo) Update(ctx context.Context, b *domain.Batch) error { return nil }
func (r *paramAssertRepo) List(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) {
    return r.onList(ctx, filter)
}

func (r *paramAssertRepo) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) {
    return &ports.AdminStatsWithQueues{}, nil
}

func TestHandleListBatches_FilterSortUserRevision(t *testing.T) {
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    // request with userId, revision and sort params
    req := httptest.NewRequest("GET", "/batches?userId=uid123&revision=rev1&sortBy=completedAt&order=asc", nil)
    c.Request = req

    called := false
    repo := &paramAssertRepo{onList: func(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) {
        called = true
        if filter.UserID != "uid123" {
            t.Fatalf("expected userId uid123, got %v", filter.UserID)
        }
        if filter.Revision != "rev1" {
            t.Fatalf("expected revision rev1, got %v", filter.Revision)
        }
        if filter.SortBy != "completedAt" {
            t.Fatalf("expected sortBy completedAt, got %v", filter.SortBy)
        }
        if filter.SortDesc != false {
            t.Fatalf("expected sortDesc false, got %v", filter.SortDesc)
        }
        now := time.Now()
        b := &domain.Batch{ID: "b1", Status: "completed", TotalRows: 10, CompletedCount: 10, FailedCount: 0, CreatedAt: now}
        return []*domain.Batch{b}, 1, nil
    }}

    deps := &di.Deps{BatchRepo: repo}
    HandleListBatches(c, deps)
    if !called { t.Fatalf("expected repo.List to be called") }
    if rw.Code != http.StatusOK { t.Fatalf("expected 200 got %d", rw.Code) }
    var resp map[string]any
    if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil { t.Fatalf("decode: %v", err) }
}

func TestHandleListBatches_CursorPagination(t *testing.T) {
    // ensure cursor is parsed and passed through
    cursorTime := time.Now().Add(-time.Hour).UTC()
    cursorStr := cursorTime.Format(time.RFC3339)

    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    req := httptest.NewRequest("GET", "/batches?cursor="+cursorStr+"&limit=1", nil)
    c.Request = req

    repo := &paramAssertRepo{onList: func(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) {
        if filter.Cursor == "" {
            t.Fatalf("expected cursor passed")
        }
        // ensure cursor equals provided string
        if filter.Cursor != cursorStr {
            t.Fatalf("expected cursor %s got %s", cursorStr, filter.Cursor)
        }
        b := &domain.Batch{ID: "b2", Status: "processing", TotalRows: 5, CompletedCount: 2, FailedCount: 1, CreatedAt: time.Now()}
        return []*domain.Batch{b}, 1, nil
    }}
    deps := &di.Deps{BatchRepo: repo}
    HandleListBatches(c, deps)
    if rw.Code != http.StatusOK { t.Fatalf("expected 200 got %d", rw.Code) }
}

func TestHandleListBatches_OrderDesc(t *testing.T) {
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    req := httptest.NewRequest("GET", "/batches?sortBy=createdAt&order=desc", nil)
    c.Request = req

    repo := &paramAssertRepo{onList: func(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) {
        if filter.SortBy != "createdAt" { t.Fatalf("expected sortBy createdAt") }
        if filter.SortDesc != true { t.Fatalf("expected sortDesc true") }
        b := &domain.Batch{ID: "b3", Status: "pending", TotalRows: 0, CompletedCount: 0, FailedCount: 0, CreatedAt: time.Now()}
        return []*domain.Batch{b}, 1, nil
    }}
    deps := &di.Deps{BatchRepo: repo}
    HandleListBatches(c, deps)
    if rw.Code != http.StatusOK { t.Fatalf("expected 200 got %d", rw.Code) }
}

