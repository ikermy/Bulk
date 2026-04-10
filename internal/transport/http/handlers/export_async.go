package handlers

import (
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ikermy/Bulk/internal/di"
	"github.com/ikermy/Bulk/internal/logging"
	apperr "github.com/ikermy/Bulk/internal/transport/http/apperror"
)

// ExportPresignTTL is the default TTL for presigned URLs.
// FIXME: TTL is not specified in Bulk_Service_TZ — make configurable if required.
const ExportPresignTTL = time.Hour

// HandleStartExport POST /api/v1/batch/:id/export
func HandleStartExport(c *gin.Context, deps *di.Deps) {
	id := c.Param("id")
	if deps == nil || deps.ExportManager == nil {
		apperr.WriteError(c, http.StatusNotFound, "MISSING_DEPENDENCY", "export manager not configured", nil)
		return
	}
	// determine logger and request context
	logger := getLoggerFromDeps(deps)
	ctxReq := requestCtx(c)
	traceID := logging.TraceIDFromCtx(ctxReq)
	if logger != nil {
		logger.Info("export_enqueue_requested", "traceId", traceID, "exportId", id)
	}
	if err := deps.ExportManager.Enqueue(id); err != nil {
		if logger != nil {
			logger.Error("export_enqueue_failed", "traceId", traceID, "exportId", id, "error", err)
		}
		apperr.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	if logger != nil {
		logger.Info("export_enqueued", "traceId", traceID, "exportId", id)
	}
	c.JSON(http.StatusAccepted, gin.H{"id": id, "status": "queued"})
}

// HandleGetExport GET /api/v1/export/:id
// Behavior: if storage supports presign, return presigned URL (TTL 1h by default).
// If ?download=true is provided or presign not available, proxy the file contents.
func HandleGetExport(c *gin.Context, deps *di.Deps) {
	id := c.Param("id")
	if deps == nil || deps.ExportManager == nil {
		apperr.WriteError(c, http.StatusNotFound, "MISSING_DEPENDENCY", "export manager not configured", nil)
		return
	}
	// determine logger and request context
	logger := getLoggerFromDeps(deps)
	ctxReq := requestCtx(c)
	traceID := logging.TraceIDFromCtx(ctxReq)
	if res, ok := deps.ExportManager.Get(id); ok {
		// try presign if storage configured and not forced download
		download := c.Query("download") == "true"
		if deps.Storage != nil && res.StorageID != "" && !download {
			ttl := ExportPresignTTL
			if url, err := deps.Storage.Presign(res.StorageID, ttl); err == nil {
				c.JSON(http.StatusOK, gin.H{
					"id":         res.ID,
					"status":     res.Status,
					"url":        url,
					"expires_at": time.Now().Add(ttl).UTC().Format(time.RFC3339),
					"error":      res.Error,
				})
				return
			}
			// else fallthrough to proxy
		}

		if deps.Storage != nil && res.StorageID != "" {
			if rc, err := deps.Storage.Get(res.StorageID); err == nil {
				defer rc.Close()
				c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
				c.Header("Content-Disposition", "attachment; filename=results_"+res.ID+".xlsx")
				if _, err := io.Copy(c.Writer, rc); err != nil {
					// streaming error
					if logger != nil {
						logger.Error("export_stream_failed", "traceId", traceID, "exportId", res.ID, "error", err)
					}
					apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to stream file", nil)
					return
				}
				return
			}
		}

		// fallback: return metadata only
		c.JSON(http.StatusOK, gin.H{
			"id":         res.ID,
			"status":     res.Status,
			"storage_id": res.StorageID,
			"error":      res.Error,
			"created_at": res.CreatedAt,
			"updated_at": res.UpdatedAt,
		})
		return
	}
	apperr.WriteError(c, http.StatusNotFound, "NOT_FOUND", "export not found", nil)
}
