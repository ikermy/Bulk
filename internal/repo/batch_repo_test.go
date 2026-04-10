package repo

import (
    "context"
    "regexp"
    "testing"
    "time"

    "github.com/DATA-DOG/go-sqlmock"
    "github.com/stretchr/testify/require"
    "github.com/ikermy/Bulk/internal/domain"
    "github.com/ikermy/Bulk/internal/batch"
    "github.com/ikermy/Bulk/internal/ports"
)

func TestBatchRepository_Create_GetByID_Update(t *testing.T) {
    db, mock, err := sqlmock.New()
    require.NoError(t, err)
    defer db.Close()

    r := NewBatchRepository(db)
    ctx := context.Background()

    // Create expectation
    exp := mock.ExpectExec(regexp.QuoteMeta("INSERT INTO batches (id,user_id,status,revision,file_storage_id,total_rows,valid_rows,approved_count,completed_count,failed_count,completed_at,transaction_ids) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)"))
    // when TransactionIDs is nil, JSON marshal produces {"transactions":null}
    exp.WithArgs("bid", nil, "pending", "rev1", nil, 10, 8, 0, 0, 2, nil, `{"transactions":null}`).
        WillReturnResult(sqlmock.NewResult(1, 1))

    now := time.Now()
    dbBatch := sqlmock.NewRows([]string{"id","user_id","status","revision","file_storage_id","total_rows","valid_rows","approved_count","completed_count","failed_count","created_at","completed_at","transaction_ids"}).
        AddRow("bid", "", "pending", "rev1", "", 10, 8, 0, 0, 2, now, nil, `{"transactions":[]}`)

    // GetByID expectation
    qexp := mock.ExpectQuery(regexp.QuoteMeta("SELECT id,user_id,status,revision,file_storage_id,total_rows,valid_rows,approved_count,completed_count,failed_count,created_at,completed_at,transaction_ids FROM batches WHERE id=$1"))
    qexp.WithArgs("bid").WillReturnRows(dbBatch)

    // Update expectation
    uexp := mock.ExpectExec(regexp.QuoteMeta("UPDATE batches SET status=$1, file_storage_id=$2, total_rows=$3, valid_rows=$4, approved_count=$5, completed_count=$6, failed_count=$7, completed_at=$8, transaction_ids=$9 WHERE id=$10"))
    uexp.WithArgs("completed", nil, 10, 8, 0, 0, 2, now, `{"transactions":null}`, "bid").WillReturnResult(sqlmock.NewResult(1, 1))

    // Call Create
    b := &domain.Batch{ID: "bid", Status: domain.BatchStatus("pending"), Revision: "rev1", TotalRows: 10, ValidRows: 8, FailedCount: 2}
    err = r.Create(ctx, b)
    require.NoError(t, err)

    // GetByID via repo wrapper
    got, err := r.GetByID(ctx, "bid")
    require.NoError(t, err)
    require.Equal(t, "bid", got.ID)
    require.Equal(t, "rev1", got.Revision)

    // Update via wrapper
    dbb := &domain.Batch{ID: "bid", Status: domain.BatchStatus("completed"), TotalRows: 10, ValidRows: 8, FailedCount: 2}
    dbb.CompletedAt = &now
    err = r.Update(ctx, dbb)
    require.NoError(t, err)

    require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchRepository_ListAndAdminStats(t *testing.T) {
    db, mock, err := sqlmock.New()
    require.NoError(t, err)
    defer db.Close()

    r := NewBatchRepository(db)
    ctx := context.Background()

    // ListWithFilters: count
    cnt := mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM batches WHERE 1=1"))
    cnt.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

    // ListWithFilters: rows
    rows := sqlmock.NewRows([]string{"id","user_id","status","revision","file_storage_id","total_rows","valid_rows","approved_count","completed_count","failed_count","created_at","completed_at","transaction_ids"}).
        AddRow("b1", "u", "pending", "r", "f", 1, 1, 0, 0, 0, time.Now(), nil, `{"transactions":[]}`)
    // SELECT ... ORDER BY created_at DESC LIMIT $1 OFFSET $2
    // default filter.SortDesc == false -> ORDER BY created_at ASC
    q := mock.ExpectQuery(regexp.QuoteMeta("SELECT id,user_id,status,revision,file_storage_id,total_rows,valid_rows,approved_count,completed_count,failed_count,created_at,completed_at,transaction_ids FROM batches WHERE 1=1 ORDER BY created_at ASC LIMIT $1 OFFSET $2"))
    q.WillReturnRows(rows)

    // Call List via wrapper with default filter (page/limit zero -> set to 1/20)
    res, total, err := r.List(ctx, ports.BatchFilter{})
    require.NoError(t, err)
    require.Equal(t, 2, total)
    require.Len(t, res, 1)

    // AdminStats queries
    mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM batches WHERE 1=1")).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
    mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM batch_jobs j JOIN batches b ON j.batch_id=b.id WHERE 1=1")).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))
    // failQuery has j.status=$1 WHERE 1=1 (code concatenates WHERE twice)
    mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM batch_jobs j JOIN batches b ON j.batch_id=b.id WHERE j.status=$1 WHERE 1=1")).WithArgs(string(batch.JobStatusFailed)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
    // avgQuery: SELECT AVG(...) FROM batches WHERE completed_at IS NOT NULL WHERE 1=1
    mock.ExpectQuery(regexp.QuoteMeta("SELECT AVG(EXTRACT(EPOCH FROM (completed_at - created_at)) * 1000) FROM batches WHERE completed_at IS NOT NULL WHERE 1=1")).WillReturnRows(sqlmock.NewRows([]string{"avg"}).AddRow(123.4))
    mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM batch_jobs WHERE status IN ($1,$2)")).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
    mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM batch_jobs WHERE status=$1")).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

    stats, err := r.AdminStats(ctx, nil, nil)
    require.NoError(t, err)
    require.Equal(t, 5, stats.BatchesCreated)
    require.Equal(t, 10, stats.JobsProcessed)

    require.NoError(t, mock.ExpectationsWereMet())
}




