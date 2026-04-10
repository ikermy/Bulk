package testutil

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/ikermy/Bulk/internal/billing"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/ports"
	pkgval "github.com/ikermy/Bulk/pkg/validation"
	"github.com/stretchr/testify/require"
)

func TestMocks_Defaults(t *testing.T) {
	ctx := context.Background()

	mb := &MockBatchRepo{}
	require.NoError(t, mb.Create(ctx, nil))
	b, err := mb.GetByID(ctx, "x")
	require.NoError(t, err)
	require.Nil(t, b)
	require.NoError(t, mb.Update(ctx, nil))
	l, n, err := mb.List(ctx, ports.BatchFilter{})
	require.NoError(t, err)
	require.Nil(t, l)
	require.Equal(t, 0, n)
	stats, err := mb.AdminStats(ctx, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, stats)

	mj := &MockJobRepo{}
	require.NoError(t, mj.Create(ctx, nil))
	_, err = mj.GetByBatch(ctx, "b")
	require.NoError(t, err)
	require.NoError(t, mj.UpdateStatus(ctx, "j", "s"))
	_, err = mj.GetResultsByBatch(ctx, "b")
	require.NoError(t, err)
	require.NoError(t, mj.UpdateBillingTransactionID(ctx, "j", nil))
	require.NoError(t, mj.UpdateStatusWithResult(ctx, "j", "s", ports.JobResult{}))

	mbill := &MockBilling{}
	_, err = mbill.Quote(ctx, "u", 1)
	require.NoError(t, err)
	_, err = mbill.BlockBatch(ctx, "u", 1, "b")
	require.NoError(t, err)
	require.NoError(t, mbill.RefundTransactions(ctx, "u", []string{"t"}, "b"))

	mp := &MockProducer{}
	require.NoError(t, mp.Publish(ctx, "t", nil, "m"))
	require.NoError(t, mp.Close())

	ms := &MockStorage{}
	_, err = ms.Save("k", strings.NewReader("x"))
	require.NoError(t, err)
	_, err = ms.Presign("k", time.Minute)
	require.NoError(t, err)
	_, err = ms.PublicURL("k")
	require.NoError(t, err)

	mv := &MockValidator{}
	res, err := mv.ValidateRow(ctx, map[string]string{}, "rev")
	require.NoError(t, err)
	require.Equal(t, true, res.Valid)

	// also test reading from Save default returned reader doesn't exist (nil) -> no read
	r, err := ms.Save("k2", strings.NewReader("x"))
	require.NoError(t, err)
	_ = r
	_ = ioutil.NopCloser(strings.NewReader("x"))

	_ = pkgval.ValidationResult{}
}

// TestMockBatchRepo_WithFns проверяет ветку с установленными Fn-функциями.
func TestMockBatchRepo_WithFns(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("batch error")

	mb := &MockBatchRepo{
		CreateFn: func(ctx context.Context, b *domain.Batch) error { return wantErr },
		GetByIDFn: func(ctx context.Context, id string) (*domain.Batch, error) {
			return &domain.Batch{ID: id}, nil
		},
		UpdateFn: func(ctx context.Context, b *domain.Batch) error { return wantErr },
		ListFn: func(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) {
			return []*domain.Batch{{ID: "b1"}}, 1, nil
		},
		AdminStatsFn: func(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) {
			return &ports.AdminStatsWithQueues{AdminStats: ports.AdminStats{BatchesCreated: 5}}, nil
		},
	}

	require.ErrorIs(t, mb.Create(ctx, nil), wantErr)

	got, err := mb.GetByID(ctx, "myid")
	require.NoError(t, err)
	require.Equal(t, "myid", got.ID)

	require.ErrorIs(t, mb.Update(ctx, &domain.Batch{}), wantErr)

	list, total, err := mb.List(ctx, ports.BatchFilter{})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, list, 1)

	stats, err := mb.AdminStats(ctx, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 5, stats.BatchesCreated)
}

