//go:build integration
// +build integration

package kafka

import (
	"context"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	kgo "github.com/segmentio/kafka-go"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Minimal integration test that starts a Kafka container (bitnami by default),
// creates a topic, writes and reads a message. Prints container logs when the test fails.
func TestKafka_Container_E2E(t *testing.T) {
	if os.Getenv("RUN_INT_TESTS") != "1" {
		t.Skip("skipping integration test; set RUN_INT_TESTS=1 to run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	// try several possible images (allow override via KAFKA_TEST_IMAGE)
	envImage := os.Getenv("KAFKA_TEST_IMAGE")
	candidates := []string{}
	if envImage != "" {
		candidates = append(candidates, envImage)
	}
	// prefer Bitnami images, then fall back to others
	candidates = append(candidates,
		// Bitnami preferred
		"bitnami/kafka:3.5.1", "bitnami/kafka:3.5.0", "bitnami/kafka:3.4.0", "bitnami/kafka:latest",
		// Confluent images (fallback)
		"confluentinc/cp-kafka:7.4.1", "confluentinc/cp-kafka:7.3.0",
		// Wurstmeister older image (fallback)
		"wurstmeister/kafka:2.13-2.8.0",
	)

	var cont tc.Container
	var startErr error
	var chosen string
	for _, img := range candidates {
		req := tc.GenericContainerRequest{
			ContainerRequest: tc.ContainerRequest{
				Image:        img,
				ExposedPorts: []string{"9092/tcp"},
				Env: map[string]string{
					// bitnami kafka requires this to enable PLAINTEXT listener
					"ALLOW_PLAINTEXT_LISTENER": "yes",
				},
				WaitingFor: wait.ForListeningPort("9092/tcp").WithStartupTimeout(120 * time.Second),
			},
			Started: true,
		}
		cont, startErr = tc.GenericContainer(ctx, req)
		if startErr == nil {
			chosen = img
			break
		}
		t.Logf("image %s failed to start: %v", img, startErr)
	}
	if startErr != nil {
		t.Skipf("no available kafka image could be started from candidates: %v", startErr)
	}
	t.Logf("started kafka container using image %s", chosen)

	// Ensure container is terminated at the end
	defer func() {
		_ = cont.Terminate(context.Background())
	}()

	// If test fails, dump container logs to test output to help debugging
	defer func() {
		if t.Failed() {
			if rc, err := cont.Logs(context.Background()); err == nil {
				b, _ := io.ReadAll(rc)
				t.Logf("--- kafka container logs:\n%s\n--- end kafka logs", string(b))
			} else {
				t.Logf("failed to fetch container logs: %v", err)
			}
		}
	}()

	host, err := cont.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}
	mp, err := cont.MappedPort(ctx, nat.Port("9092/tcp"))
	if err != nil {
		t.Fatalf("failed to get mapped port: %v", err)
	}

	broker := fmt.Sprintf("%s:%s", host, mp.Port())

	// create writer and reader using kafka-go
	topic := "int_test_topic"

	w := kgo.NewWriter(kgo.WriterConfig{Brokers: []string{broker}, Topic: topic})
	defer w.Close()

	msg := kgo.Message{Key: []byte("k"), Value: []byte("hello")}
	writeCtx, writeCancel := context.WithTimeout(ctx, 30*time.Second)
	defer writeCancel()
	if err := w.WriteMessages(writeCtx, msg); err != nil {
		t.Fatalf("write messages failed: %v", err)
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
