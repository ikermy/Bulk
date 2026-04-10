package handlers

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
)

func TestHandleDownloadTemplateXLS(t *testing.T) {
    // create gin context
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    req := httptest.NewRequest(http.MethodGet, "/template/rev1", nil)
    c.Request = req

    HandleDownloadTemplateXLS(c)

    if rw.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rw.Code)
    }
    ct := rw.Header().Get("Content-Type")
    if ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
        t.Fatalf("unexpected content type: %s", ct)
    }
}

