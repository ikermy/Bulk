package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ikermy/Bulk/internal/billing"
	"github.com/ikermy/Bulk/internal/di"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/parser"
	"github.com/ikermy/Bulk/internal/ports"
)

// helper to decode response into map
func decodeResp(t *testing.T, rw *httptest.ResponseRecorder) map[string]any {
	var resp map[string]any
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	return resp
}

func TestHandleUpload_MissingRevision_Returns_MISSING_REVISION(t *testing.T) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, _ := w.CreateFormFile("file", "data.csv")
	fw.Write([]byte("col1,col2\nval1,val2\n"))
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Request = req

	deps := createDepsForTest()
	// intentionally do not set revision
	HandleUpload(c, deps)

	if rw.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", rw.Code)
	}
	resp := decodeResp(t, rw)
	if resp["code"] != "MISSING_REVISION" {
		t.Fatalf("expected code MISSING_REVISION, got %v", resp["code"])
	}
}

func TestHandleUpload_InvalidFileFormat_Returns_INVALID_FILE_FORMAT(t *testing.T) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, _ := w.CreateFormFile("file", "bad.xlsx")
	fw.Write([]byte("not-a-zip"))
	// include revision
	_ = w.WriteField("revision", "rev1")
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Request = req

	deps := createDepsForTest()
	HandleUpload(c, deps)

	if rw.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", rw.Code)
	}
	resp := decodeResp(t, rw)
	if resp["code"] != "INVALID_FILE_FORMAT" {
		t.Fatalf("expected code INVALID_FILE_FORMAT, got %v", resp["code"])
	}
}

func TestHandleUpload_EmptyFile_Returns_EMPTY_FILE(t *testing.T) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, _ := w.CreateFormFile("file", "data.csv")
	// write empty content
	fw.Write([]byte(""))
	_ = w.WriteField("revision", "rev1")
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Request = req

	deps := createDepsForTest()
	HandleUpload(c, deps)

	if rw.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", rw.Code)
	}
	resp := decodeResp(t, rw)
	if resp["code"] != "EMPTY_FILE" {
		t.Fatalf("expected code EMPTY_FILE, got %v", resp["code"])
	}
}

func TestHandleUpload_TooManyRows_Returns_TOO_MANY_ROWS(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("col1,col2\n")
	// generate 1005 rows (parser default maxRows is 1000)
	for i := 0; i < 1005; i++ {
		sb.WriteString("v1,v2\n")
	}

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, _ := w.CreateFormFile("file", "data.csv")
	fw.Write([]byte(sb.String()))
	_ = w.WriteField("revision", "rev1")
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Request = req

	deps := createDepsForTest()
	HandleUpload(c, deps)

	if rw.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", rw.Code)
	}
	resp := decodeResp(t, rw)
	if resp["code"] != "TOO_MANY_ROWS" {
		t.Fatalf("expected code TOO_MANY_ROWS, got %v", resp["code"])
	}
}

func TestHandleUpload_PartialAvailable_Returns_PARTIAL_AVAILABLE(t *testing.T) {
	// create small CSV with 5 valid rows
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, _ := w.CreateFormFile("file", "data.csv")
	fw.Write([]byte("first_name,last_name\n1,1\n2,2\n3,3\n4,4\n5,5\n"))
	_ = w.WriteField("revision", "rev1")
	w.Close()

	// prepare deps with billing client that restricts AllowedTotal to 2
	deps := createDepsForTest()
	deps.BillingClient = &mockBillingPartial{allowed: 2}

	req := httptest.NewRequest(http.MethodPost, "/upload", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Request = req

	HandleUpload(c, deps)

	if rw.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402 got %d", rw.Code)
	}
	resp := decodeResp(t, rw)
	if resp["code"] != "PARTIAL_AVAILABLE" {
		t.Fatalf("expected code PARTIAL_AVAILABLE, got %v", resp["code"])
	}
}

func TestHandleUpload_FileTooLarge_Returns_FILE_TOO_LARGE(t *testing.T) {
	// create ~11MB payload to exceed 10MB limit
	large := bytes.Repeat([]byte("a"), 11*1024*1024)
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, _ := w.CreateFormFile("file", "big.csv")
	fw.Write(large)
	_ = w.WriteField("revision", "rev1")
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Request = req

	deps := createDepsForTest()
	HandleUpload(c, deps)

	if rw.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 got %d", rw.Code)
	}
	resp := decodeResp(t, rw)
	if resp["code"] != "FILE_TOO_LARGE" {
		t.Fatalf("expected code FILE_TOO_LARGE, got %v", resp["code"])
	}
}

func TestParserErrorMapping_InvalidHeaders(t *testing.T) {
	status, code, msg, details, ok := parserErrorResponse(parser.ErrInvalidHeaders)
	if !ok {
		t.Fatalf("expected mapping for ErrInvalidHeaders")
	}
	if status != http.StatusBadRequest || code != "INVALID_HEADERS" {
		t.Fatalf("unexpected mapping: %d %s %s", status, code, msg)
	}
	// details may be nil for sentinel ErrInvalidHeaders
	_ = details
}

// mockBillingPartial implements ports.BillingClient minimal methods used in tests
type mockBillingPartial struct{ allowed int }

