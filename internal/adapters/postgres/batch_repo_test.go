package postgres

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestBatch_ToDomain(t *testing.T) {
	now := time.Now()
	b := &Batch{ID: "b1", UserID: "u1", Status: "S", Revision: "r", FileStorageID: "f", TotalRows: 3, ValidRows: 2, ApprovedCount: 1, CompletedCount: 1, FailedCount: 0, CreatedAt: now, CompletedAt: now, TransactionIDs: []string{"t1"}, Priority: "p", TimeoutMs: 123}
	d := b.ToDomain()
	if d == nil {
		t.Fatalf("expected domain batch, got nil")
	}
	if d.ID != "b1" || d.Priority != "p" || d.TimeoutMs != 123 {
		t.Fatalf("unexpected domain conversion: %+v", d)
	}
}

func TestGetByID_ParseTransactionIDs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewBatchRepository(db)

	cols := []string{"id", "user_id", "status", "revision", "file_storage_id", "total_rows", "valid_rows", "approved_count", "completed_count", "failed_count", "created_at", "completed_at", "transaction_ids"}
	txJSON := `{"transactions":["tx1","tx2"],"config":{"priority":"high","timeout":60000}}`
	// use driver.Value for completed_at as NULL
	rows := sqlmock.NewRows(cols).AddRow("b1", "u1", "S", "r", "f", 10, 9, 1, 0, 0, time.Now(), nil, txJSON)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id,user_id,status,revision,file_storage_id,total_rows,valid_rows,approved_count,completed_count,failed_count,created_at,completed_at,transaction_ids FROM batches WHERE id=$1")).WithArgs("b1").WillReturnRows(rows)

	b, err := repo.GetByID(context.Background(), "b1")
	if err != nil {
		t.Fatalf("GetByID returned error: %v", err)
	}
	if len(b.TransactionIDs) != 2 {
		t.Fatalf("expected 2 transaction ids, got %v", b.TransactionIDs)
	}
	if b.Priority != "high" || b.TimeoutMs != 60000 {
		t.Fatalf("expected priority/timeout parsed from JSON wrapper, got %s/%d", b.Priority, b.TimeoutMs)
	}
	// ensure all expectations met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
