package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ikermy/Bulk/internal/di"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/ports"
)

type mockValRepo2 struct{}

func (m *mockValRepo2) GetValidationErrors(ctx context.Context, batchID string) ([]*ports.ValidationError, error) {
	return []*ports.ValidationError{{RowNumber: 1, Field: "f", ErrorCode: "E1", ErrorMessage: "m", OriginalValue: "v1"}}, nil
}

type mockJobRepo2 struct{}

func (m *mockJobRepo2) GetResultsByBatch(ctx context.Context, batchID string) ([]*ports.JobResult, error) {
	return []*ports.JobResult{{JobID: "j1", RowNumber: 1, BuildID: "b1", BarcodeURLs: "[\"p1\",\"c1\"]", ErrorCode: "", ErrorMessage: ""}}, nil
}

// implement other methods of ports.JobRepository as no-op to satisfy interface
func (m *mockJobRepo2) Create(ctx context.Context, j *domain.Job) error { return nil }
func (m *mockJobRepo2) GetByBatch(ctx context.Context, batchID string) ([]*domain.Job, error) { return nil, nil }
func (m *mockJobRepo2) UpdateStatus(ctx context.Context, jobID string, status string) error { return nil }
func (m *mockJobRepo2) UpdateBillingTransactionID(ctx context.Context, jobID string, txID *string) error { return nil }

// UpdateStatusWithResult added for interface compatibility in tests
func (m *mockJobRepo2) UpdateStatusWithResult(ctx context.Context, jobID string, status string, result ports.JobResult) error { return nil }

func TestHandleDownloadErrorsXLS_ReturnsXLS(t *testing.T) {
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	// set param id
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "b1"})
	deps := &di.Deps{ValidationRepo: &mockValRepo2{}}
	HandleDownloadErrorsXLS(c, deps)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rw.Code)
	}
	if rw.Header().Get("Content-Type") == "" {
		t.Fatalf("expected content-type set")
	}
	if rw.Body.Len() == 0 {
		t.Fatalf("expected non-empty body")
	}
}

func TestHandleDownloadResultsXLS_ReturnsXLS(t *testing.T) {
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "b1"})
	deps := &di.Deps{JobRepo: &mockJobRepo2{}}
	HandleDownloadResultsXLS(c, deps)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rw.Code)
	}
	if rw.Header().Get("Content-Type") == "" {
		t.Fatalf("expected content-type set")
	}
	if rw.Body.Len() == 0 {
		t.Fatalf("expected non-empty body")
	}
}


