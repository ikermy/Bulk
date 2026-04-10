//go:build integration
// +build integration

package http

import (
    "bytes"
    "context"
    "encoding/json"
    "io"
    "mime/multipart"
    "net/http"
    "net/http/httptest"
    "os"
    "strings"
    "testing"

    "github.com/gin-gonic/gin"
    di "github.com/ikermy/Bulk/internal/di"
    handlers "github.com/ikermy/Bulk/internal/transport/http/handlers"
    svc "github.com/ikermy/Bulk/internal/usecase/bulk"
)

// TestIntegration_Upload_Multipart отправляет multipart-запрос к маршруту /api/v1/upload
// и проверяет, что хендлер вернул success и непустой batchId.
func TestIntegration_Upload_Multipart(t *testing.T) {
    if os.Getenv("RUN_INT_TESTS") != "1" {
        t.Skip("skipping integration tests; set RUN_INT_TESTS=1 to run")
    }

    // создаём минимальный сервис без внешних зависимостей
    _ = context.Background()
    service := svc.NewService(nil, nil, nil, nil, nil, nil)
    deps := &di.Deps{Service: service}

    router := gin.New()
    router.POST("/api/v1/upload", func(c *gin.Context) { handlers.HandleUpload(c, deps) })
    srv := httptest.NewServer(router)
    defer srv.Close()

    var b bytes.Buffer
    w := multipart.NewWriter(&b)
    fw, err := w.CreateFormFile("file", "data.csv")
    if err != nil {
        t.Fatalf("create form file: %v", err)
    }
    _, _ = io.Copy(fw, strings.NewReader("col1,col2\nval1,val2\n"))
    _ = w.WriteField("revision", "rev-integ")
    w.Close()

    req, err := http.NewRequest("POST", srv.URL+"/api/v1/upload", &b)
    if err != nil {
        t.Fatalf("new request: %v", err)
    }
    req.Header.Set("Content-Type", w.FormDataContentType())

    resp, err := srv.Client().Do(req)
    if err != nil {
        t.Fatalf("post failed: %v", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        t.Fatalf("expected status 200, got %d", resp.StatusCode)
    }

    var body map[string]any
    if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
        t.Fatalf("decode response: %v", err)
    }
    if ok, _ := body["success"].(bool); !ok {
        t.Fatalf("expected success=true, got: %v", body)
    }
    if bid, _ := body["batchId"].(string); bid == "" {
        t.Fatalf("expected non-empty batchId, got: %v", body)
    }
}

