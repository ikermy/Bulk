package validation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBFFValidator_ValidateRow(t *testing.T) {
	// mock server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"valid": true}`))
	}))
	defer srv.Close()

	v := NewBFFValidator(srv.URL, 0, "")
	res, err := v.ValidateRow(context.Background(), map[string]string{"col_1": "v"}, "rev1")
	if err != nil {
		t.Fatalf("validate error: %v", err)
	}
	if res == nil || !res.Valid {
		t.Fatalf("expected valid true, got %v", res)
	}
}
