//go:build integration
// +build integration

package kafka

import (
	"context"
	"os"
	"testing"
	"time"

	kgo "github.com/segmentio/kafka-go"
)

// This integration test expects Kafka to be reachable at localhost:9092
func TestKafka_PublishConsume(t *testing.T) {
	if os.Getenv("RUN_INT_TESTS") != "1" {
		t.Skip("skipping integration test; set RUN_INT_TESTS=1 to run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	topic := "int_test_topic"
	// create writer
	w := kgo.NewWriter(kgo.WriterConfig{Brokers: []string{"localhost:9092"}, Topic: topic})
	defer w.Close()

	msg := kgo.Message{Key: []byte("k"), Value: []byte("hello")}
	if err := w.WriteMessages(ctx, msg); err != nil {
		t.Fatalf("write messages failed: %v", err)
	}

	r := kgo.NewReader(kgo.ReaderConfig{Brokers: []string{"localhost:9092"}, Topic: topic, GroupID: "int-test-group"})
	defer r.Close()

	m, err := r.ReadMessage(ctx)
	if err != nil {
		t.Fatalf("read message failed: %v", err)
	}
	if string(m.Value) != "hello" {
		t.Fatalf("unexpected message value: %s", string(m.Value))
	}
}
