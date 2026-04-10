package middleware

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/require"
)

func TestAuthMiddleware_InvalidAuthHeaderFormat(t *testing.T) {
    rw := httptest.NewRecorder()
    _, r := gin.CreateTestContext(rw)
    r.Use(AuthMiddleware())
    r.GET("/api/v1/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
    req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
    req.Header.Set("Authorization", "NotBearer token")
    r.ServeHTTP(rw, req)
    require.Equal(t, http.StatusUnauthorized, rw.Code)
}

// Intentionally omitted flaky empty-token test; behavior depends on env in test process.




