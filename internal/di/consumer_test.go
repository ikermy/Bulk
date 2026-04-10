package di

import (
    "context"
    "testing"
    "time"

    "github.com/ikermy/Bulk/internal/kafka"
)

type mockJobRepoForConsumer struct{
    lastJobID string
    lastStatus string
    lastTxID *string
}
func (m *mockJobRepoForConsumer) Create(ctx context.Context, j *kafka.BulkJobEvent) error { return nil }
func (m *mockJobRepoForConsumer) GetByBatch(ctx context.Context, batchID string) ([]*kafka.BulkJobEvent, error) { return nil, nil }
func (m *mockJobRepoForConsumer) UpdateStatus(ctx context.Context, jobID string, status string) error { m.lastJobID = jobID; m.lastStatus = status; return nil }
func (m *mockJobRepoForConsumer) UpdateBillingTransactionID(ctx context.Context, jobID string, txID *string) error { m.lastTxID = txID; return nil }

type mockTaggerForConsumer struct{
    lastEventType string
}
func (m *mockTaggerForConsumer) TagEvent(ctx context.Context, eventType string, payload any) error { m.lastEventType = eventType; return nil }

type mockBatchManagerForConsumer struct{
    lastBatchID string
}
func (m *mockBatchManagerForConsumer) OnJobStatusChange(ctx context.Context, batchID string) error { m.lastBatchID = batchID; return nil }

func TestConsumerHandlerCallsReposAndManager(t *testing.T) {
    // Create mocks
    jobRepo := &mockJobRepoForConsumer{}
    tagger := &mockTaggerForConsumer{}
    bm := &mockBatchManagerForConsumer{}

    // Create handler similar to DI
    handler := func(ctx context.Context, ev kafka.BulkResultEvent) error {
        if jobRepo != nil {
            _ = jobRepo.UpdateStatus(ctx, ev.JobID, ev.Status)
            if ev.Billing != nil && ev.Billing.TransactionId != "" {
                _ = jobRepo.UpdateBillingTransactionID(ctx, ev.JobID, &ev.Billing.TransactionId)
            }
        }
        if tagger != nil {
            _ = tagger.TagEvent(ctx, "bulk.result", ev)
        }
        if bm != nil {
            _ = bm.OnJobStatusChange(ctx, ev.BatchID)
        }
        return nil
    }

    ev := kafka.BulkResultEvent{EventType: "bulk.result", JobID: "jid1", BatchID: "bid1", Status: "completed", Timestamp: time.Now(), Billing: &struct{Status string `json:"status,omitempty"`; TransactionId string `json:"transactionId,omitempty"`}{Status: "charged", TransactionId: "tx-1"}}
    if err := handler(context.Background(), ev); err != nil { t.Fatalf("handler err: %v", err) }

    if jobRepo.lastJobID != "jid1" || jobRepo.lastStatus != "completed" {
        t.Fatalf("jobRepo not updated: got %s/%s", jobRepo.lastJobID, jobRepo.lastStatus)
    }
    if tagger.lastEventType != "bulk.result" {
        t.Fatalf("tagger not called: %v", tagger.lastEventType)
    }
    if bm.lastBatchID != "bid1" {
        t.Fatalf("batch manager not notified: %v", bm.lastBatchID)
    }
    if jobRepo.lastTxID == nil || *jobRepo.lastTxID != "tx-1" {
        t.Fatalf("billing tx id not persisted: %v", jobRepo.lastTxID)
    }
}

