package handlers

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ikermy/Bulk/internal/batch"
	"github.com/ikermy/Bulk/internal/di"
	"github.com/ikermy/Bulk/internal/domain"
	"github.com/ikermy/Bulk/internal/logging"
	"github.com/ikermy/Bulk/internal/ports"
	apperr "github.com/ikermy/Bulk/internal/transport/http/apperror"
	pkgxls "github.com/ikermy/Bulk/pkg/xls"
)

// helper: extract a logger from deps with fallback to service logger
func getLoggerFromDeps(deps *di.Deps) logging.Logger {
	if deps == nil {
		return nil
	}
	if deps.Logger != nil {
		return deps.Logger
	}
	if deps.Service != nil && deps.Service.Logger != nil {
		return deps.Service.Logger
	}
	return nil
}

// helper: safe request context extraction
func requestCtx(c *gin.Context) context.Context {
	if c == nil || c.Request == nil {
		return context.Background()
	}
	return c.Request.Context()
}

// helper: default topic name for bulk.job
func getBulkJobTopic() string {
	t := os.Getenv("KAFKA_TOPIC_BULK_JOB")
	if t == "" {
		return "bulk.job"
	}
	return t
}

// helper: respond with accepted processing payload
func respondAcceptedProcessing(c *gin.Context, id string, queued int, estimated int) {
	c.JSON(http.StatusAccepted, gin.H{"success": true, "batchId": id, "status": string(batch.BatchStatusProcessing), "jobsQueued": queued, "estimatedTimeSeconds": estimated})
}

// parseOptionalTime parses RFC3339 time string and returns pointer to time.Time or nil
func parseOptionalTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	return nil
}

// requireBatch loads batch by id and writes appropriate HTTP error if missing.
// Returns (batch, true) on success, (nil, false) if handler already responded with error.
func requireBatch(c *gin.Context, deps *di.Deps, id string) (*domain.Batch, bool) {
	if deps == nil || deps.BatchRepo == nil {
		apperr.WriteError(c, http.StatusNotFound, "MISSING_DEPENDENCY", "batch repo not configured", nil)
		return nil, false
	}
	b, err := deps.BatchRepo.GetByID(requestCtx(c), id)
	if err != nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to fetch batch", nil)
		return nil, false
	}
	if b == nil {
		apperr.WriteError(c, http.StatusNotFound, "BATCH_NOT_FOUND", "batch not found", nil)
		return nil, false
	}
	return b, true
}

type ConfirmRequest struct {
	Action string `json:"action" binding:"required,oneof=generate_all generate_partial generate_valid cancel"`
	Count  *int   `json:"count,omitempty"`
}

