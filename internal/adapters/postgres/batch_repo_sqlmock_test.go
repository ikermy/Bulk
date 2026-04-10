package postgres

import (
    "context"
    "regexp"
    "testing"
    "time"

    "github.com/DATA-DOG/go-sqlmock"
	"github.com/ikermy/Bulk/internal/batch"
	"github.com/stretchr/testify/require"
)

func TestBatchRepository_CreateExec(t *testing.T) {
    db, mock, err := sqlmock.New()
    require.NoError(t, err)
    defer db.Close()

    repo := NewBatchRepository(db)

    // Expect insert (we match by prefix to avoid strict SQL formatting checks)
    mock.ExpectExec(regexp.QuoteMeta("INSERT INTO batches")).WithArgs("b1", nil, "S", "r", nil, 10, 9, 1, 1, 0, nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))

    b := &Batch{ID: "b1", UserID: "", Status: "S", Revision: "r", FileStorageID: "", TotalRows: 10, ValidRows: 9, ApprovedCount: 1, CompletedCount: 1, FailedCount: 0, TransactionIDs: []string{"tx1"}}
    err = repo.Create(context.Background(), b)
    require.NoError(t, err)
    require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchRepository_UpdateNilAndExec(t *testing.T) {
    db, mock, err := sqlmock.New()
    require.NoError(t, err)
    defer db.Close()

    repo := NewBatchRepository(db)

    // Update with nil should return nil immediately
    require.NoError(t, repo.Update(context.Background(), nil))

    // Expect update exec for non-nil batch
    mock.ExpectExec(regexp.QuoteMeta("UPDATE batches SET status=")).WithArgs("S", nil, 10, 9, 1, 1, 0, sqlmock.AnyArg(), sqlmock.AnyArg(), "b1").WillReturnResult(sqlmock.NewResult(1, 1))

    b := &Batch{ID: "b1", FileStorageID: "", Status: "S", TotalRows: 10, ValidRows: 9, ApprovedCount: 1, CompletedCount: 1, FailedCount: 0, CompletedAt: time.Time{}, TransactionIDs: []string{"tx1"}}
    err = repo.Update(context.Background(), b)
    require.NoError(t, err)
    require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchRepository_ListWithFilters_Basic(t *testing.T) {
    db, mock, err := sqlmock.New()
    require.NoError(t, err)
    defer db.Close()

    repo := NewBatchRepository(db)

    // For simple filter with status set, we expect COUNT query then rows query
    cols := []string{"id", "user_id", "status", "revision", "file_storage_id", "total_rows", "valid_rows", "approved_count", "completed_count", "failed_count", "created_at", "completed_at", "transaction_ids"}
    now := time.Now()
    rows := sqlmock.NewRows(cols).AddRow("b1", "u1", "S", "r", "f", 10, 9, 1, 1, 0, now, nil, `[]`)

    // cnt query
    mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM batches WHERE 1=1 AND status=$1")).WithArgs("S").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
    // rows query
    mock.ExpectQuery(regexp.QuoteMeta("SELECT id,user_id,status,revision,file_storage_id,total_rows,valid_rows,approved_count,completed_count,failed_count,created_at,completed_at,transaction_ids FROM batches WHERE 1=1 AND status=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3")).WithArgs("S", 10, 0).WillReturnRows(rows)

    res, total, err := repo.ListWithFilters(context.Background(), "S", "", "", nil, nil, "", true, nil, 10, 0)
    require.NoError(t, err)
    require.Equal(t, 1, total)
    require.Len(t, res, 1)
    require.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchRepository_AdminStats_HappyAndError(t *testing.T) {
    db, mock, err := sqlmock.New()
    require.NoError(t, err)
    defer db.Close()

    repo := NewBatchRepository(db)

    // Happy path: six QueryRow calls
    mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM batches WHERE 1=1")).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))
    mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM batch_jobs j JOIN batches b ON j.batch_id=b.id WHERE 1=1")).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))
    mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM batch_jobs j JOIN batches b ON j.batch_id=b.id WHERE j.status=$1 WHERE 1=1")).WithArgs(string(batch.JobStatusFailed)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
    mock.ExpectQuery(regexp.QuoteMeta("SELECT AVG(EXTRACT(EPOCH FROM (completed_at - created_at)) * 1000) FROM batches WHERE completed_at IS NOT NULL WHERE 1=1")).WillReturnRows(sqlmock.NewRows([]string{"avg"}).AddRow(123.45))
    mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM batch_jobs WHERE status IN ($1,$2)")).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
    mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM batch_jobs WHERE status=$1")).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))

    stats, err := repo.AdminStats(context.Background(), nil, nil)
    require.NoError(t, err)
    require.NotNil(t, stats)

    // Error path: force first query to return error
    db2, mock2, err := sqlmock.New()
    require.NoError(t, err)
    defer db2.Close()
    repo2 := NewBatchRepository(db2)
    mock2.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM batches WHERE 1=1")).WillReturnError(sqlmock.ErrCancelled)
    _, err = repo2.AdminStats(context.Background(), nil, nil)
    require.Error(t, err)
    require.NoError(t, mock.ExpectationsWereMet())
    require.NoError(t, mock2.ExpectationsWereMet())
}



