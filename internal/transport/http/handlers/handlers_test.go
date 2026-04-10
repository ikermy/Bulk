package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ikermy/Bulk/internal/billing"
	"github.com/ikermy/Bulk/internal/di"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/ports"
	"github.com/ikermy/Bulk/internal/usecase/bulk"
	"github.com/ikermy/Bulk/internal/validation"
)

// simple mocks implementing ports
type mockBatchRepo struct{}

func (m *mockBatchRepo) Create(ctx context.Context, b *domain.Batch) error { return nil }
func (m *mockBatchRepo) GetByID(ctx context.Context, id string) (*domain.Batch, error) {
	return &domain.Batch{ID: id}, nil
}
func (m *mockBatchRepo) Update(ctx context.Context, b *domain.Batch) error { return nil }
func (m *mockBatchRepo) List(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) { return nil, 0, nil }

// AdminStats mock implementation
func (m *mockBatchRepo) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) {
	return &ports.AdminStatsWithQueues{}, nil
}

type mockJobRepo struct{ jobs []*domain.Job }

func (m *mockJobRepo) Create(ctx context.Context, j *domain.Job) error {
	m.jobs = append(m.jobs, j)
	return nil
}
func (m *mockJobRepo) GetByBatch(ctx context.Context, batchID string) ([]*domain.Job, error) {
	return m.jobs, nil
}
func (m *mockJobRepo) UpdateStatus(ctx context.Context, jobID string, status string) error {
	return nil
}

func (m *mockJobRepo) UpdateBillingTransactionID(ctx context.Context, jobID string, txID *string) error {
	return nil
}

// UpdateStatusWithResult added for interface compatibility in tests
func (m *mockJobRepo) UpdateStatusWithResult(ctx context.Context, jobID string, status string, result ports.JobResult) error {
	return nil
}

func (m *mockJobRepo) GetResultsByBatch(ctx context.Context, batchID string) ([]*ports.JobResult, error) {
	return nil, nil
}

type mockBilling struct{}

func (m *mockBilling) Quote(ctx context.Context, user string, count int) (*billing.QuoteResponse, error) {
	return &billing.QuoteResponse{CanProcess: true, Requested: count, AllowedTotal: count}, nil
}
func (m *mockBilling) BlockBatch(ctx context.Context, user string, count int, batchID string) (*billing.BlockBatchResponse, error) {
	return &billing.BlockBatchResponse{TransactionIDs: []string{"tx1"}}, nil
}

func (m *mockBilling) RefundTransactions(ctx context.Context, user string, transactionIDs []string, batchID string) error {
	return nil
}

type mockProducer struct{ published int }

func (m *mockProducer) Publish(ctx context.Context, topic string, key []byte, msg any) error {
	m.published++
	return nil
}

func (m *mockProducer) Close() error { return nil }

func createDepsForTest() *di.Deps {
	batchRepo := &mockBatchRepo{}
	jobRepo := &mockJobRepo{}
	billingClient := (*mockBilling)(nil) // nil billing to trigger default ready path
	producer := &mockProducer{}
	// use nil validator to avoid external BFF calls in unit tests
	var validator *validation.BFFValidator = nil
	svc := bulk.NewService(batchRepo, jobRepo, validator, nil, nil, nil)
	return &di.Deps{Logger: nil, BatchRepo: batchRepo, JobRepo: jobRepo, BillingClient: billingClient, Producer: producer, Service: svc}
}

func TestHandleUpload_ReturnsReady(t *testing.T) {
	// prepare multipart form with file
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, _ := w.CreateFormFile("file", "data.csv")
	fw.Write([]byte("first_name,last_name\nval1,val2\n"))
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rw := httptest.NewRecorder()

	// gin context
	c, _ := gin.CreateTestContext(rw)
	c.Request = req
	// include revision in form for required validation (Test constructs multipart without revision)
	req.Form = map[string][]string{"revision": {"test-rev"}}

	deps := createDepsForTest()
	HandleUpload(c, deps)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rw.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if resp["status"] != "ready" {
		t.Fatalf("expected status ready, got %v", resp["status"])
	}
}

func TestHandleConfirm_QueuesJobs(t *testing.T) {
	// prepare request body
	body := strings.NewReader(`{"action":"generate_all"}`)
	req := httptest.NewRequest(http.MethodPost, "/batch/b1/confirm", body)
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(rw)
	c.Request = req
	// set param id
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "b1"})

	// prepare deps: job repo with some jobs
	jobRepo := &mockJobRepo{jobs: []*domain.Job{{ID: "j1", BatchID: "b1", RowNumber: 1}, {ID: "j2", BatchID: "b1", RowNumber: 2}}}
	batchRepo := &mockBatchRepo{}
	producer := &mockProducer{}
	// create service using mocks
	validator := validation.NewBFFValidator("", 0, "")
	svc := bulk.NewService(batchRepo, jobRepo, validator, nil, producer, nil)
	deps := &di.Deps{Logger: nil, BatchRepo: batchRepo, JobRepo: jobRepo, BillingClient: nil, Producer: producer, Service: svc}

	HandleConfirm(c, deps)

	if rw.Code != http.StatusAccepted {
		t.Fatalf("expected 202 got %d", rw.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	// check estimatedTimeSeconds present and non-negative
	if v, ok := resp["estimatedTimeSeconds"]; !ok {
		t.Fatalf("estimatedTimeSeconds missing")
	} else {
		// json numbers decode into float64
		if f, ok := v.(float64); !ok {
			t.Fatalf("estimatedTimeSeconds has wrong type: %T", v)
		} else if f < 0 {
			t.Fatalf("estimatedTimeSeconds negative: %v", f)
		}
	}
}