// HandleBatchStatus — HTTP handler: возвращает агрегированное состояние партии (completed/failed/total)
// для клиента. Реализует GET /batch/{id}/status
// HandleBatchStatus — HTTP handler: возвращает агрегированное состояние партии.
// Реализует GET /api/v1/batch/{batchId} и соответствует ТЗ §3.4.
// Ответ содержит: batchId, status, progress (total/completed/failed/pending/percentComplete),
// results (successful[] и failed[]), billing (charged/refunded/pending), createdAt, updatedAt.
func HandleBatchStatus(c *gin.Context, deps *di.Deps) {
	id := c.Param("id")

	// load batch — 404 если не существует, 200 для любого статуса (включая completed/cancelled)
	var batchTotal, completed, failed int
	var createdAt time.Time
	var updatedAt time.Time
	status := string(batch.BatchStatusPending)
	if deps.BatchRepo != nil {
		b, err := deps.BatchRepo.GetByID(context.Background(), id)
		if err != nil {
			apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to fetch batch", nil)
			return
		}
		if b == nil {
			apperr.WriteError(c, http.StatusNotFound, "BATCH_NOT_FOUND", "batch not found", nil)
			return
		}
		batchTotal = b.TotalRows
		completed = b.CompletedCount
		failed = b.FailedCount
		createdAt = b.CreatedAt
		if b.CompletedAt != nil {
			updatedAt = *b.CompletedAt
		} else {
			updatedAt = b.CreatedAt
		}
		if b.Status != "" {
			status = string(b.Status)
		}
	}

	// fallback: if no batchTotal, try to infer from jobs
	if batchTotal == 0 && deps.JobRepo != nil {
		if jobs, err := deps.JobRepo.GetByBatch(context.Background(), id); err == nil {
			batchTotal = len(jobs)
			// if counts are not set, compute from job statuses
			if completed == 0 && failed == 0 {
				for _, j := range jobs {
					switch j.Status {
					case batch.JobStatusCompleted:
						completed++
					case batch.JobStatusFailed:
						failed++
					}
				}
			}
		}
	}

	pending := batchTotal - completed - failed
	if pending < 0 {
		pending = 0
	}
	var percent = 0.0
	if batchTotal > 0 {
		percent = (float64(completed) * 100.0) / float64(batchTotal)
	}

	// collect results: separate successful and failed
	successful := make([]gin.H, 0)
	failedArr := make([]gin.H, 0)
	if deps.JobRepo != nil {
		if results, err := deps.JobRepo.GetResultsByBatch(context.Background(), id); err == nil && results != nil {
			for _, r := range results {
				if r.ErrorCode == "" && r.ErrorMessage == "" {
					pdf, code := pkgxls.ParseBarcodeURLs(r.BarcodeURLs)
					item := gin.H{"row": r.RowNumber, "buildId": r.BuildID, "barcodeUrls": gin.H{"pdf417": pdf, "code128": code}}
					successful = append(successful, item)
				} else {
					item := gin.H{"row": r.RowNumber, "error": gin.H{"code": r.ErrorCode, "message": r.ErrorMessage}}
					failedArr = append(failedArr, item)
				}
			}
		}
	}

	// Формирование результатов и информации по биллингу согласно ТЗ §4.3
	// Results:
	//  - successful: массив объектов { row:int, buildId:string, barcodeUrls: { pdf417:string, code128:string } }
	//  - failed: массив объектов { row:int, error: { code:string, message:string } }
	// Billing:
	//  - charged: количество успешно начисленных/обработанных
	//  - refunded: количество возвращённых/ошибочных
	//  - pending: количество ожидающих
	//  - bySource: (опционально) детализация по источникам квот, если доступна

	billing := gin.H{"charged": completed, "refunded": failed, "pending": pending}

	// Обработчик возвращает JSON, соответствующий модели ТЗ §4.3 `BatchStatusResponse`:
	//   {
	//     "batchId": string,
	//     "status": string,             // BatchStatus (строка)
	//     "progress": { total, completed, failed, pending, percentComplete },
	//     "results": { successful: [], failed: [] },
	//     "billing": { charged, refunded, pending },
	//     "createdAt": string (RFC3339),
	//     "updatedAt": string (RFC3339),
	//   }

	var createdStr, updatedStr string
	if !createdAt.IsZero() {
		createdStr = createdAt.UTC().Format(time.RFC3339)
	}
	if !updatedAt.IsZero() {
		updatedStr = updatedAt.UTC().Format(time.RFC3339)
	}

	c.JSON(http.StatusOK, gin.H{
		"batchId": id,
		"status":  status,
		"progress": gin.H{
			"total":           batchTotal,
			"completed":       completed,
			"failed":          failed,
			"pending":         pending,
			"percentComplete": percent,
		},
		"results":   gin.H{"successful": successful, "failed": failedArr},
		"billing":   billing,
		"createdAt": createdStr,
		"updatedAt": updatedStr,
	})
}

