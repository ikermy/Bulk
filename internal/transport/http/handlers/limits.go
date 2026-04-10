package handlers

import (
    "net/http"

    "github.com/gin-gonic/gin"
    cfg "github.com/ikermy/Bulk/internal/config"
    "github.com/ikermy/Bulk/internal/limits"
    apperr "github.com/ikermy/Bulk/internal/transport/http/apperror"
)

// BulkLimits holds admin-configurable limits for bulk processing.
type BulkLimits struct {
    MaxFileSize        int `json:"maxFileSize"`        // bytes
    MaxRowsPerBatch    int `json:"maxRowsPerBatch"`
    MaxConcurrentBatches int `json:"maxConcurrentBatches"`
    MaxBatchesPerHour  int `json:"maxBatchesPerHour"`
}

// SetDefaultLimitsFromConfig initializes in-memory limits from app config (called from router init).
func SetDefaultLimitsFromConfig(c *cfg.Config) {
    if c == nil {
        return
    }
    // Update central limits store from config
    l := limits.Limits{}
    if c.Limits.MaxFileSizeMB > 0 {
        l.MaxFileSize = c.Limits.MaxFileSizeMB * 1024 * 1024
    }
    if c.Limits.MaxRowsPerBatch > 0 {
        l.MaxRowsPerBatch = c.Limits.MaxRowsPerBatch
    }
    if c.Limits.MaxConcurrentBatches > 0 {
        l.MaxConcurrentBatches = c.Limits.MaxConcurrentBatches
    }
    if c.Limits.MaxBatchesPerHour > 0 {
        l.MaxBatchesPerHour = c.Limits.MaxBatchesPerHour
    }
    limits.UpdateIfPositive(l)
}

// HandleGetBulkLimits returns current in-memory bulk limits (GET /api/v1/admin/config/bulk-limits)
func HandleGetBulkLimits(c *gin.Context) {
    l := limits.Get()
    c.JSON(http.StatusOK, BulkLimits{MaxFileSize: l.MaxFileSize, MaxRowsPerBatch: l.MaxRowsPerBatch, MaxConcurrentBatches: l.MaxConcurrentBatches, MaxBatchesPerHour: l.MaxBatchesPerHour})
}

// HandlePutBulkLimits updates in-memory bulk limits (PUT /api/v1/admin/config/bulk-limits)
func HandlePutBulkLimits(c *gin.Context) {
    var req BulkLimits
    if err := c.ShouldBindJSON(&req); err != nil {
        apperr.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "invalid body", map[string]any{"details": err.Error()})
        return
    }
    // basic validation
    if req.MaxFileSize <= 0 || req.MaxRowsPerBatch <= 0 || req.MaxConcurrentBatches <= 0 || req.MaxBatchesPerHour <= 0 {
        apperr.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "limits must be positive", nil)
        return
    }
    // update central limits store
    limits.UpdateIfPositive(limits.Limits{MaxFileSize: req.MaxFileSize, MaxRowsPerBatch: req.MaxRowsPerBatch, MaxConcurrentBatches: req.MaxConcurrentBatches, MaxBatchesPerHour: req.MaxBatchesPerHour})
    l := limits.Get()
    c.JSON(http.StatusOK, gin.H{"success": true, "config": BulkLimits{MaxFileSize: l.MaxFileSize, MaxRowsPerBatch: l.MaxRowsPerBatch, MaxConcurrentBatches: l.MaxConcurrentBatches, MaxBatchesPerHour: l.MaxBatchesPerHour}})
}

