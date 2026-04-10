package repo

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/ports"
	"github.com/stretchr/testify/require"
)

func TestRepoJobRepository_WrapsAdapter(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	r := NewJobRepository(db)
	ctx := context.Background()

	// Create: expect adapter INSERT
	exp := mock.ExpectExec(regexp.QuoteMeta("INSERT INTO batch_jobs (id,batch_id,row_number,status,input_data,billing_transaction_id) VALUES ($1,$2,$3,$4,$5::jsonb,$6)"))
	exp.WithArgs("jid", "bid", 1, "pending", "{}", nil).WillReturnResult(sqlmock.NewResult(1, 1))

	j := &domain.Job{ID: "jid", BatchID: "bid", RowNumber: 1, Status: domain.JobStatus("pending"), InputData: "{}"}
	err = r.Create(ctx, j)
	require.NoError(t, err)

	// GetByBatch: adapter returns two rows
	rows := sqlmock.NewRows([]string{"id", "batch_id", "row_number", "status", "input_data", "billing_transaction_id"}).
		AddRow("jid", "bid", 1, "pending", "{}", nil)
	qexp := mock.ExpectQuery(regexp.QuoteMeta("SELECT id,batch_id,row_number,status,input_data,billing_transaction_id FROM batch_jobs WHERE batch_id=$1"))
	qexp.WithArgs("bid").WillReturnRows(rows)

	got, err := r.GetByBatch(ctx, "bid")
	require.NoError(t, err)
	require.Len(t, got, 1)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRepoJobRepository_UpdateAndResults(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	r := NewJobRepository(db)
	ctx := context.Background()

	// UpdateStatus expectation
	exp := mock.ExpectExec(regexp.QuoteMeta("UPDATE batch_jobs SET status=$1 WHERE id=$2"))
	exp.WithArgs("queued", "jid").WillReturnResult(sqlmock.NewResult(1, 1))
	err = r.UpdateStatus(ctx, "jid", "queued")
	require.NoError(t, err)

	// UpdateBillingTransactionID with value
	exp2 := mock.ExpectExec(regexp.QuoteMeta("UPDATE batch_jobs SET billing_transaction_id=$1 WHERE id=$2"))
	exp2.WithArgs("tx-1", "jid").WillReturnResult(sqlmock.NewResult(1, 1))
	tx := "tx-1"
	err = r.UpdateBillingTransactionID(ctx, "jid", &tx)
	require.NoError(t, err)

	// UpdateBillingTransactionID with nil
	exp3 := mock.ExpectExec(regexp.QuoteMeta("UPDATE batch_jobs SET billing_transaction_id=$1 WHERE id=$2"))
	exp3.WithArgs(nil, "jid").WillReturnResult(sqlmock.NewResult(1, 1))
	err = r.UpdateBillingTransactionID(ctx, "jid", nil)
	require.NoError(t, err)

	// UpdateStatusWithResult: expect complex update
	exp4 := mock.ExpectExec(regexp.QuoteMeta("UPDATE batch_jobs SET status=$1, build_id=$2, barcode_urls=$3::jsonb, error_code=$4, error_message=$5, billing_transaction_id=$6 WHERE id=$7"))
	exp4.WithArgs("completed", "build-1", "{\"pdf417\":\"u\"}", nil, nil, nil, "jid").WillReturnResult(sqlmock.NewResult(1, 1))
	jr := ports.JobResult{JobID: "jid", BuildID: "build-1", BarcodeURLs: `{"pdf417":"u"}`}
	err = r.UpdateStatusWithResult(ctx, "jid", "completed", jr)
	require.NoError(t, err)

	// GetResultsByBatch: return one row
	rows := sqlmock.NewRows([]string{"id", "row_number", "build_id", "barcode_urls", "error_code", "error_message"}).
		AddRow("jid", 2, "build-1", `{"pdf417":"u"}`, nil, nil)
	qexp := mock.ExpectQuery(regexp.QuoteMeta("SELECT id, row_number, build_id, barcode_urls, error_code, error_message FROM batch_jobs WHERE batch_id=$1"))
	qexp.WithArgs("bid").WillReturnRows(rows)

	res, err := r.GetResultsByBatch(ctx, "bid")
	require.NoError(t, err)
	require.Len(t, res, 1)

	require.NoError(t, mock.ExpectationsWereMet())
}