// HandleConfirm — HTTP handler: подтверждение партии для генерации (partial/complete).
// Выполняет блокировку через Billing BFF (BlockBatch) и публикует одобренные job'ы в Kafka (bulk.job).
// Поддерживает сценарий частичной обработки
// HandleConfirm — HTTP handler: подтверждение партии для генерации (generate_all / generate_partial).
// Выполняет блокировку через Billing BFF (BlockBatch) и публикует одобренные job'ы в Kafka (bulk.job).
// Поддерживает сценарий частичной обработки и возвращает оценку времени обработки в секундах.
// Возвращаемый ответ: 202 Accepted { success: true, batchId: "...", status: "processing", jobsQueued: N, estimatedTimeSeconds: S }
// См. ТЗ §3.3
func HandleConfirm(c *gin.Context, deps *di.Deps) {
	id := c.Param("id")

	// Ранняя защита: deps обязателен для всех операций (ТЗ §3.3).
	if deps == nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVER_ERROR", "service unavailable", nil)
		return
	}

	var req ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid body", map[string]any{"details": err.Error()})
		return
	}

	// determine count
	count := 0
	if req.Action == "generate_all" {
		if deps.JobRepo != nil {
			jobs, _ := deps.JobRepo.GetByBatch(context.Background(), id)
			count = len(jobs)
		}
	} else if req.Action == "generate_partial" && req.Count != nil {
		count = *req.Count
	} else if req.Action == "generate_valid" {
		// use batch.ValidRows as the number of valid rows to process
		if deps.BatchRepo != nil {
			if b, err := deps.BatchRepo.GetByID(context.Background(), id); err == nil && b != nil {
				count = b.ValidRows
			}
		}
	} else {
		apperr.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "unsupported action", nil)
		return
	}

	// determine logger
	logger := getLoggerFromDeps(deps)

	// block billing via BFF (prefer service flow if available)
	if deps.Service != nil {
		// delegate to service ConfirmBatch which performs block, save tx ids, publish and rollback on failure
		queued, err := deps.Service.ConfirmBatch(context.Background(), id, count, nil, nil)
		if err != nil {
			apperr.WriteError(c, http.StatusPaymentRequired, "INSUFFICIENT_FUNDS", "billing block failed", nil)
			return
		}
		// log batch confirmation event (TZ §13.2)
		if logger != nil {
			traceID := logging.TraceIDFromCtx(context.Background())
			logger.Info("batch_confirmed", "traceId", traceID, "batchId", id, "action", req.Action, "count", count)
		}
		perJobSeconds := 3
		estimated := queued * perJobSeconds
		respondAcceptedProcessing(c, id, queued, estimated)
		return
	}

	if deps.BillingClient != nil {
		// safe ctx extraction
		ctxReq := requestCtx(c)
		traceID := logging.TraceIDFromCtx(ctxReq)
		if logger != nil {
			logger.Info("billing_block_started", "traceId", traceID, "batchId", id, "count", count)
		}
		// ВАЖНО: BlockBatch возвращает идентификаторы транзакций, которые необходимо
		// сохранять вместе с записью партии. При отмене эти transactionIds используются
		// для выполнения refund/unblock через Billing BFF.
		// Текущая реализация игнорирует ответ. Для полного соответствия ТЗ §3.5
		// нужно сохранять BlockBatchResponse.TransactionIDs в записи batch.
		_, err := deps.BillingClient.BlockBatch(context.Background(), "", count, id)
		if err != nil {
			if logger != nil {
				logger.Error("billing_block_failed", "traceId", traceID, "batchId", id, "error", err)
			}
			apperr.WriteError(c, http.StatusPaymentRequired, "INSUFFICIENT_FUNDS", "billing block failed", nil)
			return
		}
		if logger != nil {
			logger.Info("billing_block_succeeded", "traceId", traceID, "batchId", id, "count", count)
		}
	}

	// set batch status to processing via manager if available
	if deps.BatchManager != nil {
		_ = deps.BatchManager.SetProcessing(context.Background(), id)
	}

	// fetch jobs and publish up to count
	queued := 0
	if deps.JobRepo != nil && deps.Producer != nil {
		jobs, _ := deps.JobRepo.GetByBatch(context.Background(), id)
		for i, j := range jobs {
			if i >= count {
				break
			}
			// create event согласно ТЗ §7.2 / §8.2 (fallback path when Service == nil)
			evt := map[string]any{
				"eventType":          "bulk.job",
				"jobId":              j.ID,
				"batchId":            j.BatchID,
				"userId":             "",
				"rowNumber":          j.RowNumber,
				"revision":           "",
				"fields":             map[string]string{},
				"billingPreApproved": false,
				"timestamp":          time.Now().UTC().Format(time.RFC3339),
				"metadata":           map[string]any{"source": "bulk-service", "correlationId": j.ID},
			}
			// log publishing
			ctxReq2 := requestCtx(c)
			traceID := logging.TraceIDFromCtx(ctxReq2)
			if logger != nil {
				// Job queued is a DEBUG-level event per TZ §13.2; keep publishing log as INFO
				logger.Debug("job_queued", "traceId", traceID, "batchId", j.BatchID, "jobId", j.ID, "rowNumber", j.RowNumber)
				logger.Info("publishing_job", "traceId", traceID, "jobId", j.ID, "batchId", j.BatchID)
			}
			topic := getBulkJobTopic()
			perr := deps.Producer.Publish(context.Background(), topic, nil, evt)
			if perr != nil {
				if logger != nil {
					logger.Error("publishing_job_failed", "traceId", traceID, "jobId", j.ID, "batchId", j.BatchID, "error", perr)
				}
			} else {
				if logger != nil {
					logger.Info("publishing_job_succeeded", "traceId", traceID, "jobId", j.ID, "batchId", j.BatchID)
				}
			}
			// update status
			if err := deps.JobRepo.UpdateStatus(context.Background(), j.ID, string(batch.JobStatusQueued)); err == nil {
				queued++
				if logger != nil {
					logger.Info("job_queued", "traceId", traceID, "jobId", j.ID, "batchId", j.BatchID)
				}
			}
		}
	}

	// estimate time: per ТЗ §3.3 фиксированная оценка 3 секунды на задачу
	perJobSeconds := 3
	estimated := queued * perJobSeconds

	respondAcceptedProcessing(c, id, queued, estimated)
}

