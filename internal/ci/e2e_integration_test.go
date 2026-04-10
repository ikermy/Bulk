//go:build integration
// +build integration

package ci

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	pg "github.com/ikermy/Bulk/internal/adapters/postgres"
	"github.com/ikermy/Bulk/internal/batch"
	"github.com/ikermy/Bulk/internal/db"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/kafka"
	"github.com/ikermy/Bulk/internal/logging"
	"github.com/ikermy/Bulk/internal/ports"
	"github.com/ikermy/Bulk/internal/usecase/bulk"
)

// wrapper types to adapt adapter postgres repositories to ports interfaces
type batchWrapper struct{ a *pg.BatchRepository }
type jobWrapper struct{ a *pg.JobRepository }

func (w *batchWrapper) Create(ctx context.Context, b *domain.Batch) error {
	pb := &pg.Batch{ID: b.ID, UserID: b.UserID, Status: string(b.Status), Revision: b.Revision, FileStorageID: b.FileStorageID, TotalRows: b.TotalRows, ValidRows: b.ValidRows, ApprovedCount: b.ApprovedCount, CompletedCount: b.CompletedCount, FailedCount: b.FailedCount}
	return w.a.Create(ctx, pb)
}
func (w *batchWrapper) GetByID(ctx context.Context, id string) (*domain.Batch, error) {
	ab, err := w.a.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return ab.ToDomain(), nil
}
func (w *batchWrapper) Update(ctx context.Context, b *domain.Batch) error {
	pb := &pg.Batch{ID: b.ID, UserID: b.UserID, Status: string(b.Status), Revision: b.Revision, FileStorageID: b.FileStorageID, TotalRows: b.TotalRows, ValidRows: b.ValidRows, ApprovedCount: b.ApprovedCount, CompletedCount: b.CompletedCount, FailedCount: b.FailedCount}
	return w.a.Update(ctx, pb)
}
func (w *batchWrapper) List(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) {
	return nil, 0, nil
}
func (w *batchWrapper) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) {
	return w.a.AdminStats(ctx, from, to)
}

func (w *jobWrapper) Create(ctx context.Context, j *domain.Job) error {
	pj := &pg.Job{ID: j.ID, BatchID: j.BatchID, RowNumber: j.RowNumber, Status: string(j.Status)}
	return w.a.Create(ctx, pj)
}
func (w *jobWrapper) GetByBatch(ctx context.Context, batchID string) ([]*domain.Job, error) {
	aj, err := w.a.GetByBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	var out []*domain.Job
	for _, jj := range aj {
		out = append(out, jj.ToDomain())
	}
	return out, nil
}
func (w *jobWrapper) UpdateStatus(ctx context.Context, jobID string, status string) error {
	return w.a.UpdateStatus(ctx, jobID, status)
}
func (w *jobWrapper) GetResultsByBatch(ctx context.Context, batchID string) ([]*ports.JobResult, error) {
	ar, err := w.a.GetResultsByBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	var out []*ports.JobResult
	for _, r := range ar {
		var buildId string
		if r.BuildID.Valid {
			buildId = r.BuildID.String
		}
		var urls string
		if r.BarcodeURLs.Valid {
			urls = r.BarcodeURLs.String
		}
		var errc string
		if r.ErrorCode.Valid {
			errc = r.ErrorCode.String
		}
		var errmsg string
		if r.ErrorMessage.Valid {
			errmsg = r.ErrorMessage.String
		}
		out = append(out, &ports.JobResult{JobID: r.JobID, RowNumber: r.RowNumber, BuildID: buildId, BarcodeURLs: urls, ErrorCode: errc, ErrorMessage: errmsg})
	}
	return out, nil
}

func (w *jobWrapper) UpdateBillingTransactionID(ctx context.Context, jobID string, txID *string) error {
	return w.a.UpdateBillingTransactionID(ctx, jobID, txID)
}

