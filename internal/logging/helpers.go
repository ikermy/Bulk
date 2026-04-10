package logging

import (
    "context"
    "fmt"

    "go.opentelemetry.io/otel/trace"
)

// TraceIDFromCtx извлекает trace id из контекста (если доступен), иначе пустая строка
func TraceIDFromCtx(ctx context.Context) string {
    if ctx == nil {
        return ""
    }
    sc := trace.SpanContextFromContext(ctx)
    if sc.IsValid() {
        return fmt.Sprintf("%v", sc.TraceID())
    }
    return ""
}

// FieldsFromCtx возвращает базовые поля для логирования, содержащие traceId если есть
func FieldsFromCtx(ctx context.Context) map[string]any {
    return map[string]any{"traceId": TraceIDFromCtx(ctx)}
}

// FromContext returns a SugaredLogger prepopulated with traceId field from context.
// If base is nil, NewLogger(nil) is used as a fallback. This mirrors the pattern
// from TZ §13.3: enrich logger with request-scoped fields before logging events.
func FromContext(ctx context.Context, base Logger) Logger {
    if base == nil {
        base = NewLogger(nil)
    }
    tid := TraceIDFromCtx(ctx)
    if tid == "" {
        return base
    }
    return base.With("traceId", tid)
}

