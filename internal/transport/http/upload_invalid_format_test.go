package http_test

import (
    "bytes"
    "mime/multipart"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"

    transporthttp "github.com/ikermy/Bulk/internal/transport/http"
    "github.com/ikermy/Bulk/internal/metrics"
    "github.com/ikermy/Bulk/internal/limits"
    "github.com/prometheus/client_golang/prometheus/testutil"
)

// Test that uploading a file with mismatched magic bytes/extension is rejected with 400
// and metrics increment for reason=invalid_format
func TestUpload_InvalidFormatReturns400(t *testing.T) {
    // set small limit so body isn't too big for test
    limits.Set(limits.Limits{MaxFileSize: 1024 * 1024})

    var b bytes.Buffer
    w := multipart.NewWriter(&b)
    // name it .xlsx but write non-zip content to trigger invalid format
    fw, err := w.CreateFormFile("file", "bad.xlsx")
    if err != nil {
        t.Fatalf("create form file: %v", err)
    }
    if _, err := fw.Write([]byte("not-a-zip-file-content")); err != nil {
        t.Fatalf("write file data: %v", err)
    }
    w.Close()

    req := httptest.NewRequest("POST", "/api/v1/upload", &b)
    req.Header.Set("Content-Type", w.FormDataContentType())

    os.Setenv("INTERNAL_SERVICE_JWT", "test-internal-token")
    req.Header.Set("Authorization", "Bearer test-internal-token")

    r := transporthttp.NewRouter(nil, nil)
    rr := httptest.NewRecorder()

    c := metrics.UploadsRejectedTotal.WithLabelValues("invalid_format")
    before := testutil.ToFloat64(c)

    r.ServeHTTP(rr, req)

    if rr.Code != http.StatusBadRequest {
        t.Fatalf("expected status 400, got %d, body: %s", rr.Code, rr.Body.String())
    }

    after := testutil.ToFloat64(c)
    if after-before < 1 {
        t.Fatalf("expected uploads_rejected_total{reason=\"invalid_format\"} to increase by >=1, before=%v after=%v", before, after)
    }
}

