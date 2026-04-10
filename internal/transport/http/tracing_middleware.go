package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

func TracingMiddleware() gin.HandlerFunc {
	tracer := otel.Tracer("bulk-service/http")
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		name := c.Request.Method + " " + c.FullPath()
		ctx, span := tracer.Start(ctx, name, trace.WithAttributes(semconv.HTTPMethodKey.String(c.Request.Method), attribute.String("http.target", c.Request.URL.Path)))
		defer span.End()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
		span.SetAttributes(semconv.HTTPStatusCodeKey.Int(c.Writer.Status()))
		if c.Writer.Status() >= http.StatusInternalServerError {
			span.SetAttributes(attribute.String("error", "true"))
		}
	}
}
