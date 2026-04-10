package http

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ikermy/Bulk/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsMiddleware observes request counts and durations.
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		duration := time.Since(start).Seconds()
		status := fmt.Sprintf("%d", c.Writer.Status())
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		metrics.HTTPRequestsTotal.WithLabelValues(c.Request.Method, route, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(route).Observe(duration)
	}
}

// RegisterMetrics adds middleware and the /metrics endpoint to the router.
func RegisterMetrics(r *gin.Engine) {
	r.Use(MetricsMiddleware())
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
}
