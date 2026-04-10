package batch

import (
	"context"
	"errors"
	"time"

	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/history"
	"github.com/ikermy/Bulk/internal/logging"
	"github.com/ikermy/Bulk/internal/metrics"
	"github.com/ikermy/Bulk/internal/ports"
)

type BatchManager struct {
	batchRepo ports.BatchRepository
	jobRepo   ports.JobRepository
	billing   ports.BillingClient
	history   *history.Tagger
	Logger    logging.Logger
	// producer and bulkStatusTopic are used to publish batch status updates to Kafka
	// in accordance with TZ §8.1 (topic "bulk.status"). If producer==nil or
	// bulkStatusTopic=="" publishing is disabled.
	producer        ports.KafkaProducer
	bulkStatusTopic string
}

// BatchManager — управление жизненным циклом batch'ов (finalize/set processing/set cancelled).
// Принятие решения о статусе партии делегируется сюда, однако публикация задач в Kafka
// в текущей реализации выполняется на уровне Service/Handlers

// NewBatchManager constructs a BatchManager.
// Дополнительно принимает producer и bulkStatusTopic для публикации событий в Kafka
// в соответствии с ТЗ §8.1 (топик "bulk.status"). Если producer==nil или topic=="",
// публикация не выполняется.
func NewBatchManager(batchRepo ports.BatchRepository, jobRepo ports.JobRepository, billing ports.BillingClient, hist *history.Tagger, producer ports.KafkaProducer, bulkStatusTopic string) *BatchManager {
	return &BatchManager{batchRepo: batchRepo, jobRepo: jobRepo, billing: billing, history: hist, producer: producer, bulkStatusTopic: bulkStatusTopic}
}

