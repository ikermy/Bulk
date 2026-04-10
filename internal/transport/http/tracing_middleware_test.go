package http

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTracingMiddleware_Basic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(TracingMiddleware())
	r.GET("/ping", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ping", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 got %d", w.Code)
	}
}
