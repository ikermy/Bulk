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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	broker := "localhost:9092"
	topic := "int_test_topic"

	// Create topic explicitly (retry until broker is ready)
	var conn *kgo.Conn
	var connErr error
	for i := 0; i < 15; i++ {
		conn, connErr = kgo.Dial("tcp", broker)
		if connErr == nil {
			break
		}
		t.Logf("dial attempt %d failed: %v", i+1, connErr)
		time.Sleep(time.Second)
	}
	if connErr != nil {
		t.Fatalf("could not dial broker %s: %v", broker, connErr)
	}
	_ = conn.CreateTopics(kgo.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1})
	_ = conn.Close()

	// Writer with retry
	w := kgo.NewWriter(kgo.WriterConfig{Brokers: []string{broker}, Topic: topic})
	defer w.Close()

	msg := kgo.Message{Key: []byte("k"), Value: []byte("hello")}
	var writeErr error
	for i := 0; i < 10; i++ {
		writeErr = w.WriteMessages(context.Background(), msg)
		if writeErr == nil {
			break
		}
		t.Logf("write attempt %d failed: %v, retrying...", i+1, writeErr)
		time.Sleep(time.Second)
	}
	if writeErr != nil {
		t.Fatalf("write messages failed: %v", writeErr)
	}

	r := kgo.NewReader(kgo.ReaderConfig{Brokers: []string{broker}, Topic: topic, GroupID: "int-test-group"})
	defer r.Close()

	readCtx, readCancel := context.WithTimeout(ctx, 30*time.Second)
	defer readCancel()
	m, err := r.ReadMessage(readCtx)
	if err != nil {
		t.Fatalf("read message failed: %v", err)
	}
	if string(m.Value) != "hello" {
		t.Fatalf("unexpected message value: %s", string(m.Value))
	}
}
