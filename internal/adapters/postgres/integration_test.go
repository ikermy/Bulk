package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ikermy/Bulk/internal/db"
)

// Integration test for Postgres repositories. Runs only when RUN_INT_TESTS=1
func TestPostgresIntegration_CreateAndGet(t *testing.T) {
	if os.Getenv("RUN_INT_TESTS") != "1" {
		t.Skip("skipping integration tests; set RUN_INT_TESTS=1 to run")
	}
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set")
	}
	db, err := db.Connect(dsn)
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	// use small timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	repo := NewBatchRepository(db)
	b := &Batch{ID: "itest-1", UserID: "u", Status: "pending", Revision: "r", TotalRows: 1, ValidRows: 1}
	if err := repo.Create(ctx, b); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	got, err := repo.GetByID(ctx, "itest-1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.ID != b.ID {
		t.Fatalf("expected %s got %s", b.ID, got.ID)
	}
}