func HandleCancel(c *gin.Context, deps *di.Deps) {
	id := c.Param("id")
	// determine logger
	logger := getLoggerFromDeps(deps)
	ctxReq3 := requestCtx(c)
	traceID := logging.TraceIDFromCtx(ctxReq3)

	// Capture pre-cancel state: pending job count and billing transaction IDs
	// for correct refund response per ТЗ §3.5:
	// { "refund": { "pending": N, "refunded": M, "transactionIds": [...] } }
	var pendingCount int
	var txIDs []string
	if deps != nil && deps.BatchRepo != nil {
		if b, err := deps.BatchRepo.GetByID(context.Background(), id); err == nil && b != nil {
			txIDs = b.BillingTransactionIDs
		}
	}
	if deps != nil && deps.JobRepo != nil {
		if jobs, err := deps.JobRepo.GetByBatch(context.Background(), id); err == nil {
			for _, j := range jobs {
				switch j.Status {
				case batch.JobStatusPending, batch.JobStatusQueued, batch.JobStatusProcessing:
					pendingCount++
				}
			}
		}
	}

	if deps != nil && deps.BatchManager != nil {
		if logger != nil {
			logger.Info("batch_cancel_requested", "traceId", traceID, "batchId", id)
		}
		refunded, err := deps.BatchManager.SetCancelled(context.Background(), id)
		if err != nil {
			if logger != nil {
				logger.Error("batch_cancel_failed", "traceId", traceID, "batchId", id, "error", err)
			}
			apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to cancel batch", nil)
			return
		}
		if logger != nil {
			logger.Info("batch_cancelled", "traceId", traceID, "batchId", id, "refunded", refunded)
		}
		// ТЗ §3.5 response shape: { success, batchId, status, refund: {pending, refunded, transactionIds} }
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"batchId": id,
			"status":  "cancelled",
			"refund": gin.H{
				"pending":        pendingCount,
				"refunded":       refunded,
				"transactionIds": txIDs,
			},
		})
		return
	}
	// no batch manager configured: still return 200 with empty refund
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"batchId": id,
		"status":  "cancelled",
		"refund": gin.H{
			"pending":        0,
			"refunded":       0,
			"transactionIds": []string{},
		},
	})
}

