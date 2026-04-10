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
    "github.com/ikermy/Bulk/internal/testutil"
    "github.com/ikermy/Bulk/internal/billing"
)

func TestHandleBatchStatus_NotFound(t *testing.T) {
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    c.Params = append(c.Params, gin.Param{Key: "id", Value: "b1"})

    deps := &di.Deps{BatchRepo: &testutil.MockBatchRepo{GetByIDFn: func(ctx context.Context, id string) (*domain.Batch, error) { return nil, nil }}}
    HandleBatchStatus(c, deps)
    if rw.Code != http.StatusNotFound {
        t.Fatalf("expected 404 got %d", rw.Code)
    }
}

func TestHandleBatchStatus_WithResults(t *testing.T) {
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    c.Params = append(c.Params, gin.Param{Key: "id", Value: "b2"})

    created := time.Now().Add(-time.Minute)
    completedAt := time.Now()
    b := &domain.Batch{ID: "b2", TotalRows: 2, CompletedCount: 1, FailedCount: 1, CreatedAt: created, CompletedAt: &completedAt, Status: domain.BatchStatusProcessing}

    deps := &di.Deps{BatchRepo: &testutil.MockBatchRepo{GetByIDFn: func(ctx context.Context, id string) (*domain.Batch, error) { return b, nil }}}
    // Note: we only assert status and progress fields
    HandleBatchStatus(c, deps)
    if rw.Code != http.StatusOK {
        t.Fatalf("expected 200 got %d", rw.Code)
    }
    var resp map[string]any
    if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
        t.Fatalf("decode: %v", err)
    }
    if resp["status"] != string(b.Status) {
        t.Fatalf("expected status %s got %v", b.Status, resp["status"])
    }
}

func TestHandleConfirm_BillingPath(t *testing.T) {
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    c.Params = append(c.Params, gin.Param{Key: "id", Value: "b3"})
    // body: generate_valid
    body := `{"action":"generate_valid"}`
    req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    c.Request = req

    // set batch repo to return validRows=2
    b := &domain.Batch{ID: "b3", ValidRows: 2}
    mockBatch := &testutil.MockBatchRepo{GetByIDFn: func(ctx context.Context, id string) (*domain.Batch, error) { return b, nil }}

    // job repo returns 2 jobs
    jobs := []*domain.Job{{ID: "j1", BatchID: "b3", RowNumber: 1}, {ID: "j2", BatchID: "b3", RowNumber: 2}}
    mockJob := &testutil.MockJobRepo{GetByBatchFn: func(ctx context.Context, batchID string) ([]*domain.Job, error) { return jobs, nil }, UpdateStatusFn: func(ctx context.Context, jobID string, status string) error { return nil }}

    // billing client that succeeds
    mockBilling := &testutil.MockBilling{BlockBatchFn: func(ctx context.Context, user string, count int, batchID string) (*billing.BlockBatchResponse, error) { return &billing.BlockBatchResponse{TransactionIDs: []string{}}, nil }}

    // producer that succeeds
    mockProducer := &testutil.MockProducer{PublishFn: func(ctx context.Context, topic string, key []byte, msg any) error { return nil }}

    deps := &di.Deps{BatchRepo: mockBatch, JobRepo: mockJob, BillingClient: mockBilling, Producer: mockProducer}
    HandleConfirm(c, deps)
    if rw.Code != http.StatusAccepted {
        t.Fatalf("expected 202 got %d", rw.Code)
    }
}

func TestHandleCancel_NoManager(t *testing.T) {
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    c.Params = append(c.Params, gin.Param{Key: "id", Value: "b4"})
    deps := &di.Deps{}
    HandleCancel(c, deps)
    if rw.Code != http.StatusOK {
        t.Fatalf("expected 200 got %d", rw.Code)
    }
}

func TestHandleAdminGetBatch(t *testing.T) {
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    c.Params = append(c.Params, gin.Param{Key: "id", Value: "b5"})

    b := &domain.Batch{ID: "b5", UserID: "u1", Status: domain.BatchStatusProcessing, Revision: "rev1", CreatedAt: time.Now()}
    jobs := []*domain.Job{{ID: "j1", BatchID: "b5", RowNumber: 2, Status: domain.JobStatusCompleted}}
    mockBatch := &testutil.MockBatchRepo{GetByIDFn: func(ctx context.Context, id string) (*domain.Batch, error) { return b, nil }}
    mockJob := &testutil.MockJobRepo{GetByBatchFn: func(ctx context.Context, batchID string) ([]*domain.Job, error) { return jobs, nil }}

    deps := &di.Deps{BatchRepo: mockBatch, JobRepo: mockJob}
    HandleAdminGetBatch(c, deps)
    if rw.Code != http.StatusOK {
        t.Fatalf("expected 200 got %d", rw.Code)
    }
}

func TestHandleAdminRestartBatch(t *testing.T) {
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    c.Params = append(c.Params, gin.Param{Key: "id", Value: "b6"})
    body := `{"restartMode":"failed_only","force":false}`
    req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    c.Request = req

    jobs := []*domain.Job{{ID: "j1", BatchID: "b6", RowNumber: 1, Status: domain.JobStatusFailed}, {ID: "j2", BatchID: "b6", RowNumber: 2, Status: domain.JobStatusCompleted}}
    mockBatch := &testutil.MockBatchRepo{GetByIDFn: func(ctx context.Context, id string) (*domain.Batch, error) { return &domain.Batch{ID: "b6"}, nil }}
    mockJob := &testutil.MockJobRepo{GetByBatchFn: func(ctx context.Context, batchID string) ([]*domain.Job, error) { return jobs, nil }, UpdateStatusFn: func(ctx context.Context, jobID string, status string) error { return nil }}
    mockProducer := &testutil.MockProducer{PublishFn: func(ctx context.Context, topic string, key []byte, msg any) error { return nil }}
    deps := &di.Deps{BatchRepo: mockBatch, JobRepo: mockJob, Producer: mockProducer}
    HandleAdminRestartBatch(c, deps)
    if rw.Code != http.StatusOK {
        t.Fatalf("expected 200 got %d", rw.Code)
    }
}





