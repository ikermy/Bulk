package handlers

import (
    "bytes"
    "errors"
    "io"
    "mime/multipart"
    "net/http"
    "net/http/httptest"
    "testing"
    "github.com/gin-gonic/gin"

    "github.com/ikermy/Bulk/internal/di"
    svc "github.com/ikermy/Bulk/internal/usecase/bulk"
    "github.com/ikermy/Bulk/internal/limits"
    "github.com/ikermy/Bulk/internal/parser"
)

func TestParserErrorResponse_EmptyFile(t *testing.T) {
    status, code, msg, _, ok := parserErrorResponse(parser.ErrEmptyFile)
    if !ok { t.Fatalf("expected handled error") }
    if status != http.StatusBadRequest { t.Fatalf("expected 400, got %d", status) }
    if code != "EMPTY_FILE" { t.Fatalf("expected EMPTY_FILE, got %s", code) }
    if msg == "" { t.Fatalf("expected non-empty message") }
}

func TestParserErrorResponse_TooManyRows(t *testing.T) {
    status, code, _, _, ok := parserErrorResponse(parser.ErrTooManyRows)
    if !ok { t.Fatalf("expected handled error") }
    if status != http.StatusBadRequest { t.Fatalf("expected 400, got %d", status) }
    if code != "TOO_MANY_ROWS" { t.Fatalf("expected TOO_MANY_ROWS, got %s", code) }
}

func TestParserErrorResponse_InvalidHeaders(t *testing.T) {
    ihe := &parser.InvalidHeadersError{Missing: []string{"a","b"}}
    status, code, _, details, ok := parserErrorResponse(ihe)
    if !ok { t.Fatalf("expected handled error") }
    if status != http.StatusBadRequest { t.Fatalf("expected 400, got %d", status) }
    if code != "INVALID_HEADERS" { t.Fatalf("expected INVALID_HEADERS, got %s", code) }
    if details == nil { t.Fatalf("expected details map for InvalidHeadersError") }
}

func TestParserErrorResponse_Unhandled(t *testing.T) {
    status, code, _, _, ok := parserErrorResponse(errors.New("other"))
    if ok { t.Fatalf("expected unhandled error") }
    if status != 0 || code != "" { t.Fatalf("expected zero values for unhandled error") }
}


// helper to build multipart request with a single file part and optional fields
func makeMultipart(bodyContent []byte, filename string, fields map[string]string) (io.Reader, string, error) {
    var buf bytes.Buffer
    w := multipart.NewWriter(&buf)
    for k, v := range fields {
        if err := w.WriteField(k, v); err != nil {
            return nil, "", err
        }
    }
    fw, err := w.CreateFormFile("file", filename)
    if err != nil {
        return nil, "", err
    }
    if _, err := fw.Write(bodyContent); err != nil {
        return nil, "", err
    }
    _ = w.Close()
    return &buf, w.FormDataContentType(), nil
}

func TestHandleUpload_MissingFile(t *testing.T) {
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    // empty request
    req := httptest.NewRequest(http.MethodPost, "/", nil)
    c.Request = req

    HandleUpload(c, nil)
    if rw.Code != http.StatusBadRequest {
        t.Fatalf("expected 400 got %d", rw.Code)
    }
}

func TestHandleUpload_ValidateTooLarge(t *testing.T) {
    // stub ValidateUpload to simulate ErrFileTooLarge
    orig := ValidateUpload
    ValidateUpload = func(file multipart.File, header *multipart.FileHeader) error { return ErrFileTooLarge }
    defer func() { ValidateUpload = orig }()

    body, ctype, err := makeMultipart([]byte("small"), "test.csv", map[string]string{"revision": "r1"})
    if err != nil {
        t.Fatalf("make multipart: %v", err)
    }
    req := httptest.NewRequest(http.MethodPost, "/", body)
    req.Header.Set("Content-Type", ctype)
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    c.Request = req

    HandleUpload(c, &di.Deps{})
    if rw.Code != http.StatusRequestEntityTooLarge {
        t.Fatalf("expected 413 got %d", rw.Code)
    }
}

func TestHandleUpload_MissingRevision(t *testing.T) {
    // use real ValidateUpload (csv) so creating content works
    csv := []byte("first_name,last_name\nval1,val2\n")
    body, ctype, err := makeMultipart(csv, "test.csv", nil)
    if err != nil {
        t.Fatalf("make multipart: %v", err)
    }
    req := httptest.NewRequest(http.MethodPost, "/", body)
    req.Header.Set("Content-Type", ctype)
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    c.Request = req

    HandleUpload(c, &di.Deps{})
    if rw.Code != http.StatusBadRequest {
        t.Fatalf("expected 400 got %d", rw.Code)
    }
}

func TestHandleUpload_ServiceUnavailable(t *testing.T) {
    // provide file + revision but deps.Service is nil -> 500
    csv := []byte("first_name,last_name\nval1,val2\n")
    body, ctype, err := makeMultipart(csv, "test.csv", map[string]string{"revision": "r1"})
    if err != nil {
        t.Fatalf("make multipart: %v", err)
    }
    req := httptest.NewRequest(http.MethodPost, "/", body)
    req.Header.Set("Content-Type", ctype)
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    c.Request = req

    HandleUpload(c, &di.Deps{})
    if rw.Code != http.StatusInternalServerError {
        t.Fatalf("expected 500 got %d", rw.Code)
    }
}

func TestHandleUpload_Success_NoBilling(t *testing.T) {
    // ensure limits large enough for test file
    prev := limits.Get()
    limits.Set(limits.Limits{MaxFileSize: 10 * 1024 * 1024})
    defer limits.Set(prev)

    csv := []byte("first_name,last_name\nval1,val2\n")
    body, ctype, err := makeMultipart(csv, "test.csv", map[string]string{"revision": "r1"})
    if err != nil {
        t.Fatalf("make multipart: %v", err)
    }
    req := httptest.NewRequest(http.MethodPost, "/", body)
    req.Header.Set("Content-Type", ctype)
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    c.Request = req

    // create a minimal real service that can parse the CSV
    service := svc.NewService(nil, nil, nil, nil, nil, nil)
    deps := &di.Deps{Service: service}

    HandleUpload(c, deps)
    if rw.Code != http.StatusOK {
        t.Fatalf("expected 200 got %d, body: %s", rw.Code, rw.Body.String())
    }
}


