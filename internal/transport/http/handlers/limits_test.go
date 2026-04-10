package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	cfg "github.com/ikermy/Bulk/internal/config"
	"github.com/ikermy/Bulk/internal/limits"
	"github.com/stretchr/testify/require"
)

func TestGetPutBulkLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// initial get
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/", nil)
	c.Request = req

	HandleGetBulkLimits(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// put invalid
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	body := `{"maxFileSize":0,"maxRowsPerBatch":0,"maxConcurrentBatches":0,"maxBatchesPerHour":0}`
	req2 := httptest.NewRequest("PUT", "/", strings.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	c2.Request = req2
	HandlePutBulkLimits(c2)
	if w2.Code == http.StatusOK {
		t.Fatalf("expected validation error for zero limits")
	}

	// put valid
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	body3 := `{"maxFileSize":1024,"maxRowsPerBatch":10,"maxConcurrentBatches":2,"maxBatchesPerHour":5}`
	req3 := httptest.NewRequest("PUT", "/", strings.NewReader(body3))
	req3.Header.Set("Content-Type", "application/json")
	c3.Request = req3
	HandlePutBulkLimits(c3)
	if w3.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid put, got %d", w3.Code)
	}
}

func TestSetDefaultLimitsFromConfig(t *testing.T) {
	// Ensure calling SetDefaultLimitsFromConfig with nil is safe
	SetDefaultLimitsFromConfig(nil)

	// call with config values
	c := &cfg.Config{}
	c.Limits.MaxFileSizeMB = 1
	c.Limits.MaxRowsPerBatch = 2
	c.Limits.MaxConcurrentBatches = 3
	c.Limits.MaxBatchesPerHour = 4
	// preserve original global limits and restore after test to avoid flakiness
	orig := limits.Get()
	defer limits.Set(orig)
	SetDefaultLimitsFromConfig(c)
	l := limits.Get()
	require.Equal(t, 1*1024*1024, l.MaxFileSize)
	require.Equal(t, 2, l.MaxRowsPerBatch)
	require.Equal(t, 3, l.MaxConcurrentBatches)
	require.Equal(t, 4, l.MaxBatchesPerHour)
}
