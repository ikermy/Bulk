package logging

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func TestTraceIDAndFieldsFromCtx(t *testing.T) {
	// nil context
	require.Equal(t, "", TraceIDFromCtx(nil))
	require.Equal(t, map[string]any{"traceId": ""}, FieldsFromCtx(nil))

	// create a valid SpanContext with a non-zero trace id
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID([16]byte{1, 2, 3}), SpanID: trace.SpanID([8]byte{1,2,3,4,5,6,7,8}), TraceFlags: trace.FlagsSampled})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	tid := TraceIDFromCtx(ctx)
	require.NotEqual(t, "", tid)
	f := FieldsFromCtx(ctx)
	require.Equal(t, tid, f["traceId"])
}

func TestFromContextAndNewLogger(t *testing.T) {
	base := zap.NewNop().Sugar()
	// without trace should return same base
	got := FromContext(context.Background(), base)
	require.Equal(t, base, got)

	// with trace should return a logger (non-nil) that can be used
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID([16]byte{4, 5, 6}), SpanID: trace.SpanID([8]byte{8,7,6,5,4,3,2,1}), TraceFlags: trace.FlagsSampled})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	got2 := FromContext(ctx, base)
	require.NotNil(t, got2)

	// NewLogger should honor environment overrides and not panic
	os.Setenv("SERVICE_NAME", "sname")
	os.Setenv("SERVICE_VERSION", "sv")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("LOG_FORMAT", "console")
	defer func() {
		os.Unsetenv("SERVICE_NAME")
		os.Unsetenv("SERVICE_VERSION")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("LOG_FORMAT")
	}()
	l := NewLogger(nil)
	require.NotNil(t, l)
}


