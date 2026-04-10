//go:build integration
// +build integration

package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	container "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/gin-gonic/gin"
	"github.com/ikermy/Bulk/internal/db"
	di "github.com/ikermy/Bulk/internal/di"
	"github.com/ikermy/Bulk/internal/repo"
	"github.com/ikermy/Bulk/internal/storage"
	handlers "github.com/ikermy/Bulk/internal/transport/http/handlers"
	svc "github.com/ikermy/Bulk/internal/usecase/bulk"

	kgo "github.com/segmentio/kafka-go"
)

// TestE2E_FullFlow performs upload -> validate -> confirm -> optional kafka roundtrip
func TestE2E_FullFlow(t *testing.T) {
	if os.Getenv("RUN_INT_TESTS") != "1" {
		t.Skip("skipping integration tests; set RUN_INT_TESTS=1 to run")
	}
	ctx := context.Background()

	// Start Postgres container
	pgReq := tc.ContainerRequest{
		Image:        "postgres:15",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(60 * time.Second),
	}
	pgCont, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{ContainerRequest: pgReq, Started: true})
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	defer pgCont.Terminate(ctx)

	pgHost, _ := pgCont.Host(ctx)
	pgPort, _ := pgCont.MappedPort(ctx, "5432/tcp")
	dsn := "postgres://test:test@" + pgHost + ":" + pgPort.Port() + "/testdb?sslmode=disable"

	// wait a bit for DB readiness
	time.Sleep(2 * time.Second)

	dbConn, err := db.Connect(dsn)
	if err != nil {
		// retry a few times
		var last error
		for i := 0; i < 10; i++ {
			time.Sleep(500 * time.Millisecond)
			dbConn, last = db.Connect(dsn)
			if last == nil {
				err = nil
				break
			}
		}
		if err != nil && dbConn == nil {
			t.Fatalf("db connect failed: %v", err)
		}
	}
	defer dbConn.Close()

	// apply migrations
	migRel := filepath.Join("migrations", "001_init.sql")
	cwd, _ := os.Getwd()
	var migBytes []byte
	var found bool
	dir := cwd
	for i := 0; i < 8; i++ {
		try := filepath.Join(dir, migRel)
		if _, err := os.Stat(try); err == nil {
			migBytes, err = os.ReadFile(try)
			if err != nil {
				t.Fatalf("read migration: %v", err)
			}
			found = true
			break
		}
		dir = filepath.Dir(dir)
	}
	if !found {
		t.Fatalf("migration file not found: %s", migRel)
	}
	if _, err := dbConn.Exec(string(migBytes)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	// Start MinIO container
	minioReq := tc.ContainerRequest{
		Image:        "quay.io/minio/minio:latest",
		ExposedPorts: []string{"9000/tcp", "9001/tcp"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minioadmin",
			"MINIO_ROOT_PASSWORD": "minioadmin",
		},
		Cmd:        []string{"server", "/data", "--console-address", ":9001"},
		WaitingFor: wait.ForListeningPort("9000/tcp").WithStartupTimeout(60 * time.Second),
	}
	minioCont, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{ContainerRequest: minioReq, Started: true})
	if err != nil {
		t.Fatalf("failed to start minio container: %v", err)
	}
	defer minioCont.Terminate(ctx)

	minioHost, _ := minioCont.Host(ctx)
	minioPort, _ := minioCont.MappedPort(ctx, "9000/tcp")
	endpoint := minioHost + ":" + minioPort.Port()

	// set env for NewFileClientFromEnv to pick up
	os.Setenv("STORAGE_ENDPOINT", endpoint)
	os.Setenv("STORAGE_ACCESS_KEY", "minioadmin")
	os.Setenv("STORAGE_SECRET_KEY", "minioadmin")
	os.Setenv("STORAGE_BUCKET", "bulk-e2e")
	os.Setenv("STORAGE_USE_SSL", "false")
	os.Setenv("STORAGE_BASE_URL", "http://"+endpoint)

	// create storage client
	sc, err := storage.NewFileClientFromEnv()
	if err != nil {
		t.Fatalf("storage init failed: %v", err)
	}

	// prepare repos and service
	batchRepo := repo.NewBatchRepository(dbConn)
	jobRepo := repo.NewJobRepository(dbConn)
	service := svc.NewService(batchRepo, jobRepo, nil, nil, nil, sc)

	// sanity check: call service directly with CSV content to ensure parsing works
	if _, err := service.CreateBatchFromFile(context.Background(), strings.NewReader("first_name,last_name\nval1,val2\n"), "rev-e2e"); err != nil {
		t.Fatalf("direct service parse failed: %v", err)
	}

	// create real HTTP server with gin and upload handler
	router := gin.New()
	deps := &di.Deps{Service: service, BatchRepo: batchRepo, JobRepo: jobRepo}
	router.POST("/api/v1/upload", func(c *gin.Context) { handlers.HandleUpload(c, deps) })
	srv := httptest.NewServer(router)
	defer srv.Close()

	// perform upload via multipart
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		fw, _ := mw.CreateFormFile("file", "data.csv")
		io.Copy(fw, strings.NewReader("first_name,last_name\nval1,val2\n"))
		mw.WriteField("revision", "rev-e2e")
		mw.Close()
		pw.Close()
	}()

	reqHTTP, err := http.NewRequest("POST", srv.URL+"/api/v1/upload", pr)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	reqHTTP.Header.Set("Content-Type", mw.FormDataContentType())
	respHTTP, err := srv.Client().Do(reqHTTP)
	if err != nil {
		t.Fatalf("http post failed: %v", err)
	}
	defer respHTTP.Body.Close()

	var resp map[string]any
	if err := json.NewDecoder(respHTTP.Body).Decode(&resp); err != nil {
		t.Fatalf("decode handler response: %v", err)
	}
	bid, _ := resp["batchId"].(string)
	if bid == "" {
		t.Fatalf("handler returned empty batchId: %v", resp)
	}

	// verify jobs were created
	jobs, err := jobRepo.GetByBatch(context.Background(), bid)
	if err != nil {
		t.Fatalf("get jobs failed: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatalf("expected jobs to be created, got 0")
	}

	// Prepare confirm: choose whether to use real Kafka or stub
	useKafka := os.Getenv("RUN_KAFKA_INT") == "1"
	var kafkaBroker string
	var writer *kgo.Writer
	var reader *kgo.Reader
	received := make(chan kgo.Message, 100)
	if useKafka {
		// Decide which kafka image to use. Default to apache/kafka unless overridden.
		kafkaImg := os.Getenv("KAFKA_TEST_IMAGE")
		if kafkaImg == "" {
			kafkaImg = "apache/kafka:3.7.0"
		}
		t.Logf("selected KAFKA_TEST_IMAGE=%s", kafkaImg)

		// If the chosen image is Confluent (or cp- prefix), run zookeeper + confluent kafka as before.
		lower := strings.ToLower(kafkaImg)
		if strings.Contains(lower, "confluent") || strings.Contains(lower, "cp-") {
			// start zookeeper
			zkReq := tc.ContainerRequest{
				Image:        "confluentinc/cp-zookeeper:7.4.1",
				ExposedPorts: []string{"2181/tcp"},
				Env:          map[string]string{"ZOOKEEPER_CLIENT_PORT": "2181", "ZOOKEEPER_TICK_TIME": "2000"},
				WaitingFor:   wait.ForListeningPort("2181/tcp").WithStartupTimeout(60 * time.Second),
			}
			zkCont, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{ContainerRequest: zkReq, Started: true})
			if err != nil {
				t.Logf("failed to start zookeeper, skipping kafka roundtrip: %v", err)
				useKafka = false
			} else {
				defer zkCont.Terminate(ctx)
				zkHost, _ := zkCont.Host(ctx)
				zkPort, _ := zkCont.MappedPort(ctx, "2181/tcp")
				zkAddr := zkHost + ":" + zkPort.Port()

				// start confluent kafka pointing to zookeeper
				kafkaReq := tc.ContainerRequest{
					Image:        kafkaImg,
					ExposedPorts: []string{"9092/tcp"},
					Env: map[string]string{
						"KAFKA_ZOOKEEPER_CONNECT":                zkAddr,
						"KAFKA_LISTENERS":                        "PLAINTEXT://0.0.0.0:9092",
						"KAFKA_ADVERTISED_LISTENERS":             "PLAINTEXT://127.0.0.1:9092",
						"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR": "1",
						"KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS": "0",
					},
					WaitingFor: wait.ForListeningPort("9092/tcp").WithStartupTimeout(120 * time.Second),
				}
				kafkaCont, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{ContainerRequest: kafkaReq, Started: true})
				if err != nil {
					t.Logf("failed to start kafka container, skipping kafka roundtrip: %v", err)
					useKafka = false
				} else {
					defer kafkaCont.Terminate(ctx)
					host, _ := kafkaCont.Host(ctx)
					mp, _ := kafkaCont.MappedPort(ctx, "9092/tcp")
					kafkaBroker = host + ":" + mp.Port()
					t.Logf("kafka broker: %s", kafkaBroker)

					// create topic
					dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
					defer dialCancel()
					conn, err := kgo.DialContext(dialCtx, "tcp", kafkaBroker)
					if err == nil {
						topic := os.Getenv("KAFKA_TOPIC_BULK_JOB")
						if topic == "" {
							topic = "bulk.job"
						}
						_ = conn.CreateTopics(kgo.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1})
						_ = conn.Close()
					} else {
						t.Logf("warning: could not dial kafka to create topic: %v", err)
					}

					// start reader
					topic := os.Getenv("KAFKA_TOPIC_BULK_JOB")
					if topic == "" {
						topic = "bulk.job"
					}
					reader = kgo.NewReader(kgo.ReaderConfig{Brokers: []string{kafkaBroker}, Topic: topic, GroupID: "e2e-group"})
					go func() {
						for {
							m, err := reader.ReadMessage(context.Background())
							if err != nil {
								return
							}
							received <- m
						}
					}()

					// writer will be created in publishFunc
				}
			}
		} else {
			// For apache/kafka and other non-Confluent images: single container in KRaft mode.
			isApache := strings.Contains(strings.ToLower(kafkaImg), "apache/kafka")

			// Find a free host port so advertised listener matches the mapped port.
			var hostPort int
			if isApache {
				l, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Logf("could not find free port, skipping kafka roundtrip: %v", err)
					useKafka = false
				} else {
					hostPort = l.Addr().(*net.TCPAddr).Port
					l.Close()
					t.Logf("binding kafka to host port %d", hostPort)
				}
			}

			var env map[string]string
			var waitStrategy wait.Strategy
			if isApache {
				env = map[string]string{
					"KAFKA_NODE_ID":                          "1",
					"KAFKA_PROCESS_ROLES":                    "broker,controller",
					"KAFKA_LISTENERS":                        "PLAINTEXT://0.0.0.0:9092,CONTROLLER://0.0.0.0:9093",
					"KAFKA_ADVERTISED_LISTENERS":             fmt.Sprintf("PLAINTEXT://localhost:%d", hostPort),
					"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP":   "CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT",
					"KAFKA_CONTROLLER_QUORUM_VOTERS":         "1@localhost:9093",
					"KAFKA_CONTROLLER_LISTENER_NAMES":        "CONTROLLER",
					"CLUSTER_ID":                             "MkU3OEVBNTcwNTJENDM2Qk",
					"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR": "1",
				}
				waitStrategy = wait.ForLog("Kafka Server started").WithStartupTimeout(120 * time.Second)
			} else {
				env = map[string]string{"ALLOW_PLAINTEXT_LISTENER": "yes"}
				waitStrategy = wait.ForListeningPort("9092/tcp").WithStartupTimeout(120 * time.Second)
			}

			if useKafka {
				kafkaReq := tc.ContainerRequest{
					Image:        kafkaImg,
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
					WaitingFor: waitStrategy,
				}
				kafkaCont, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{ContainerRequest: kafkaReq, Started: true})
				if err != nil {
					t.Logf("failed to start kafka container, skipping kafka roundtrip: %v", err)
					useKafka = false
				} else {
					defer kafkaCont.Terminate(ctx)

					// Determine broker address
					if isApache {
						kafkaBroker = fmt.Sprintf("localhost:%d", hostPort)
					} else {
						host, _ := kafkaCont.Host(ctx)
						mp, _ := kafkaCont.MappedPort(ctx, "9092/tcp")
						kafkaBroker = host + ":" + mp.Port()
					}
					t.Logf("kafka broker: %s", kafkaBroker)

					// create topic with retries
					dialCtx, dialCancel := context.WithTimeout(ctx, 30*time.Second)
					defer dialCancel()
					conn, err := kgo.DialContext(dialCtx, "tcp", kafkaBroker)
					if err == nil {
						topic := os.Getenv("KAFKA_TOPIC_BULK_JOB")
						if topic == "" {
							topic = "bulk.job"
						}
						_ = conn.CreateTopics(kgo.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1})
						_ = conn.Close()
					} else {
						t.Logf("warning: could not dial kafka to create topic: %v", err)
					}

					// start reader
					topic := os.Getenv("KAFKA_TOPIC_BULK_JOB")
					if topic == "" {
						topic = "bulk.job"
					}
					reader = kgo.NewReader(kgo.ReaderConfig{Brokers: []string{kafkaBroker}, Topic: topic, GroupID: "e2e-group"})
					go func() {
						for {
							m, err := reader.ReadMessage(context.Background())
							if err != nil {
								return
							}
							received <- m
						}
					}()
				}
			}
		}
	}

	// blockFunc stub
	blockFunc := func(ctx context.Context, user string, count int, batchID string) (interface{}, error) {
		// pretend billing approved
		return map[string]any{"blocked": count}, nil
	}

	// prepare publishFunc
	publishErrs := make(chan error, 10)
	publishFunc := func(ctx context.Context, topic string, key []byte, msg any) error {
		// marshal msg to json
		b, _ := json.Marshal(msg)
		if useKafka {
			if writer == nil {
				writer = kgo.NewWriter(kgo.WriterConfig{Brokers: []string{kafkaBroker}, Topic: topic})
			}
			writeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			e := writer.WriteMessages(writeCtx, kgo.Message{Key: key, Value: b})
			if e != nil {
				publishErrs <- e
			}
			return e
		}
		// stub: just record success
		publishErrs <- nil
		return nil
	}

	// call ConfirmBatch to enqueue and publish
	queued, err := service.ConfirmBatch(context.Background(), bid, len(jobs), blockFunc, publishFunc)
	if err != nil {
		t.Fatalf("ConfirmBatch failed: %v", err)
	}
	if queued == 0 {
		t.Fatalf("expected some queued jobs, got 0")
	}

	// allow some time for publish operations
	time.Sleep(1 * time.Second)

	// verify job statuses updated to queued
	jobsAfter, err := jobRepo.GetByBatch(context.Background(), bid)
	if err != nil {
		t.Fatalf("get jobs after confirm failed: %v", err)
	}
	queuedCount := 0
	for _, j := range jobsAfter {
		if j.Status == "queued" {
			queuedCount++
		}
	}
	if queuedCount != queued {
		t.Fatalf("expected %d queued jobs, got %d", queued, queuedCount)
	}

	// if kafka used, verify messages consumed
	if useKafka {
		// wait up to 30s for messages
		deadline := time.Now().Add(30 * time.Second)
		recvd := 0
		for time.Now().Before(deadline) && recvd < queued {
			select {
			case <-received:
				recvd++
			case e := <-publishErrs:
				if e != nil {
					t.Fatalf("publish error: %v", e)
				}
			case <-time.After(500 * time.Millisecond):
			}
		}
		if recvd < queued {
			t.Fatalf("expected to receive %d messages from kafka, got %d", queued, recvd)
		}
		// clean reader/writer
		if writer != nil {
			_ = writer.Close()
		}
		if reader != nil {
			_ = reader.Close()
		}
	}

	// cleanup env vars
	os.Unsetenv("STORAGE_ENDPOINT")
	os.Unsetenv("STORAGE_ACCESS_KEY")
	os.Unsetenv("STORAGE_SECRET_KEY")
	os.Unsetenv("STORAGE_BUCKET")
	os.Unsetenv("STORAGE_USE_SSL")
	os.Unsetenv("STORAGE_BASE_URL")
}
