package bulk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/ikermy/Bulk/internal/billing"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/ports"
)

// mockBatchRepo records Create calls
type mockBatchRepo struct {
	mu      sync.Mutex
	created []*domain.Batch
	updated []*domain.Batch
}

func (m *mockBatchRepo) Create(ctx context.Context, b *domain.Batch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created = append(m.created, b)
	return nil
}

func (m *mockBatchRepo) GetByID(ctx context.Context, id string) (*domain.Batch, error) {
	// return a minimal batch object so that service can persist transaction ids in Update
	return &domain.Batch{ID: id}, nil
}

func (m *mockBatchRepo) Update(ctx context.Context, b *domain.Batch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updated = append(m.updated, b)
	return nil
}

// implement List for compatibility with updated interface
func (m *mockBatchRepo) List(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) {
	return nil, 0, nil
}

// AdminStats mock implementation
func (m *mockBatchRepo) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) {
	return &ports.AdminStatsWithQueues{}, nil
}

// mockJobRepo records Create and UpdateStatus calls and can return jobs
type mockJobRepo struct {
	mu      sync.Mutex
	created int
	updated map[string]string
	jobs    []*domain.Job
	results []*ports.JobResult
}

func (m *mockJobRepo) Create(ctx context.Context, j *domain.Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.created++
	return nil
}

func (m *mockJobRepo) GetByBatch(ctx context.Context, batchID string) ([]*domain.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobs, nil
}

func (m *mockJobRepo) UpdateStatus(ctx context.Context, jobID string, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updated == nil {
		m.updated = map[string]string{}
	}
	m.updated[jobID] = status
	return nil
}

func (m *mockJobRepo) UpdateBillingTransactionID(ctx context.Context, jobID string, txID *string) error {
	// no-op for tests
	return nil
}

// UpdateStatusWithResult added for interface compatibility in tests
func (m *mockJobRepo) UpdateStatusWithResult(ctx context.Context, jobID string, status string, result ports.JobResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updated == nil {
		m.updated = map[string]string{}
	}
	m.updated[jobID] = status
	return nil
}

func (m *mockJobRepo) GetResultsByBatch(ctx context.Context, batchID string) ([]*ports.JobResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.results, nil
}

// (mockBilling omitted — not needed for current tests)

// simple mockBilling implementing needed methods for tests
type mockBillingImpl struct{
	allowed int
	RefundFunc func(ctx context.Context, user string, transactionIDs []string, batchID string) error
}
func (m *mockBillingImpl) Quote(ctx context.Context, user string, count int) (*billing.QuoteResponse, error) {
	return &billing.QuoteResponse{CanProcess: m.allowed>0, Requested: count, AllowedTotal: m.allowed}, nil
}
func (m *mockBillingImpl) BlockBatch(ctx context.Context, user string, count int, batchID string) (*billing.BlockBatchResponse, error) {
	txs := []string{}
	for i:=0;i<count;i++ { txs = append(txs, fmt.Sprintf("tx-%d", i)) }
	return &billing.BlockBatchResponse{TransactionIDs: txs}, nil
}
func (m *mockBillingImpl) RefundTransactions(ctx context.Context, user string, transactionIDs []string, batchID string) error {
	if m.RefundFunc != nil { return m.RefundFunc(ctx,user,transactionIDs,batchID) }
	return nil
}

func TestCreateBatchFromFile_NoValidator(t *testing.T) {
	csv := "first_name,last_name\nval1a,val1b\nval2a,val2b\nval3a,val3b\n"

	batchRepo := &mockBatchRepo{}
	jobRepo := &mockJobRepo{}

	svc := NewService(batchRepo, jobRepo, nil, nil, nil, nil)

	res, err := svc.CreateBatchFromFile(context.Background(), strings.NewReader(csv), "rev1")
	if err != nil {
		t.Fatalf("CreateBatchFromFile error: %v", err)
	}
	if res.TotalRows != 3 {
		t.Fatalf("expected total 3, got %d", res.TotalRows)
	}
	if res.ValidRows != 3 {
		t.Fatalf("expected valid 3, got %d", res.ValidRows)
	}
	if res.InvalidRows != 0 {
		t.Fatalf("expected invalid 0, got %d", res.InvalidRows)
	}

	if jobRepo.created != 3 {
		t.Fatalf("expected 3 jobs created, got %d", jobRepo.created)
	}
}

