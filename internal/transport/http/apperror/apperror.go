package apperror

import (
    "github.com/gin-gonic/gin"
)

// WriteError writes a standardized error response matching OpenAPI ErrorResponse
// Example body: {"code":"INVALID_FILE_FORMAT","message":"...","details":{...}}
func WriteError(c *gin.Context, httpStatus int, code string, message string, details map[string]any) {
    body := gin.H{"code": code, "message": message}
    if details != nil {
        body["details"] = details
    }
    c.JSON(httpStatus, body)
}

