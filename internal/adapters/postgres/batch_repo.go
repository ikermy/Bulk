package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"time"

	"github.com/ikermy/Bulk/internal/batch"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/ports"
)

type Batch struct {
	ID             string    `db:"id"`
	UserID         string    `db:"user_id"`
	Status         string    `db:"status"`
	Revision       string    `db:"revision"`
	FileStorageID  string    `db:"file_storage_id"`
	TotalRows      int       `db:"total_rows"`
	ValidRows      int       `db:"valid_rows"`
	ApprovedCount  int       `db:"approved_count"`
	CompletedCount int       `db:"completed_count"`
	FailedCount    int       `db:"failed_count"`
	CreatedAt      time.Time `db:"created_at"`
	CompletedAt    time.Time `db:"completed_at"`
	// TransactionIDs хранит массив идентификаторов транзакций, зарезервированных в Billing
	// Соответствует колонке batches.transaction_ids (JSONB). Соответствует ТЗ §9.2.
	TransactionIDs []string `db:"transaction_ids"`
	// Priority and TimeoutMs are stored inside transaction_ids JSONB as part of a wrapper object
	Priority  string
	TimeoutMs int
}

type BatchRepository struct {
	db *sql.DB
}

func NewBatchRepository(db *sql.DB) *BatchRepository { return &BatchRepository{db: db} }