func TestConfirmBatch_PublishAndUpdate(t *testing.T) {
	// prepare mock jobs
	jobs := []*domain.Job{}
	for i := 0; i < 5; i++ {
		jobs = append(jobs, &domain.Job{ID: fmt.Sprintf("job-%d", i), BatchID: "b1", RowNumber: i + 1})
	}

	jobRepo := &mockJobRepo{jobs: jobs}
	batchRepo := &mockBatchRepo{}

	svc := NewService(batchRepo, jobRepo, nil, nil, nil, nil)

	published := 0
	publishFunc := func(ctx context.Context, topic string, key []byte, msg any) error {
		published++
		// validate message structure according to п.7.2 ТЗ (no transaction case)
		// msg is expected to be a map[string]any
		var m map[string]any
		// try to handle both map and arbitrary types by marshalling
		b, _ := json.Marshal(msg)
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("failed to unmarshal published msg: %v", err)
		}
		if m["batchId"] != "b1" {
			t.Fatalf("expected batchId b1, got %v", m["batchId"])
		}
		if m["jobId"] == nil {
			t.Fatalf("expected jobId present")
		}
		// buildId must equal jobId
		if m["buildId"] != m["jobId"] {
			t.Fatalf("expected buildId == jobId, got buildId=%v jobId=%v", m["buildId"], m["jobId"])
		}
		if _, ok := m["rowNumber"]; !ok {
			t.Fatalf("expected rowNumber present")
		}
		if _, ok := m["fields"]; !ok {
			t.Fatalf("expected fields present")
		}
		if bp, ok := m["billingPreApproved"]; ok {
			if bp.(bool) != false {
				t.Fatalf("expected billingPreApproved false, got %v", bp)
			}
		}
		if _, ok := m["transactionId"]; ok {
			t.Fatalf("did not expect transactionId in this scenario")
		}
		return nil
	}

	blockFunc := func(ctx context.Context, user string, count int, batchID string) (interface{}, error) {
		return nil, nil
	}

	queued, err := svc.ConfirmBatch(context.Background(), "b1", 3, blockFunc, publishFunc)
	if err != nil {
		t.Fatalf("ConfirmBatch error: %v", err)
	}
	if queued != 3 {
		t.Fatalf("expected queued 3, got %d", queued)
	}
	if published != 3 {
		t.Fatalf("expected published 3, got %d", published)
	}
	if len(jobRepo.updated) != 3 {
		t.Fatalf("expected 3 updates, got %d", len(jobRepo.updated))
	}
}

func TestConfirmBatch_WithTransaction_PublishesTransaction(t *testing.T) {
	// prepare mock jobs
	jobs := []*domain.Job{}
	for i := 0; i < 5; i++ {
		jobs = append(jobs, &domain.Job{ID: fmt.Sprintf("job-%d", i), BatchID: "b1", RowNumber: i + 1})
	}

	jobRepo := &mockJobRepo{jobs: jobs}
	batchRepo := &mockBatchRepo{}

	svc := NewService(batchRepo, jobRepo, nil, nil, nil, nil)

	published := 0
	publishFunc := func(ctx context.Context, topic string, key []byte, msg any) error {
		published++
		var m map[string]any
		b, _ := json.Marshal(msg)
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("failed to unmarshal published msg: %v", err)
		}
		// transaction fields must be present
		if tx, ok := m["transactionId"]; !ok || tx == nil {
			t.Fatalf("expected transactionId present, got %v", tx)
		}
		if bt, ok := m["billingTransactionId"]; !ok || bt == nil {
			t.Fatalf("expected billingTransactionId present, got %v", bt)
		}
		if bp, ok := m["billingPreApproved"]; !ok || bp.(bool) != true {
			t.Fatalf("expected billingPreApproved true, got %v", bp)
		}
		// buildId must equal jobId
		if m["buildId"] != m["jobId"] {
			t.Fatalf("expected buildId == jobId, got buildId=%v jobId=%v", m["buildId"], m["jobId"])
		}
		return nil
	}

	blockFunc := func(ctx context.Context, user string, count int, batchID string) (interface{}, error) {
		// return transaction IDs equal to count
		txs := []string{}
		for i := 0; i < count; i++ {
			txs = append(txs, uuid.New().String())
		}
		return &billing.BlockBatchResponse{TransactionIDs: txs}, nil
	}

	queued, err := svc.ConfirmBatch(context.Background(), "b1", 3, blockFunc, publishFunc)
	if err != nil {
		t.Fatalf("ConfirmBatch error: %v", err)
	}
	if queued != 3 {
		t.Fatalf("expected queued 3, got %d", queued)
	}
	if published != 3 {
		t.Fatalf("expected published 3, got %d", published)
	}
	if len(jobRepo.updated) != 3 {
		t.Fatalf("expected 3 updates, got %d", len(jobRepo.updated))
	}
}

