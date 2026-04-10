//go:build integration
// +build integration

package xls

import (
	"os"
	"testing"
)

// Интеграционный тест для разбора .xls. Для локального запуска установите
// переменную окружения XLS_FIXTURE с путём к реальному небольшому .xls-файлу,
// либо разместите файл testdata/xls_sample.xls.
func TestReadFirstSheet_XLSIntegration(t *testing.T) {
	fixture := os.Getenv("XLS_FIXTURE")
	if fixture == "" {
		// look for testdata
		if _, err := os.Stat("testdata/xls_sample.xls"); err == nil {
			fixture = "testdata/xls_sample.xls"
		}
	}
	if fixture == "" {
		t.Skip("No .xls fixture provided; set XLS_FIXTURE or add testdata/xls_sample.xls to run integration test")
	}

	f, err := os.Open(fixture)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	// pass to reader
	rows, err := ReadFirstSheet(f)
	if err != nil {
		t.Fatalf("ReadFirstSheet failed: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected at least one row in fixture")
	}
}
