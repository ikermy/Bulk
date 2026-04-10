package bulk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ikermy/Bulk/internal/ports"
	"github.com/ikermy/Bulk/internal/validation"
	pkgvalidation "github.com/ikermy/Bulk/pkg/validation"
)

func TestCreateBatchFromFile_EmptyParserError(t *testing.T) {
	svc := NewService(nil, nil, nil, nil, nil, nil)
	_, err := svc.CreateBatchFromFile(context.Background(), strings.NewReader(""), "rev")
	if err == nil {
		t.Fatalf("expected parser error for empty file")
	}
}

func TestCreateBatchFromFile_ValidatorInvalid(t *testing.T) {
	// start a test HTTP server that emulates validation BFF
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// always return invalid
		vr := pkgvalidation.ValidationResult{Valid: false, Errors: []pkgvalidation.ValidationError{{Code: "E1", Message: "bad"}}}
		b, _ := json.Marshal(vr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	}))
	defer ts.Close()

	batchRepo := &mockBatchRepo{}
	jobRepo := &mockJobRepo{}
	// create real BFF validator pointing to our test server
	v := validation.NewBFFValidator(ts.URL, time.Second, "")
	svc := NewService(batchRepo, jobRepo, v, nil, nil, nil)

	// use headers that parser recognizes (aliases in column_detector.go)
	res, err := svc.CreateBatchFromFile(context.Background(), strings.NewReader("first_name,last_name\n1,2\n"), "rev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.InvalidRows == 0 {
		t.Fatalf("expected invalid rows due to validator, got 0")
	}
	if len(res.Errors) == 0 {
		t.Fatalf("expected validation errors collected")
	}
}

func TestExportResultsXLS_Formats(t *testing.T) {
	// prepare results with different barcode formats
	tests := []struct{ name, in, want1, want2 string }{
		{"object", `{"pdf417":"P","code128":"C"}`, "P", "C"},
		{"array", `["P","C"]`, "P", "C"},
		{"csv", `P,C`, "P", "C"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			jobRepo := &mockJobRepo{results: []*ports.JobResult{{JobID: "j1", RowNumber: 2, BarcodeURLs: tc.in}}}

			svc := NewService(nil, jobRepo, nil, nil, nil, nil)
			rc, err := svc.ExportResultsXLS(context.Background(), "b1", 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			defer rc.Close()
			b, _ := io.ReadAll(rc)
			if !bytes.Contains(b, []byte(tc.want1)) || !bytes.Contains(b, []byte(tc.want2)) {
				t.Fatalf("expected file to contain %q and %q", tc.want1, tc.want2)
			}
		})
	}
}

