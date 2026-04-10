package handlers

import (
    "net/http"

    "github.com/gin-gonic/gin"
    apperr "github.com/ikermy/Bulk/internal/transport/http/apperror"
    "github.com/xuri/excelize/v2"
)

// HandleDownloadTemplateXLS генерирует XLSX-шаблон для указанной ревизии
// в соответствии с ТЗ §5.4. Возвращает .xlsx с заголовками и примерной строкой.
func HandleDownloadTemplateXLS(c *gin.Context) {
    revision := c.Param("revision")
    _ = revision // пока не используется, зарезервировано для шаблонов по ревизии

    f := excelize.NewFile()
    defer func() { _ = f.Close() }()

    sheet := f.GetSheetName(0)
    headers := []string{"DAC", "DCS", "DAG", "DAI", "DAJ", "DAK", "DBB", "DBA", "DBD"}
    // записываем заголовки в первую строку
    for i, h := range headers {
        cell, _ := excelize.CoordinatesToCellName(i+1, 1)
        if err := f.SetCellValue(sheet, cell, h); err != nil {
            apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to build template", nil)
            return
        }
    }
    // примерная строка согласно ТЗ
    sample := []interface{}{"JOHN", "DOE", "123 MAIN ST", "LOS ANGELES", "CA", "90001", "19900115", "20280115", "20240115"}
    for i, v := range sample {
        cell, _ := excelize.CoordinatesToCellName(i+1, 2)
        if err := f.SetCellValue(sheet, cell, v); err != nil {
            apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to build template", nil)
            return
        }
    }

    c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
    c.Header("Content-Disposition", "attachment; filename=template_"+revision+".xlsx")
    c.Status(http.StatusOK)
    if err := f.Write(c.Writer); err != nil {
        apperr.WriteError(c, http.StatusInternalServerError, "SERVICE_ERROR", "failed to write template", nil)
    }
}
