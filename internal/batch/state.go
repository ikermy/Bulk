package batch

import "github.com/ikermy/Bulk/internal/domain"

// BatchStatus Переопределяем алиасы типов и констант из пакета domain, чтобы избежать
// дублирования определений статусов в разных пакетах.
type BatchStatus = domain.BatchStatus

var (
	BatchStatusPending          = domain.BatchStatusPending
	BatchStatusValidationErrors = domain.BatchStatusValidationErrors
	BatchStatusPartialAvailable = domain.BatchStatusPartialAvailable
	BatchStatusReady            = domain.BatchStatusReady
	BatchStatusProcessing       = domain.BatchStatusProcessing
	BatchStatusCompleted        = domain.BatchStatusCompleted
	BatchStatusCancelled        = domain.BatchStatusCancelled
)

type JobStatus = domain.JobStatus

var (
	JobStatusPending    = domain.JobStatusPending
	JobStatusQueued     = domain.JobStatusQueued
	JobStatusProcessing = domain.JobStatusProcessing
	JobStatusCompleted  = domain.JobStatusCompleted
	JobStatusFailed     = domain.JobStatusFailed
	JobStatusRefunded   = domain.JobStatusRefunded
)
