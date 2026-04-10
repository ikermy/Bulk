package di

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ikermy/Bulk/internal/kafka"
	"github.com/ikermy/Bulk/internal/ports"
	"github.com/stretchr/testify/require"
)

// ------- локальные моки для handler-тестов -------

type spyJobRepo struct {
	lastJobID  string
	lastStatus string
	lastResult ports.JobResult
}

func (m *spyJobRepo) UpdateStatusWithResult(ctx context.Context, jobID string, status string, result ports.JobResult) error {
	m.lastJobID = jobID
	m.lastStatus = status
	m.lastResult = result
	return nil
}

type spyTagger struct {
	tagEventCalled  bool
	tagBulkIDCalled bool
	lastEventType   string
	lastBulkID      string
}

func (m *spyTagger) TagEvent(ctx context.Context, eventType string, payload any) error {
	m.tagEventCalled = true
	m.lastEventType = eventType
	return nil
}
func (m *spyTagger) TagBulkID(ctx context.Context, bulkID string, jobID string, barcodeURLs map[string]string) error {
	m.tagBulkIDCalled = true
	m.lastBulkID = bulkID
	return nil
}

type spyBatchManager struct {
	lastBatchID string
}

func (m *spyBatchManager) OnJobStatusChange(ctx context.Context, batchID string) error {
	m.lastBatchID = batchID
	return nil
}

type spyLogger struct {
	infoCalls  []string
	errorCalls []string
	warnCalls  []string
}

func (l *spyLogger) Info(args ...interface{}) {
	if len(args) > 0 {
		l.infoCalls = append(l.infoCalls, fmt.Sprint(args[0]))
	}
}
func (l *spyLogger) Error(args ...interface{}) {
	if len(args) > 0 {
		l.errorCalls = append(l.errorCalls, fmt.Sprint(args[0]))
	}
}
func (l *spyLogger) Warn(args ...interface{}) {
	if len(args) > 0 {
		l.warnCalls = append(l.warnCalls, fmt.Sprint(args[0]))
	}
}
func (l *spyLogger) Debug(args ...interface{}) {}

// TestDIHandler_StatusCompleted проверяет ветку "completed".
func TestDIHandler_StatusCompleted(t *testing.T) {
	jobRepo := &spyJobRepo{}
	hist := &spyTagger{}
	bm := &spyBatchManager{}
	log := &spyLogger{}
	handler := buildResultHandler(jobRepo, hist, bm, log)

	ev := kafka.BulkResultEvent{
		JobID:   "job-1",
		BatchID: "batch-1",
		Status:  "completed",
		BuildID: "build-1",
	}
	require.NoError(t, handler(context.Background(), ev))

	require.Equal(t, "job-1", jobRepo.lastJobID)
	require.Equal(t, "completed", jobRepo.lastStatus)
	require.Contains(t, log.infoCalls, "job_completed")
	require.Empty(t, log.errorCalls)
	require.True(t, hist.tagEventCalled)
	require.True(t, hist.tagBulkIDCalled)
	require.Equal(t, "batch-1", bm.lastBatchID)
}

// TestDIHandler_StatusFailed проверяет ветку "failed".
func TestDIHandler_StatusFailed(t *testing.T) {
	jobRepo := &spyJobRepo{}
	hist := &spyTagger{}
	bm := &spyBatchManager{}
	log := &spyLogger{}
	handler := buildResultHandler(jobRepo, hist, bm, log)

	ev := kafka.BulkResultEvent{
		JobID:   "job-2",
		BatchID: "batch-2",
		Status:  "failed",
	}
	require.NoError(t, handler(context.Background(), ev))

	require.Equal(t, "job-2", jobRepo.lastJobID)
	require.Equal(t, "failed", jobRepo.lastStatus)
	require.Contains(t, log.errorCalls, "job_failed")
	require.Empty(t, log.infoCalls)
}

// TestDIHandler_StatusOther проверяет default-ветку switch.
func TestDIHandler_StatusOther(t *testing.T) {
	jobRepo := &spyJobRepo{}
	log := &spyLogger{}
	handler := buildResultHandler(jobRepo, nil, nil, log)

	ev := kafka.BulkResultEvent{
		JobID:  "job-3",
		Status: "processing",
	}
	require.NoError(t, handler(context.Background(), ev))
	require.Empty(t, log.infoCalls)
	require.Empty(t, log.errorCalls)
}

