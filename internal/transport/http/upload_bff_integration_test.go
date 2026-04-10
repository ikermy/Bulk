//go:build integration
// +build integration

package http

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	bill "github.com/ikermy/Bulk/internal/billing"
	cfg "github.com/ikermy/Bulk/internal/config"
	"github.com/ikermy/Bulk/internal/di"
	"github.com/ikermy/Bulk/internal/usecase/bulk"
	"github.com/ikermy/Bulk/internal/validation"
)

// helper: start a mock BFF server that can respond differently per test via state
func startMockBFF(t *testing.T, validateHandler func(w http.ResponseWriter, r *http.Request), quoteHandler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	h := http.NewServeMux()
	h.HandleFunc("/internal/validate", func(w http.ResponseWriter, r *http.Request) {
		validateHandler(w, r)
	})
	h.HandleFunc("/internal/billing/quote", func(w http.ResponseWriter, r *http.Request) {
		quoteHandler(w, r)
	})
	// also accept block-batch but return ok
	h.HandleFunc("/internal/billing/block-batch", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"transactionIds":["tx1"]}`))
	})
	return httptest.NewServer(h)
}

func doUpload(t *testing.T, srvURL string, fileContent string, revision string) (*http.Response, []byte) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, err := w.CreateFormFile("file", "data.csv")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = io.Copy(fw, strings.NewReader(fileContent))
	_ = w.WriteField("revision", revision)
	w.Close()

	req, err := http.NewRequest(http.MethodPost, srvURL+"/api/v1/upload", &b)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, body
}

func TestUpload_ReadyFlow(t *testing.T) {
	if os.Getenv("RUN_INT_TESTS") != "1" {
		t.Skip("skipping integration tests; set RUN_INT_TESTS=1 to run")
	}
	// mock BFF: validate => valid=true; quote => allowedTotal >= count
	validate := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"valid":true}`))
	}
	quote := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// return allowedTotal big enough and unitPrice
		w.Write([]byte(`{"canProcess":true,"partial":false,"requested":5,"allowedTotal":5,"unitPrice":1.5,"bySource":{"subscription":{"units":2},"credits":{"units":3}}}`))
	}
	bff := startMockBFF(t, validate, quote)
	defer bff.Close()

	// setup service deps
	validator := validation.NewBFFValidator(bff.URL, 2*time.Second, "")
	billingClient := bill.NewBFFBillingClient(bff.URL, 2*time.Second, "")
	svc := bulk.NewService(nil, nil, validator, billingClient, nil, nil)
	deps := &di.Deps{Service: svc, BillingClient: billingClient}

	// set env token before NewRouter
	os.Setenv("INTERNAL_SERVICE_JWT", "test-token")
	defer os.Unsetenv("INTERNAL_SERVICE_JWT")

	router := NewRouter(&cfg.Config{}, deps)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, body := doUpload(t, srv.URL, "first_name,last_name\nval1,val2\nval3,val4\nval5,val6\nval7,val8\nval9,val10", "rev1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(body))
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if out["status"] != "ready" {
		t.Fatalf("expected status ready, got %v", out["status"])
	}
	// check billing.bySource exists
	billing, ok := out["billing"].(map[string]any)
	if !ok {
		t.Fatalf("missing billing: %v", out)
	}
	bySrc, ok := billing["bySource"].(map[string]any)
	if !ok || len(bySrc) == 0 {
		t.Fatalf("billing.bySource missing or empty: %v", billing)
	}
	// estimatedCost in summary
	summary, _ := out["summary"].(map[string]any)
	if _, ok := summary["estimatedCost"]; !ok {
		t.Fatalf("estimatedCost missing in summary: %v", summary)
	}
}

func TestUpload_PartialFlow(t *testing.T) {
	if os.Getenv("RUN_INT_TESTS") != "1" {
		t.Skip("skipping integration tests; set RUN_INT_TESTS=1 to run")
	}
	// validate ok; quote allows fewer than requested
	validate := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"valid":true}`))
	}
	quote := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"canProcess":true,"requested":5,"allowedTotal":2,"unitPrice":2.0,"bySource":{"subscription":{"units":1},"credits":{"units":1}}}`))
	}
	bff := startMockBFF(t, validate, quote)
	defer bff.Close()

	validator := validation.NewBFFValidator(bff.URL, 2*time.Second, "")
	billingClient := bill.NewBFFBillingClient(bff.URL, 2*time.Second, "")
	svc := bulk.NewService(nil, nil, validator, billingClient, nil, nil)
	deps := &di.Deps{Service: svc, BillingClient: billingClient}

	os.Setenv("INTERNAL_SERVICE_JWT", "test-token")
	defer os.Unsetenv("INTERNAL_SERVICE_JWT")

	router := NewRouter(&cfg.Config{}, deps)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, body := doUpload(t, srv.URL, "first_name,last_name\nval1,val2\nval2,val2\nval3,val3\nval4,val4\nval5,val5", "rev1")
	// partial_available is returned as 402 Payment Required per TZ §10.1
	if resp.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d body=%s", resp.StatusCode, string(body))
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if out["code"] != "PARTIAL_AVAILABLE" {
		t.Fatalf("expected code PARTIAL_AVAILABLE, got %v", out["code"])
	}
	// options are in details.options
	details, ok := out["details"].(map[string]any)
	if !ok {
		t.Fatalf("details missing: %v", out)
	}
	opt, ok := details["options"].(map[string]any)
	if !ok {
		t.Fatalf("options missing in details: %v", details)
	}
	if _, ok := opt["generatePartial"].(map[string]any); !ok {
		t.Fatalf("generatePartial missing: %v", opt)
	}
}

func TestUpload_ValidationErrorsFlow(t *testing.T) {
	if os.Getenv("RUN_INT_TESTS") != "1" {
		t.Skip("skipping integration tests; set RUN_INT_TESTS=1 to run")
	}
	// validate returns errors for a row
	validate := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"valid":false,"errors":[{"field":"DAC","code":"INVALID_CHARACTERS","message":"Содержит недопустимые символы","value":"John123","row":5}]}`))
	}
	// quote should not be called but implement stub
	quote := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"canProcess":true,"requested":1,"allowedTotal":1}`))
	}
	bff := startMockBFF(t, validate, quote)
	defer bff.Close()

	validator := validation.NewBFFValidator(bff.URL, 2*time.Second, "")
	billingClient := bill.NewBFFBillingClient(bff.URL, 2*time.Second, "")
	svc := bulk.NewService(nil, nil, validator, billingClient, nil, nil)
	deps := &di.Deps{Service: svc}

	os.Setenv("INTERNAL_SERVICE_JWT", "test-token")
	defer os.Unsetenv("INTERNAL_SERVICE_JWT")

	router := NewRouter(&cfg.Config{}, deps)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, body := doUpload(t, srv.URL, "first_name,last_name\nval1,val2\nval2,val2\nval3,val3\nval4,val4\nval5,John123", "rev1")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.StatusCode, string(body))
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if out["status"] != "validation_errors" {
		t.Fatalf("expected status validation_errors, got %v", out["status"])
	}
	errs, ok := out["errors"].([]interface{})
	if !ok || len(errs) == 0 {
		t.Fatalf("expected errors array in response, got %v", out)
	}
}
