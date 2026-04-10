package xls

import (
	"bytes"
	"os"
	"testing"
)

func TestReadFirstSheet_Fixture(t *testing.T) {
	path := "testdata/xls_sample.xls"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	// read raw bytes for debugging
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file bytes: %v", err)
	}
	t.Logf("fixture size=%d", len(data))
	if len(data) < 200 {
		t.Logf("fixture content: %q", string(data))
	} else {
		t.Logf("fixture head: %q", string(data[:200]))
	}

	rows, err := ReadFirstSheet(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ReadFirstSheet error: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected rows > 0, got 0")
	}
}
