package batch

import (
	"context"
	"errors"
	"testing"

	"github.com/ikermy/Bulk/internal/billing"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/ports"
)

// spyProducer records Publish calls
type spyProducer struct {
	Calls     int
	LastTopic string
	LastMsg   any
}

func (s *spyProducer) Publish(ctx context.Context, topic string, key []byte, msg any) error {
	s.Calls++
	s.LastTopic = topic
	s.LastMsg = msg
	return nil
}
func (s *spyProducer) Close() error { return nil }

// billing stub that returns error on RefundTransactions
type refundErrBilling struct{}

func (r *refundErrBilling) Quote(ctx context.Context, user string, count int) (*billing.QuoteResponse, error) {
	return nil, nil
}
func (r *refundErrBilling) BlockBatch(ctx context.Context, user string, count int, batchID string) (*billing.BlockBatchResponse, error) {
	return &billing.BlockBatchResponse{TransactionIDs: []string{}}, nil
}
func (r *refundErrBilling) RefundTransactions(ctx context.Context, user string, transactionIDs []string, batchID string) error {
	return errors.New("refund failed")
}

// billing stub that returns error on Quote
type quoteErrBilling struct{}

func (q *quoteErrBilling) Quote(ctx context.Context, user string, count int) (*billing.QuoteResponse, error) {
	return nil, errors.New("boom")
}
func (q *quoteErrBilling) BlockBatch(ctx context.Context, user string, count int, batchID string) (*billing.BlockBatchResponse, error) {
	return &billing.BlockBatchResponse{TransactionIDs: []string{}}, nil
}
func (q *quoteErrBilling) RefundTransactions(ctx context.Context, user string, transactionIDs []string, batchID string) error {
	return nil
}

func TestPublishStatus_CallsProducer(t *testing.T) {
	b := &domain.Batch{ID: "bpub", TotalRows: 10, CompletedCount: 2, FailedCount: 1, Status: domain.BatchStatusProcessing}
	mb := &mockBatchRepo{b: b}
	prod := &spyProducer{}
	mgr := NewBatchManager(mb, nil, nil, nil, prod, "bulk.status")

	mgr.publishStatus(context.Background(), b)

	if prod.Calls == 0 {
		t.Fatalf("expected producer.Publish to be called")
	}
	if prod.LastTopic != "bulk.status" {
		t.Fatalf("expected topic bulk.status, got %s", prod.LastTopic)
	}
}

func TestSetCancelled_BillingRefundError(t *testing.T) {
	// prepare batch with stored tx ids
	b := &domain.Batch{ID: "bcancel", Status: domain.BatchStatusReady, BillingTransactionIDs: []string{"tx1"}}
	mb := &mockBatchRepo{b: b}

	// job repo: one refunded, one pending
	jr := &mockJobRepo{jobs: []*domain.Job{{ID: "j1", Status: domain.JobStatusRefunded}, {ID: "j2", Status: domain.JobStatusPending}}}

	// create manager with billing client that returns error for RefundTransactions
	mgr := NewBatchManager(mb, jr, nil, nil, nil, "")

	// replace mgr.billing with an implementation that errors on RefundTransactions
	mgr.billing = &refundErrBilling{}
	refunded, err := mgr.SetCancelled(context.Background(), "bcancel")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refunded != 1 {
		t.Fatalf("expected refunded count 1, got %d", refunded)
	}
	if mb.b.BillingTransactionIDs == nil || len(mb.b.BillingTransactionIDs) == 0 {
		t.Fatalf("expected billing tx ids to remain when refund fails")
	}
}

func TestFinalizeAfterUpload_BillingQuoteError(t *testing.T) {
	mb := &mockBatchRepo{b: &domain.Batch{ID: "bqe", Status: domain.BatchStatusPending}}
	// create billing client that returns error on Quote
	mgr := NewBatchManager(mb, &mockJobRepo{}, &quoteErrBilling{}, nil, nil, "")
	if err := mgr.FinalizeAfterUpload(context.Background(), "bqe", 5, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mb.b.Status != BatchStatusReady {
		t.Fatalf("expected ready when billing quote errors, got %s", mb.b.Status)
	}
}

// errJobRepo returns error on GetByBatch to exercise error path
type errJobRepo struct{}
func (e *errJobRepo) Create(ctx context.Context, j *domain.Job) error { return nil }
func (e *errJobRepo) GetByBatch(ctx context.Context, batchID string) ([]*domain.Job, error) { return nil, errors.New("job repo error") }
func (e *errJobRepo) UpdateStatus(ctx context.Context, jobID string, status string) error { return nil }
func (e *errJobRepo) UpdateBillingTransactionID(ctx context.Context, jobID string, txID *string) error { return nil }
func (e *errJobRepo) UpdateStatusWithResult(ctx context.Context, jobID string, status string, result ports.JobResult) error { return nil }
func (e *errJobRepo) GetResultsByBatch(ctx context.Context, batchID string) ([]*ports.JobResult, error) { return nil, nil }

func TestOnJobStatusChange_JobRepoError(t *testing.T) {
	mb := &mockBatchRepo{b: &domain.Batch{ID: "bx", Status: domain.BatchStatusProcessing}}
	jr := &errJobRepo{}
	mgr := NewBatchManager(mb, jr, nil, nil, nil, "")
	if err := mgr.OnJobStatusChange(context.Background(), "bx"); err == nil {
		t.Fatalf("expected error when job repo fails")
	}
}


