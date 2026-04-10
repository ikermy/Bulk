package kafka

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ikermy/Bulk/internal/metrics"
)

type StubProducer struct{}

func NewStubProducer() *StubProducer { return &StubProducer{} }

func (p *StubProducer) Publish(ctx context.Context, topic string, key []byte, value any) error {
	start := time.Now()
	_, err := json.Marshal(value)
	dur := time.Since(start).Seconds()
	if err != nil {
		metrics.KafkaPublishTotal.WithLabelValues(topic, "error").Inc()
	} else {
		metrics.KafkaPublishTotal.WithLabelValues(topic, "success").Inc()
	}
	metrics.KafkaPublishDuration.WithLabelValues(topic).Observe(dur)
	return err
}

func (p *StubProducer) Close() error { return nil }
