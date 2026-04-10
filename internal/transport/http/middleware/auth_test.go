package middleware

import (
    "net/http"
    "net/http/httptest"
    "os"
    "testing"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/require"
)

func TestAuthMiddleware_HealthAllowed(t *testing.T) {
    rw := httptest.NewRecorder()
    _, r := gin.CreateTestContext(rw)
    // attach middleware and a handler that writes 200
    r.Use(AuthMiddleware())
    r.GET("/health", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
    req := httptest.NewRequest(http.MethodGet, "/health", nil)
    r.ServeHTTP(rw, req)
    require.Equal(t, http.StatusOK, rw.Code)
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
    rw := httptest.NewRecorder()
    _, r := gin.CreateTestContext(rw)
    r.Use(AuthMiddleware())
    r.GET("/api/v1/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
    req := httptest.NewRequest(http.MethodGet, "/api/v1/x", nil)
    r.ServeHTTP(rw, req)
    require.Equal(t, http.StatusUnauthorized, rw.Code)
}

func TestAuthMiddleware_LegacyAdminToken(t *testing.T) {
    // enable legacy admin token via env
    os.Setenv("ADMIN_JWT", "admintoken")
    defer os.Unsetenv("ADMIN_JWT")

    rw := httptest.NewRecorder()
    _, r := gin.CreateTestContext(rw)
    r.Use(AuthMiddleware())
    r.GET("/api/v1/admin/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
    req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/x", nil)
    req.Header.Set("Authorization", "Bearer admintoken")
    r.ServeHTTP(rw, req)
    require.Equal(t, http.StatusOK, rw.Code)
}