func TestConfirmBatch_PersistsTransactionIDs(t *testing.T) {
	// prepare mock jobs
	jobs := []*domain.Job{}
	for i := 0; i < 3; i++ {
		jobs = append(jobs, &domain.Job{ID: fmt.Sprintf("job-%d", i), BatchID: "b1", RowNumber: i + 1})
	}

	jobRepo := &mockJobRepo{jobs: jobs}
	batchRepo := &mockBatchRepo{}

	svc := NewService(batchRepo, jobRepo, nil, nil, nil, nil)

	blockFunc := func(ctx context.Context, user string, count int, batchID string) (interface{}, error) {
		txs := []string{}
		for i := 0; i < count; i++ {
			txs = append(txs, uuid.New().String())
		}
		return &billing.BlockBatchResponse{TransactionIDs: txs}, nil
	}
	publishFunc := func(ctx context.Context, topic string, key []byte, msg any) error { return nil }

	queued, err := svc.ConfirmBatch(context.Background(), "b1", 3, blockFunc, publishFunc)
	if err != nil {
		t.Fatalf("ConfirmBatch error: %v", err)
	}
	if queued != 3 {
		t.Fatalf("expected queued 3, got %d", queued)
	}
	if len(batchRepo.updated) == 0 {
		t.Fatalf("expected batchRepo.Update to be called at least once")
	}
	// last update should contain BillingTransactionIDs
	last := batchRepo.updated[len(batchRepo.updated)-1]
	if len(last.BillingTransactionIDs) != 3 {
		t.Fatalf("expected 3 persisted transaction ids, got %d", len(last.BillingTransactionIDs))
	}
}

func TestConfirmBatch_PublishError_TriggersRefundAndCancel(t *testing.T) {
	// prepare mock jobs
	jobs := []*domain.Job{}
	for i := 0; i < 3; i++ {
		jobs = append(jobs, &domain.Job{ID: fmt.Sprintf("job-%d", i), BatchID: "b1", RowNumber: i + 1})
	}

	jobRepo := &mockJobRepo{jobs: jobs}
	batchRepo := &mockBatchRepo{}

	refunded := false
	mbilling := &mockBillingImpl{allowed: 3, RefundFunc: func(ctx context.Context, user string, transactionIDs []string, batchID string) error {
		refunded = true
		return nil
	}}

	svc := NewService(batchRepo, jobRepo, nil, mbilling, nil, nil)

	// blockFunc returns valid UUID tx ids so they will be assigned and trigger refund on publish error
	blockFunc := func(ctx context.Context, user string, count int, batchID string) (interface{}, error) {
		txs := []string{}
		for i := 0; i < count; i++ {
			txs = append(txs, uuid.New().String())
		}
		return &billing.BlockBatchResponse{TransactionIDs: txs}, nil
	}

	// publishFunc returns error to simulate failure
	publishFunc := func(ctx context.Context, topic string, key []byte, msg any) error {
		return fmt.Errorf("publish failed")
	}

	queued, err := svc.ConfirmBatch(context.Background(), "b1", 3, blockFunc, publishFunc)
	if err != nil {
		t.Fatalf("ConfirmBatch error: %v", err)
	}
	// since publishing failed, queued should be 0
	if queued != 0 {
		t.Fatalf("expected queued 0 on publish failure, got %d", queued)
	}
	if !refunded {
		t.Fatalf("expected refund to be attempted")
	}
}

