//go:build integration
// +build integration

package kafka

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	container "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	kgo "github.com/segmentio/kafka-go"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Minimal integration test that starts a Kafka container (apache/kafka by default),
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
	// prefer apache/kafka (official), then confluent fallback
	candidates = append(candidates,
		"apache/kafka:3.7.0",
		"apache/kafka:3.6.2",
		// Confluent images (fallback, requires zookeeper — handled below)
		"confluentinc/cp-kafka:7.4.1",
	)

	var cont tc.Container
	var startErr error
	var chosen string
	var chosenHostPort int

	for _, img := range candidates {
		isApache := strings.Contains(img, "apache/kafka")
		hp := freeTCPPort(t)

		var env map[string]string
		var hostMod func(*container.HostConfig)
		var waitFor wait.Strategy

		if isApache {
			env = apacheKafkaEnv(hp)
			hostMod = func(hc *container.HostConfig) {
				port, _ := network.ParsePort("9092/tcp")
				hc.PortBindings = network.PortMap{
					port: []network.PortBinding{{HostPort: fmt.Sprintf("%d", hp)}},
				}
			}
			waitFor = wait.ForLog("Kafka Server started").WithStartupTimeout(120 * time.Second)
		} else {
			env = map[string]string{
				"ALLOW_PLAINTEXT_LISTENER": "yes",
			}
			hostMod = func(hc *container.HostConfig) {}
			waitFor = wait.ForListeningPort("9092/tcp").WithStartupTimeout(120 * time.Second)
		}

		req := tc.GenericContainerRequest{
			ContainerRequest: tc.ContainerRequest{
				Image:              img,
				ExposedPorts:       []string{"9092/tcp"},
				Env:                env,
				HostConfigModifier: hostMod,
				WaitingFor:         waitFor,
			},
			Started: true,
		}
		cont, startErr = tc.GenericContainer(ctx, req)
		if startErr == nil {
			chosen = img
			chosenHostPort = hp
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

	// Determine broker address
	var broker string
	if strings.Contains(chosen, "apache/kafka") {
		broker = fmt.Sprintf("localhost:%d", chosenHostPort)
	} else {
		host, err := cont.Host(ctx)
		if err != nil {
			t.Fatalf("failed to get container host: %v", err)
		}
		mp, err := cont.MappedPort(ctx, "9092/tcp")
		if err != nil {
			t.Fatalf("failed to get mapped port: %v", err)
		}
		broker = fmt.Sprintf("%s:%s", host, mp.Port())
	}
	t.Logf("kafka broker: %s", broker)

	// create writer and reader using kafka-go
	topic := "int_test_topic"

	// Create topic explicitly with retries (auto-create can lag on first request)
	var conn *kgo.Conn
	var dialErr error
	for i := 0; i < 10; i++ {
		conn, dialErr = kgo.Dial("tcp", broker)
		if dialErr == nil {
			break
		}
		t.Logf("dial attempt %d failed: %v", i+1, dialErr)
		time.Sleep(500 * time.Millisecond)
	}
	if dialErr != nil {
		t.Fatalf("could not dial broker: %v", dialErr)
	}
	_ = conn.CreateTopics(kgo.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1})
	_ = conn.Close()

	w := kgo.NewWriter(kgo.WriterConfig{Brokers: []string{broker}, Topic: topic})
	defer w.Close()

	msg := kgo.Message{Key: []byte("k"), Value: []byte("hello")}
	writeCtx, writeCancel := context.WithTimeout(ctx, 30*time.Second)
	defer writeCancel()
	var writeErr error
	for i := 0; i < 5; i++ {
		writeErr = w.WriteMessages(writeCtx, msg)
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
