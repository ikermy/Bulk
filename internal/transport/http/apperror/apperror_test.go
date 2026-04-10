package apperror

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/require"
)

func TestWriteError(t *testing.T) {
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    WriteError(c, http.StatusBadRequest, "CODE", "msg", map[string]any{"k": "v"})
    require.Equal(t, http.StatusBadRequest, rw.Code)
    require.Contains(t, rw.Body.String(), "CODE")
    require.Contains(t, rw.Body.String(), "msg")
    require.Contains(t, rw.Body.String(), "k")
}