// publishStatus publishes a bulk.status event to Kafka if producer and topic are configured.
func (m *BatchManager) publishStatus(ctx context.Context, b *domain.Batch) {
	if m == nil || m.producer == nil || m.bulkStatusTopic == "" || b == nil {
		return
	}

	total := b.TotalRows
	completed := b.CompletedCount
	failed := b.FailedCount
	pending := total - completed - failed
	if pending < 0 {
		pending = 0
	}
	var percent float64
	if total > 0 {
		percent = (float64(completed) * 100.0) / float64(total)
	}
	ev := map[string]any{
		"eventType": "bulk.status",
		"batchId":   b.ID,
		"status":    b.Status,
		"progress":  map[string]any{"total": total, "completed": completed, "failed": failed, "pending": pending, "percentComplete": percent},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	// best-effort publish; do not block on errors
	_ = m.producer.Publish(ctx, m.bulkStatusTopic, nil, ev)
}

func (m *BatchManager) FinalizeAfterUpload(ctx context.Context, batchID string, validRows, invalidRows int) error {
	if m.batchRepo == nil {
		return errors.New("batch repo not configured")
	}
	b, err := m.batchRepo.GetByID(ctx, batchID)
	if err != nil {
		return err
	}
	b.ValidRows = validRows

	if invalidRows > 0 {
		b.Status = BatchStatusValidationErrors
		if m.Logger != nil {
			m.Logger.Warn("finalize_after_upload_validation", "traceId", logging.TraceIDFromCtx(ctx), "batchId", batchID, "invalidRows", invalidRows, "validRows", validRows)
		}
		if err := m.batchRepo.Update(ctx, b); err != nil {
			return err
		}
		metrics.BatchLastUpdateTimestamp.WithLabelValues(batchID).Set(float64(time.Now().Unix()))
		m.publishStatus(ctx, b)
		return nil
	}

	if m.billing != nil {
		qr, err := m.billing.Quote(ctx, b.UserID, validRows)
		if err == nil && qr != nil {
			if qr.AllowedTotal < validRows {
				b.Status = BatchStatusPartialAvailable
				b.ApprovedCount = qr.AllowedTotal
				shortfall := validRows - qr.AllowedTotal
				if m.Logger != nil {
					m.Logger.Info("finalize_after_upload_partial", "traceId", logging.TraceIDFromCtx(ctx), "batchId", batchID, "approvedCount", qr.AllowedTotal, "requested", validRows)
					m.Logger.Warn("billing_check_failed", "traceId", logging.TraceIDFromCtx(ctx), "batchId", batchID, "shortfall", shortfall)
				}
				if m.history != nil {
					evt := map[string]any{"batchId": b.ID, "approvedCount": qr.AllowedTotal, "requested": validRows, "message": "partial available: insufficient billing allowance"}
					_ = m.history.TagEvent(ctx, "batch.partial_available", evt)
				}
				if err := m.batchRepo.Update(ctx, b); err != nil {
					return err
				}
				m.publishStatus(ctx, b)
				return nil
			}
		}
	}
	b.Status = BatchStatusReady
	if m.Logger != nil {
		m.Logger.Info("finalize_after_upload_ready", "traceId", logging.TraceIDFromCtx(ctx), "batchId", batchID, "validRows", validRows)
	}
	if err := m.batchRepo.Update(ctx, b); err != nil {
		return err
	}
	metrics.BatchLastUpdateTimestamp.WithLabelValues(batchID).Set(float64(time.Now().Unix()))
	m.publishStatus(ctx, b)
	return nil
}

func (m *BatchManager) SetProcessing(ctx context.Context, batchID string) error {
	if m.batchRepo == nil {
		return errors.New("batch repo not configured")
	}
	b, err := m.batchRepo.GetByID(ctx, batchID)
	if err != nil {
		return err
	}
	b.Status = BatchStatusProcessing
	if err := m.batchRepo.Update(ctx, b); err != nil {
		return err
	}
	metrics.BatchLastUpdateTimestamp.WithLabelValues(batchID).Set(float64(time.Now().Unix()))
	m.publishStatus(ctx, b)
	return nil
}

func (m *BatchManager) SetCancelled(ctx context.Context, batchID string) (int, error) {
	if m.batchRepo == nil {
		return 0, errors.New("batch repo not configured")
	}
	b, err := m.batchRepo.GetByID(ctx, batchID)
	if err != nil {
		return 0, err
	}
	// Согласно ТЗ §3.5, при отмене партии должен выполняться возврат для ожидающих элементов и возвращаться объект refund:
	// { pending: N, refunded: M, transactionIds: [...] }
	// Для полной реализации: получить сохранённые идентификаторы транзакций (из записи batch), вызвать API возврата в биллинге
	// и обновить статусы job'ов (например, пометить как refunded). Текущая реализация только обновляет статус.
	b.Status = BatchStatusCancelled
	if err := m.batchRepo.Update(ctx, b); err != nil {
		return 0, err
	}
	metrics.BatchLastUpdateTimestamp.WithLabelValues(batchID).Set(float64(time.Now().Unix()))
	m.publishStatus(ctx, b)

	// If billing transactions were previously reserved for this batch, attempt to refund/unblock them.
	// This implements TZ §3.5: Bulk must call Billing.RefundTransactions with saved transaction IDs
	// and mark pending jobs as refunded. Refund is best-effort: on error we log and continue.
	if m.billing != nil && len(b.BillingTransactionIDs) > 0 {
		if err := m.billing.RefundTransactions(ctx, b.UserID, b.BillingTransactionIDs, batchID); err != nil {
			if m.Logger != nil {
				m.Logger.Error("billing_refund_failed", "traceId", logging.TraceIDFromCtx(ctx), "batchId", batchID, "error", err)
			}
		} else {
			if m.jobRepo != nil {
				if jobs, jerr := m.jobRepo.GetByBatch(ctx, batchID); jerr == nil {
					for _, j := range jobs {
						switch j.Status {
						case JobStatusPending, JobStatusQueued, JobStatusProcessing:
							_ = m.jobRepo.UpdateStatus(ctx, j.ID, string(domain.JobStatusRefunded))
						}
					}
				} else {
					if m.Logger != nil {
						m.Logger.Warn("failed_to_list_jobs_for_refund", "traceId", logging.TraceIDFromCtx(ctx), "batchId", batchID, "error", jerr)
					}
				}
			}
			// clear stored tx ids
			txs := b.BillingTransactionIDs
			b.BillingTransactionIDs = nil
			if uerr := m.batchRepo.Update(ctx, b); uerr != nil {
				if m.Logger != nil {
					m.Logger.Warn("failed_to_clear_billing_txids", "traceId", logging.TraceIDFromCtx(ctx), "batchId", batchID, "error", uerr)
				}
			}
			if m.history != nil {
				_ = m.history.TagEvent(ctx, "batch.refunded", map[string]any{"batchId": batchID, "transactionIds": txs})
			}
		}
	}

	refundedCount := 0
	if m.jobRepo != nil {
		if jobs, err := m.jobRepo.GetByBatch(ctx, batchID); err == nil {
			for _, j := range jobs {
				if j.Status == domain.JobStatusRefunded {
					refundedCount++
				}
			}
		}
	}
	if m.Logger != nil {
		m.Logger.Info("batch_cancelled", "traceId", logging.TraceIDFromCtx(ctx), "batchId", batchID, "refundedCount", refundedCount)
	}
	return refundedCount, nil
}

// OnJobStatusChange recalculates batch progress and updates status accordingly.
// If all jobs are completed (or no pending/queued/processing) -> completed.
// Otherwise if there are still pending jobs -> processing.
func (m *BatchManager) OnJobStatusChange(ctx context.Context, batchID string) error {
	if m.batchRepo == nil || m.jobRepo == nil {
		return errors.New("repos not configured")
	}
	jobs, err := m.jobRepo.GetByBatch(ctx, batchID)
	if err != nil {
		return err
	}
	total := len(jobs)
	if total == 0 {
		return nil
	}
	pending := 0
	completed := 0
	failed := 0
	for _, j := range jobs {
		switch j.Status {
		case JobStatusPending, JobStatusQueued, JobStatusProcessing:
			pending++
		case JobStatusCompleted, JobStatusRefunded:
			completed++
		case JobStatusFailed:
			failed++
			completed++
		default:
			pending++
		}
	}
	b, err := m.batchRepo.GetByID(ctx, batchID)
	if err != nil {
		return err
	}
	b.CompletedCount = completed
	b.FailedCount = failed
	if pending == 0 {
		b.Status = BatchStatusCompleted
		now := time.Now().UTC()
		b.CompletedAt = &now
		// логируем ключевое событие завершения партии (TZ §13.2)
		if m.Logger != nil {
			m.Logger.Info("batch_completed", "traceId", logging.TraceIDFromCtx(ctx), "batchId", batchID, "completed", completed, "failed", failed)
		}
	} else {
		b.Status = BatchStatusProcessing
	}
	if err := m.batchRepo.Update(ctx, b); err != nil {
		return err
	}
	metrics.BatchLastUpdateTimestamp.WithLabelValues(batchID).Set(float64(time.Now().Unix()))
	m.publishStatus(ctx, b)
	return nil
}
