package http_test

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ikermy/Bulk/internal/limits"
	"github.com/ikermy/Bulk/internal/metrics"
	transporthttp "github.com/ikermy/Bulk/internal/transport/http"
	handlers "github.com/ikermy/Bulk/internal/transport/http/handlers"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// Test that if ValidateUpload returns an error (validation error), handler replies 400
// and metrics increment for reason=validation_error
func TestUpload_ValidationErrorReturns400(t *testing.T) {
	limits.Set(limits.Limits{MaxFileSize: 1024 * 1024})

	// swap ValidateUpload with a stub that returns an error
	orig := handlers.ValidateUpload
	handlers.ValidateUpload = func(file multipart.File, header *multipart.FileHeader) error {
		return errors.New("forced validation error")
	}
	defer func() { handlers.ValidateUpload = orig }()

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, err := w.CreateFormFile("file", "ok.csv")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write([]byte("a,b,c\n1,2,3\n")); err != nil {
		t.Fatalf("write file data: %v", err)
	}
	w.Close()

	req := httptest.NewRequest("POST", "/api/v1/upload", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())

	os.Setenv("INTERNAL_SERVICE_JWT", "test-internal-token")
	req.Header.Set("Authorization", "Bearer test-internal-token")

	r := transporthttp.NewRouter(nil, nil)
	rr := httptest.NewRecorder()

	c := metrics.UploadsRejectedTotal.WithLabelValues("validation_error")
	before := testutil.ToFloat64(c)

	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d, body: %s", rr.Code, rr.Body.String())
	}

	after := testutil.ToFloat64(c)
	if after-before < 1 {
		t.Fatalf("expected uploads_rejected_total{reason=\"validation_error\"} to increase by >=1, before=%v after=%v", before, after)
	}
}
