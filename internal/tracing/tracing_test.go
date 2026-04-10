package tracing

import (
    "context"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestShutdown_NoProvider(t *testing.T) {
    // ensure tp is nil and Shutdown returns nil
    tp = nil
    require.NoError(t, Shutdown(context.Background()))
}

func TestStartTracing_And_Shutdown(t *testing.T) {
    // StartTracing should initialize a tracer provider and Shutdown should succeed
    // Use a collector URL; the exporter constructor does not perform network calls here.
    require.NoError(t, StartTracing(context.Background(), "test-service", "http://localhost:14268/api/traces"))
    // ensure we cleanup
    require.NoError(t, Shutdown(context.Background()))
    // reset tp
    tp = nil
}


