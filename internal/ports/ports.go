package ports

import (
	"context"
	"time"

	"github.com/ikermy/Bulk/internal/billing"
	"github.com/ikermy/Bulk/internal/domain"
)

// BatchRepository defines methods to work with batches
type BatchRepository interface {
	Create(ctx context.Context, b *domain.Batch) error
	GetByID(ctx context.Context, id string) (*domain.Batch, error)
	// Update persists fields of batch (FileStorageID, ValidRows, Status, etc.)
	Update(ctx context.Context, b *domain.Batch) error
	// List returns paged batches matching filter and total count
	List(ctx context.Context, filter BatchFilter) ([]*domain.Batch, int, error)
	// AdminStats returns aggregated metrics for admin dashboard between from..to (inclusive)
	AdminStats(ctx context.Context, from *time.Time, to *time.Time) (*AdminStatsWithQueues, error)
}

// BatchFilter holds parameters for listing batches
type BatchFilter struct {
	// Параметры фильтра для запроса GET /api/v1/batches (ТЗ §3.6):
	//  - Status: фильтр по статусу (например: pending, ready, processing, completed, cancelled)
	//  - From, To: границы по дате (RFC3339)
	//  - Page: номер страницы (начинается с 1)
	//  - Limit: количество записей на страницу
	Status string
	From   *time.Time
	To     *time.Time
	Page   int
	Limit  int
	// optional filters
	UserID   string
	Revision string
	// sorting: accepted values: "createdAt", "completedAt"
	SortBy string
	// descending if true
	SortDesc bool
	// cursor for cursor-based pagination: RFC3339 timestamp string
	Cursor string
}

// JobRepository defines methods to work with jobs
type JobRepository interface {
	Create(ctx context.Context, j *domain.Job) error
	GetByBatch(ctx context.Context, batchID string) ([]*domain.Job, error)
	UpdateStatus(ctx context.Context, jobID string, status string) error
	GetResultsByBatch(ctx context.Context, batchID string) ([]*JobResult, error)
	// UpdateBillingTransactionID sets billing_transaction_id for a job (nullable)
	UpdateBillingTransactionID(ctx context.Context, jobID string, txID *string) error
	// UpdateStatusWithResult atomically updates job status and result fields
	// (build_id, barcode_urls, error_code, error_message, billing_transaction_id).
	// This ensures no inconsistency between status and result when processing bulk.result
	// messages (see TZ §8.4 Consumer Implementation).
	UpdateStatusWithResult(ctx context.Context, jobID string, status string, result JobResult) error
}

// AdminStats represents aggregated statistics for admin dashboard
type AdminStats struct {
	BatchesCreated          int     `json:"batchesCreated"`
	JobsProcessed           int     `json:"jobsProcessed"`
	JobsFailed              int     `json:"jobsFailed"`
	AverageProcessingTimeMs float64 `json:"averageProcessingTimeMs"`
}

type QueuesStats struct {
	BulkJobPending    int `json:"bulkJobPending"`
	BulkResultPending int `json:"bulkResultPending"`
}

// AdminStatsWithQueues wraps AdminStats and queue sizes
type AdminStatsWithQueues struct {
	AdminStats
	Queues QueuesStats `json:"queues"`
}

// JobResult represents result/failure info for a processed job
type JobResult struct {
	JobID        string
	RowNumber    int
	BuildID      string
	BarcodeURLs  string // JSON string with URLs
	ErrorCode    string
	ErrorMessage string
	// BillingTransactionID holds optional transaction id associated with this job result
	BillingTransactionID string `json:"billingTransactionId,omitempty"`
}

// ValidationError represents a row-level validation error for a batch
type ValidationError struct {
	RowNumber     int
	Field         string
	ErrorCode     string
	ErrorMessage  string
	OriginalValue string
}

// BillingClient defines methods used to interact with billing via BFF
type BillingClient interface {
	Quote(ctx context.Context, user string, count int) (*billing.QuoteResponse, error)
	// BlockBatch резервирует единицы биллинга и возвращает идентификаторы транзакций.
	// Реализация должна возвращать transaction IDs, которые сервис обязан сохранить
	// вместе с записью batch, чтобы при отмене выполнить возврат/разблокировку по этим ID (см. ТЗ §3.5).
	BlockBatch(ctx context.Context, user string, count int, batchID string) (*billing.BlockBatchResponse, error)
	// RefundTransactions выполняет откат/возврат ранее заблокированных транзакций
	RefundTransactions(ctx context.Context, user string, transactionIDs []string, batchID string) error
}

// BillingClient — порт для взаимодействия с Billing через BFF

// ValidationRepository provides access to validation errors
type ValidationRepository interface {
	GetValidationErrors(ctx context.Context, batchID string) ([]*ValidationError, error)
}

// ExtendedJobRepository provides access to job results
type ExtendedJobRepository interface {
	GetResultsByBatch(ctx context.Context, batchID string) ([]*JobResult, error)
}

// KafkaProducer minimal interface used in the project
type KafkaProducer interface {
	Publish(ctx context.Context, topic string, key []byte, msg any) error
	Close() error
}
