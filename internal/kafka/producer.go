package kafka

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/ikermy/Bulk/internal/metrics"
)

type Producer interface {
	Publish(ctx context.Context, topic string, key []byte, value any) error
	Close() error
}

type RealProducer struct {
	writers    map[string]writerIface
	brokers    []string
	retryCount int
}

// RealProducer — Kafka publisher: публикует сообщения в топики (например, "bulk.job") и поддерживает retry/DLQ/OTel

func NewProducer(brokers string) *RealProducer {
	b := strings.Split(brokers, ",")
	return &RealProducer{writers: make(map[string]writerIface), brokers: b, retryCount: 3}
}

// writerIface abstracts the minimal writer used by RealProducer so tests can inject fakes.
type writerIface interface {
	WriteMessages(ctx context.Context, msg kafka.Message) error
	Close() error
}

type writerAdapter struct{
	w *kafka.Writer
}

func (a *writerAdapter) WriteMessages(ctx context.Context, msg kafka.Message) error {
	return a.w.WriteMessages(ctx, msg)
}

func (a *writerAdapter) Close() error { return a.w.Close() }

// newKafkaWriter is a factory used to create writerIface. Tests can replace it.
var newKafkaWriter = func(brokers []string, topic string) writerIface {
	// Use explicit Writer construction to avoid deprecated NewWriter/WriterConfig usage.
	w := &kafka.Writer{
		Addr:                     kafka.TCP(brokers...),
		Topic:                    topic,
		AllowAutoTopicCreation:   true,
	}
	return &writerAdapter{w: w}
}

func (p *RealProducer) writerFor(topic string) writerIface {
	if w, ok := p.writers[topic]; ok {
		return w
	}
	w := newKafkaWriter(p.brokers, topic)
	p.writers[topic] = w
	return w
}

// carrier adapts kafka message headers for OpenTelemetry propagation
type headerCarrier struct {
	headers *[]kafka.Header
}

func (c headerCarrier) Get(key string) string {
	for _, h := range *c.headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c headerCarrier) Set(key, value string) {
	*c.headers = append(*c.headers, kafka.Header{Key: key, Value: []byte(value)})
}

func (c headerCarrier) Keys() []string {
	var ks []string
	for _, h := range *c.headers {
		ks = append(ks, h.Key)
	}
	return ks
}

func (p *RealProducer) Publish(ctx context.Context, topic string, key []byte, value any) error {
	tracer := otel.Tracer("bulk-service/kafka")
	ctx, span := tracer.Start(ctx, "kafka.Publish", trace.WithAttributes(attribute.String("kafka.topic", topic), attribute.Bool("kafka.key_present", len(key) > 0)))
	defer span.End()

	start := time.Now()
	b, _ := json.Marshal(value)
	dur := time.Since(start).Seconds()

	var headers []kafka.Header
	carrier := headerCarrier{headers: &headers}
	propagation.TraceContext{}.Inject(ctx, carrier)

	msg := kafka.Message{Key: key, Value: b, Headers: headers}
	w := p.writerFor(topic)
	var lastErr error
	for attempt := 0; attempt <= p.retryCount; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(100*(1<<uint(attempt-1))) * time.Millisecond)
		}
		if err := w.WriteMessages(ctx, msg); err != nil {
			lastErr = err
			metrics.KafkaPublishTotal.WithLabelValues(topic, "error").Inc()
			metrics.KafkaPublishDuration.WithLabelValues(topic).Observe(dur)
			span.SetAttributes(attribute.String("kafka.result", "error"))
			continue
		}
		// success
		lastErr = nil
		metrics.KafkaPublishTotal.WithLabelValues(topic, "success").Inc()
		metrics.KafkaPublishDuration.WithLabelValues(topic).Observe(dur)
		span.SetAttributes(attribute.String("kafka.result", "success"))
		break
	}
	return lastErr
}

func (p *RealProducer) Close() error {
	for _, w := range p.writers {
		_ = w.Close()
	}
	return nil
}
