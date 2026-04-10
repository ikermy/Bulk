package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ikermy/Bulk/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func TestQuote_IncrementsMetrics(t *testing.T) {
	// mock billing server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		w.WriteHeader(200)
		w.Write([]byte(`{"canGenerate":true,"allowedTotal":100}`))
	}))
	defer srv.Close()

	c := NewBFFBillingClient(srv.URL, 2*time.Second, "")
	_, err := c.Quote(context.Background(), "user1", 10)
	if err != nil {
		t.Fatalf("Quote failed: %v", err)
	}

	// check counter increment for success
	if got := testutil.ToFloat64(metrics.BillingCallsTotal.WithLabelValues("Quote", "success")); got < 1 {
		t.Fatalf("expected BillingCallsTotal Quote success >=1 got %v", got)
	}
	// check histogram sample_count > 0 for method=Quote
	mf, err := gatherMetric("bulk_service_billing_call_duration_seconds")
	if err != nil {
		t.Fatalf("gather billing call duration: %v", err)
	}
	var found bool
	for _, m := range mf.Metric {
		for _, lp := range m.Label {
			if lp.GetName() == "method" && lp.GetValue() == "Quote" {
				if m.GetHistogram().GetSampleCount() == 0 {
					t.Fatalf("expected histogram sample_count > 0 for Quote")
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("billing duration metric for Quote not found")
	}
}

func TestBlockBatch_IncrementsMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"transactionIds":["t1","t2"]}`))
	}))
	defer srv.Close()

	c := NewBFFBillingClient(srv.URL, 2*time.Second, "")
	_, err := c.BlockBatch(context.Background(), "user1", 2, "batch1")
	if err != nil {
		t.Fatalf("BlockBatch failed: %v", err)
	}
	if got := testutil.ToFloat64(metrics.BillingCallsTotal.WithLabelValues("BlockBatch", "success")); got < 1 {
		t.Fatalf("expected BillingCallsTotal BlockBatch success >=1 got %v", got)
	}
	mf, err := gatherMetric("bulk_service_billing_call_duration_seconds")
	if err != nil {
		t.Fatalf("gather billing call duration: %v", err)
	}
	var found bool
	found = false
	for _, m := range mf.Metric {
		for _, lp := range m.Label {
			if lp.GetName() == "method" && lp.GetValue() == "BlockBatch" {
				if m.GetHistogram().GetSampleCount() == 0 {
					t.Fatalf("expected histogram sample_count > 0 for BlockBatch")
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("billing duration metric for BlockBatch not found")
	}
}

func gatherMetric(name string) (*dto.MetricFamily, error) {
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil, err
	}
	for _, mf := range mfs {
		if mf.GetName() == name {
			return mf, nil
		}
	}
	return nil, nil
}

func TestQuote_SendsContextAndHeaders(t *testing.T) {
	// enable standard traceparent injection for the test
	otel.SetTextMapPropagator(propagation.TraceContext{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		// context.source must be present
		ctxObj, ok := body["context"].(map[string]any)
		if !ok {
			t.Fatalf("expected context object in request")
		}
		if src, ok := ctxObj["source"].(string); !ok || src != "bulk" {
			t.Fatalf("expected context.source='bulk', got %v", ctxObj["source"])
		}
		// Authorization header must be set when token provided
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("expected Authorization header, got %s", r.Header.Get("Authorization"))
		}
		// Note: traceparent injection depends on having a valid SpanContext / tracer
		// in unit tests this may be a no-op. We check Authorization and body above.

		w.WriteHeader(200)
		w.Write([]byte(`{"canProcess":true,"allowedTotal":100}`))
	}))
	defer srv.Close()

	c := NewBFFBillingClient(srv.URL, 2*time.Second, "secret")
	// create a span so propagator injects traceparent
	ctx, span := otel.Tracer("test/billing").Start(context.Background(), "test-span")
	defer span.End()
	_, err := c.Quote(ctx, "user1", 10)
	if err != nil {
		t.Fatalf("Quote failed: %v", err)
	}
}

