package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	KafkaPublishTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "bulk_service", Name: "kafka_publish_total", Help: "Total kafka publish attempts"},
		[]string{"topic", "result"},
	)
	KafkaPublishDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Namespace: "bulk_service", Name: "kafka_publish_duration_seconds", Help: "Kafka publish duration seconds", Buckets: prometheus.DefBuckets},
		[]string{"topic"},
	)
	// HTTP metrics
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "bulk_service", Name: "http_requests_total", Help: "Total HTTP requests"},
		[]string{"method", "route", "status"},
	)
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Namespace: "bulk_service", Name: "http_request_duration_seconds", Help: "HTTP request duration seconds", Buckets: prometheus.ExponentialBuckets(0.01, 2, 15)},
		[]string{"route"},
	)
	// Batch / Job metrics
	BatchJobsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "bulk_service", Name: "batch_jobs_total", Help: "Total jobs created"},
		[]string{"batch_id", "status"},
	)
	// Upload metrics (see UploadsRejectedTotal below)
	BatchJobsPending = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: "bulk_service", Name: "batch_jobs_pending", Help: "Number of pending jobs per batch"},
		[]string{"batch_id"},
	)
	BatchProcessingDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Namespace: "bulk_service", Name: "batch_processing_duration_seconds", Help: "Time to process a batch", Buckets: prometheus.ExponentialBuckets(0.1, 2, 12)},
		[]string{"batch_id"},
	)

	// Additional metrics requested in TZ §15.1
	BatchesCreatedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "bulk_service", Name: "batches_created_total", Help: "Created batches total"},
		[]string{"status"},
	)
	BatchesCompletedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "bulk_service", Name: "batches_completed_total", Help: "Completed batches total"},
		[]string{"status"},
	)
	JobsQueuedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{Namespace: "bulk_service", Name: "jobs_queued_total", Help: "Jobs queued total"},
	)
	JobsCompletedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "bulk_service", Name: "jobs_completed_total", Help: "Jobs completed total"},
		[]string{"status"},
	)
	JobsFailedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "bulk_service", Name: "jobs_failed_total", Help: "Jobs failed total"},
		[]string{"error_code"},
	)
	JobProcessingDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{Namespace: "bulk_service", Name: "job_processing_duration_seconds", Help: "Time to process a job", Buckets: prometheus.ExponentialBuckets(0.01, 2, 12)},
	)
	ValidationErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "bulk_service", Name: "validation_errors_total", Help: "Validation errors total"},
		[]string{"error_code", "field"},
	)
	FileSizeBytes = prometheus.NewHistogram(
		prometheus.HistogramOpts{Namespace: "bulk_service", Name: "file_size_bytes", Help: "Uploaded file sizes in bytes", Buckets: prometheus.ExponentialBuckets(512, 2, 16)},
	)
	RowsPerBatch = prometheus.NewHistogram(
		prometheus.HistogramOpts{Namespace: "bulk_service", Name: "rows_per_batch", Help: "Number of rows per batch", Buckets: prometheus.ExponentialBuckets(10, 2, 12)},
	)
	// Per-batch row counts for SLO filtering and inspections
	BatchRowsTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: "bulk_service", Name: "batch_rows_total", Help: "Total rows per batch"},
		[]string{"batch_id"},
	)

	// Timestamp of last batch update (epoch seconds), per batch_id
	BatchLastUpdateTimestamp = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Namespace: "bulk_service", Name: "batch_last_update_timestamp", Help: "Last update timestamp for batch (epoch seconds)"},
		[]string{"batch_id"},
	)

	// BFF request errors (per method)
	BFFRequestErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "bulk_service", Name: "bff_request_errors_total", Help: "BFF request errors total"},
		[]string{"method"},
	)

	// Kafka queue depth (if available from exporter)
	KafkaQueueDepth = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "bulk_service", Name: "kafka_queue_depth", Help: "Approximate Kafka queue depth"})
	BFFRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Namespace: "bulk_service", Name: "bff_request_duration_seconds", Help: "BFF request duration seconds", Buckets: prometheus.DefBuckets},
		[]string{"endpoint"},
	)
	// Upload metrics
	UploadsRejectedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "bulk_service", Name: "uploads_rejected_total", Help: "Rejected upload attempts"},
		[]string{"reason"},
	)
	// Storage and billing errors
	StorageErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "bulk_service", Name: "storage_errors_total", Help: "Storage related errors"},
		[]string{"operation", "result"},
	)
	// Storage operations total (for computing error rate)
	StorageOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "bulk_service", Name: "storage_operations_total", Help: "Storage operations total"},
		[]string{"operation", "result"},
	)
	BillingErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "bulk_service", Name: "billing_errors_total", Help: "Billing errors total"},
		[]string{"method", "result"},
	)
	DBOpenConnections   = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "bulk_service", Name: "db_open_connections", Help: "DB open connections"})
	DBIdleConnections   = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "bulk_service", Name: "db_idle_connections", Help: "DB idle connections"})
	BillingCallsTotal   = prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: "bulk_service", Name: "billing_calls_total", Help: "Billing calls total"}, []string{"method", "result"})
	BillingCallDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Namespace: "bulk_service", Name: "billing_call_duration_seconds", Help: "Billing call duration seconds", Buckets: prometheus.DefBuckets},
		[]string{"method"},
	)
	KafkaConsumeTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{Namespace: "bulk_service", Name: "kafka_consume_total", Help: "Total kafka consume attempts"},
		[]string{"topic", "result"},
	)
	KafkaConsumeDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{Namespace: "bulk_service", Name: "kafka_consume_duration_seconds", Help: "Kafka consume duration seconds", Buckets: prometheus.DefBuckets},
		[]string{"topic"},
	)
)

func init() {
	prometheus.MustRegister(KafkaPublishTotal)
	prometheus.MustRegister(KafkaPublishDuration)
	prometheus.MustRegister(HTTPRequestsTotal)
	prometheus.MustRegister(HTTPRequestDuration)
	prometheus.MustRegister(BatchJobsTotal)
	prometheus.MustRegister(BatchJobsPending)
	prometheus.MustRegister(BatchProcessingDuration)
	prometheus.MustRegister(StorageErrorsTotal)
	prometheus.MustRegister(StorageOperationsTotal)
	prometheus.MustRegister(BillingErrorsTotal)
	prometheus.MustRegister(DBOpenConnections)
	prometheus.MustRegister(DBIdleConnections)
	prometheus.MustRegister(BillingCallsTotal)
	prometheus.MustRegister(BillingCallDuration)
	prometheus.MustRegister(KafkaConsumeTotal)
	prometheus.MustRegister(KafkaConsumeDuration)
	prometheus.MustRegister(UploadsRejectedTotal)
	prometheus.MustRegister(BatchesCreatedTotal)
	prometheus.MustRegister(BatchesCompletedTotal)
	prometheus.MustRegister(JobsQueuedTotal)
	prometheus.MustRegister(JobsCompletedTotal)
	prometheus.MustRegister(JobsFailedTotal)
	prometheus.MustRegister(JobProcessingDuration)
	prometheus.MustRegister(ValidationErrorsTotal)
	prometheus.MustRegister(FileSizeBytes)
	prometheus.MustRegister(RowsPerBatch)
	prometheus.MustRegister(BatchRowsTotal)
	prometheus.MustRegister(BatchLastUpdateTimestamp)
	prometheus.MustRegister(BFFRequestErrorsTotal)
	prometheus.MustRegister(KafkaQueueDepth)
	prometheus.MustRegister(BFFRequestDuration)
}
