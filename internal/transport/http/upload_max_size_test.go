package http_test

import (
    "bytes"
    "mime/multipart"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/ikermy/Bulk/internal/limits"
    "github.com/ikermy/Bulk/internal/metrics"
    transporthttp "github.com/ikermy/Bulk/internal/transport/http"
    "github.com/prometheus/client_golang/prometheus/testutil"
)

// Test that uploading a file larger than runtime limit returns 413 and increments metric
func TestUpload_TooLargeReturns413(t *testing.T) {
    gin.SetMode(gin.TestMode)

    // set a small runtime limit (1 KB)
    limits.Set(limits.Limits{MaxFileSize: 1024})

    // build multipart body with a file > 1KB
    var b bytes.Buffer
    w := multipart.NewWriter(&b)
    fw, err := w.CreateFormFile("file", "big.csv")
    if err != nil {
        t.Fatalf("create form file: %v", err)
    }
    // write 2KB of data
    data := bytes.Repeat([]byte("a"), 2048)
    if _, err := fw.Write(data); err != nil {
        t.Fatalf("write file data: %v", err)
    }
    w.Close()

    req := httptest.NewRequest("POST", "/api/v1/upload", &b)
    req.Header.Set("Content-Type", w.FormDataContentType())

    // create router and serve
    // enable legacy internal service token for test and set header accordingly
    // set env token so middleware allows legacy token path
    // AuthMiddleware reads env on construction in NewRouter; set INTERNAL_SERVICE_JWT
    // and include the same token in Authorization header so request passes auth.
    // we import os here to set env
    // (add os import above)
    os.Setenv("INTERNAL_SERVICE_JWT", "test-internal-token")
    req.Header.Set("Authorization", "Bearer test-internal-token")

    r := transporthttp.NewRouter(nil, nil)
    rr := httptest.NewRecorder()

    // measure metric delta to avoid flakes when tests run in a suite
    c := metrics.UploadsRejectedTotal.WithLabelValues("too_large")
    before := testutil.ToFloat64(c)

    r.ServeHTTP(rr, req)

    if rr.Code != http.StatusRequestEntityTooLarge {
        t.Fatalf("expected status 413, got %d, body: %s", rr.Code, rr.Body.String())
    }

    // metric should have incremented for reason=too_large
    after := testutil.ToFloat64(c)
    if after-before < 1 {
        t.Fatalf("expected uploads_rejected_total{reason=\"too_large\"} to increase by >=1, before=%v after=%v", before, after)
    }
}




