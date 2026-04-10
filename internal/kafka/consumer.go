package kafka

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/ikermy/Bulk/internal/logging"
	"github.com/ikermy/Bulk/internal/metrics"
	"github.com/ikermy/Bulk/internal/ports"
)

type ResultConsumer struct {
	reader     readerIface
	topic      string
	handler    func(ctx context.Context, ev BulkResultEvent) error
	producer   ports.KafkaProducer
	dlqTopic   string
	retryCount int
	logger     logging.Logger
}

// ResultConsumer — потребляет сообщения из топика bulk.result (результаты обработки Generation),
// реализует retry и публикацию в DLQ при ошибках

func NewConsumer(brokers string, topic string, groupID string, handler func(ctx context.Context, ev BulkResultEvent) error, producer ports.KafkaProducer, dlqTopic string, retryCount int, logger logging.Logger) *ResultConsumer {
	brs := []string{brokers}
	r := newKafkaReader(brs, topic, groupID)
	return &ResultConsumer{reader: r, topic: topic, handler: handler, producer: producer, dlqTopic: dlqTopic, retryCount: retryCount, logger: logger}
}

// header carrier used to extract trace context
type headerCarrierReader struct {
	headers []kafka.Header
}

func (c *headerCarrierReader) Get(key string) string {
	for _, h := range c.headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c *headerCarrierReader) Keys() []string {
	var ks []string
	for _, h := range c.headers {
		ks = append(ks, h.Key)
	}
	return ks
}

func (c *headerCarrierReader) Set(key, value string) {
	// Implemented to satisfy propagation.TextMapCarrier; not typically used when extracting
	c.headers = append(c.headers, kafka.Header{Key: key, Value: []byte(value)})
}

// readerIface abstracts kafka.Reader for testing
type readerIface interface {
	ReadMessage(ctx context.Context) (kafka.Message, error)
	Close() error
}

type readerAdapter struct{
	r *kafka.Reader
}

func (a *readerAdapter) ReadMessage(ctx context.Context) (kafka.Message, error) {
	return a.r.ReadMessage(ctx)
}

func (a *readerAdapter) Close() error { return a.r.Close() }

// newKafkaReader factory creates readerIface; tests can replace it
var newKafkaReader = func(brokers []string, topic string, groupID string) readerIface {
	return &readerAdapter{r: kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, Topic: topic, GroupID: groupID})}
}

func (c *ResultConsumer) Start(ctx context.Context) error {
	tracer := otel.Tracer("bulk-service/kafka")
	for {
		m, err := c.reader.ReadMessage(ctx)
		if err != nil {
			return err
		}
		// extract context
		carrier := headerCarrierReader{headers: m.Headers}
		ctxMsg := propagation.TraceContext{}.Extract(ctx, &carrier)
		ctxSpan, span := tracer.Start(ctxMsg, "kafka.Consumer.ProcessMessage", trace.WithAttributes(attribute.String("kafka.topic", c.topic)))
		start := time.Now()
		// no structured wrapper here; use logger directly
		// parse message into BulkResultEvent
		var ev BulkResultEvent
		if err := json.Unmarshal(m.Value, &ev); err != nil {
			metrics.KafkaConsumeTotal.WithLabelValues(c.topic, "error").Inc()
			span.SetAttributes(attribute.String("kafka.result", "invalid_payload"))
			span.RecordError(err)
			span.End()
			continue
		}

		// call handler if provided with retry and DLQ
		if c.handler != nil {
			var attempt int
			var hErr error
			for attempt = 0; attempt <= c.retryCount; attempt++ {
				hErr = c.handler(ctxMsg, ev)
				if hErr == nil {
					break
				}
				// backoff: exponential
				time.Sleep(time.Duration(100*(1<<attempt)) * time.Millisecond)
			}
			if hErr != nil {
				metrics.KafkaConsumeTotal.WithLabelValues(c.topic, "error").Inc()
				span.SetAttributes(attribute.String("kafka.result", "handler_error"))
				span.RecordError(hErr)
				if c.logger != nil {
					// try extract ids from event
					c.logger.Error("consumer_handler_error", "batchId", ev.BatchID, "jobId", ev.JobID, "error", hErr)
				}
				// publish to DLQ if configured
				if c.producer != nil && c.dlqTopic != "" {
					if perr := c.producer.Publish(ctxMsg, c.dlqTopic, nil, ev); perr != nil {
						if c.logger != nil {
							c.logger.Error("consumer_dlq_publish_failed", "batchId", ev.BatchID, "jobId", ev.JobID, "error", perr)
						}
					} else {
						if c.logger != nil {
							c.logger.Info("consumer_dlq_published", "batchId", ev.BatchID, "jobId", ev.JobID)
						}
					}
				}
			} else {
				metrics.KafkaConsumeTotal.WithLabelValues(c.topic, "success").Inc()
				metrics.KafkaConsumeDuration.WithLabelValues(c.topic).Observe(time.Since(start).Seconds())
				span.SetAttributes(attribute.String("kafka.result", "processed"))
				if c.logger != nil {
					c.logger.Info("consumer_processed", "batchId", ev.BatchID, "jobId", ev.JobID, "status", ev.Status)
				}
			}
		} else {
			// no handler: just mark as processed
			metrics.KafkaConsumeTotal.WithLabelValues(c.topic, "success").Inc()
			metrics.KafkaConsumeDuration.WithLabelValues(c.topic).Observe(time.Since(start).Seconds())
			span.SetAttributes(attribute.String("kafka.result", "processed_no_handler"))
		}

		span.End()
		_ = ctxSpan
	}
}

func (c *ResultConsumer) Close() error {
	return c.reader.Close()
}
