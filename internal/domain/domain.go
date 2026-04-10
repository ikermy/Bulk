package domain

import "time"

// BatchStatus представляет возможные статусы партии (см. ТЗ §3.4)
type BatchStatus string

const (
	BatchStatusPending          BatchStatus = "pending"
	BatchStatusValidationErrors BatchStatus = "validation_errors"
	BatchStatusPartialAvailable BatchStatus = "partial_available"
	BatchStatusReady            BatchStatus = "ready"
	BatchStatusProcessing       BatchStatus = "processing"
	BatchStatusCompleted        BatchStatus = "completed"
	BatchStatusCancelled        BatchStatus = "cancelled"
)

// JobStatus представляет возможные статусы отдельной задачи
type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusQueued     JobStatus = "queued"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
	JobStatusRefunded   JobStatus = "refunded"
)

type Batch struct {
	ID            string
	UserID        string
	Status        BatchStatus
	Revision      string
	FileStorageID string
	TotalRows     int
	ValidRows     int
	CreatedAt     time.Time
	ApprovedCount int
	CompletedCount int
	FailedCount int
	CompletedAt  *time.Time
	// BillingTransactionIDs хранит идентификаторы транзакций, зарезервированных в Billing
	// Может содержать несколько ID (например, по одному на партию/пакет/группу строк)
	BillingTransactionIDs []string
	// Priority and TimeoutMs are optional admin-set configuration for batch processing
	Priority  string
	TimeoutMs int
}

// Примечание: поля CreatedAt, CompletedAt, TotalRows, CompletedCount и FailedCount
// используются функцией `HandleBatchStatus` для формирования ответа, описанного в ТЗ §3.4.

type Job struct {
	ID        string
	BatchID   string
	RowNumber int
	Status    JobStatus
	InputData string
	// BillingTransactionID — опциональный идентификатор транзакции биллинга, связанный с этой задачей
	BillingTransactionID *string
}
