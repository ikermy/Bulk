package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ikermy/Bulk/internal/batch"
	"github.com/ikermy/Bulk/internal/billing"
	"github.com/ikermy/Bulk/internal/di"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/testutil"
	svc "github.com/ikermy/Bulk/internal/usecase/bulk"
	"github.com/stretchr/testify/require"
)

// reuse makeMultipart from other tests in this package

func TestHandleUpload_BillingReady(t *testing.T) {
	csv := []byte("first_name,last_name\nval1,val2\n")
	body, ctype, err := makeMultipart(csv, "test.csv", map[string]string{"revision": "r1"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", ctype)
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Request = req

	service := svc.NewService(nil, nil, nil, nil, nil, nil)

	// billing client that allows full processing
	mb := &testutil.MockBilling{}
	mb.QuoteFn = func(ctx context.Context, user string, count int) (*billing.QuoteResponse, error) {
		return &billing.QuoteResponse{AllowedTotal: count, CanProcess: true, UnitPrice: 0}, nil
	}
	deps := &di.Deps{Service: service, BillingClient: mb}

	// call handler
	HandleUpload(c, deps)
	require.Equal(t, http.StatusOK, rw.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rw.Body.Bytes(), &out))
	require.Equal(t, "ready", out["status"])
}

func TestHandleUpload_BillingPartialAndError(t *testing.T) {
	csv := []byte("first_name,last_name\nval1,val2\n")
	body, ctype, err := makeMultipart(csv, "test.csv", map[string]string{"revision": "r1"})
	require.NoError(t, err)

	// Partial available: return a QuoteResponse with AllowedTotal < validRows via custom MockBilling
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", ctype)
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Request = req

	service := svc.NewService(nil, nil, nil, nil, nil, nil)

	// Use default MockBilling (no QuoteFn) which returns an empty QuoteResponse (AllowedTotal=0)
	deps := &di.Deps{Service: service, BillingClient: &testutil.MockBilling{}}
	HandleUpload(c, deps)
	// default MockBilling returns zero AllowedTotal -> PARTIAL_AVAILABLE -> 402
	require.Equal(t, http.StatusPaymentRequired, rw.Code)

	// billing error path: make Quote return error
	body2, ctype2, err := makeMultipart(csv, "test.csv", map[string]string{"revision": "r1"})
	require.NoError(t, err)
	req2 := httptest.NewRequest(http.MethodPost, "/", body2)
	req2.Header.Set("Content-Type", ctype2)
	rw2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rw2)
	c2.Request = req2
	mbErr := &testutil.MockBilling{}
	mbErr.QuoteFn = func(ctx context.Context, user string, count int) (*billing.QuoteResponse, error) {
		return nil, fmt.Errorf("boom")
	}
	deps2 := &di.Deps{Service: service, BillingClient: mbErr}
	HandleUpload(c2, deps2)
	require.Equal(t, http.StatusServiceUnavailable, rw2.Code)
}

func TestHandleUpload_BatchManagerFinalizeCalled(t *testing.T) {
	csv := []byte("first_name,last_name\nval1,val2\n")
	body, ctype, err := makeMultipart(csv, "test.csv", map[string]string{"revision": "r1"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", ctype)
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Request = req

	service := svc.NewService(nil, nil, nil, nil, nil, nil)

	// mock batch repo to observe Update calls
	called := false
	mbr := &testutil.MockBatchRepo{}
	mbr.GetByIDFn = func(ctx context.Context, id string) (*domain.Batch, error) { return &domain.Batch{ID: id}, nil }
	mbr.UpdateFn = func(ctx context.Context, b *domain.Batch) error { called = true; return nil }

	mgr := batch.NewBatchManager(mbr, nil, nil, nil, nil, "")
	deps := &di.Deps{Service: service, BatchManager: mgr}
	HandleUpload(c, deps)
	require.Equal(t, http.StatusOK, rw.Code)
	require.True(t, called, "expected BatchRepo.Update to be called by BatchManager.FinalizeAfterUpload")
}

func TestHandleUpload_BillingWithEstimate(t *testing.T) {
	csv := []byte("first_name,last_name\nval1,val2\n")
	body, ctype, err := makeMultipart(csv, "test.csv", map[string]string{"revision": "r1"})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", ctype)
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Request = req

	service := svc.NewService(nil, nil, nil, nil, nil, nil)

	// billing client that allows full processing and provides unit price + bySource
	mb := &testutil.MockBilling{}
	mb.QuoteFn = func(ctx context.Context, user string, count int) (*billing.QuoteResponse, error) {
		return &billing.QuoteResponse{AllowedTotal: count, CanProcess: true, UnitPrice: 0.5, BySource: billing.QuoteBreakdown{Subscription: billing.SourceBreakdown{Units: 10}}}, nil
	}
	deps := &di.Deps{Service: service, BillingClient: mb}

	HandleUpload(c, deps)
	require.Equal(t, http.StatusOK, rw.Code)
	var out map[string]any
	require.NoError(t, json.Unmarshal(rw.Body.Bytes(), &out))
	require.Equal(t, "ready", out["status"])
	// summary should include estimatedCost when unitPrice is set
	summary, ok := out["summary"].(map[string]any)
	require.True(t, ok)
	if _, ok := summary["estimatedCost"]; !ok {
		t.Fatalf("expected estimatedCost in summary")
	}
}