func (m *mockBillingPartial) Quote(ctx context.Context, user string, count int) (*billing.QuoteResponse, error) {
	return &billing.QuoteResponse{CanProcess: false, Requested: count, AllowedTotal: m.allowed, BySource: billing.QuoteBreakdown{}, UnitPrice: 0}, nil
}
func (m *mockBillingPartial) BlockBatch(ctx context.Context, user string, count int, batchID string) (*billing.BlockBatchResponse, error) {
	return nil, nil
}
func (m *mockBillingPartial) RefundTransactions(ctx context.Context, user string, transactionIDs []string, batchID string) error {
	return nil
}

func TestHandleConfirm_InsufficientFunds_Returns_INSUFFICIENT_FUNDS(t *testing.T) {
	// prepare request body
	body := strings.NewReader(`{"action":"generate_all"}`)
	req := httptest.NewRequest(http.MethodPost, "/batch/b1/confirm", body)
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Request = req
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "b1"})

	// deps: no Service, but BillingClient that returns error
	deps := &di.Deps{BatchRepo: &mockBatchRepo{}, JobRepo: &mockJobRepo{}, BillingClient: &mockBillingErr{err: errors.New("no funds")}}

	HandleConfirm(c, deps)

	if rw.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402 got %d", rw.Code)
	}
	resp := decodeResp(t, rw)
	if resp["code"] != "INSUFFICIENT_FUNDS" {
		t.Fatalf("expected code INSUFFICIENT_FUNDS, got %v", resp["code"])
	}
}

type mockBillingErr struct{ err error }

func (m *mockBillingErr) Quote(ctx context.Context, user string, count int) (*billing.QuoteResponse, error) {
	return nil, m.err
}
func (m *mockBillingErr) BlockBatch(ctx context.Context, user string, count int, batchID string) (*billing.BlockBatchResponse, error) {
	return nil, m.err
}
func (m *mockBillingErr) RefundTransactions(ctx context.Context, user string, transactionIDs []string, batchID string) error {
	return nil
}

func TestHandleBatchStatus_BatchNotFound_And_Conflicts(t *testing.T) {
	// not found
	req := httptest.NewRequest(http.MethodGet, "/batch/b1/status", nil)
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Request = req
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "b1"})

	deps := &di.Deps{BatchRepo: &mockBatchRepoNil{}}
	HandleBatchStatus(c, deps)
	if rw.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d", rw.Code)
	}
	resp := decodeResp(t, rw)
	if resp["code"] != "BATCH_NOT_FOUND" {
		t.Fatalf("expected code BATCH_NOT_FOUND, got %v", resp["code"])
	}

	// batch is completed — теперь возвращает 200 со status="completed" (не 409)
	rw = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(rw)
	c.Request = req
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "b2"})
	deps = &di.Deps{BatchRepo: &mockBatchRepoStatus{status: domain.BatchStatusCompleted}}
	HandleBatchStatus(c, deps)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200 for completed batch got %d", rw.Code)
	}
	resp = decodeResp(t, rw)
	if resp["status"] != string(domain.BatchStatusCompleted) {
		t.Fatalf("expected status completed, got %v", resp["status"])
	}

	// batch is cancelled — теперь возвращает 200 со status="cancelled" (не 409)
	rw = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(rw)
	c.Request = req
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "b3"})
	deps = &di.Deps{BatchRepo: &mockBatchRepoStatus{status: domain.BatchStatusCancelled}}
	HandleBatchStatus(c, deps)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200 for cancelled batch got %d", rw.Code)
	}
	resp = decodeResp(t, rw)
	if resp["status"] != string(domain.BatchStatusCancelled) {
		t.Fatalf("expected status cancelled, got %v", resp["status"])
	}
}

// mockBatchRepoNil returns nil batch
type mockBatchRepoNil struct{}

func (m *mockBatchRepoNil) Create(ctx context.Context, b *domain.Batch) error { return nil }
func (m *mockBatchRepoNil) GetByID(ctx context.Context, id string) (*domain.Batch, error) {
	return nil, nil
}
func (m *mockBatchRepoNil) Update(ctx context.Context, b *domain.Batch) error { return nil }
func (m *mockBatchRepoNil) List(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) {
	return nil, 0, nil
}
func (m *mockBatchRepoNil) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) {
	return &ports.AdminStatsWithQueues{}, nil
}

type mockBatchRepoStatus struct{ status domain.BatchStatus }

func (m *mockBatchRepoStatus) Create(ctx context.Context, b *domain.Batch) error { return nil }
func (m *mockBatchRepoStatus) GetByID(ctx context.Context, id string) (*domain.Batch, error) {
	return &domain.Batch{ID: id, Status: m.status}, nil
}
func (m *mockBatchRepoStatus) Update(ctx context.Context, b *domain.Batch) error { return nil }
func (m *mockBatchRepoStatus) List(ctx context.Context, filter ports.BatchFilter) ([]*domain.Batch, int, error) {
	return nil, 0, nil
}
func (m *mockBatchRepoStatus) AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*ports.AdminStatsWithQueues, error) {
	return &ports.AdminStatsWithQueues{}, nil
}

// mockJobRepo used in some deps
// reuse mockJobRepo from handlers_test.go

// mockBilling and mockProducer are already defined in handlers_test.go — reuse where possible


