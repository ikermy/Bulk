package postgres

import (
	"context"
	"database/sql"

	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/ports"
)

type Job struct {
	ID        string `db:"id"`
	BatchID   string `db:"batch_id"`
	RowNumber int    `db:"row_number"`
	Status    string `db:"status"`
	InputData string `db:"input_data"`
	// BillingTransactionID хранит nullable UUID из схемы (batch_jobs.billing_transaction_id)
	BillingTransactionID sql.NullString `db:"billing_transaction_id"`
}

type JobRepository struct {
	db *sql.DB
}

func NewJobRepository(db *sql.DB) *JobRepository { return &JobRepository{db: db} }

func (r *JobRepository) Create(ctx context.Context, j *Job) error {
	input := j.InputData
	if input == "" {
		input = "{}"
	}
	// input_data is JSONB in the schema; cast the parameter to jsonb
	_, err := r.db.ExecContext(ctx, `INSERT INTO batch_jobs (id,batch_id,row_number,status,input_data,billing_transaction_id) VALUES ($1,$2,$3,$4,$5::jsonb,$6)`, j.ID, j.BatchID, j.RowNumber, j.Status, input, nullableStringArg(j.BillingTransactionID))
	return err
}

func (r *JobRepository) GetByBatch(ctx context.Context, batchID string) ([]*Job, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id,batch_id,row_number,status,input_data,billing_transaction_id FROM batch_jobs WHERE batch_id=$1", batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []*Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.BatchID, &j.RowNumber, &j.Status, &j.InputData, &j.BillingTransactionID); err != nil {
			return nil, err
		}
		jobs = append(jobs, &j)
	}
	return jobs, nil
}

func (r *JobRepository) UpdateStatus(ctx context.Context, jobID string, status string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE batch_jobs SET status=$1 WHERE id=$2", status, jobID)
	return err
}

// ToDomain converts adapter Job to domain.Job
func (j *Job) ToDomain() *domain.Job {
	if j == nil {
		return nil
	}
	var bt *string
	if j.BillingTransactionID.Valid {
		s := j.BillingTransactionID.String
		bt = &s
	}
	return &domain.Job{ID: j.ID, BatchID: j.BatchID, RowNumber: j.RowNumber, Status: domain.JobStatus(j.Status), InputData: j.InputData, BillingTransactionID: bt}
}

// UpdateBillingTransactionID updates billing_transaction_id column for a job
func (r *JobRepository) UpdateBillingTransactionID(ctx context.Context, jobID string, txID *string) error {
	var arg interface{}
	if txID == nil {
		arg = nil
	} else {
		arg = *txID
	}
	_, err := r.db.ExecContext(ctx, "UPDATE batch_jobs SET billing_transaction_id=$1 WHERE id=$2", arg, jobID)
	return err
}

// UpdateStatusWithResult updates status and result fields atomically.
func (r *JobRepository) UpdateStatusWithResult(ctx context.Context, jobID string, status string, result ports.JobResult) error {
	// prepare nullable args
	var buildArg interface{}
	if result.BuildID == "" {
		buildArg = nil
	} else {
		buildArg = result.BuildID
	}
	var barcodeArg interface{}
	if result.BarcodeURLs == "" {
		barcodeArg = nil
	} else {
		barcodeArg = result.BarcodeURLs
	}
	var errCodeArg interface{}
	if result.ErrorCode == "" {
		errCodeArg = nil
	} else {
		errCodeArg = result.ErrorCode
	}
	var errMsgArg interface{}
	if result.ErrorMessage == "" {
		errMsgArg = nil
	} else {
		errMsgArg = result.ErrorMessage
	}
	var billingArg interface{}
	if result.BillingTransactionID == "" {
		billingArg = nil
	} else {
		billingArg = result.BillingTransactionID
	}

	// Update build_id, barcode_urls (jsonb), error_code, error_message, billing_transaction_id and status atomically
	_, err := r.db.ExecContext(ctx, `UPDATE batch_jobs SET status=$1, build_id=$2, barcode_urls=$3::jsonb, error_code=$4, error_message=$5, billing_transaction_id=$6 WHERE id=$7`, status, buildArg, barcodeArg, errCodeArg, errMsgArg, billingArg, jobID)
	return err
}

// nullableStringArg is helper to pass sql.NullString or nil into Exec parameters
func nullableStringArg(ns sql.NullString) interface{} {
	if !ns.Valid {
		return nil
	}
	return ns.String
}

// Result represents a job result row returned by the adapter
type Result struct {
	JobID        string
	RowNumber    int
	BuildID      sql.NullString
	BarcodeURLs  sql.NullString
	ErrorCode    sql.NullString
	ErrorMessage sql.NullString
}

// GetResultsByBatch returns results (build ids, urls, errors) for jobs in a batch
func (r *JobRepository) GetResultsByBatch(ctx context.Context, batchID string) ([]*Result, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, row_number, build_id, barcode_urls, error_code, error_message FROM batch_jobs WHERE batch_id=$1", batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []*Result
	for rows.Next() {
		var it Result
		if err := rows.Scan(&it.JobID, &it.RowNumber, &it.BuildID, &it.BarcodeURLs, &it.ErrorCode, &it.ErrorMessage); err != nil {
			return nil, err
		}
		res = append(res, &it)
	}
	return res, nil
}

// ToPort converts adapter Result to ports.JobResult
func (res *Result) ToPort() *ports.JobResult {
	if res == nil {
		return nil
	}
	jr := &ports.JobResult{JobID: res.JobID, RowNumber: res.RowNumber}
	if res.BuildID.Valid {
		jr.BuildID = res.BuildID.String
	}
	if res.BarcodeURLs.Valid {
		jr.BarcodeURLs = res.BarcodeURLs.String
	}
	if res.ErrorCode.Valid {
		jr.ErrorCode = res.ErrorCode.String
	}
	if res.ErrorMessage.Valid {
		jr.ErrorMessage = res.ErrorMessage.String
	}
	return jr
}

