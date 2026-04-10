package postgres

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// TestBatch_ToDomain_NilReceiver проверяет, что nil-ресивер возвращает nil.
func TestBatch_ToDomain_NilReceiver(t *testing.T) {
	var b *Batch
	got := b.ToDomain()
	require.Nil(t, got)
}

// TestGetByID_PlainArrayJSON проверяет ветку парсинга JSON-массива (без wrapper-объекта).
func TestGetByID_PlainArrayJSON(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewBatchRepository(db)
	cols := []string{"id", "user_id", "status", "revision", "file_storage_id",
		"total_rows", "valid_rows", "approved_count", "completed_count", "failed_count",
		"created_at", "completed_at", "transaction_ids"}

	// plain JSON array (legacy format without wrapper)
	txJSON := `["tx1","tx2"]`
	rows := sqlmock.NewRows(cols).AddRow("b2", "u2", "S", "r", "f", 5, 5, 0, 0, 0, time.Now(), nil, txJSON)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id,user_id,status,revision,file_storage_id,total_rows,valid_rows,approved_count,completed_count,failed_count,created_at,completed_at,transaction_ids FROM batches WHERE id=$1")).
		WithArgs("b2").
		WillReturnRows(rows)

	b, err := repo.GetByID(context.Background(), "b2")
	require.NoError(t, err)
	require.Len(t, b.TransactionIDs, 2)
	require.Equal(t, "tx1", b.TransactionIDs[0])
	require.Equal(t, "tx2", b.TransactionIDs[1])
	// no config in plain array → Priority and TimeoutMs are zero values
	require.Equal(t, "", b.Priority)
	require.Equal(t, 0, b.TimeoutMs)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGetByID_DBError проверяет, что ошибка БД пробрасывается наружу.
func TestGetByID_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewBatchRepository(db)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id,user_id,status,revision,file_storage_id,total_rows,valid_rows,approved_count,completed_count,failed_count,created_at,completed_at,transaction_ids FROM batches WHERE id=$1")).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	_, err = repo.GetByID(context.Background(), "missing")
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestBatchRepository_ListWithFilters_AllParams проверяет ветку с полными фильтрами (userID,
// revision, from, to, cursor, sortBy=completedAt).
func TestBatchRepository_ListWithFilters_AllParams(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewBatchRepository(db)

	now := time.Now()
	fromT := now.Add(-24 * time.Hour)
	toT := now
	cursorT := now.Add(-1 * time.Hour)

	cols := []string{"id", "user_id", "status", "revision", "file_storage_id",
		"total_rows", "valid_rows", "approved_count", "completed_count", "failed_count",
		"created_at", "completed_at", "transaction_ids"}

	// COUNT query — match any query starting with SELECT COUNT
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM batches`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Rows query
	mock.ExpectQuery(`SELECT id,user_id`).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow("b3", "u3", "completed", "rev2", "f3", 2, 2, 1, 1, 0, now, now, `{"transactions":["t3"]}`))

	res, total, err := repo.ListWithFilters(
		context.Background(),
		"completed",   // status
		"u3",          // userID
		"rev2",        // revision
		&fromT,        // from
		&toT,          // to
		"completedAt", // sortBy
		true,          // desc
		&cursorT,      // cursor
		5,             // limit
		0,             // offset
	)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, res, 1)
	require.Equal(t, "b3", res[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestBatchRepository_ListWithFilters_CursorAsc проверяет ветку курсора при ASC-сортировке.
func TestBatchRepository_ListWithFilters_CursorAsc(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewBatchRepository(db)
	cursorT := time.Now().Add(-1 * time.Hour)

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM batches`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`SELECT id,user_id`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "user_id", "status", "revision", "file_storage_id",
			"total_rows", "valid_rows", "approved_count", "completed_count", "failed_count",
			"created_at", "completed_at", "transaction_ids",
		}))

	_, _, err = repo.ListWithFilters(
		context.Background(),
		"", "", "",
		nil, nil,
		"",    // sortBy = createdAt
		false, // asc
		&cursorT,
		10, 0,
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestValidationRepository_GetValidationErrors проверяет успешное чтение ошибок валидации.
func TestValidationRepository_GetValidationErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewValidationRepository(db)

	cols := []string{"row_number", "field", "error_code", "error_message", "original_value"}
	rows := sqlmock.NewRows(cols).
		AddRow(1, "passportNumber", "INVALID_FORMAT", "Invalid format", "abc").
		AddRow(2, "fullName", "MISSING_MANDATORY", "Field is required", "")

	mock.ExpectQuery(regexp.QuoteMeta("SELECT row_number, field, error_code, error_message, original_value FROM batch_validation_errors WHERE batch_id=$1 ORDER BY row_number")).
		WithArgs("batch-abc").
		WillReturnRows(rows)

	res, err := repo.GetValidationErrors(context.Background(), "batch-abc")
	require.NoError(t, err)
	require.Len(t, res, 2)
	require.Equal(t, 1, res[0].RowNumber)
	require.Equal(t, "passportNumber", res[0].Field)
	require.Equal(t, "INVALID_FORMAT", res[0].ErrorCode)
	require.Equal(t, "Invalid format", res[0].ErrorMessage)
	require.Equal(t, "abc", res[0].OriginalValue)
	require.Equal(t, 2, res[1].RowNumber)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestValidationRepository_GetValidationErrors_DBError проверяет ошибку БД.
func TestValidationRepository_GetValidationErrors_DBError(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewValidationRepository(db)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT row_number, field, error_code, error_message, original_value FROM batch_validation_errors WHERE batch_id=$1 ORDER BY row_number")).
		WithArgs("bad-id").
		WillReturnError(sql.ErrConnDone)

	_, err = repo.GetValidationErrors(context.Background(), "bad-id")
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGetByID_NullCompletedAt проверяет ветку completedAt.Valid == false.
func TestGetByID_NullCompletedAt(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewBatchRepository(db)
	cols := []string{"id", "user_id", "status", "revision", "file_storage_id",
		"total_rows", "valid_rows", "approved_count", "completed_count", "failed_count",
		"created_at", "completed_at", "transaction_ids"}

	rows := sqlmock.NewRows(cols).AddRow("b4", "u4", "pending", "r", "f", 1, 0, 0, 0, 0, time.Now(), nil, nil)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id,user_id,status,revision,file_storage_id,total_rows,valid_rows,approved_count,completed_count,failed_count,created_at,completed_at,transaction_ids FROM batches WHERE id=$1")).
		WithArgs("b4").WillReturnRows(rows)

	b, err := repo.GetByID(context.Background(), "b4")
	require.NoError(t, err)
	require.True(t, b.CompletedAt.IsZero())
	require.NoError(t, mock.ExpectationsWereMet())
}
