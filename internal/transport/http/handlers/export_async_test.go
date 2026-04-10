package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ikermy/Bulk/internal/di"
	exp "github.com/ikermy/Bulk/internal/exporter"
)

type fakeManager struct {
	res *exp.ExportResult
}

func (f *fakeManager) Enqueue(id string) error                 { return nil }
func (f *fakeManager) Get(id string) (*exp.ExportResult, bool) { return f.res, f.res != nil }

type mockStorage struct {
	presign    string
	presignErr error
	data       []byte
}

func (m *mockStorage) Save(name string, r io.Reader) (string, error) { return "", nil }
func (m *mockStorage) Presign(name string, expiry time.Duration) (string, error) {
	if m.presignErr != nil {
		return "", m.presignErr
	}
	return m.presign, nil
}
func (m *mockStorage) Get(name string) (io.ReadCloser, error) {
	if m.data == nil {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(m.data)), nil
}
func (m *mockStorage) PublicURL(name string) (string, error) { return "", nil }

// Note: we import packages inside test function to avoid unused imports at top-level in apply_patch

func TestHandleGetExport_Presign(t *testing.T) {
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "e1"})

	mgr := &fakeManager{res: &exp.ExportResult{ID: "e1", Status: exp.StatusDone, StorageID: "exports/e1.xlsx"}}
	ms := &mockStorage{presign: "https://signed.example.com/obj"}
	deps := &di.Deps{ExportManager: mgr, Storage: ms}

	HandleGetExport(c, deps)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rw.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if resp["url"] != "https://signed.example.com/obj" {
		t.Fatalf("expected presigned url returned")
	}
}

func TestHandleGetExport_Proxy(t *testing.T) {
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "e2"})

	data := []byte("xlsxdata")
	mgr := &fakeManager{res: &exp.ExportResult{ID: "e2", Status: exp.StatusDone, StorageID: "exports/e2.xlsx"}}
	ms := &mockStorage{presignErr: errors.New("no presign"), data: data}
	deps := &di.Deps{ExportManager: mgr, Storage: ms}

	HandleGetExport(c, deps)
	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rw.Code)
	}
	if rw.Header().Get("Content-Type") == "" {
		t.Fatalf("expected content-type set")
	}
	if rw.Body.Len() != len(data) {
		t.Fatalf("expected body length %d got %d", len(data), rw.Body.Len())
	}
}

func TestHandleStartExport_Success(t *testing.T) {
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "ex1"})

	mgr := &fakeManager{res: &exp.ExportResult{ID: "ex1", Status: exp.StatusPending}}
	deps := &di.Deps{ExportManager: mgr}

	HandleStartExport(c, deps)
	if rw.Code != http.StatusAccepted {
		t.Fatalf("expected 202 got %d", rw.Code)
	}
	var resp map[string]any
	if err := json.NewDecoder(rw.Body).Decode(&resp); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if resp["id"] != "ex1" {
		t.Fatalf("expected id ex1 returned")
	}
	if resp["status"] != "queued" {
		t.Fatalf("expected status queued")
	}
}

type errManager struct{}

func (e *errManager) Enqueue(id string) error                 { return errors.New("boom") }
func (e *errManager) Get(id string) (*exp.ExportResult, bool) { return nil, false }

func TestHandleStartExport_EnqueueError(t *testing.T) {
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "ex2"})

	mgr := &errManager{}
	deps := &di.Deps{ExportManager: mgr}

	HandleStartExport(c, deps)
	if rw.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", rw.Code)
	}
}

func TestHandleStartExport_MissingDependency(t *testing.T) {
	rw := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rw)
	c.Params = append(c.Params, gin.Param{Key: "id", Value: "ex3"})

	// nil deps
	HandleStartExport(c, nil)
	if rw.Code != http.StatusNotFound {
		t.Fatalf("expected 404 got %d", rw.Code)
	}
}
