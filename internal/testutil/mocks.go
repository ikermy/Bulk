package testutil

import (
	"context"
	"io"
	"time"

	"github.com/ikermy/Bulk/internal/billing"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/ports"
	pkgval "github.com/ikermy/Bulk/pkg/validation"
)

// Lightweight mocks with hookable functions used by tests across packages.
type MockBatchRepo struct {
	CreateFn     func(ctx context.Context, b *domain.Batch) error
	GetByIDFn    func(ctx context.Context, id string) (*domain.Batch, error)
	UpdateFn     func(ctx context.Context, b *domain.Batch) error
	ListFn       func(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error)
	AdminStatsFn func(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error)
}

func (m *MockBatchRepo) Create(ctx context.Context, b *domain.Batch) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, b)
	}
	return nil
}
func (m *MockBatchRepo) GetByID(ctx context.Context, id string) (*domain.Batch, error) {
	if m.GetByIDFn != nil {
		return m.GetByIDFn(ctx, id)
	}
	return nil, nil
}
func (m *MockBatchRepo) Update(ctx context.Context, b *domain.Batch) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, b)
	}
	return nil
}
func (m *MockBatchRepo) List(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, filter)
	}
	return nil, 0, nil
}
func (m *MockBatchRepo) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) {
	if m.AdminStatsFn != nil {
		return m.AdminStatsFn(ctx, from, to)
	}
	return &ports.AdminStatsWithQueues{}, nil
}

type MockJobRepo struct {
	CreateFn                        func(ctx context.Context, j *domain.Job) error
	GetByBatchFn                    func(ctx context.Context, batchID string) ([]*domain.Job, error)
	UpdateStatusFn                  func(ctx context.Context, jobID string, status string) error
	GetResultsByBatchFn             func(ctx context.Context, batchID string) ([]*ports.JobResult, error)
	UpdateBillingTransactionIDFn    func(ctx context.Context, jobID string, txID *string) error
	UpdateStatusWithResultFn        func(ctx context.Context, jobID string, status string, result ports.JobResult) error
}

func (m *MockJobRepo) Create(ctx context.Context, j *domain.Job) error {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, j)
	}
	return nil
}
func (m *MockJobRepo) GetByBatch(ctx context.Context, batchID string) ([]*domain.Job, error) {
	if m.GetByBatchFn != nil {
		return m.GetByBatchFn(ctx, batchID)
	}
	return nil, nil
}
func (m *MockJobRepo) UpdateStatus(ctx context.Context, jobID string, status string) error {
	if m.UpdateStatusFn != nil {
		return m.UpdateStatusFn(ctx, jobID, status)
	}
	return nil
}
func (m *MockJobRepo) GetResultsByBatch(ctx context.Context, batchID string) ([]*ports.JobResult, error) {
	if m.GetResultsByBatchFn != nil {
		return m.GetResultsByBatchFn(ctx, batchID)
	}
	return nil, nil
}
func (m *MockJobRepo) UpdateBillingTransactionID(ctx context.Context, jobID string, txID *string) error {
	if m.UpdateBillingTransactionIDFn != nil {
		return m.UpdateBillingTransactionIDFn(ctx, jobID, txID)
	}
	return nil
}
func (m *MockJobRepo) UpdateStatusWithResult(ctx context.Context, jobID string, status string, result ports.JobResult) error {
	if m.UpdateStatusWithResultFn != nil {
		return m.UpdateStatusWithResultFn(ctx, jobID, status, result)
	}
	return nil
}

type MockBilling struct {
	QuoteFn               func(ctx context.Context, user string, count int) (*billing.QuoteResponse, error)
	BlockBatchFn          func(ctx context.Context, user string, count int, batchID string) (*billing.BlockBatchResponse, error)
	RefundTransactionsFn  func(ctx context.Context, user string, transactionIDs []string, batchID string) error
}
func (m *MockBilling) Quote(ctx context.Context, user string, count int) (*billing.QuoteResponse, error) { if m.QuoteFn != nil { return m.QuoteFn(ctx, user, count) }; return &billing.QuoteResponse{}, nil }
func (m *MockBilling) BlockBatch(ctx context.Context, user string, count int, batchID string) (*billing.BlockBatchResponse, error) { if m.BlockBatchFn != nil { return m.BlockBatchFn(ctx, user, count, batchID) }; return &billing.BlockBatchResponse{}, nil }
func (m *MockBilling) RefundTransactions(ctx context.Context, user string, transactionIDs []string, batchID string) error { if m.RefundTransactionsFn != nil { return m.RefundTransactionsFn(ctx, user, transactionIDs, batchID) }; return nil }

type MockProducer struct { PublishFn func(ctx context.Context, topic string, key []byte, msg any) error; CloseFn func() error }
func (m *MockProducer) Publish(ctx context.Context, topic string, key []byte, msg any) error { if m.PublishFn != nil { return m.PublishFn(ctx, topic, key, msg) }; return nil }
func (m *MockProducer) Close() error { if m.CloseFn != nil { return m.CloseFn() }; return nil }

type MockStorage struct { SaveFn func(name string, r io.Reader) (string, error); PresignFn func(name string, ttl time.Duration) (string, error); PublicURLFn func(name string) (string, error) }
func (m *MockStorage) Save(name string, r io.Reader) (string, error) { if m.SaveFn != nil { return m.SaveFn(name, r) }; return "", nil }
func (m *MockStorage) Presign(name string, ttl time.Duration) (string, error) { if m.PresignFn != nil { return m.PresignFn(name, ttl) }; return "", nil }
func (m *MockStorage) PublicURL(name string) (string, error) { if m.PublicURLFn != nil { return m.PublicURLFn(name) }; return "", nil }

type MockValidator struct { ValidateRowFn func(ctx context.Context, fields map[string]string, revision string) (*pkgval.ValidationResult, error) }
func (m *MockValidator) ValidateRow(ctx context.Context, fields map[string]string, revision string) (*pkgval.ValidationResult, error) { if m.ValidateRowFn != nil { return m.ValidateRowFn(ctx, fields, revision) }; return &pkgval.ValidationResult{Valid: true}, nil }
