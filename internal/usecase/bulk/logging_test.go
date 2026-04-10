package bulk

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ikermy/Bulk/internal/domain"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Test that CreateBatchFromFile logs batch_created event
// helper: create a sugared logger that writes to an in-memory buffer
func newSugaredLoggerWithBuffer() (*zap.SugaredLogger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	encCfg := zap.NewProductionEncoderConfig()
	enc := zapcore.NewJSONEncoder(encCfg)
	ws := zapcore.AddSync(buf)
	core := zapcore.NewCore(enc, ws, zap.InfoLevel)
	logger := zap.New(core).Sugar()
	return logger, buf
}

func TestCreateBatchFromFile_logs_batch_created(t *testing.T) {
	logger, buf := newSugaredLoggerWithBuffer()

	batchRepo := &mockBatchRepo{}
	jobRepo := &mockJobRepo{}
	svc := NewService(batchRepo, jobRepo, nil, nil, nil, nil)
	svc.Logger = logger

	csv := "first_name,last_name\nv1,v2\n"
	res, err := svc.CreateBatchFromFile(context.Background(), strings.NewReader(csv), "rev1")
	if err != nil {
		t.Fatalf("CreateBatchFromFile error: %v", err)
	}
	if res == nil || res.BatchID == "" {
		t.Fatalf("unexpected result: %+v", res)
	}

	out := buf.String()
	if !strings.Contains(out, "batch_created") {
		t.Fatalf("expected batch_created log entry, got output: %s", out)
	}
}

// Test that ConfirmBatch logs publishing_job and job_status_updated
func TestConfirmBatch_logs_publish_and_status(t *testing.T) {
	logger, buf := newSugaredLoggerWithBuffer()

	batchID := "b-log"
	jobs := []*domain.Job{{ID: "job-1", BatchID: batchID, RowNumber: 1}}
	jobRepo := &mockJobRepo{jobs: jobs}
	batchRepo := &mockBatchRepo{}

	svc := NewService(batchRepo, jobRepo, nil, nil, nil, nil)
	svc.Logger = logger

	publishFunc := func(ctx context.Context, topic string, key []byte, msg any) error { return nil }

	queued, err := svc.ConfirmBatch(context.Background(), batchID, 1, nil, publishFunc)
	if err != nil {
		t.Fatalf("ConfirmBatch error: %v", err)
	}
	if queued != 1 {
		t.Fatalf("expected queued 1, got %d", queued)
	}

	out := buf.String()
	if !strings.Contains(out, "publishing_job") || !strings.Contains(out, "job_status_updated") {
		t.Fatalf("expected publishing_job and job_status_updated in logs, got: %s", out)
	}
}
