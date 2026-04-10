package http

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ikermy/Bulk/internal/config"
	"github.com/ikermy/Bulk/internal/di"
	"github.com/ikermy/Bulk/internal/limits"
	"github.com/ikermy/Bulk/internal/transport/http/handlers"
	"github.com/ikermy/Bulk/internal/transport/http/middleware"
)

// NewRouter builds gin engine and registers routes using injected dependencies (di.Deps)
func NewRouter(cfg *config.Config, deps *di.Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(TracingMiddleware())
	// auth and rate limit middleware as per TZ
	r.Use(middleware.AuthMiddleware())
	r.Use(middleware.RateLimitMiddleware())
	// register metrics and middleware
	RegisterMetrics(r)

	// synchronize Gin's MaxMultipartMemory with runtime limits to avoid unnecessary disk writes
	// По умолчанию gin.Engine.MaxMultipartMemory == 32 << 20 (32 MiB). Это контролирует,
	// сколько multipart-частей хранится в памяти до записи на диск. Однако это НЕ ограничивает
	// общий размер тела запроса — поэтому дополнительная защита через http.MaxBytesReader
	// в обработчике загрузки всё равно необходима. Здесь мы синхронизируем MaxMultipartMemory
	// с runtime-лимитом из `limits` чтобы при небольших лимитах multipart части оставались в памяти
	// и не писались на диск ненужным образом.
	r.MaxMultipartMemory = int64(limits.Get().MaxFileSize)

	// health — GET /health: состояние сервиса (ТЗ §3: Health)
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	// readiness — GET /ready: проверяет доступность зависимостей (DB Ping, Kafka producer configured)
	// Это важно для Kubernetes readinessProbe: приложение должно возвращать 200 только когда
	// оно готово принимать трафик (DB доступен, и при наличии Kafka-брокеров инициализирован producer).
	r.GET("/ready", func(c *gin.Context) {
		// short timeout for readiness checks
		ctx := c.Request.Context()
		if deps != nil && deps.DB != nil {
			tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			if err := deps.DB.PingContext(tctx); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unready", "db": "ping_failed"})
				return
			}
		}
		// if Kafka configured in cfg, ensure producer is present
		if cfg != nil && cfg.Kafka.Brokers != "" {
			if deps == nil || deps.Producer == nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unready", "kafka": "producer_missing"})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	api := r.Group("/api/v1")
	{
		// Upload endpoint — вход с Frontend Web App: принимает XLS
		api.POST("/upload", func(c *gin.Context) { handlers.HandleUpload(c, deps) })
		api.GET("/batches", func(c *gin.Context) { handlers.HandleListBatches(c, deps) })
		api.GET("/batch/:id", func(c *gin.Context) { handlers.HandleBatchStatus(c, deps) })
		// Alias for batch status to match TZ: GET /api/v1/batch/{id}/status
		api.GET("/batch/:id/status", func(c *gin.Context) { handlers.HandleBatchStatus(c, deps) })
		// Confirm batch (ТЗ §3.3 POST /api/v1/batch/{batchId}/confirm)
		api.POST("/batch/:id/confirm", func(c *gin.Context) { handlers.HandleConfirm(c, deps) })
		api.POST("/batch/:id/cancel", func(c *gin.Context) { handlers.HandleCancel(c, deps) })
		// async export
		api.POST("/batch/:id/export", func(c *gin.Context) { handlers.HandleStartExport(c, deps) })
		api.GET("/export/:id", func(c *gin.Context) { handlers.HandleGetExport(c, deps) })
		// Скачивание ошибок валидации для партии (ТЗ §3.7)
		api.GET("/batch/:id/errors.xlsx", func(c *gin.Context) { handlers.HandleDownloadErrorsXLS(c, deps) })
		// Скачивание результатов (buildId, URLs) для партии (ТЗ §3.8)
		api.GET("/batch/:id/results.xlsx", func(c *gin.Context) { handlers.HandleDownloadResultsXLS(c, deps) })
		// Template download for revision (ТЗ §5.4)
		api.GET("/template/:revision", func(c *gin.Context) { handlers.HandleDownloadTemplateXLS(c) })
	}

	// initialize handlers in-memory defaults
	handlers.SetDefaultLimitsFromConfig(cfg)

	admin := r.Group("/api/v1/admin")
	{
		admin.GET("/stats", func(c *gin.Context) { handlers.HandleAdminStats(c, deps) })
		// Admin API: list batches (ТЗ §10.5.1 GET /admin/batches)
		// Возвращает shape: { "batches": [...], "pagination": { "page": 1, "perPage": 20, "total": 150 } }
		admin.GET("/batches", func(c *gin.Context) { handlers.HandleAdminListBatches(c, deps) })
		// Admin API: batch details (ТЗ §10.5.3 GET /admin/batches/{batchId})
		admin.GET("/batches/:id", func(c *gin.Context) { handlers.HandleAdminGetBatch(c, deps) })
		// Admin API: update batch config (ТЗ §10.5.5 PUT /admin/batches/{batchId}/config)
		admin.PUT("/batches/:id/config", func(c *gin.Context) { handlers.HandleAdminUpdateBatchConfig(c, deps) })
		// Admin API: bulk limits config (ТЗ §10.5.6)
		admin.PUT("/config/bulk-limits", func(c *gin.Context) { handlers.HandlePutBulkLimits(c) })
		admin.GET("/config/bulk-limits", func(c *gin.Context) { handlers.HandleGetBulkLimits(c) })
		// Admin API: list batches for specific user (ТЗ §10.5.2 GET /admin/users/{userId}/batches)
		admin.GET("/users/:userId/batches", func(c *gin.Context) { handlers.HandleAdminListBatches(c, deps) })

		// Admin API: restart failed jobs in batch (ТЗ §10.5.4 POST /admin/batches/{batchId}/restart)
		admin.POST("/batches/:id/restart", func(c *gin.Context) { handlers.HandleAdminRestartBatch(c, deps) })
	}

	return r
}
