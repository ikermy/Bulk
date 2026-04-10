package handlers

import (
    "bytes"
    "errors"
    "io"
    "testing"
)

func TestMaxBytesReader_ReadAndClose(t *testing.T) {
    data := []byte("hello")
    rc := io.NopCloser(bytes.NewReader(data))
    mr := MaxBytesReader(rc, 3)

    buf := make([]byte, 10)
    n, err := mr.Read(buf)
    if err != nil && !errors.Is(err, io.EOF) {
        t.Fatalf("unexpected read error: %v", err)
    }
    if n != 3 {
        t.Fatalf("expected 3 bytes read, got %d", n)
    }

    // subsequent read should return ErrRequestBodyTooLarge
    n, err = mr.Read(buf)
    if err == nil || !errors.Is(err, ErrRequestBodyTooLarge) {
        t.Fatalf("expected ErrRequestBodyTooLarge, got %v, n=%d", err, n)
    }

    // Close should close underlying reader without error
    if err := mr.Close(); err != nil {
        t.Fatalf("close failed: %v", err)
    }
}

func TestMaxBytesReader_ZeroLimit(t *testing.T) {
    rc := io.NopCloser(bytes.NewReader([]byte("x")))
    mr := MaxBytesReader(rc, 0)
    buf := make([]byte, 1)
    n, err := mr.Read(buf)
    if n != 0 || !errors.Is(err, ErrRequestBodyTooLarge) {
        t.Fatalf("expected ErrRequestBodyTooLarge for zero limit, got n=%d err=%v", n, err)
    }
}

