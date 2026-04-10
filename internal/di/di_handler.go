package di

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/ikermy/Bulk/internal/kafka"
	"github.com/ikermy/Bulk/internal/logging"
	"github.com/ikermy/Bulk/internal/ports"
)

// historyTagger is the subset of history.Tagger used by the result handler.
type historyTagger interface {
	TagEvent(ctx context.Context, eventType string, payload any) error
	TagBulkID(ctx context.Context, bulkID string, jobID string, barcodeURLs map[string]string) error
}

// batchNotifier is called after a job status changes to recalculate batch progress.
type batchNotifier interface {
	OnJobStatusChange(ctx context.Context, batchID string) error
}

// jobUpdater is the minimal interface for updating job results atomically.
type jobUpdater interface {
	UpdateStatusWithResult(ctx context.Context, jobID string, status string, result ports.JobResult) error
}

// diLogger is a minimal logging interface that *zap.SugaredLogger and test spies satisfy.
type diLogger interface {
	Info(args ...interface{})
	Error(args ...interface{})
	Warn(args ...interface{})
}

// buildResultHandler constructs the handler func for bulk.result Kafka events.
// Extracted to enable unit testing without a real Kafka cluster.
func buildResultHandler(
	jobRepo jobUpdater,
	hist historyTagger,
	bm batchNotifier,
	logger diLogger,
) func(ctx context.Context, ev kafka.BulkResultEvent) error {
	return func(ctx context.Context, ev kafka.BulkResultEvent) error {
		// Build JobResult payload and perform an atomic update of status + result
		if jobRepo != nil {
			// marshal barcodeUrls map to JSON string if present
			var burls string
			if ev.BarcodeURLs != nil {
				if bb, jerr := json.Marshal(ev.BarcodeURLs); jerr == nil {
					burls = string(bb)
				}
			}
			res := ports.JobResult{JobID: ev.JobID, BuildID: ev.BuildID, BarcodeURLs: burls}
			// include billing transaction id in atomic update if present and valid
			if ev.Billing != nil && ev.Billing.TransactionId != "" {
				if _, perr := uuid.Parse(ev.Billing.TransactionId); perr == nil {
					res.BillingTransactionID = ev.Billing.TransactionId
				} else if logger != nil {
					logger.Warn("invalid_billing_txid_received", "jobId", ev.JobID, "txId", ev.Billing.TransactionId)
				}
			}
			// perform atomic update: status + result fields (TZ §8.4)
			_ = jobRepo.UpdateStatusWithResult(ctx, ev.JobID, ev.Status, res)
		}
		// tag event in history
		if hist != nil {
			_ = hist.TagEvent(ctx, "bulk.result", ev)
			// Tag barcodes with bulk_id (batchId) per TZ §1.2 and §2.2
			if ev.BatchID != "" && ev.JobID != "" {
				_ = hist.TagBulkID(ctx, ev.BatchID, ev.JobID, ev.BarcodeURLs)
			}
		}
		// log job completion/failure events according to TZ §13.2
		if logger != nil {
			traceID := logging.TraceIDFromCtx(ctx)
			switch ev.Status {
			case "completed":
				logger.Info("job_completed", "traceId", traceID, "jobId", ev.JobID, "buildId", ev.BuildID)
			case "failed":
				logger.Error("job_failed", "traceId", traceID, "jobId", ev.JobID, "errorCode", ev.Status)
			default:
				// generic processed event already logged by consumer
			}
		}
		// notify batch manager to recalculate batch status/progress
		if bm != nil {
			_ = bm.OnJobStatusChange(ctx, ev.BatchID)
		}
		return nil
	}
}

