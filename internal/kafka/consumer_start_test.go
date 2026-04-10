package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/require"
)

type fakeReader struct {
	msgs []kafka.Message
}

func (f *fakeReader) ReadMessage(ctx context.Context) (kafka.Message, error) {
	if len(f.msgs) == 0 {
		return kafka.Message{}, io.EOF
	}
	m := f.msgs[0]
	f.msgs = f.msgs[1:]
	return m, nil
}

func (f *fakeReader) Close() error { return nil }

type spyProducer struct {
	called bool
	topic  string
}

func (s *spyProducer) Publish(ctx context.Context, topic string, key []byte, msg any) error {
	s.called = true
	s.topic = topic
	return nil
}
func (s *spyProducer) Close() error { return nil }

func TestResultConsumer_Start_InvalidPayload(t *testing.T) {
	// fake reader returns invalid JSON then EOF
	fr := &fakeReader{msgs: []kafka.Message{{Value: []byte("not-json")}}}
	orig := newKafkaReader
	newKafkaReader = func(brokers []string, topic string, groupID string) readerIface { return fr }
	defer func() { newKafkaReader = orig }()

	rc := NewConsumer("", "topic", "group", nil, nil, "dlq", 1, nil)
	err := rc.Start(context.Background())
	require.Error(t, err)
	// since fake reader returns EOF after one message, Start should return io.EOF
	require.True(t, errors.Is(err, io.EOF))
}

func TestResultConsumer_Start_HandlerError_PublishesDLQ(t *testing.T) {
	// prepare event value
	ev := BulkResultEvent{EventType: "e", JobID: "j1", BatchID: "b1", Status: "failed"}
	b, _ := json.Marshal(ev)
	fr := &fakeReader{msgs: []kafka.Message{{Value: b}}}
	orig := newKafkaReader
	newKafkaReader = func(brokers []string, topic string, groupID string) readerIface { return fr }
	defer func() { newKafkaReader = orig }()

	spy := &spyProducer{}
	// handler always returns error to trigger DLQ publish
	handler := func(ctx context.Context, ev BulkResultEvent) error { return errors.New("handler error") }
	rc := NewConsumer("", "topic", "group", handler, spy, "dlq-topic", 0, nil)
	err := rc.Start(context.Background())
	// Start will return io.EOF eventually
	require.Error(t, err)
	// spy should have been called to publish to DLQ
	require.True(t, spy.called)
	require.Equal(t, "dlq-topic", spy.topic)
}
