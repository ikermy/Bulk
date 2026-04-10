package validation

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    pkgval "github.com/ikermy/Bulk/pkg/validation"
    "github.com/stretchr/testify/require"
)

func TestValidateRow_Success(t *testing.T) {
    // start test server that returns valid ValidationResult
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        require.Equal(t, "/internal/validate", r.URL.Path)
        w.Header().Set("Content-Type", "application/json")
        _ = json.NewEncoder(w).Encode(pkgval.ValidationResult{Valid: true})
    }))
    defer srv.Close()

    v := NewBFFValidator(srv.URL, 2*time.Second, "service-token")
    row := map[string]string{"passportNumber": "ABC123"}
    res, err := v.ValidateRow(context.Background(), row, "v1")
    require.NoError(t, err)
    require.NotNil(t, res)
    require.True(t, res.Valid)
}

func TestValidateRow_BFFErrorStatus(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusInternalServerError)
        w.Write([]byte("boom"))
    }))
    defer srv.Close()

    v := NewBFFValidator(srv.URL, 2*time.Second, "")
    _, err := v.ValidateRow(context.Background(), map[string]string{"a": "b"}, "v1")
    require.Error(t, err)
}

func TestValidateRow_DecodeError(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte("not-json"))
    }))
    defer srv.Close()

    v := NewBFFValidator(srv.URL, 2*time.Second, "")
    _, err := v.ValidateRow(context.Background(), map[string]string{"a": "b"}, "v1")
    require.Error(t, err)
}

