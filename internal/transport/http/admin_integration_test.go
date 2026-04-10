//go:build integration
// +build integration

package http

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"
    "time"

    cfg "github.com/ikermy/Bulk/internal/config"
    di "github.com/ikermy/Bulk/internal/di"
    "github.com/ikermy/Bulk/internal/domain"
    "github.com/ikermy/Bulk/internal/ports"
)

// fake repo used by integration test
type fakeRepo struct{
    batches []*domain.Batch
    total int
}
func (f *fakeRepo) Create(ctx context.Context, b *domain.Batch) error { return nil }
func (f *fakeRepo) GetByID(ctx context.Context, id string) (*domain.Batch, error) { return nil, nil }
func (f *fakeRepo) Update(ctx context.Context, b *domain.Batch) error { return nil }
func (f *fakeRepo) List(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) { return f.batches, f.total, nil }
func (f *fakeRepo) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) { return &ports.AdminStatsWithQueues{}, nil }

// TestIntegration_Admin_ListUserBatches поднимает роутер и делает реальный HTTP-запрос к
// GET /api/v1/admin/users/{userId}/batches с admin JWT (env ADMIN_JWT).
func TestIntegration_Admin_ListUserBatches(t *testing.T) {
    if os.Getenv("RUN_INT_TESTS") != "1" {
        t.Skip("skipping integration tests; set RUN_INT_TESTS=1 to run")
    }

    // выставляем ADMIN_JWT чтобы AuthMiddleware пропустил запрос к admin API
    os.Setenv("ADMIN_JWT", "admin-test-token")
    defer os.Unsetenv("ADMIN_JWT")

    // prepare data
    now := time.Now().UTC()
    b := &domain.Batch{ID: "batch-1", UserID: "user-123", Status: "processing", TotalRows: 10, CompletedCount: 4, FailedCount: 1, CreatedAt: now}
    repo := &fakeRepo{batches: []*domain.Batch{b}, total: 1}

    deps := &di.Deps{BatchRepo: repo}
    router := NewRouter(&cfg.Config{}, deps)
    srv := httptest.NewServer(router)
    defer srv.Close()

    req, err := http.NewRequest("GET", srv.URL+"/api/v1/admin/users/user-123/batches?page=1&perPage=2", nil)
    if err != nil {
        t.Fatalf("new request failed: %v", err)
    }
    req.Header.Set("Authorization", "Bearer admin-test-token")

    resp, err := srv.Client().Do(req)
    if err != nil {
        t.Fatalf("http do failed: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Fatalf("expected 200, got %d", resp.StatusCode)
    }

    var body map[string]any
    if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
        t.Fatalf("decode body: %v", err)
    }
    batches, ok := body["batches"].([]any)
    if !ok {
        t.Fatalf("expected batches array, got %v", body["batches"])
    }
    if len(batches) != 1 {
        t.Fatalf("expected 1 batch, got %d", len(batches))
    }
    // verify userId
    first := batches[0].(map[string]any)
    if first["userId"] != "user-123" {
        t.Fatalf("expected userId user-123, got %v", first["userId"])
    }
}


