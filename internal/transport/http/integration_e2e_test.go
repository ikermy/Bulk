//go:build integration
// +build integration

package http

import (
    "bytes"
    "context"
    "database/sql"
    "io"
    "net/http"
    "mime/multipart"
    "encoding/json"
    "net/http/httptest"
    "os"
    "path/filepath"
    "strings"
    "testing"
    "time"

    tc "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/wait"

    "github.com/ikermy/Bulk/internal/db"
    "github.com/ikermy/Bulk/internal/repo"
    "github.com/ikermy/Bulk/internal/storage"
    svc "github.com/ikermy/Bulk/internal/usecase/bulk"
    "github.com/gin-gonic/gin"
    di "github.com/ikermy/Bulk/internal/di"
    handlers "github.com/ikermy/Bulk/internal/transport/http/handlers"
)

func TestE2E_Upload_SaveAndDBUpdate(t *testing.T) {
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

    // apply migrations — locate migrations/001_init.sql by searching upward
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

    // prepare multipart request with small CSV
    var b bytes.Buffer
    w := multipart.NewWriter(&b)
    fw, _ := w.CreateFormFile("file", "data.csv")
    io.Copy(fw, strings.NewReader("first_name,last_name\nval1,val2\n"))
    _ = w.WriteField("revision", "rev-e2e")
    w.Close()

    // sanity check: call service directly with CSV content to ensure parsing works
    if _, err := service.CreateBatchFromFile(context.Background(), strings.NewReader("first_name,last_name\nval1,val2\n"), "rev-e2e"); err != nil {
        t.Fatalf("direct service parse failed: %v", err)
    }

    // create a real HTTP server with gin so multipart parsing behaves like production
    router := gin.New()
    deps := &di.Deps{Service: service, BatchRepo: batchRepo, JobRepo: jobRepo}
    router.POST("/api/v1/upload", func(c *gin.Context) { handlers.HandleUpload(c, deps) })
    srv := httptest.NewServer(router)
    defer srv.Close()

    // stream multipart with io.Pipe to ensure server reads correctly
    pr, pw := io.Pipe()
    mw := multipart.NewWriter(pw)

    go func() {
        // write file part
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

    // verify DB: select file_storage_id and valid_rows
    row := dbConn.QueryRow("SELECT file_storage_id, valid_rows FROM batches WHERE id=$1", bid)
    var fileStorageID sql.NullString
    var validRows int
    if err := row.Scan(&fileStorageID, &validRows); err != nil {
        t.Fatalf("select batch failed: %v", err)
    }
    if !fileStorageID.Valid || fileStorageID.String == "" {
        t.Fatalf("expected file_storage_id to be set, got empty")
    }
    // JSON decoder returns numbers as float64
    summary, _ := resp["summary"].(map[string]any)
    var expectedValid int
    if v, ok := summary["validRows"].(float64); ok {
        expectedValid = int(v)
    }
    if validRows != expectedValid {
        t.Fatalf("expected valid_rows %d, got %d", expectedValid, validRows)
    }

    // cleanup env vars
    os.Unsetenv("STORAGE_ENDPOINT")
    os.Unsetenv("STORAGE_ACCESS_KEY")
    os.Unsetenv("STORAGE_SECRET_KEY")
    os.Unsetenv("STORAGE_BUCKET")
    os.Unsetenv("STORAGE_USE_SSL")
    os.Unsetenv("STORAGE_BASE_URL")
}









