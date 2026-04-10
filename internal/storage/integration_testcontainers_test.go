//go:build integration
// +build integration

package storage

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMinIO_WithTestcontainers(t *testing.T) {
	if os.Getenv("RUN_INT_TESTS") != "1" {
		t.Skip("skipping integration tests; set RUN_INT_TESTS=1 to run")
	}
	ctx := context.Background()

	req := tc.ContainerRequest{
		Image:        "quay.io/minio/minio:latest",
		ExposedPorts: []string{"9000/tcp", "9001/tcp"},
		Env: map[string]string{
			"MINIO_ROOT_USER":     "minioadmin",
			"MINIO_ROOT_PASSWORD": "minioadmin",
		},
		Cmd:        []string{"server", "/data", "--console-address", ":9001"},
		WaitingFor: wait.ForListeningPort("9000/tcp").WithStartupTimeout(60 * time.Second),
	}

	cont, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		t.Fatalf("failed to start minio container: %v", err)
	}
	defer cont.Terminate(ctx)

	host, err := cont.Host(ctx)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	p, err := cont.MappedPort(ctx, "9000/tcp")
	if err != nil {
		t.Fatalf("mapped port: %v", err)
	}
	endpoint := host + ":" + p.Port()

	// set env for NewFileClientFromEnv
	os.Setenv("STORAGE_ENDPOINT", endpoint)
	os.Setenv("STORAGE_ACCESS_KEY", "minioadmin")
	os.Setenv("STORAGE_SECRET_KEY", "minioadmin")
	os.Setenv("STORAGE_BUCKET", "bulk-test")
	os.Setenv("STORAGE_USE_SSL", "false")
	os.Setenv("STORAGE_BASE_URL", "http://"+endpoint)

	// give minio short time to be ready
	time.Sleep(2 * time.Second)

	fc, err := NewFileClientFromEnv()
	if err != nil {
		t.Fatalf("NewFileClientFromEnv failed: %v", err)
	}

	// upload small content
	id, err := fc.Save("testdir/test.txt", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if id == "" {
		t.Fatalf("empty id returned")
	}

	// Save success is considered sufficient verification for this integration test
	t.Logf("object saved with id=%s", id)

	// cleanup env
	os.Unsetenv("STORAGE_ENDPOINT")
	os.Unsetenv("STORAGE_ACCESS_KEY")
	os.Unsetenv("STORAGE_SECRET_KEY")
	os.Unsetenv("STORAGE_BUCKET")
	os.Unsetenv("STORAGE_USE_SSL")
	os.Unsetenv("STORAGE_BASE_URL")
}

