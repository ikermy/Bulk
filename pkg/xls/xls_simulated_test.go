package xls

import (
    "bytes"
    "testing"

    "github.com/stretchr/testify/require"
)

func TestReadFirstSheet_XLS_Simulated(t *testing.T) {
    // prepare data that triggers OLE detection
    data := []byte{0xD0, 0xCF, 0x11, 0xE0, 1, 2, 3}

    // replace readXLSFile to avoid a real .xls dependency
    orig := readXLSFile
    defer func() { readXLSFile = orig }()
    readXLSFile = func(path string) ([][]string, error) {
        return [][]string{{"aa", "bb"}, {"1", "2"}}, nil
    }

    rows, err := ReadFirstSheet(bytes.NewReader(data))
    require.NoError(t, err)
    require.Len(t, rows, 2)
    require.Equal(t, "aa", rows[0][0])
}