// TestDIHandler_InvalidBillingUUID проверяет ветку с невалидным UUID биллинга.
func TestDIHandler_InvalidBillingUUID(t *testing.T) {
	jobRepo := &spyJobRepo{}
	log := &spyLogger{}
	handler := buildResultHandler(jobRepo, nil, nil, log)

	ev := kafka.BulkResultEvent{
		JobID:   "job-4",
		BatchID: "batch-4",
		Status:  "completed",
		Billing: &struct {
			Status        string `json:"status,omitempty"`
			TransactionId string `json:"transactionId,omitempty"`
		}{
			TransactionId: "not-a-uuid",
		},
	}
	require.NoError(t, handler(context.Background(), ev))
	require.Empty(t, jobRepo.lastResult.BillingTransactionID)
	require.Contains(t, log.warnCalls, "invalid_billing_txid_received")
}

// TestDIHandler_ValidBillingUUID проверяет ветку с валидным UUID биллинга.
func TestDIHandler_ValidBillingUUID(t *testing.T) {
	jobRepo := &spyJobRepo{}
	log := &spyLogger{}
	handler := buildResultHandler(jobRepo, nil, nil, log)

	validUUID := "550e8400-e29b-41d4-a716-446655440000"
	ev := kafka.BulkResultEvent{
		JobID:   "job-5",
		BatchID: "batch-5",
		Status:  "completed",
		Billing: &struct {
			Status        string `json:"status,omitempty"`
			TransactionId string `json:"transactionId,omitempty"`
		}{
			TransactionId: validUUID,
		},
	}
	require.NoError(t, handler(context.Background(), ev))
	require.Equal(t, validUUID, jobRepo.lastResult.BillingTransactionID)
	require.Empty(t, log.warnCalls)
}

// TestDIHandler_NoBatchID проверяет что TagBulkID НЕ вызывается при пустом BatchID.
func TestDIHandler_NoBatchID(t *testing.T) {
	jobRepo := &spyJobRepo{}
	hist := &spyTagger{}
	log := &spyLogger{}
	handler := buildResultHandler(jobRepo, hist, nil, log)

	ev := kafka.BulkResultEvent{
		JobID:   "job-6",
		BatchID: "", // empty
		Status:  "completed",
	}
	require.NoError(t, handler(context.Background(), ev))
	require.True(t, hist.tagEventCalled)
	require.False(t, hist.tagBulkIDCalled, "TagBulkID should not be called when BatchID is empty")
}

// TestDIHandler_WithTimestamp проверяет что обработчик корректно работает с полным событием.
func TestDIHandler_WithTimestamp(t *testing.T) {
	jobRepo := &spyJobRepo{}
	hist := &spyTagger{}
	bm := &spyBatchManager{}
	log := &spyLogger{}
	handler := buildResultHandler(jobRepo, hist, bm, log)

	ev := kafka.BulkResultEvent{
		EventType:   "bulk.result",
		JobID:       "job-ts",
		BatchID:     "batch-ts",
		Status:      "completed",
		BuildID:     "build-ts",
		BarcodeURLs: map[string]string{"row1": "https://cdn/1.png"},
		Timestamp:   time.Now(),
	}
	require.NoError(t, handler(context.Background(), ev))
	require.Equal(t, "batch-ts", hist.lastBulkID)
}

// TestDIHandler_BarcodeURLsMarshal проверяет маршалинг barcodeURLs в JSON.
func TestDIHandler_BarcodeURLsMarshal(t *testing.T) {
	jobRepo := &spyJobRepo{}
	handler := buildResultHandler(jobRepo, nil, nil, nil)

	ev := kafka.BulkResultEvent{
		JobID:       "job-m",
		Status:      "completed",
		BarcodeURLs: map[string]string{"row1": "https://cdn/bar.png"},
	}
	require.NoError(t, handler(context.Background(), ev))
	require.NotEmpty(t, jobRepo.lastResult.BarcodeURLs, "barcodeURLs should be marshaled to JSON")
}
