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

	cfg "github.com/ikermy/Bulk/internal/config"
	di "github.com/ikermy/Bulk/internal/di"
	svc "github.com/ikermy/Bulk/internal/usecase/bulk"
)

// TestIntegration_Router_Health проверяет доступность /health с минимальными deps
func TestIntegration_Router_Health(t *testing.T) {
	if os.Getenv("RUN_INT_TESTS") != "1" {
		t.Skip("skipping integration tests; set RUN_INT_TESTS=1 to run")
	}

	cfg := &cfg.Config{}
	deps := &di.Deps{} // для /health deps не требуются

	router := NewRouter(cfg, deps)
	srv := httptest.NewServer(router)
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("http get /health failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if status, _ := body["status"].(string); status != "ok" {
		t.Fatalf("expected status=ok, got %v", body)
	}
}

// TestIntegration_Upload_MinimalDeps проверяет /api/v1/upload с минимальным Service (без БД/стора)
func TestIntegration_Upload_MinimalDeps(t *testing.T) {
	if os.Getenv("RUN_INT_TESTS") != "1" {
		t.Skip("skipping integration tests; set RUN_INT_TESTS=1 to run")
	}

	// выставляем env-token, чтобы AuthMiddleware пропустил запрос
	os.Setenv("INTERNAL_SERVICE_JWT", "test-token")
	defer os.Unsetenv("INTERNAL_SERVICE_JWT")

	// минимальный сервис: NewService с nil-репозиториями — подходит для CreateBatchFromFile
	_ = context.Background()
	service := svc.NewService(nil, nil, nil, nil, nil, nil)
	deps := &di.Deps{Service: service}
	cfg := &cfg.Config{}

	router := NewRouter(cfg, deps)
	srv := httptest.NewServer(router)
	defer srv.Close()

	// подготовить multipart тело
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, err := w.CreateFormFile("file", "data.csv")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	_, _ = io.Copy(fw, strings.NewReader("first_name,last_name\nval1,val2\n"))
	_ = w.WriteField("revision", "rev-min")
	w.Close()

	req, err := http.NewRequest("POST", srv.URL+"/api/v1/upload", &b)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer test-token")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if ok, _ := body["success"].(bool); !ok {
		t.Fatalf("expected success=true, got %v", body)
	}
	if bid, _ := body["batchId"].(string); bid == "" {
		t.Fatalf("expected non-empty batchId, got %v", body)
	}
}
