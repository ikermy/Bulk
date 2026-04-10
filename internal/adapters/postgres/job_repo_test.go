package postgres

import (
    "context"
    "database/sql"
    "regexp"
    "testing"

    "github.com/DATA-DOG/go-sqlmock"
    "github.com/stretchr/testify/require"
    "github.com/ikermy/Bulk/internal/ports"
)

func TestNullableStringArg(t *testing.T) {
    var ns sql.NullString
    // invalid -> nil
    if got := nullableStringArg(ns); got != nil {
        t.Fatalf("expected nil, got %v", got)
    }
    ns = sql.NullString{String: "x", Valid: true}
    if got := nullableStringArg(ns); got != "x" {
        t.Fatalf("expected 'x', got %v", got)
    }
}

func TestJob_ToDomain_NilAndValid(t *testing.T) {
    var j *Job
    if got := j.ToDomain(); got != nil {
        t.Fatalf("expected nil for nil receiver")
    }

    j2 := &Job{ID: "id1", BatchID: "b1", RowNumber: 5, Status: "completed", InputData: "{}", BillingTransactionID: sql.NullString{String: "tx1", Valid: true}}
    d := j2.ToDomain()
    require.NotNil(t, d)
    require.Equal(t, "id1", d.ID)
    require.Equal(t, "b1", d.BatchID)
    require.Equal(t, 5, d.RowNumber)
    require.Equal(t, "{}", d.InputData)
    require.NotNil(t, d.BillingTransactionID)
    require.Equal(t, "tx1", *d.BillingTransactionID)
}

func TestJobRepository_DBOperations(t *testing.T) {
    db, mock, err := sqlmock.New()
    require.NoError(t, err)
    defer db.Close()

    r := NewJobRepository(db)

    ctx := context.Background()

    // Create
    exp := mock.ExpectExec(regexp.QuoteMeta("INSERT INTO batch_jobs (id,batch_id,row_number,status,input_data,billing_transaction_id) VALUES ($1,$2,$3,$4,$5::jsonb,$6)"))
    exp.WithArgs("jid", "bid", 1, "pending", "{}", nil).WillReturnResult(sqlmock.NewResult(1, 1))

    err = r.Create(ctx, &Job{ID: "jid", BatchID: "bid", RowNumber: 1, Status: "pending", InputData: "{}", BillingTransactionID: sql.NullString{Valid: false}})
    require.NoError(t, err)

    // GetByBatch
    rows := sqlmock.NewRows([]string{"id", "batch_id", "row_number", "status", "input_data", "billing_transaction_id"}).
        AddRow("jid", "bid", 1, "pending", "{}", nil).
        AddRow("jid2", "bid", 2, "completed", "{\"a\":1}", "txid")

    qexp := mock.ExpectQuery(regexp.QuoteMeta("SELECT id,batch_id,row_number,status,input_data,billing_transaction_id FROM batch_jobs WHERE batch_id=$1"))
    qexp.WithArgs("bid").WillReturnRows(rows)

    got, err := r.GetByBatch(ctx, "bid")
    require.NoError(t, err)
    require.Len(t, got, 2)

    // UpdateStatus
    exp2 := mock.ExpectExec(regexp.QuoteMeta("UPDATE batch_jobs SET status=$1 WHERE id=$2"))
    exp2.WithArgs("done", "jid").WillReturnResult(sqlmock.NewResult(1, 1))
    err = r.UpdateStatus(ctx, "jid", "done")
    require.NoError(t, err)

    // UpdateBillingTransactionID: nil
    exp3 := mock.ExpectExec(regexp.QuoteMeta("UPDATE batch_jobs SET billing_transaction_id=$1 WHERE id=$2"))
    exp3.WithArgs(nil, "jid").WillReturnResult(sqlmock.NewResult(1, 1))
    err = r.UpdateBillingTransactionID(ctx, "jid", nil)
    require.NoError(t, err)

    // UpdateBillingTransactionID: value
    tx := "tx1"
    exp4 := mock.ExpectExec(regexp.QuoteMeta("UPDATE batch_jobs SET billing_transaction_id=$1 WHERE id=$2"))
    exp4.WithArgs(tx, "jid").WillReturnResult(sqlmock.NewResult(1, 1))
    err = r.UpdateBillingTransactionID(ctx, "jid", &tx)
    require.NoError(t, err)

    // UpdateStatusWithResult: various nullable fields
    exp5 := mock.ExpectExec(regexp.QuoteMeta("UPDATE batch_jobs SET status=$1, build_id=$2, barcode_urls=$3::jsonb, error_code=$4, error_message=$5, billing_transaction_id=$6 WHERE id=$7"))
    exp5.WithArgs("done", nil, nil, nil, nil, nil, "jid").WillReturnResult(sqlmock.NewResult(1, 1))

    res := ports.JobResult{JobID: "jid", RowNumber: 1}
    err = r.UpdateStatusWithResult(ctx, "jid", "done", res)
    require.NoError(t, err)

    // GetResultsByBatch
    rows2 := sqlmock.NewRows([]string{"id", "row_number", "build_id", "barcode_urls", "error_code", "error_message"}).
        AddRow("j1", 1, nil, nil, nil, nil).
        AddRow("j2", 2, "bld", "[\"u\"]", "E1", "msg")

    qexp2 := mock.ExpectQuery(regexp.QuoteMeta("SELECT id, row_number, build_id, barcode_urls, error_code, error_message FROM batch_jobs WHERE batch_id=$1"))
    qexp2.WithArgs("bid").WillReturnRows(rows2)

    got2, err := r.GetResultsByBatch(ctx, "bid")
    require.NoError(t, err)
    require.Len(t, got2, 2)

    // ensure all expectations met
    require.NoError(t, mock.ExpectationsWereMet())
}


