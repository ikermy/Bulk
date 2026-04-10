package batch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ikermy/Bulk/internal/billing"
	bill "github.com/ikermy/Bulk/internal/billing"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/history"
	"github.com/ikermy/Bulk/internal/ports"
)

// simple mocks
type mockBatchRepo struct {
	b *domain.Batch
}

func (m *mockBatchRepo) Create(ctx context.Context, b *domain.Batch) error { return nil }
func (m *mockBatchRepo) GetByID(ctx context.Context, id string) (*domain.Batch, error) {
	if m.b == nil {
		return nil, errors.New("not found")
	}
	return m.b, nil
}
func (m *mockBatchRepo) Update(ctx context.Context, b *domain.Batch) error { m.b = b; return nil }

// AdminStats mock implementation for tests
func (m *mockBatchRepo) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) {
	return &ports.AdminStatsWithQueues{}, nil
}

// implement List for interface compatibility in tests
func (m *mockBatchRepo) List(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) {
	if m.b == nil {
		return nil, 0, nil
	}
	return []*domain.Batch{m.b}, 1, nil
}

type mockJobRepo struct {
	jobs []*domain.Job
}

func (m *mockJobRepo) Create(ctx context.Context, j *domain.Job) error { return nil }
func (m *mockJobRepo) GetByBatch(ctx context.Context, batchID string) ([]*domain.Job, error) {
	return m.jobs, nil
}
func (m *mockJobRepo) UpdateStatus(ctx context.Context, jobID string, status string) error {
	return nil
}

// add UpdateBillingTransactionID to satisfy new interface
func (m *mockJobRepo) UpdateBillingTransactionID(ctx context.Context, jobID string, txID *string) error {
	return nil
}

// UpdateStatusWithResult added for interface compatibility in tests
func (m *mockJobRepo) UpdateStatusWithResult(ctx context.Context, jobID string, status string, result ports.JobResult) error {
	return nil
}

func (m *mockJobRepo) GetResultsByBatch(ctx context.Context, batchID string) ([]*ports.JobResult, error) {
	// return empty results for tests
	return nil, nil
}

type mockBilling struct {
	allowed int
}

func (m *mockBilling) Quote(ctx context.Context, user string, count int) (*billing.QuoteResponse, error) {
	return &bill.QuoteResponse{CanProcess: m.allowed > 0, Requested: count, AllowedTotal: m.allowed}, nil
}
func (m *mockBilling) BlockBatch(ctx context.Context, user string, count int, batchID string) (*billing.BlockBatchResponse, error) {
	return &bill.BlockBatchResponse{TransactionIDs: []string{}}, nil
}

func (m *mockBilling) RefundTransactions(ctx context.Context, user string, transactionIDs []string, batchID string) error {
	return nil
}

type stubProducer struct{}

func (s *stubProducer) Publish(ctx context.Context, topic string, key []byte, msg any) error {
	return nil
}
func (s *stubProducer) Close() error { return nil }

// use real history.Tagger backed by stubProducer

func TestFinalizeAfterUpload_validationErrors(t *testing.T) {
	mb := &mockBatchRepo{b: &domain.Batch{ID: "b1", Status: domain.BatchStatusPending, ValidRows: 0}}
	mgr := NewBatchManager(mb, &mockJobRepo{}, nil, nil, nil, "")
	if err := mgr.FinalizeAfterUpload(context.Background(), "b1", 5, 2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mb.b.Status != BatchStatusValidationErrors {
		t.Fatalf("expected validation_errors, got %s", mb.b.Status)
	}
}

func TestFinalizeAfterUpload_partialAvailable(t *testing.T) {
	mb := &mockBatchRepo{b: &domain.Batch{ID: "b2", Status: domain.BatchStatusPending, ValidRows: 0}}
	billing := &mockBilling{allowed: 3}
	prod := &stubProducer{}
	hist := history.NewTagger(prod, "test.history")
	mgr := NewBatchManager(mb, &mockJobRepo{}, billing, hist, prod, "")
	if err := mgr.FinalizeAfterUpload(context.Background(), "b2", 5, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mb.b.Status != BatchStatusPartialAvailable {
		t.Fatalf("expected partial_available, got %s", mb.b.Status)
	}
	if mb.b.ApprovedCount != 3 {
		t.Fatalf("expected approved_count 3, got %d", mb.b.ApprovedCount)
	}
}

func TestFinalizeAfterUpload_ready(t *testing.T) {
	mb := &mockBatchRepo{b: &domain.Batch{ID: "b3", Status: domain.BatchStatusPending, ValidRows: 0}}
	billing := &mockBilling{allowed: 10}
	mgr := NewBatchManager(mb, &mockJobRepo{}, billing, nil, nil, "")
	if err := mgr.FinalizeAfterUpload(context.Background(), "b3", 5, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mb.b.Status != BatchStatusReady {
		t.Fatalf("expected ready, got %s", mb.b.Status)
	}
}

func TestOnJobStatusChange_completion(t *testing.T) {
	jobs := []*domain.Job{
		{ID: "j1", Status: domain.JobStatusCompleted},
		{ID: "j2", Status: domain.JobStatusFailed},
	}
	mb := &mockBatchRepo{b: &domain.Batch{ID: "b4", Status: domain.BatchStatusProcessing}}
	jr := &mockJobRepo{jobs: jobs}
	mgr := NewBatchManager(mb, jr, nil, nil, nil, "")
	if err := mgr.OnJobStatusChange(context.Background(), "b4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mb.b.Status != BatchStatusCompleted {
		t.Fatalf("expected completed, got %s", mb.b.Status)
	}
	if mb.b.CompletedCount != 2 {
		t.Fatalf("expected completed_count 2, got %d", mb.b.CompletedCount)
	}
	if mb.b.FailedCount != 1 {
		t.Fatalf("expected failed_count 1, got %d", mb.b.FailedCount)
	}
	if mb.b.CompletedAt == nil {
		t.Fatalf("expected CompletedAt to be set")
	}
}

func TestSetProcessingAndCancelled(t *testing.T) {
	mb := &mockBatchRepo{b: &domain.Batch{ID: "b5", Status: domain.BatchStatusReady}}
	mgr := NewBatchManager(mb, &mockJobRepo{}, nil, nil, nil, "")
	if err := mgr.SetProcessing(context.Background(), "b5"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if mb.b.Status != BatchStatusProcessing {
		t.Fatalf("expected processing, got %s", mb.b.Status)
	}
	refunded, err := mgr.SetCancelled(context.Background(), "b5")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if mb.b.Status != BatchStatusCancelled {
		t.Fatalf("expected cancelled, got %s", mb.b.Status)
	}
	if refunded < 0 {
		t.Fatalf("expected non-negative refunded count, got %d", refunded)
	}
}
