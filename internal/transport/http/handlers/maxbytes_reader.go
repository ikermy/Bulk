package handlers

import (
    "errors"
    "io"
)

// ErrRequestBodyTooLarge возвращается, когда клиент пытается прочитать больше байт,
// чем разрешено. Используется вместо проверки текста ошибки от http.MaxBytesReader.
var ErrRequestBodyTooLarge = errors.New("request body too large")

type maxBytesReadCloser struct {
    rc io.ReadCloser
    n  int64
}

// MaxBytesReader возвращает io.ReadCloser, который при достижении лимита
// возвращает ErrRequestBodyTooLarge. Это позволяет корректно проверять ошибку
// через errors.Is без привязки к тексту ошибки.
func MaxBytesReader(r io.ReadCloser, n int64) io.ReadCloser {
    return &maxBytesReadCloser{rc: r, n: n}
}

func (m *maxBytesReadCloser) Read(p []byte) (int, error) {
    if m.n <= 0 {
        return 0, ErrRequestBodyTooLarge
    }
    if int64(len(p)) > m.n {
        p = p[:m.n]
    }
    nr, err := m.rc.Read(p)
    m.n -= int64(nr)
    if err != nil {
        // propagate underlying error (including io.EOF)
        return nr, err
    }
    if m.n <= 0 {
        // If we've exhausted the budget exactly, next Read should return sentinel
        // but to be consistent, if current Read consumed up to the limit, return nil
        // and subsequent Read will return ErrRequestBodyTooLarge.
    }
    return nr, nil
}

func (m *maxBytesReadCloser) Close() error {
    return m.rc.Close()
}