func (r *BatchRepository) Create(ctx context.Context, b *Batch) error {
	// handle nullable UUID fields: if empty string -> NULL
	var userID interface{}
	if b.UserID == "" {
		userID = nil
	} else {
		userID = b.UserID
	}
	var fileStorage interface{}
	if b.FileStorageID == "" {
		fileStorage = nil
	} else {
		fileStorage = b.FileStorageID
	}
	// marshal wrapper with transactions + optional config into transaction_ids JSONB
	txArg := txArgFromBatch(b)
	_, err := r.db.ExecContext(ctx, `INSERT INTO batches (id,user_id,status,revision,file_storage_id,total_rows,valid_rows,approved_count,completed_count,failed_count,completed_at,transaction_ids) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, b.ID, userID, b.Status, b.Revision, fileStorage, b.TotalRows, b.ValidRows, b.ApprovedCount, b.CompletedCount, b.FailedCount, nil, txArg)
	return err
}

func (r *BatchRepository) GetByID(ctx context.Context, id string) (*Batch, error) {
	var b Batch
	// include transaction_ids JSONB
	row := r.db.QueryRowContext(ctx, "SELECT id,user_id,status,revision,file_storage_id,total_rows,valid_rows,approved_count,completed_count,failed_count,created_at,completed_at,transaction_ids FROM batches WHERE id=$1", id)
	var completedAt sql.NullTime
	var txNull sql.NullString
	var userIDNull sql.NullString
	var fileStorageNull sql.NullString
	err := row.Scan(&b.ID, &userIDNull, &b.Status, &b.Revision, &fileStorageNull, &b.TotalRows, &b.ValidRows, &b.ApprovedCount, &b.CompletedCount, &b.FailedCount, &b.CreatedAt, &completedAt, &txNull)
	if err != nil {
		return nil, err
	}
	if userIDNull.Valid {
		b.UserID = userIDNull.String
	}
	if fileStorageNull.Valid {
		b.FileStorageID = fileStorageNull.String
	}
	if completedAt.Valid {
		b.CompletedAt = completedAt.Time
	}
	if txNull.Valid {
		ids, pri, to := parseTransactionIDs(txNull)
		b.TransactionIDs = append(b.TransactionIDs, ids...)
		b.Priority = pri
		b.TimeoutMs = to
	}
	return &b, nil
}

func (r *BatchRepository) Update(ctx context.Context, b *Batch) error {
	if b == nil {
		return nil
	}
	var fileStorage interface{}
	if b.FileStorageID == "" {
		fileStorage = nil
	} else {
		fileStorage = b.FileStorageID
	}
	// marshal wrapper with transactions + optional config into transaction_ids JSONB
	txArg := txArgFromBatch(b)
	_, err := r.db.ExecContext(ctx, "UPDATE batches SET status=$1, file_storage_id=$2, total_rows=$3, valid_rows=$4, approved_count=$5, completed_count=$6, failed_count=$7, completed_at=$8, transaction_ids=$9 WHERE id=$10", b.Status, fileStorage, b.TotalRows, b.ValidRows, b.ApprovedCount, b.CompletedCount, b.FailedCount, b.CompletedAt, txArg, b.ID)
	return err
}

// ToDomain converts adapter Batch to domain.Batch
func (b *Batch) ToDomain() *domain.Batch {
	if b == nil {
		return nil
	}
	var completedAt *time.Time
	if !b.CompletedAt.IsZero() {
		completedAt = &b.CompletedAt
	}
	return &domain.Batch{ID: b.ID, UserID: b.UserID, Status: domain.BatchStatus(b.Status), Revision: b.Revision, FileStorageID: b.FileStorageID, TotalRows: b.TotalRows, ValidRows: b.ValidRows, ApprovedCount: b.ApprovedCount, CompletedCount: b.CompletedCount, FailedCount: b.FailedCount, CreatedAt: b.CreatedAt, CompletedAt: completedAt, BillingTransactionIDs: b.TransactionIDs, Priority: b.Priority, TimeoutMs: b.TimeoutMs}
}

// ListWithFilters New List variant with extended filters used by repo wrapper
func (r *BatchRepository) ListWithFilters(ctx context.Context, status string, userID string, revision string, from *time.Time, to *time.Time, sortBy string, desc bool, cursor *time.Time, limit int, offset int) ([]*Batch, int, error) {
	var args []interface{}
	where := " WHERE 1=1"
	idx := 1
	if status != "" {
		where += " AND status=$" + strconv.Itoa(idx)
		args = append(args, status)
		idx++
	}
	if userID != "" {
		where += " AND user_id=$" + strconv.Itoa(idx)
		args = append(args, userID)
		idx++
	}
	if revision != "" {
		where += " AND revision=$" + strconv.Itoa(idx)
		args = append(args, revision)
		idx++
	}
	if from != nil {
		where += " AND created_at >= $" + strconv.Itoa(idx)
		args = append(args, *from)
		idx++
	}
	if to != nil {
		where += " AND created_at <= $" + strconv.Itoa(idx)
		args = append(args, *to)
		idx++
	}
	if cursor != nil {
		// cursor-mode: fetch rows before (or after) cursor depending on sort order
		if sortBy == "completedAt" {
			if desc {
				where += " AND completed_at < $" + strconv.Itoa(idx)
			} else {
				where += " AND completed_at > $" + strconv.Itoa(idx)
			}
		} else {
			if desc {
				where += " AND created_at < $" + strconv.Itoa(idx)
			} else {
				where += " AND created_at > $" + strconv.Itoa(idx)
			}
		}
		args = append(args, *cursor)
		idx++
	}

	// total count
	cntQuery := "SELECT COUNT(*) FROM batches" + where
	var total int
	if err := r.db.QueryRowContext(ctx, cntQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// build order clause
	orderBy := "created_at"
	if sortBy == "completedAt" {
		orderBy = "completed_at"
	}
	dir := "DESC"
	if !desc {
		dir = "ASC"
	}

	// results
	// include transaction_ids in list results
	query := "SELECT id,user_id,status,revision,file_storage_id,total_rows,valid_rows,approved_count,completed_count,failed_count,created_at,completed_at,transaction_ids FROM batches" + where + " ORDER BY " + orderBy + " " + dir + " LIMIT $" + strconv.Itoa(idx) + " OFFSET $" + strconv.Itoa(idx+1)
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var res []*Batch
	for rows.Next() {
		var b Batch
		var completedAt sql.NullTime
		var txNull sql.NullString
		var userIDNull sql.NullString
		var fileStorageNull sql.NullString
		if err := rows.Scan(&b.ID, &userIDNull, &b.Status, &b.Revision, &fileStorageNull, &b.TotalRows, &b.ValidRows, &b.ApprovedCount, &b.CompletedCount, &b.FailedCount, &b.CreatedAt, &completedAt, &txNull); err != nil {
			return nil, 0, err
		}
		if userIDNull.Valid {
			b.UserID = userIDNull.String
		}
		if fileStorageNull.Valid {
			b.FileStorageID = fileStorageNull.String
		}
		if completedAt.Valid {
			b.CompletedAt = completedAt.Time
		}
		if txNull.Valid {
			ids, pri, to := parseTransactionIDs(txNull)
			b.TransactionIDs = append(b.TransactionIDs, ids...)
			b.Priority = pri
			b.TimeoutMs = to
		}
		res = append(res, &b)
	}
	return res, total, nil
}

// AdminStats performs aggregated queries to produce admin dashboard metrics
func (r *BatchRepository) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) {
	// build where clause for time range on created_at
	where := " WHERE 1=1"
	var args []interface{}
	idx := 1
	if from != nil {
		where += " AND created_at >= $" + strconv.Itoa(idx)
		args = append(args, *from)
		idx++
	}
	if to != nil {
		where += " AND created_at <= $" + strconv.Itoa(idx)
		args = append(args, *to)
		idx++
	}

	// batches created
	var batchesCreated int
	q1 := "SELECT COUNT(*) FROM batches" + where
	if err := r.db.QueryRowContext(ctx, q1, args...).Scan(&batchesCreated); err != nil {
		return nil, err
	}

	// jobs processed and failed: use batch_jobs join batches (filter by batches.created_at)
	var jobsProcessed int
	var jobsFailed int
	procQuery := "SELECT COUNT(*) FROM batch_jobs j JOIN batches b ON j.batch_id=b.id" + where
	if err := r.db.QueryRowContext(ctx, procQuery, args...).Scan(&jobsProcessed); err != nil {
		return nil, err
	}
	failQuery := "SELECT COUNT(*) FROM batch_jobs j JOIN batches b ON j.batch_id=b.id WHERE j.status=$1" + where
	fargs := append([]interface{}{string(batch.JobStatusFailed)}, args...)
	if err := r.db.QueryRowContext(ctx, failQuery, fargs...).Scan(&jobsFailed); err != nil {
		return nil, err
	}

	// average processing time: average(completed_at - created_at) in milliseconds for completed batches
	avgMs := 0.0
	avgQuery := "SELECT AVG(EXTRACT(EPOCH FROM (completed_at - created_at)) * 1000) FROM batches WHERE completed_at IS NOT NULL" + where
	if err := r.db.QueryRowContext(ctx, avgQuery, args...).Scan(&avgMs); err != nil {
		return nil, err
	}

	// queues: count pending jobs and pending results
	var bulkJobPending int
	var bulkResultPending int
	// bulkJobPending: jobs with status in (pending, queued)
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM batch_jobs WHERE status IN ($1,$2)", string(batch.JobStatusPending), string(batch.JobStatusQueued)).Scan(&bulkJobPending); err != nil {
		return nil, err
	}
	// bulkResultPending: approximate as jobs in status processing (waiting for result)
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM batch_jobs WHERE status=$1", string(batch.JobStatusProcessing)).Scan(&bulkResultPending); err != nil {
		return nil, err
	}

	return &ports.AdminStatsWithQueues{AdminStats: ports.AdminStats{BatchesCreated: batchesCreated, JobsProcessed: jobsProcessed, JobsFailed: jobsFailed, AverageProcessingTimeMs: avgMs}, Queues: ports.QueuesStats{BulkJobPending: bulkJobPending, BulkResultPending: bulkResultPending}}, nil
}

// parseTransactionIDs parses the transaction_ids JSONB which may be either
// a plain JSON array of strings or a wrapper object { transactions: [...], config: { priority, timeout } }
func parseTransactionIDs(txNull sql.NullString) (ids []string, priority string, timeoutMs int) {
	var raw any
	if err := json.Unmarshal([]byte(txNull.String), &raw); err != nil {
		return nil, "", 0
	}
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				ids = append(ids, s)
			}
		}
	case map[string]any:
		if txs, ok := v["transactions"]; ok {
			if arr, ok2 := txs.([]any); ok2 {
				for _, item := range arr {
					if s, ok3 := item.(string); ok3 {
						ids = append(ids, s)
					}
				}
			}
		}
		if cfgv, ok := v["config"]; ok {
			if cm, ok2 := cfgv.(map[string]any); ok2 {
				if p, ok3 := cm["priority"].(string); ok3 {
					priority = p
				}
				if t, ok4 := cm["timeout"]; ok4 {
					switch tt := t.(type) {
					case float64:
						timeoutMs = int(tt)
					case int:
						timeoutMs = tt
					}
				}
			}
		}
	}
	return ids, priority, timeoutMs
}

// txArgFromBatch builds JSONB argument for transaction_ids from Batch fields
func txArgFromBatch(b *Batch) interface{} {
	wrapper := map[string]any{"transactions": b.TransactionIDs}
	if b.Priority != "" || b.TimeoutMs > 0 {
		wrapper["config"] = map[string]any{"priority": b.Priority, "timeout": b.TimeoutMs}
	}
	if tb, merr := json.Marshal(wrapper); merr == nil {
		return string(tb)
	}
	return nil
}
