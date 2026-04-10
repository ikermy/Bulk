package kafka

import (
    "context"
    "errors"
    "sync"
    "testing"

    "github.com/segmentio/kafka-go"
    "github.com/stretchr/testify/require"
)

type fakeWriter struct{
    mu sync.Mutex
    calls int
    failUntil int
}

func (f *fakeWriter) WriteMessages(ctx context.Context, msg kafka.Message) error {
    f.mu.Lock(); defer f.mu.Unlock()
    f.calls++
    if f.calls <= f.failUntil {
        return errors.New("write failed")
    }
    return nil
}
func (f *fakeWriter) Close() error { return nil }

func TestRealProducer_Publish_RetrySuccess(t *testing.T) {
    // replace factory to return fake writer that fails twice then succeeds
    orig := newKafkaWriter
    fw := &fakeWriter{failUntil: 2}
    newKafkaWriter = func(brokers []string, topic string) writerIface { return fw }
    defer func() { newKafkaWriter = orig }()

    p := NewProducer("")
    // ensure retryCount high enough
    p.retryCount = 3
    err := p.Publish(context.Background(), "topic-test", nil, map[string]string{"a":"b"})
    require.NoError(t, err)
    require.GreaterOrEqual(t, fw.calls, 3)
}

func TestRealProducer_Publish_AllFail(t *testing.T) {
    orig := newKafkaWriter
    fw := &fakeWriter{failUntil: 10}
    newKafkaWriter = func(brokers []string, topic string) writerIface { return fw }
    defer func() { newKafkaWriter = orig }()

    p := NewProducer("")
    p.retryCount = 2
    err := p.Publish(context.Background(), "topic-test", nil, map[string]string{"a":"b"})
    require.Error(t, err)
}

