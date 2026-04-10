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
	di "github.com/ikermy/Bulk/internal/di"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/ports"
)

// Mocks used only in this test file (unique names to avoid collisions)
type restartJobRepoMock struct {
	jobs    []*domain.Job
	updated map[string]string
}

func (m *restartJobRepoMock) Create(ctx context.Context, j *domain.Job) error { return nil }
func (m *restartJobRepoMock) GetByBatch(ctx context.Context, batchID string) ([]*domain.Job, error) {
	return m.jobs, nil
}
func (m *restartJobRepoMock) UpdateStatus(ctx context.Context, jobID string, status string) error {
	if m.updated == nil {
		m.updated = map[string]string{}
	}
	m.updated[jobID] = status
	return nil
}
func (m *restartJobRepoMock) GetResultsByBatch(ctx context.Context, batchID string) ([]*ports.JobResult, error) {
	return nil, nil
}
func (m *restartJobRepoMock) UpdateBillingTransactionID(ctx context.Context, jobID string, txID *string) error {
	return nil
}
func (m *restartJobRepoMock) UpdateStatusWithResult(ctx context.Context, jobID string, status string, result ports.JobResult) error {
	return nil
}

type restartProducerMock struct{ published []map[string]any }

func (m *restartProducerMock) Publish(ctx context.Context, topic string, key []byte, msg any) error {
	b, _ := json.Marshal(msg)
	var v map[string]any
	_ = json.Unmarshal(b, &v)
	m.published = append(m.published, v)
	return nil
}
func (m *restartProducerMock) Close() error { return nil }

type restartBatchRepoMock struct{}

func (m *restartBatchRepoMock) Create(ctx context.Context, b *domain.Batch) error { return nil }
func (m *restartBatchRepoMock) GetByID(ctx context.Context, id string) (*domain.Batch, error) {
	return &domain.Batch{ID: id, UserID: "u1"}, nil
}
func (m *restartBatchRepoMock) Update(ctx context.Context, b *domain.Batch) error { return nil }
func (m *restartBatchRepoMock) List(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) {
	return nil, 0, nil
}
func (m *restartBatchRepoMock) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) {
	return &ports.AdminStatsWithQueues{}, nil
}

func TestHandleAdminRestartBatch_FailedOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jobs := []*domain.Job{
		{ID: "j1", BatchID: "b1", RowNumber: 1, Status: domain.JobStatusFailed},
		{ID: "j2", BatchID: "b1", RowNumber: 2, Status: domain.JobStatusCompleted},
		{ID: "j3", BatchID: "b1", RowNumber: 3, Status: domain.JobStatusFailed},
	}
	mj := &restartJobRepoMock{jobs: jobs}
	mp := &restartProducerMock{}
	mb := &restartBatchRepoMock{}
	deps := &di.Deps{JobRepo: mj, Producer: mp, BatchRepo: mb}

	body := `{"restartMode":"failed_only","force":false}`
	req := httptest.NewRequest("POST", "/admin/batches/b1/restart", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "b1"}}

	HandleAdminRestartBatch(c, deps)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d, body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	restarted, ok := resp["restarted"].(float64)
	if !ok || int(restarted) != 2 {
		t.Fatalf("expected restarted=2, got %v", resp["restarted"])
	}
	skipped, ok := resp["skipped"].(float64)
	if !ok || int(skipped) != 1 {
		t.Fatalf("expected skipped=1, got %v", resp["skipped"])
	}

	if len(mp.published) != 2 {
		t.Fatalf("expected 2 published events, got %d", len(mp.published))
	}
	if mj.updated == nil || len(mj.updated) != 2 {
		t.Fatalf("expected 2 updated job statuses, got %v", mj.updated)
	}
}