// HandleListBatches GET /api/v1/batches
// возвращает историю batch'ов пользователя (ТЗ §3.6).
// Поддерживаемые query-параметры:
//   - status (string): фильтр по статусу (pending, ready, processing, completed, cancelled и т.д.)
//   - page (int): страница (начинается с 1)
//   - limit (int): количество записей на страницу
//   - from (datetime, RFC3339): начиная с даты
//   - to (datetime, RFC3339): по дату
//
// Формат ответа:
//
//	{
//	  "data": [ { "batchId": "uuid", "status": "completed", "totalJobs": 100, "completedJobs": 98, "failedJobs": 2, "createdAt": "...", "completedAt": "..." } ],
//	  "meta": { "page": 1, "limit": 20, "total": 15 }
//	}
func HandleListBatches(c *gin.Context, deps *di.Deps) {
	// parse query params
	status := c.Query("status")
	page := 1
	if p := c.Query("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	limit := 20
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	fromPtr := parseOptionalTime(c.Query("from"))
	toPtr := parseOptionalTime(c.Query("to"))

	userId := c.Query("userId")
	// allow userId to be provided as path parameter for admin-specific route
	if pid := c.Param("userId"); pid != "" {
		userId = pid
	}
	revision := c.Query("revision")
	sortBy := c.Query("sortBy")
	order := c.Query("order")
	cursor := c.Query("cursor")

	if deps.BatchRepo == nil {
		apperr.WriteError(c, http.StatusNotFound, "MISSING_DEPENDENCY", "batch repo not configured", nil)
		return
	}

	filter := ports.BatchFilter{Status: status, From: fromPtr, To: toPtr, Page: page, Limit: limit, UserID: userId, Revision: revision, SortBy: sortBy, SortDesc: order == "desc", Cursor: cursor}
	batches, total, err := deps.BatchRepo.List(context.Background(), filter)
	if err != nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to list batches", nil)
		return
	}

	data := make([]gin.H, 0, len(batches))
	for _, b := range batches {
		// ensure status is serialized as plain string
		item := gin.H{"batchId": b.ID, "status": string(b.Status), "totalJobs": b.TotalRows, "completedJobs": b.CompletedCount, "failedJobs": b.FailedCount, "createdAt": b.CreatedAt}
		if b.CompletedAt != nil {
			item["completedAt"] = b.CompletedAt
		}
		data = append(data, item)
	}

	c.JSON(http.StatusOK, gin.H{"data": data, "meta": gin.H{"page": page, "limit": limit, "total": total}})
}

