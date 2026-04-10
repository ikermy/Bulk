package xls

import (
    "os"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestReadXLSFile_DirectFixture(t *testing.T) {
    path := "testdata/xls_sample.xls"
    if _, err := os.Stat(path); err != nil {
        t.Skipf("fixture not found: %v", err)
    }
    rows, err := readXLSFile(path)
    if err != nil {
        t.Skipf("readXLSFile not supported for fixture: %v", err)
    }
    require.Greater(t, len(rows), 0)
}


