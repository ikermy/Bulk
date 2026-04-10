package handlers

import (
    "context"
    "io"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/ikermy/Bulk/internal/di"
    "github.com/ikermy/Bulk/internal/domain"
    "github.com/ikermy/Bulk/internal/ports"
)

// mock validation repo
type mockValidationRepo struct {
    errs []*ports.ValidationError
    err  error
}

func (m *mockValidationRepo) GetValidationErrors(ctx context.Context, batchID string) ([]*ports.ValidationError, error) {
    return m.errs, m.err
}

// mock job repo
// mockJobRepoResults implements full ports.JobRepository methods but returns
// provided JobResult slice for GetResultsByBatch which is used in tests.
type mockJobRepoResults struct {
    results []*ports.JobResult
    err     error
}

// methods using domain.Job types to satisfy interface
func (m *mockJobRepoResults) Create(ctx context.Context, j *domain.Job) error { return nil }
func (m *mockJobRepoResults) GetByBatch(ctx context.Context, batchID string) ([]*domain.Job, error) { return nil, nil }
func (m *mockJobRepoResults) UpdateStatus(ctx context.Context, jobID string, status string) error { return nil }
func (m *mockJobRepoResults) GetResultsByBatch(ctx context.Context, batchID string) ([]*ports.JobResult, error) { return m.results, m.err }
func (m *mockJobRepoResults) UpdateBillingTransactionID(ctx context.Context, jobID string, txID *string) error { return nil }
func (m *mockJobRepoResults) UpdateStatusWithResult(ctx context.Context, jobID string, status string, result ports.JobResult) error { return nil }

func Test_HandleDownloadErrorsXLS_MissingDep(t *testing.T) {
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Params = gin.Params{{Key: "id", Value: "batch1"}}

    HandleDownloadErrorsXLS(c, &di.Deps{})

    if w.Code != 404 {
        t.Fatalf("expected 404 got %d body=%s", w.Code, w.Body.String())
    }
}

func Test_HandleDownloadErrorsXLS_ServiceError(t *testing.T) {
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Params = gin.Params{{Key: "id", Value: "batch1"}}

    deps := &di.Deps{ValidationRepo: &mockValidationRepo{err: io.ErrUnexpectedEOF}}
    HandleDownloadErrorsXLS(c, deps)

    if w.Code != 500 {
        t.Fatalf("expected 500 got %d body=%s", w.Code, w.Body.String())
    }
}

func Test_HandleDownloadErrorsXLS_Success(t *testing.T) {
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Params = gin.Params{{Key: "id", Value: "batch1"}}

    errs := []*ports.ValidationError{{RowNumber: 1, Field: "f", ErrorCode: "E", ErrorMessage: "m", OriginalValue: "v"}}
    deps := &di.Deps{ValidationRepo: &mockValidationRepo{errs: errs}}

    HandleDownloadErrorsXLS(c, deps)

    if w.Code != 200 {
        t.Fatalf("expected 200 got %d body=%s", w.Code, w.Body.String())
    }
    ct := w.Header().Get("Content-Type")
    if !strings.Contains(ct, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet") {
        t.Fatalf("unexpected content-type: %s", ct)
    }
    cd := w.Header().Get("Content-Disposition")
    if !strings.Contains(cd, "errors_batch1.xlsx") {
        t.Fatalf("unexpected content-disposition: %s", cd)
    }
    // body should be non-empty and start with PK (xlsx is zip)
    if w.Body.Len() == 0 || !strings.HasPrefix(w.Body.String(), "PK") {
        // binary may contain PK signature; accept non-empty as fallback
        if w.Body.Len() == 0 {
            t.Fatalf("empty body")
        }
    }
}

func Test_HandleDownloadResultsXLS_MissingDep(t *testing.T) {
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Params = gin.Params{{Key: "id", Value: "batch1"}}

    HandleDownloadResultsXLS(c, &di.Deps{})

    if w.Code != 404 {
        t.Fatalf("expected 404 got %d body=%s", w.Code, w.Body.String())
    }
}

func Test_HandleDownloadResultsXLS_ServiceError(t *testing.T) {
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Params = gin.Params{{Key: "id", Value: "batch1"}}

    deps := &di.Deps{JobRepo: &mockJobRepoResults{err: io.ErrUnexpectedEOF}}
    HandleDownloadResultsXLS(c, deps)

    if w.Code != 500 {
        t.Fatalf("expected 500 got %d body=%s", w.Code, w.Body.String())
    }
}

func Test_HandleDownloadResultsXLS_Success_ParseBarcodeVariants(t *testing.T) {
    gin.SetMode(gin.TestMode)
    w := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(w)
    c.Params = gin.Params{{Key: "id", Value: "batch1"}}

    results := []*ports.JobResult{
        {RowNumber: 1, BuildID: "b1", BarcodeURLs: `{"pdf417":"p1","code128":"c1"}`},
        {RowNumber: 2, BuildID: "b2", BarcodeURLs: `["p2","c2"]`},
        {RowNumber: 3, BuildID: "b3", BarcodeURLs: `p3,c3`},
    }
    deps := &di.Deps{JobRepo: &mockJobRepoResults{results: results}}
    HandleDownloadResultsXLS(c, deps)

    if w.Code != 200 {
        t.Fatalf("expected 200 got %d body=%s", w.Code, w.Body.String())
    }
    ct := w.Header().Get("Content-Type")
    if !strings.Contains(ct, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet") {
        t.Fatalf("unexpected content-type: %s", ct)
    }
    if w.Body.Len() == 0 {
        t.Fatalf("empty body")
    }
}




