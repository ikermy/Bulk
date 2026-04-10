package handlers

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"
    "fmt"

    "github.com/gin-gonic/gin"
    "github.com/ikermy/Bulk/internal/di"
    "github.com/ikermy/Bulk/internal/ports"
    "github.com/ikermy/Bulk/internal/testutil"
    "github.com/stretchr/testify/require"
)

func TestHandleAdminStats_Defaults(t *testing.T) {
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

    HandleAdminStats(c, &di.Deps{})
    require.Equal(t, http.StatusOK, rw.Code)
    var out map[string]any
    require.NoError(t, json.Unmarshal(rw.Body.Bytes(), &out))
    require.Equal(t, float64(0), out["totalBatches"].(float64))
}

func TestHandleAdminStats_FromRepo(t *testing.T) {
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    mock := &testutil.MockBatchRepo{}
    c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
    mock.AdminStatsFn = func(ctx context.Context, f *time.Time, t *time.Time) (*ports.AdminStatsWithQueues, error) {
        return &ports.AdminStatsWithQueues{AdminStats: ports.AdminStats{BatchesCreated: 5, JobsProcessed: 10, JobsFailed: 1, AverageProcessingTimeMs: 123.4}, Queues: ports.QueuesStats{BulkJobPending: 2, BulkResultPending: 3}}, nil
    }
    HandleAdminStats(c, &di.Deps{BatchRepo: mock})
    require.Equal(t, http.StatusOK, rw.Code)
    var out map[string]any
    require.NoError(t, json.Unmarshal(rw.Body.Bytes(), &out))
    require.Equal(t, float64(5), out["totalBatches"].(float64))
}

func TestHandleAdminStats_RepoError(t *testing.T) {
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    mock := &testutil.MockBatchRepo{}
    c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
    mock.AdminStatsFn = func(ctx context.Context, f *time.Time, t *time.Time) (*ports.AdminStatsWithQueues, error) {
        return nil, fmt.Errorf("boom")
    }
    HandleAdminStats(c, &di.Deps{BatchRepo: mock})
    require.Equal(t, http.StatusInternalServerError, rw.Code)
    var out map[string]any
    require.NoError(t, json.Unmarshal(rw.Body.Bytes(), &out))
    // apperror writes code "SERVICE_ERROR"
    if out["code"] != "SERVICE_ERROR" {
        t.Fatalf("expected SERVICE_ERROR code, got %v", out["code"])
    }
}






