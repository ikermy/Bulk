package handlers

import (
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ikermy/Bulk/internal/di"
	apperr "github.com/ikermy/Bulk/internal/transport/http/apperror"
)

// HandleAdminStats — HTTP handler для административных метрик (GET /api/v1/admin/stats).
// Соответствует ТЗ §3.9 и §10.5.7. Возвращает вложенную структуру:
//
//	{
//	  "today": { "batchesCreated": X, "jobsProcessed": Y, "jobsFailed": Z, "averageProcessingTimeMs": V },
//	  "queues": { "bulkJobPending": A, "bulkResultPending": B }
//	}
func HandleAdminStats(c *gin.Context, deps *di.Deps) {
	// parse optional from/to query params (RFC3339). Default: start of today .. now
	now := time.Now().UTC()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	to := now
	if fs := c.Query("from"); fs != "" {
		if t, err := time.Parse(time.RFC3339, fs); err == nil {
			from = t
		}
	}
	if ts := c.Query("to"); ts != "" {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			to = t
		}
	}

	var batchesCreated int
	var jobsProcessed int
	var jobsFailed int
	var avgProcessingTimeMs float64
	var bulkJobPending int
	var bulkResultPending int

	if deps != nil && deps.BatchRepo != nil {
		s, err := deps.BatchRepo.AdminStats(c.Request.Context(), &from, &to)
		if err != nil {
			apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to compute admin stats", nil)
			return
		}
		if s != nil {
			batchesCreated = s.BatchesCreated
			jobsProcessed = s.JobsProcessed
			jobsFailed = s.JobsFailed
			avgProcessingTimeMs = math.Round(s.AverageProcessingTimeMs)
			bulkJobPending = s.Queues.BulkJobPending
			bulkResultPending = s.Queues.BulkResultPending
		}
	}

	// Response combines ТЗ §3.9 nested shape { today, queues } and ТЗ §10.5.7 flat shape:
	// { totalBatches, activeBatches, completedToday, failedToday, avgProcessingTime }
	c.JSON(http.StatusOK, gin.H{
		// ТЗ §10.5.7 flat fields
		"totalBatches":    batchesCreated,
		"activeBatches":   0, // not tracked separately in current stats model
		"completedToday":  batchesCreated,
		"failedToday":     jobsFailed,
		"avgProcessingTime": int64(avgProcessingTimeMs),
		// ТЗ §3.9 nested shape
		"today": gin.H{
			"batchesCreated":          batchesCreated,
			"jobsProcessed":           jobsProcessed,
			"jobsFailed":              jobsFailed,
			"averageProcessingTimeMs": avgProcessingTimeMs,
		},
		"queues": gin.H{
			"bulkJobPending":    bulkJobPending,
			"bulkResultPending": bulkResultPending,
		},
	})
}



