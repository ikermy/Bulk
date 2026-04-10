//go:build integration
// +build integration

package kafka

import (
	"context"
	"fmt"
	"io"
	"net"
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

// apacheKafkaEnv returns the KRaft environment variables for apache/kafka image.
// advertisedPort is the host port that will be mapped to container's 9092.
func apacheKafkaEnv(advertisedPort int) map[string]string {
	return map[string]string{
		"KAFKA_NODE_ID":                          "1",
		"KAFKA_PROCESS_ROLES":                    "broker,controller",
		"KAFKA_LISTENERS":                        "PLAINTEXT://0.0.0.0:9092,CONTROLLER://0.0.0.0:9093",
		"KAFKA_ADVERTISED_LISTENERS":             fmt.Sprintf("PLAINTEXT://localhost:%d", advertisedPort),
		"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP":   "CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT",
		"KAFKA_CONTROLLER_QUORUM_VOTERS":         "1@localhost:9093",
		"KAFKA_CONTROLLER_LISTENER_NAMES":        "CONTROLLER",
		"CLUSTER_ID":                             "MkU3OEVBNTcwNTJENDM2Qk",
		"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR": "1",
	}
}

// freeTCPPort returns an available TCP port on localhost.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// This is a minimal integration test that starts a Kafka container (apache/kafka by default),
// produces and consumes a single message. The test is skipped unless RUN_INT_TESTS=1.
// If the test fails, container logs are collected and emitted via t.Logf for debugging.
func TestKafka_BitnamiMinimal(t *testing.T) {
	if os.Getenv("RUN_INT_TESTS") != "1" {
		t.Skip("skipping integration test; set RUN_INT_TESTS=1 to run")
	}

	ctx := context.Background()

	img := os.Getenv("KAFKA_TEST_IMAGE")
	if img == "" {
		img = "apache/kafka:3.7.0"
	}
	t.Logf("using KAFKA_TEST_IMAGE=%s", img)

	isApache := strings.Contains(img, "apache/kafka")

	// Find a free host port so that the advertised listener matches the mapped port.
	hostPort := freeTCPPort(t)
	t.Logf("binding kafka to host port %d", hostPort)

	var env map[string]string
	var waitFor wait.Strategy
	if isApache {
		env = apacheKafkaEnv(hostPort)
		waitFor = wait.ForLog("Kafka Server started").WithStartupTimeout(120 * time.Second)
	} else {
		env = map[string]string{"ALLOW_PLAINTEXT_LISTENER": "yes"}
		waitFor = wait.ForListeningPort("9092/tcp").WithStartupTimeout(120 * time.Second)
	}

	req := tc.GenericContainerRequest{
		ContainerRequest: tc.ContainerRequest{
			Image:        img,
			ExposedPorts: []string{"9092/tcp"},
			Env:          env,
			HostConfigModifier: func(hc *container.HostConfig) {
				if isApache {
					port, _ := network.ParsePort("9092/tcp")
					hc.PortBindings = network.PortMap{
						port: []network.PortBinding{{HostPort: fmt.Sprintf("%d", hostPort)}},
					}
				}
			},
			WaitingFor: waitFor,
		},
		Started: true,
	}

	cont, err := tc.GenericContainer(ctx, req)
	if err != nil {
		t.Fatalf("failed to start kafka container: %v", err)
	}

	// Ensure we always terminate container and collect logs on failure.
	defer func() {
		if cont != nil {
			if t.Failed() {
				if rc, err := cont.Logs(ctx); err == nil {
					if b, err := io.ReadAll(rc); err == nil {
						t.Logf("kafka-container-logs:\n%s", string(b))
					} else {
						t.Logf("failed to read container logs: %v", err)
					}
				} else {
					t.Logf("failed to fetch container logs: %v", err)
				}
			}
			_ = cont.Terminate(ctx)
		}
	}()

	// Determine broker address.
	var addr string
	if isApache {
		addr = fmt.Sprintf("localhost:%d", hostPort)
	} else {
		host, err := cont.Host(ctx)
		if err != nil {
			t.Fatalf("failed to get container host: %v", err)
		}
		mp, err := cont.MappedPort(ctx, "9092/tcp")
		if err != nil {
			t.Fatalf("failed to get mapped port: %v", err)
		}
		addr = fmt.Sprintf("%s:%s", host, mp.Port())
	}
	t.Logf("kafka broker addr: %s", addr)

	topic := "int_test_topic"

	// Create topic (retry dial until broker is ready)
	var conn *kgo.Conn
	var connErr error
	for i := 0; i < 10; i++ {
		conn, connErr = kgo.Dial("tcp", addr)
		if connErr == nil {
			break
		}
		t.Logf("dial attempt %d failed: %v", i+1, connErr)
		time.Sleep(500 * time.Millisecond)
	}
	if connErr != nil {
		t.Fatalf("could not dial broker: %v", connErr)
	}
	_ = conn.CreateTopics(kgo.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1})
	_ = conn.Close()

	// Writer
	w := kgo.NewWriter(kgo.WriterConfig{Brokers: []string{addr}, Topic: topic})
	defer w.Close()

	writeCtx, writeCancel := context.WithTimeout(ctx, 60*time.Second)
	defer writeCancel()
	writeErr := fmt.Errorf("no attempt made")
	for {
		if writeCtx.Err() != nil {
			break
		}
		writeErr = w.WriteMessages(context.Background(), kgo.Message{Key: []byte("k"), Value: []byte("hello")})
		if writeErr == nil {
			break
		}
		t.Logf("write attempt failed: %v, retrying...", writeErr)
		time.Sleep(1 * time.Second)
	}
	if writeErr != nil {
		t.Fatalf("write failed: %v", writeErr)
	}

	// Reader
	r := kgo.NewReader(kgo.ReaderConfig{Brokers: []string{addr}, Topic: topic, GroupID: "int-test-group"})
	defer r.Close()

	readCtx, readCancel := context.WithTimeout(ctx, 60*time.Second)
	defer readCancel()
	var m kgo.Message
	var readErr error
	for {
		if readCtx.Err() != nil {
			break
		}
		m, readErr = r.ReadMessage(context.Background())
		if readErr == nil {
			break
		}
		t.Logf("read attempt failed: %v, retrying...", readErr)
		time.Sleep(500 * time.Millisecond)
	}
	if readErr != nil {
		t.Fatalf("read failed: %v", readErr)
	}

	if string(m.Value) != "hello" {
		t.Fatalf("unexpected message value: %s", string(m.Value))
	}
}
