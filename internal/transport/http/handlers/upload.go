package handlers

import (
	"context"
	"errors"
	"fmt"

	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ikermy/Bulk/internal/di"
	"github.com/ikermy/Bulk/internal/limits"
	"github.com/ikermy/Bulk/internal/logging"
	"github.com/ikermy/Bulk/internal/metrics"
	"github.com/ikermy/Bulk/internal/parser"
	apperr "github.com/ikermy/Bulk/internal/transport/http/apperror"
)

type UploadResponse struct {
	Success bool   `json:"success"`
	BatchID string `json:"batchId"`
	Status  string `json:"status"`
}

// Соответствие ТЗ §4.3 - UploadRequest:
// В ТЗ определена модель UploadRequest:
//   type UploadRequest struct {
//       File     multipart.File
//       Revision string `form:"revision" validate:"required"`
//   }
//
// Этот HTTP-обработчик реализует указанную модель:
// - Файл читается из multipart-поля "file" через c.Request.FormFile("file") и
//   передаётся в сервис как io.Reader.
// - Поле "revision" читается через c.Request.FormValue("revision").
// - Проверка загруженного файла (формат, размер, количество строк) выполняется
//   функцией ValidateUpload(file, header).

// HandleUpload — HTTP handler: приём XLS-файла от Frontend, валидация, сохранение/парсинг и
// делегирование обработки в Service.CreateBatchFromFile
func HandleUpload(c *gin.Context, deps *di.Deps) {
	// read/echo X-Request-ID for tracing (TZ §3.2)
	xReqID := c.GetHeader("X-Request-ID")
	if xReqID != "" {
		c.Header("X-Request-ID", xReqID)
	}
	// enforce maximum request body size to prevent resource exhaustion
	// wrap the request body so ParseMultipartForm/FormFile can't read more than allowed
	// Примечание: MaxMultipartMemory контролирует только сколько multipart-частей хранить в памяти
	// до записи на диск, но НЕ ограничивает общий размер тела запроса. Поэтому мы дополнительно
	// ограничиваем тело запроса через http.MaxBytesReader. Это защитит сервер от DoS, когда
	// клиент пытается передать очень большой body и заставляет парсер создавать большие temp-файлы.
	maxFile := int64(limits.Get().MaxFileSize)
	// add small margin for multipart overhead
	const multipartOverhead = 1024 * 16
	// quick check using Content-Length when present to return fast (быстрая проверка по Content-Length)
	if c.Request.ContentLength > 0 && c.Request.ContentLength > maxFile+multipartOverhead {
		// инкрементируем метрику отказов по причине "too_large"
		metrics.UploadsRejectedTotal.WithLabelValues("too_large").Inc()
		apperr.WriteError(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "file too large", nil)
		return
	}
	// Оборачиваем тело запроса: если downstream попытается прочитать больше, чем разрешено,
	// MaxBytesReader остановит чтение и вернёт sentinel-ошибку ErrRequestBodyTooLarge.
	// Используем собственную обёртку, чтобы избежать надёжности проверки по тексту ошибки.
	c.Request.Body = MaxBytesReader(c.Request.Body, maxFile+multipartOverhead)

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		// distinguish between missing file and body-too-large
		// now we can reliably check sentinel error
		if errors.Is(err, ErrRequestBodyTooLarge) {
			metrics.UploadsRejectedTotal.WithLabelValues("too_large").Inc()
			apperr.WriteError(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "file too large", nil)
			return
		}
		apperr.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "file is required", nil)
		return
	}
	defer func() { _ = file.Close() }()

	// determine logger to use and enrich with traceId per TZ §13.3
	var baseLogger logging.Logger
	if deps != nil && deps.Logger != nil {
		baseLogger = deps.Logger
	} else if deps != nil && deps.Service != nil && deps.Service.Logger != nil {
		baseLogger = deps.Service.Logger
	}
	// logger prepopulated with traceId from context
	logger := logging.FromContext(c.Request.Context(), baseLogger)

	// userId may be provided in header or absent
	userId := c.GetHeader("X-User-Id")

	// enrich with user/file specific fields for this request
	if logger != nil {
		logger = logger.With("userId", userId, "file", header.Filename, "fileSize", header.Size)
		logger.Info("upload_started")
	}

	// validate upload
	// ValidateUpload выполняет проверку header.Size, расширения и magic-bytes/content-type
	// (см. комментарии в validate_upload.go). При отклонении инкрементируем соответствующую метрику.
	if err := ValidateUpload(file, header); err != nil {
		switch err {
		case ErrFileTooLarge:
			metrics.UploadsRejectedTotal.WithLabelValues("too_large").Inc()
			apperr.WriteError(c, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE", "file too large", nil)
		case ErrInvalidFileFormat, ErrInvalidContentType:
			metrics.UploadsRejectedTotal.WithLabelValues("invalid_format").Inc()
			apperr.WriteError(c, http.StatusBadRequest, "INVALID_FILE_FORMAT", err.Error(), nil)
		default:
			metrics.UploadsRejectedTotal.WithLabelValues("validation_error").Inc()
			apperr.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		}
		return
	}

	revision := c.Request.FormValue("revision")
	if revision == "" {
		apperr.WriteError(c, http.StatusBadRequest, "MISSING_REVISION", "revision is required", nil)
		return
	}

	ctx := context.Background()
	// delegate to service
	if deps == nil || deps.Service == nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVER_ERROR", "service unavailable", nil)
		return
	}
	res, err := deps.Service.CreateBatchFromFile(ctx, file, revision)
	if err != nil {
		if logger != nil {
			logger.Errorw("upload_failed", "userId", userId, "error", err)
		}
		if status, code, msg, details, ok := parserErrorResponse(err); ok {
			apperr.WriteError(c, status, code, msg, details)
			return
		}
		apperr.WriteError(c, http.StatusBadRequest, "INVALID_FILE_FORMAT", "failed to parse file", nil)
		return
	}

	if logger != nil {
		logger.With("batchId", res.BatchID, "totalRows", res.TotalRows, "validRows", res.ValidRows, "invalidRows", res.InvalidRows).Info("upload_completed")
	}

	// update batch state via manager if available
	if deps.BatchManager != nil {
		_ = deps.BatchManager.FinalizeAfterUpload(context.Background(), res.BatchID, res.ValidRows, res.InvalidRows)
	}

	// if there are validation errors
	if res.InvalidRows > 0 {
		if logger != nil {
			logger.Warnw("validation_errors_returned", "batchId", res.BatchID, "userId", userId, "invalidRows", res.InvalidRows)
		}
		// map errors to API shape
		errs := make([]gin.H, 0, len(res.Errors))
		for _, e := range res.Errors {
			errs = append(errs, gin.H{"row": e.RowNumber, "field": e.Field, "code": e.ErrorCode, "message": e.ErrorMessage, "value": e.OriginalValue})
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"batchId": res.BatchID,
			"status":  "validation_errors",
			"summary": gin.H{"totalRows": res.TotalRows, "validRows": res.ValidRows, "invalidRows": res.InvalidRows},
			"errors":  errs,
			"options": gin.H{"generateValid": gin.H{"count": res.ValidRows, "message": "Generate valid rows, skip invalid?"}, "downloadErrors": gin.H{"url": "/api/v1/batch/" + res.BatchID + "/errors.xlsx"}, "cancel": gin.H{"message": "Cancel and fix file"}},
		})
		return
	}

	// billing quote via BFF
	if deps.BillingClient != nil {
		if logger != nil {
			logger.Infow("billing_quote_started", "batchId", res.BatchID, "userId", userId, "units", res.ValidRows)
		}
		qr, err := deps.BillingClient.Quote(ctx, "", res.ValidRows)
		if err != nil {
			if logger != nil {
				logger.Errorw("billing_quote_failed", "batchId", res.BatchID, "userId", userId, "error", err)
			}
			apperr.WriteError(c, http.StatusServiceUnavailable, "BILLING_ERROR", "billing quote failed", nil)
			return
		}
		if logger != nil {
			logger.Infow("billing_quote_succeeded", "batchId", res.BatchID, "userId", userId, "allowedTotal", qr.AllowedTotal)
		}
		// map billing.BySource to numeric map as expected by TZ (e.g. subscription:50, credits_type1:50)
		bySrcNums := gin.H{}
		if qr.BySource.Subscription.Units != 0 {
			bySrcNums["subscription"] = qr.BySource.Subscription.Units
		}
		if qr.BySource.Credits.Units != 0 {
			// map to TZ currency key
			bySrcNums["credits_type1"] = qr.BySource.Credits.Units
		}
		if qr.BySource.Wallet.Units != 0 {
			bySrcNums["wallet"] = qr.BySource.Wallet.Units
		}

		billingInfo := gin.H{
			"canGenerate":  qr.CanProcess,
			"canProcess":   qr.CanProcess,
			"requested":    res.ValidRows,
			"allowedTotal": qr.AllowedTotal,
			"bySource":     bySrcNums,
		}
		// shortfall as number
		if res.ValidRows > qr.AllowedTotal {
			billingInfo["shortfall"] = res.ValidRows - qr.AllowedTotal
		}
		// estimated cost and currency if available
		if qr.UnitPrice != 0 {
			est := float64(res.ValidRows) * qr.UnitPrice
			// include estimated cost in summary
			if qr.UnitPrice != 0 {
				// attach to summary below
				// continue
			}
			billingInfo["unitPrice"] = qr.UnitPrice
			billingInfo["estimatedCost"] = est
			billingInfo["currency"] = "credits_type1"
		}

		if qr.AllowedTotal >= res.ValidRows {
			summary := gin.H{"totalRows": res.TotalRows, "validRows": res.ValidRows, "invalidRows": res.InvalidRows}
			if v, ok := billingInfo["estimatedCost"]; ok {
				summary["estimatedCost"] = v
				summary["currency"] = billingInfo["currency"]
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "batchId": res.BatchID, "status": "ready", "summary": summary, "billing": billingInfo, "nextStep": "confirm"})
			return
		}

		// partial available — ТЗ §10.1 defines 402 PARTIAL_AVAILABLE
		partialCount := qr.AllowedTotal
		shortfall := res.ValidRows - partialCount
		_ = shortfall
		apperr.WriteError(c, http.StatusPaymentRequired, "PARTIAL_AVAILABLE",
			fmt.Sprintf("Insufficient funds. Generate %d of %d?", partialCount, res.ValidRows),
			map[string]any{
				"batchId": res.BatchID,
				"billing": billingInfo,
				"options": gin.H{
					"generatePartial": gin.H{
						"count":   partialCount,
						"message": fmt.Sprintf("Insufficient funds. Generate %d of %d?", partialCount, res.ValidRows),
					},
					"cancel": gin.H{
						"message": "Cancel batch and top up balance",
					},
				},
			})
		return
	}

	// default: ready
	c.JSON(http.StatusOK, gin.H{"success": true, "batchId": res.BatchID, "status": "ready", "summary": gin.H{"totalRows": res.TotalRows, "validRows": res.ValidRows, "invalidRows": res.InvalidRows}})
}

// parserErrorResponse converts known parser errors to HTTP status, TZ code and message.
// Returns handled==true if the error was recognized.
func parserErrorResponse(err error) (int, string, string, map[string]any, bool) {
	// handle sentinel/simple errors using errors.Is for compatibility with wrapped errors
	if errors.Is(err, parser.ErrEmptyFile) {
		return http.StatusBadRequest, "EMPTY_FILE", "file is empty", nil, true
	}
	if errors.Is(err, parser.ErrTooManyRows) {
		return http.StatusBadRequest, "TOO_MANY_ROWS", "too many rows", nil, true
	}
	// handle structured InvalidHeadersError
	var ihe *parser.InvalidHeadersError
	if errors.As(err, &ihe) {
		details := map[string]any{"missingHeaders": ihe.Missing}
		return http.StatusBadRequest, "INVALID_HEADERS", "invalid headers", details, true
	}
	// fallback for sentinel ErrInvalidHeaders
	if errors.Is(err, parser.ErrInvalidHeaders) {
		return http.StatusBadRequest, "INVALID_HEADERS", "invalid headers", nil, true
	}
	return 0, "", "", nil, false
}
