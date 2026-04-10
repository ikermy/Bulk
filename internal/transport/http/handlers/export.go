package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/ikermy/Bulk/internal/di"
	apperr "github.com/ikermy/Bulk/internal/transport/http/apperror"
	pkgxls "github.com/ikermy/Bulk/pkg/xls"
	"github.com/xuri/excelize/v2"
)

// parseLimitParam извлекает опциональный query-параметр `limit` (ТЗ §3.7, §3.8).
func parseLimitParam(c *gin.Context) int {
	if l := c.Query("limit"); l != "" {
		if li, err := strconv.Atoi(l); err == nil && li > 0 {
			return li
		}
	}
	return 0
}

// buildXLSXResponse создаёт XLSX-файл с указанным листом, заголовками и строками,
// после чего стримит его клиенту. writeRows вызывается для записи данных в sw;
// при ошибке внутри writeRows следует вернуть её — ответ будет 500 SERVICE_ERROR.
func buildXLSXResponse(c *gin.Context, batchID, sheetName, filenamePrefix string,
	headers []interface{}, writeRows func(sw *excelize.StreamWriter) error,
) {
	f := excelize.NewFile()
	if _, err := f.NewSheet(sheetName); err != nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to create sheet", nil)
		return
	}
	sw, err := f.NewStreamWriter(sheetName)
	if err != nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to create stream writer", nil)
		return
	}
	if err := sw.SetRow("A1", headers); err != nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to write header", nil)
		return
	}
	if err := writeRows(sw); err != nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", err.Error(), nil)
		return
	}
	if err := sw.Flush(); err != nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to flush xlsx", nil)
		return
	}
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s_%s.xlsx", filenamePrefix, batchID))
	if err := f.Write(c.Writer); err != nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to write xlsx", nil)
	}
}

// HandleDownloadErrorsXLS GET /api/v1/batch/{id}/errors.xlsx
// возвращает XLS файл с ошибками валидации для партии (ТЗ §3.7).
func HandleDownloadErrorsXLS(c *gin.Context, deps *di.Deps) {
	id := c.Param("id")
	if deps.ValidationRepo == nil {
		apperr.WriteError(c, http.StatusNotFound, "MISSING_DEPENDENCY", "validation repo not configured", nil)
		return
	}
	errs, err := deps.ValidationRepo.GetValidationErrors(context.Background(), id)
	if err != nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to fetch validation errors", nil)
		return
	}
	limit := parseLimitParam(c)
	headers := []interface{}{"row", "field", "code", "message", "original_value"}

	buildXLSXResponse(c, id, "Errors", "errors", headers, func(sw *excelize.StreamWriter) error {
		rowIndex := 2
		for i, e := range errs {
			if limit > 0 && i >= limit {
				break
			}
			cell, _ := excelize.CoordinatesToCellName(1, rowIndex)
			if err := sw.SetRow(cell, []interface{}{e.RowNumber, e.Field, e.ErrorCode, e.ErrorMessage, e.OriginalValue}); err != nil {
				return fmt.Errorf("failed to write row: %w", err)
			}
			rowIndex++
		}
		return nil
	})
}

// HandleDownloadResultsXLS GET /api/v1/batch/{id}/results.xlsx
// возвращает XLS файл с результатами генерации для партии (ТЗ §3.8).
func HandleDownloadResultsXLS(c *gin.Context, deps *di.Deps) {
	id := c.Param("id")
	if deps.JobRepo == nil {
		apperr.WriteError(c, http.StatusNotFound, "MISSING_DEPENDENCY", "job repo not configured", nil)
		return
	}
	results, err := deps.JobRepo.GetResultsByBatch(context.Background(), id)
	if err != nil {
		apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to fetch job results", nil)
		return
	}
	limit := parseLimitParam(c)
	headers := []interface{}{"row", "buildId", "pdf417_url", "code128_url", "error_code", "error_message"}

	buildXLSXResponse(c, id, "Results", "results", headers, func(sw *excelize.StreamWriter) error {
		rowIndex := 2
		for i, res := range results {
			if limit > 0 && i >= limit {
				break
			}
			// pkgxls.ParseBarcodeURLs — общая утилита из pkg/xls (ТЗ §3.8, §8.3).
			pdf, code128 := pkgxls.ParseBarcodeURLs(res.BarcodeURLs)
			cell, _ := excelize.CoordinatesToCellName(1, rowIndex)
			if err := sw.SetRow(cell, []interface{}{res.RowNumber, res.BuildID, pdf, code128, res.ErrorCode, res.ErrorMessage}); err != nil {
				return fmt.Errorf("failed to write row: %w", err)
			}
			rowIndex++
		}
		return nil
	})
}