// TestMockJobRepo_WithFns проверяет ветку с установленными Fn-функциями.
func TestMockJobRepo_WithFns(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("job error")
	txID := "tx-abc"

	mj := &MockJobRepo{
		CreateFn: func(ctx context.Context, j *domain.Job) error { return wantErr },
		GetByBatchFn: func(ctx context.Context, batchID string) ([]*domain.Job, error) {
			return []*domain.Job{{ID: "j1"}}, nil
		},
		UpdateStatusFn: func(ctx context.Context, jobID string, status string) error { return wantErr },
		GetResultsByBatchFn: func(ctx context.Context, batchID string) ([]*ports.JobResult, error) {
			return []*ports.JobResult{{JobID: "j1"}}, nil
		},
		UpdateBillingTransactionIDFn: func(ctx context.Context, jobID string, txID *string) error { return wantErr },
		UpdateStatusWithResultFn: func(ctx context.Context, jobID string, status string, result ports.JobResult) error {
			return wantErr
		},
	}

	require.ErrorIs(t, mj.Create(ctx, nil), wantErr)

	jobs, err := mj.GetByBatch(ctx, "bid")
	require.NoError(t, err)
	require.Len(t, jobs, 1)

	require.ErrorIs(t, mj.UpdateStatus(ctx, "j1", "done"), wantErr)

	results, err := mj.GetResultsByBatch(ctx, "bid")
	require.NoError(t, err)
	require.Len(t, results, 1)

	require.ErrorIs(t, mj.UpdateBillingTransactionID(ctx, "j1", &txID), wantErr)
	require.ErrorIs(t, mj.UpdateStatusWithResult(ctx, "j1", "done", ports.JobResult{}), wantErr)
}

// TestMockProducer_WithFns проверяет ветки Publish и Close с установленными функциями.
func TestMockProducer_WithFns(t *testing.T) {
	ctx := context.Background()
	pubErr := errors.New("publish error")
	closeErr := errors.New("close error")

	mp := &MockProducer{
		PublishFn: func(ctx context.Context, topic string, key []byte, msg any) error { return pubErr },
		CloseFn:   func() error { return closeErr },
	}

	require.ErrorIs(t, mp.Publish(ctx, "topic", nil, "msg"), pubErr)
	require.ErrorIs(t, mp.Close(), closeErr)
}

// TestMockStorage_WithFns проверяет ветки с установленными Fn-функциями.
func TestMockStorage_WithFns(t *testing.T) {
	saveErr := errors.New("save error")
	presignErr := errors.New("presign error")
	pubErr := errors.New("public url error")

	ms := &MockStorage{
		SaveFn:      func(name string, r io.Reader) (string, error) { return "sid", saveErr },
		PresignFn:   func(name string, ttl time.Duration) (string, error) { return "url", presignErr },
		PublicURLFn: func(name string) (string, error) { return "http://cdn/f", pubErr },
	}

	sid, err := ms.Save("k", strings.NewReader("data"))
	require.Equal(t, "sid", sid)
	require.ErrorIs(t, err, saveErr)

	url, err := ms.Presign("k", time.Minute)
	require.Equal(t, "url", url)
	require.ErrorIs(t, err, presignErr)

	purl, err := ms.PublicURL("k")
	require.Equal(t, "http://cdn/f", purl)
	require.ErrorIs(t, err, pubErr)
}

// TestMockBilling_WithFns проверяет ветки с установленными Fn-функциями.
func TestMockBilling_WithFns(t *testing.T) {
	ctx := context.Background()
	quoteErr := errors.New("quote error")
	blockErr := errors.New("block error")
	refundErr := errors.New("refund error")

	mb := &MockBilling{
		QuoteFn: func(ctx context.Context, user string, count int) (*billing.QuoteResponse, error) {
			return &billing.QuoteResponse{UnitPrice: 1.5}, quoteErr
		},
		BlockBatchFn: func(ctx context.Context, user string, count int, batchID string) (*billing.BlockBatchResponse, error) {
			return &billing.BlockBatchResponse{TransactionIDs: []string{"tx1"}}, blockErr
		},
		RefundTransactionsFn: func(ctx context.Context, user string, transactionIDs []string, batchID string) error {
			return refundErr
		},
	}

	q, err := mb.Quote(ctx, "user1", 10)
	require.ErrorIs(t, err, quoteErr)
	require.InDelta(t, 1.5, q.UnitPrice, 0.001)

	blk, err := mb.BlockBatch(ctx, "user1", 10, "bid")
	require.ErrorIs(t, err, blockErr)
	require.Equal(t, []string{"tx1"}, blk.TransactionIDs)

	require.ErrorIs(t, mb.RefundTransactions(ctx, "user1", []string{"tx1"}, "bid"), refundErr)
}

// TestMockValidator_WithFn проверяет ветку с установленной ValidateRowFn.
func TestMockValidator_WithFn(t *testing.T) {
	ctx := context.Background()
	mv := &MockValidator{
		ValidateRowFn: func(ctx context.Context, fields map[string]string, revision string) (*pkgval.ValidationResult, error) {
			return &pkgval.ValidationResult{
				Valid: false,
				Errors: []pkgval.ValidationError{
					{Code: "INVALID_FORMAT", Message: "bad field"},
				},
			}, nil
		},
	}

	res, err := mv.ValidateRow(ctx, map[string]string{"passportNumber": "xxx"}, "v1")
	require.NoError(t, err)
	require.False(t, res.Valid)
	require.Len(t, res.Errors, 1)
	require.Equal(t, "INVALID_FORMAT", res.Errors[0].Code)
}

