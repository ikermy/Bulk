package repo

import (
	"context"
	"database/sql"

	"github.com/ikermy/Bulk/internal/adapters/postgres"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/ports"
)

type JobRepository struct {
	inner *postgres.JobRepository
}

func NewJobRepository(db *sql.DB) *JobRepository {
	return &JobRepository{inner: postgres.NewJobRepository(db)}
}

func (r *JobRepository) Create(ctx context.Context, j *domain.Job) error {
	if j == nil {
		return nil
	}
	// postgres.Job.Status is a plain string, while domain.Job.Status is domain.JobStatus (underlying type string).
	// Convert explicitly to string when storing to the adapter struct.
	var bt sql.NullString
	if j.BillingTransactionID != nil {
		bt = sql.NullString{String: *j.BillingTransactionID, Valid: true}
	}
	pj := &postgres.Job{ID: j.ID, BatchID: j.BatchID, RowNumber: j.RowNumber, Status: string(j.Status), InputData: j.InputData, BillingTransactionID: bt}
	return r.inner.Create(ctx, pj)
}

func (r *JobRepository) GetByBatch(ctx context.Context, batchID string) ([]*domain.Job, error) {
	pjs, err := r.inner.GetByBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	var res []*domain.Job
	for _, pj := range pjs {
		// reuse adapter conversion to domain.Job
		if dj := pj.ToDomain(); dj != nil {
			res = append(res, dj)
		}
	}
	return res, nil
}

func (r *JobRepository) UpdateBillingTransactionID(ctx context.Context, jobID string, txID *string) error {
	return r.inner.UpdateBillingTransactionID(ctx, jobID, txID)
}

func (r *JobRepository) UpdateStatus(ctx context.Context, jobID string, status string) error {
	return r.inner.UpdateStatus(ctx, jobID, status)
}

// UpdateStatusWithResult atomically updates job status and result fields (build_id, barcode_urls,
// error_code, error_message, billing_transaction_id) in a single DB operation. This prevents
// inconsistencies between job status and its result when handling bulk.result messages (TZ §8.4).
func (r *JobRepository) UpdateStatusWithResult(ctx context.Context, jobID string, status string, result ports.JobResult) error {
	return r.inner.UpdateStatusWithResult(ctx, jobID, status, result)
}

// GetResultsByBatch returns job results (build ids, barcode urls, errors) for a batch
func (r *JobRepository) GetResultsByBatch(ctx context.Context, batchID string) ([]*ports.JobResult, error) {
	rows, err := r.inner.GetResultsByBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}
	var res []*ports.JobResult
	for _, it := range rows {
		if jr := it.ToPort(); jr != nil {
			res = append(res, jr)
		}
	}
	return res, nil
}