// HandleAdminListBatches implements Admin API listing of batches per ТЗ §10.5.1
// GET /api/v1/admin/batches
// Response shape (ТЗ §10.5.1):
//
//	{
//	  "batches": [ { "id": "batch-123", "userId": "user-456", "status": "processing", "totalJobs": 100, "completedJobs": 45, "failedJobs": 2, "createdAt": "2026-01-08T10:00:00Z" } ],
//	  "pagination": { "page": 1, "perPage": 20, "total": 150 }
//	}
func HandleAdminListBatches(c *gin.Context, deps *di.Deps) {
	// parse query params similar to public ListBatches but admin uses perPage param name
	page := 1
	if p := c.Query("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	perPage := 20
	if pp := c.Query("perPage"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 {
			perPage = v
		}
	}

	status := c.Query("status")
	fromPtr := parseOptionalTime(c.Query("from"))
	toPtr := parseOptionalTime(c.Query("to"))

	userId := c.Query("userId")
	revision := c.Query("revision")

	if deps == nil || deps.BatchRepo == nil {
		apperr.WriteError(c, http.StatusNotFound, "MISSING_DEPENDENCY", "batch repo not configured", nil)
		return
	}

	filter := ports.BatchFilter{Status: status, From: fromPtr, To: toPtr, Page: page, Limit: perPage, UserID: userId, Revision: revision}
	batches, total, err := deps.BatchRepo.List(context.Background(), filter)
	if err != nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to list batches", nil)
		return
	}

	out := make([]gin.H, 0, len(batches))
	for _, b := range batches {
		created := ""
		if !b.CreatedAt.IsZero() {
			created = b.CreatedAt.UTC().Format(time.RFC3339)
		}
		item := gin.H{
			"id":            b.ID,
			"userId":        b.UserID,
			"status":        string(b.Status),
			"totalJobs":     b.TotalRows,
			"completedJobs": b.CompletedCount,
			"failedJobs":    b.FailedCount,
			"createdAt":     created,
		}
		out = append(out, item)
	}

	pagination := gin.H{"page": page, "perPage": perPage, "total": total}
	c.JSON(http.StatusOK, gin.H{"batches": out, "pagination": pagination})
}

// HandleAdminGetBatch implements Admin API: GET /api/v1/admin/batches/{batchId}
// ТЗ §10.5.3 — возвращает детальную информацию по партии, включая список задач (jobs)
// Response example (partial):
//
//	{
//	  "id": "batch-123",
//	  "userId": "user-456",
//	  "status": "processing",
//	  "revision": "US_CA_08292017",
//	  "jobs": [ { "id": "job-1", "status": "completed", "buildId": "build-abc", "rowNumber": 2 } ],
//	  "config": { "revision": "US_CA_08292017" },
//	  "createdAt": "2026-01-08T10:00:00Z",
//	  "updatedAt": "2026-01-08T10:15:00Z"
//	}
func HandleAdminGetBatch(c *gin.Context, deps *di.Deps) {
	id := c.Param("id")

	b, ok := requireBatch(c, deps, id)
	if !ok {
		return
	}

	// build timestamps
	created := ""
	if !b.CreatedAt.IsZero() {
		created = b.CreatedAt.UTC().Format(time.RFC3339)
	}
	updated := ""
	if b.CompletedAt != nil && !b.CompletedAt.IsZero() {
		updated = b.CompletedAt.UTC().Format(time.RFC3339)
	} else if !b.CreatedAt.IsZero() {
		updated = b.CreatedAt.UTC().Format(time.RFC3339)
	}

	// collect jobs
	jobsOut := make([]gin.H, 0)
	if deps.JobRepo != nil {
		jobs, _ := deps.JobRepo.GetByBatch(context.Background(), id)
		// gather results to attach buildId (if available)
		resultsMap := map[string]string{}
		if res, err := deps.JobRepo.GetResultsByBatch(context.Background(), id); err == nil {
			for _, r := range res {
				// r.JobID may be empty for legacy records; guard
				if r.JobID != "" {
					resultsMap[r.JobID] = r.BuildID
				}
			}
		}
		for _, j := range jobs {
			buildId := ""
			if v, ok := resultsMap[j.ID]; ok {
				buildId = v
			}
			jobsOut = append(jobsOut, gin.H{"id": j.ID, "status": string(j.Status), "buildId": buildId, "rowNumber": j.RowNumber})
		}
	}

	// config: project stores revision on batch; format is not persisted currently
	configOut := gin.H{"revision": b.Revision}

	resp := gin.H{
		"id":        b.ID,
		"userId":    b.UserID,
		"status":    string(b.Status),
		"revision":  b.Revision,
		"jobs":      jobsOut,
		"config":    configOut,
		"createdAt": created,
		"updatedAt": updated,
	}

	c.JSON(http.StatusOK, resp)
}

