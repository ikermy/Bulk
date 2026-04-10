package metrics

import (
    "testing"

    dto "github.com/prometheus/client_model/go"
    "github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsCounters(t *testing.T) {
    // inc some metrics and assert values
    KafkaPublishTotal.WithLabelValues("topic", "success").Inc()
    UploadsRejectedTotal.WithLabelValues("too_large").Inc()

    if testutil.ToFloat64(KafkaPublishTotal.WithLabelValues("topic", "success")) < 1.0 {
        t.Fatalf("expected kafka publish total >=1")
    }
    if testutil.ToFloat64(UploadsRejectedTotal.WithLabelValues("too_large")) < 1.0 {
        t.Fatalf("expected uploads rejected >=1")
    }

    // observe histogram (no direct ToFloat64 check for histograms)
    HTTPRequestDuration.WithLabelValues("/test").Observe(0.05)

    // gauge set
    BatchRowsTotal.WithLabelValues("b1").Set(5)
    g := &dto.Metric{}
    if err := BatchRowsTotal.WithLabelValues("b1").Write(g); err != nil {
        t.Fatalf("write gauge failed: %v", err)
    }
}



