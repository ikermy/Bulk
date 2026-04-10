package handlers

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/ikermy/Bulk/internal/di"
    "github.com/ikermy/Bulk/internal/domain"
    "github.com/ikermy/Bulk/internal/ports"
)

type fakeUpdateBatchRepo struct{
    batch *domain.Batch
    updated bool
}
func (f *fakeUpdateBatchRepo) Create(ctx context.Context, b *domain.Batch) error { return nil }
func (f *fakeUpdateBatchRepo) GetByID(ctx context.Context, id string) (*domain.Batch, error) { return f.batch, nil }
func (f *fakeUpdateBatchRepo) Update(ctx context.Context, b *domain.Batch) error { f.updated = true; f.batch = b; return nil }
func (f *fakeUpdateBatchRepo) List(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) { return nil, 0, nil }
func (f *fakeUpdateBatchRepo) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) { return &ports.AdminStatsWithQueues{}, nil }

func TestHandleAdminUpdateBatchConfig(t *testing.T) {
    gin.SetMode(gin.TestMode)
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    body := `{"priority":"high","timeout":60000}`
    req := httptest.NewRequest("PUT", "/admin/batches/b1/config", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    c.Request = req
    c.Params = gin.Params{{Key: "id", Value: "b1"}}

    repo := &fakeUpdateBatchRepo{batch: &domain.Batch{ID: "b1", UserID: "u1"}}
    deps := &di.Deps{BatchRepo: repo}

    HandleAdminUpdateBatchConfig(c, deps)

    if rw.Code != http.StatusOK {
        t.Fatalf("expected 200 OK, got %d, body: %s", rw.Code, rw.Body.String())
    }
    var resp map[string]any
    if err := json.Unmarshal(rw.Body.Bytes(), &resp); err != nil {
        t.Fatalf("invalid response json: %v", err)
    }
    if !repo.updated {
        t.Fatalf("expected repo.Update to be called")
    }
    cfg, ok := resp["config"].(map[string]any)
    if !ok {
        t.Fatalf("config missing in response")
    }
    if cfg["priority"].(string) != "high" {
        t.Fatalf("expected priority high, got %v", cfg["priority"])
    }
}

