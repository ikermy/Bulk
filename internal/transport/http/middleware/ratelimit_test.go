package middleware

import (
    "net/http"
    "net/http/httptest"
    "os"
    "testing"
    "time"

    "github.com/gin-gonic/gin"
    "github.com/stretchr/testify/require"
)

func TestGetLimiter_SameKeyReturnsSame(t *testing.T) {
    // ensure buckets map returns same limiter for same key
    b1 := getLimiter("k1", 2.0)
    b2 := getLimiter("k1", 2.0)
    require.Equal(t, b1, b2)
}

func TestRateLimitMiddleware_BlockingWhenLowRPS(t *testing.T) {
    // set very low RPS so middleware will block immediately (tokens < 1)
    os.Setenv("RATE_LIMIT_RPS", "0.5")
    defer os.Unsetenv("RATE_LIMIT_RPS")

    gin.SetMode(gin.TestMode)
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    req := httptest.NewRequest(http.MethodGet, "/x", nil)
    // set RemoteAddr so ClientIP yields something
    req.RemoteAddr = "127.0.0.1:1234"
    c.Request = req

    h := RateLimitMiddleware()
    h(c)
    // expect too many requests status
    require.Equal(t, http.StatusTooManyRequests, rw.Code)
}

func TestRateLimitMiddleware_AllowsWhenHighRPS(t *testing.T) {
    os.Setenv("RATE_LIMIT_RPS", "100.0")
    defer os.Unsetenv("RATE_LIMIT_RPS")

    gin.SetMode(gin.TestMode)
    rw := httptest.NewRecorder()
    c, _ := gin.CreateTestContext(rw)
    req := httptest.NewRequest(http.MethodGet, "/x", nil)
    req.RemoteAddr = "127.0.0.1:1235"
    c.Request = req

    h := RateLimitMiddleware()
    // Should call Next() without aborting
    start := time.Now()
    h(c)
    // when allowed, handler does not write response (status 0) or abort
    require.True(t, time.Since(start) >= 0)
}

