package handlers

import (
    "bytes"
    "mime/multipart"
    "net/http"
    "testing"
)

// helper to create multipart.FileHeader-like setup
func makeMultipartFile(t *testing.T, filename string, content []byte) (multipart.File, *multipart.FileHeader) {
    t.Helper()
    var b bytes.Buffer
    w := multipart.NewWriter(&b)
    fw, err := w.CreateFormFile("file", filename)
    if err != nil {
        t.Fatalf("create form file: %v", err)
    }
    _, _ = fw.Write(content)
    w.Close()
    // parse back
    req, _ := http.NewRequest("POST", "/", &b)
    req.Header.Set("Content-Type", w.FormDataContentType())
    file, header, err := req.FormFile("file")
    if err != nil {
        t.Fatalf("form file parse: %v", err)
    }
    return file, header
}

func TestValidateUpload_XLSX(t *testing.T) {
    // PK\x03\x04 signature
    content := []byte{'P','K',3,4,0,0,0}
    f, h := makeMultipartFile(t, "test.xlsx", content)
    defer f.Close()
    if err := ValidateUpload(f, h); err != nil {
        t.Fatalf("expected xlsx valid, got err: %v", err)
    }
}

func TestValidateUpload_XLS(t *testing.T) {
    content := []byte{0xD0,0xCF,0x11,0xE0,0,0,0}
    f, h := makeMultipartFile(t, "test.xls", content)
    defer f.Close()
    if err := ValidateUpload(f, h); err != nil {
        t.Fatalf("expected xls valid, got err: %v", err)
    }
}

func TestValidateUpload_CSV(t *testing.T) {
    content := []byte("first_name,last_name\nval1,val2\n")
    f, h := makeMultipartFile(t, "test.csv", content)
    defer f.Close()
    if err := ValidateUpload(f, h); err != nil {
        t.Fatalf("expected csv valid, got err: %v", err)
    }
}

func TestValidateUpload_RejectExt(t *testing.T) {
    content := []byte("<html></html>")
    f, h := makeMultipartFile(t, "test.html", content)
    defer f.Close()
    if err := ValidateUpload(f, h); err == nil {
        t.Fatalf("expected invalid extension to be rejected")
    }
}

func TestValidateUpload_FileTooLarge_HeaderSize(t *testing.T) {
    // create a valid small csv content but set header.Size > maxFileSize
    content := []byte("first_name,last_name\nval1,val2\n")
    f, h := makeMultipartFile(t, "test.csv", content)
    defer f.Close()
    // artificially set header size to exceed limit
    h.Size = maxFileSize + 1
    if err := ValidateUpload(f, h); err == nil {
        t.Fatalf("expected file too large error due to header.Size, got nil")
    }
}


