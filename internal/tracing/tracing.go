package tracing

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

var tp *sdktrace.TracerProvider

// StartTracing инициализирует трассировку через OTLP HTTP экспортёр.
// jaegerURL — OTLP HTTP endpoint, например http://jaeger-host:4318
// (ранее использовался Thrift endpoint :14268/api/traces; Jaeger поддерживает OTLP начиная с v1.35).
func StartTracing(ctx context.Context, serviceName string, jaegerURL string) error {
	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(jaegerURL))
	if err != nil {
		return err
	}
	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceNameKey.String(serviceName)))
	if err != nil {
		return err
	}
	tp = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return nil
}

func Shutdown(ctx context.Context) error {
	if tp == nil {
		return nil
	}
	ctxShut, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return tp.Shutdown(ctxShut)
}
