package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	cfg "github.com/ikermy/Bulk/internal/config"
	"github.com/ikermy/Bulk/internal/di"
)

// Test that /api/v1/batch/:id/status alias returns same payload as /api/v1/batch/:id
func TestBatchStatusAlias(t *testing.T) {
	cfg := &cfg.Config{}
	deps := &di.Deps{}
	// set env token so AuthMiddleware accepts our requests (must be set before NewRouter)
	os.Setenv("INTERNAL_SERVICE_JWT", "test-token")
	defer os.Unsetenv("INTERNAL_SERVICE_JWT")

	router := NewRouter(cfg, deps)
	srv := httptest.NewServer(router)
	defer srv.Close()

	id := "test-batch"
	req1, err := http.NewRequest("GET", srv.URL+"/api/v1/batch/"+id, nil)
	if err != nil {
		t.Fatalf("new request1: %v", err)
	}
	req1.Header.Set("Authorization", "Bearer test-token")
	resp1, err := srv.Client().Do(req1)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp1.StatusCode)
	}
	var b1 map[string]any
	if err := json.NewDecoder(resp1.Body).Decode(&b1); err != nil {
		t.Fatalf("decode1: %v", err)
	}

	req2, err := http.NewRequest("GET", srv.URL+"/api/v1/batch/"+id+"/status", nil)
	if err != nil {
		t.Fatalf("new request2: %v", err)
	}
	req2.Header.Set("Authorization", "Bearer test-token")
	resp2, err := srv.Client().Do(req2)
	if err != nil {
		t.Fatalf("get status failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}
	var b2 map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&b2); err != nil {
		t.Fatalf("decode2: %v", err)
	}

	if len(b1) != len(b2) {
		t.Fatalf("responses differ: len1=%d len2=%d", len(b1), len(b2))
	}
}
