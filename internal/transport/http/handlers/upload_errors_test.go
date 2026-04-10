package handlers

import (
    "bytes"
    "mime/multipart"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/ikermy/Bulk/internal/di"
    "github.com/ikermy/Bulk/internal/usecase/bulk"
)

// reuse createDepsForTest from handlers_test.go by building minimal deps here
func createDepsForUploadTest() *di.Deps {
    // Create a dummy service instance — many tests set nils; reuse NewService with nil repos
    svc := bulk.NewService(nil, nil, nil, nil, nil, nil)
    return &di.Deps{Logger: nil, BatchRepo: nil, JobRepo: nil, BillingClient: nil, Producer: nil, Service: svc}
}

func TestHandleUpload_MissingRevision_Returns400(t *testing.T) {
    var b bytes.Buffer
    w := multipart.NewWriter(&b)
    fw, _ := w.CreateFormFile("file", "data.csv")
    _, _ = fw.Write([]byte("first_name,last_name\nval1,val2\n"))
    w.Close()

    req := httptest.NewRequest(http.MethodPost, "/upload", &b)
    req.Header.Set("Content-Type", w.FormDataContentType())
    rw := httptest.NewRecorder()

    c, _ := gin.CreateTestContext(rw)
    c.Request = req
    deps := createDepsForUploadTest()

    HandleUpload(c, deps)

    if rw.Code != http.StatusBadRequest {
        t.Fatalf("expected 400 for missing revision, got %d", rw.Code)
    }
}