// RestartRequest represents request body for restarting a batch
type RestartRequest struct {
	RestartMode string `json:"restartMode" binding:"required,oneof=failed_only"`
	Force       bool   `json:"force"`
}

// HandleAdminRestartBatch implements Admin API: POST /api/v1/admin/batches/{batchId}/restart
// ТЗ §10.5.4 — перезапускает только неудачные jobs (restartMode == "failed_only").
// Returns: { "restarted": <int>, "skipped": <int> }
func HandleAdminRestartBatch(c *gin.Context, deps *di.Deps) {
	id := c.Param("id")

	var req RestartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid body", map[string]any{"details": err.Error()})
		return
	}

	if deps == nil || deps.BatchRepo == nil || deps.JobRepo == nil {
		apperr.WriteError(c, http.StatusNotFound, "MISSING_DEPENDENCY", "required dependency not configured", nil)
		return
	}

	// verify batch exists
	_, ok := requireBatch(c, deps, id)
	if !ok {
		return
	}

	// only supported mode for now: failed_only
	if req.RestartMode != "failed_only" {
		apperr.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "unsupported restartMode", nil)
		return
	}

	// fetch jobs for batch
	jobs, jerr := deps.JobRepo.GetByBatch(context.Background(), id)
	if jerr != nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to fetch jobs", nil)
		return
	}

	restarted := 0
	skipped := 0

	topic := getBulkJobTopic()

	for _, j := range jobs {
		// decide if should restart: only jobs with status == failed
		if j.Status != domain.JobStatusFailed {
			skipped++
			continue
		}

		// attempt to publish job event
		evt := map[string]any{"eventType": "bulk.job", "jobId": j.ID, "batchId": j.BatchID, "rowNumber": j.RowNumber}
		perr := error(nil)
		if deps.Producer != nil {
			perr = deps.Producer.Publish(context.Background(), topic, nil, evt)
		}
		if perr != nil {
			// publishing failed — skip and continue
			skipped++
			continue
		}

		// update job status to queued (best-effort)
		if err := deps.JobRepo.UpdateStatus(context.Background(), j.ID, string(domain.JobStatusQueued)); err == nil {
			restarted++
		} else {
			// if update failed, we consider it skipped
			skipped++
		}
	}

	// notify batch manager to recalc status if available
	if deps.BatchManager != nil {
		_ = deps.BatchManager.SetProcessing(context.Background(), id)
	}

	c.JSON(http.StatusOK, gin.H{"restarted": restarted, "skipped": skipped})
}

// UpdateBatchConfigRequest represents admin request to update batch config
type UpdateBatchConfigRequest struct {
	Priority string `json:"priority"`
	Timeout  int    `json:"timeout"` // milliseconds
}

// HandleAdminUpdateBatchConfig implements Admin API: PUT /api/v1/admin/batches/{batchId}/config
// Updates admin-configurable fields for a batch (priority, timeout)
func HandleAdminUpdateBatchConfig(c *gin.Context, deps *di.Deps) {
	id := c.Param("id")
	var req UpdateBatchConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apperr.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid body", map[string]any{"details": err.Error()})
		return
	}

	if deps == nil || deps.BatchRepo == nil {
		apperr.WriteError(c, http.StatusNotFound, "MISSING_DEPENDENCY", "batch repo not configured", nil)
		return
	}

	b, err := deps.BatchRepo.GetByID(context.Background(), id)
	if err != nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to fetch batch", nil)
		return
	}
	if b == nil {
		apperr.WriteError(c, http.StatusNotFound, "BATCH_NOT_FOUND", "batch not found", nil)
		return
	}

	// apply changes
	if req.Priority != "" {
		b.Priority = req.Priority
	}
	if req.Timeout > 0 {
		b.TimeoutMs = req.Timeout
	}

	if err := deps.BatchRepo.Update(context.Background(), b); err != nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to update batch config", nil)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "batchId": id, "config": gin.H{"priority": b.Priority, "timeout": b.TimeoutMs}})
}