func (w *jobWrapper) UpdateStatusWithResult(ctx context.Context, jobID string, status string, result ports.JobResult) error {
	return w.a.UpdateStatusWithResult(ctx, jobID, status, result)
}

// Runs a full E2E flow against local Docker services (Postgres, Kafka).
// Requires RUN_INT_TESTS=1 and POSTGRES_TEST_DSN env var set, Kafka at localhost:9092.
func TestE2E_Flow(t *testing.T) {
	if os.Getenv("RUN_INT_TESTS") != "1" {
		t.Skip("skipping integration tests; set RUN_INT_TESTS=1 to run")
	}
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set")
	}
	// connect DB
	dbConn, err := db.Connect(dsn)
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	// no local ctx needed here

	adapterBatch := pg.NewBatchRepository(dbConn)
	adapterJob := pg.NewJobRepository(dbConn)

	// instantiate wrappers for adapter repos
	batchRepo := &batchWrapper{a: adapterBatch}
	jobRepo := &jobWrapper{a: adapterJob}

	// create service with real kafka producer
	prod := kafka.NewProducer("localhost:9092")
	svc := bulk.NewService(batchRepo, jobRepo, nil, nil, prod, nil)
	svc.Logger = logging.NewLogger(nil)

	// create manager
	mgr := batch.NewBatchManager(batchRepo, jobRepo, nil, nil, nil, "")
	mgr.Logger = svc.Logger

	// create a small CSV for upload
	csv := "first_name,last_name\nv1,v2\n"
	res, err := svc.CreateBatchFromFile(context.Background(), strings.NewReader(csv), "rev-e2e")
	if err != nil {
		t.Fatalf("CreateBatchFromFile failed: %v", err)
	}
	batchID := res.BatchID

	// confirm batch: publish jobs using producer
	_, err = svc.ConfirmBatch(context.Background(), batchID, res.ValidRows, nil, func(ctx context.Context, topic string, key []byte, msg any) error {
		return prod.Publish(ctx, topic, key, msg)
	})
	if err != nil {
		t.Fatalf("ConfirmBatch failed: %v", err)
	}

	// Fetch jobs from DB
	jobs, err := jobRepo.GetByBatch(context.Background(), batchID)
	if err != nil {
		t.Fatalf("GetByBatch failed: %v", err)
	}

	// determine topic
	topic := os.Getenv("KAFKA_TOPIC_BULK_RESULT")
	if topic == "" {
		topic = "bulk.result"
	}

	// publish bulk.result messages to Kafka (validates producer end-to-end)
	for _, j := range jobs {
		result := map[string]any{"eventType": "bulk.result", "jobId": j.ID, "batchId": j.BatchID, "status": "completed", "buildId": "build-e2e", "barcodeUrls": "[]"}
		if err := prod.Publish(context.Background(), topic, nil, result); err != nil {
			t.Fatalf("failed to publish bulk.result: %v", err)
		}
	}
	_ = prod.Close()

	// directly update job statuses to "completed" — simulates what the worker/BFF does
	// upon receiving the bulk.result Kafka message. Consumer reliability is tested
	// separately in internal/kafka integration tests.
	for _, j := range jobs {
		if err := jobRepo.UpdateStatus(context.Background(), j.ID, "completed"); err != nil {
			t.Fatalf("failed to update job status: %v", err)
		}
	}

	// call OnJobStatusChange which should detect all jobs completed and mark batch as completed
	if err := mgr.OnJobStatusChange(context.Background(), batchID); err != nil {
		t.Fatalf("OnJobStatusChange failed: %v", err)
	}

	// verify batch reached completed status
	b2, err := batchRepo.GetByID(context.Background(), batchID)
	if err != nil {
		t.Fatalf("GetByID after completion check: %v", err)
	}
	if b2.Status != batch.BatchStatusCompleted {
		t.Fatalf("expected batch status completed, got %s", b2.Status)
	}

	t.Logf("TestE2E_Flow completed successfully — batchId=%s", batchID)
}

