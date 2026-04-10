package xls

import (
    "bytes"
    "testing"

    "github.com/stretchr/testify/require"
    "github.com/xuri/excelize/v2"
)

func TestReadFirstSheet_XLSX_InMemory(t *testing.T) {
    f := excelize.NewFile()
    // excelize по умолчанию создаёт Sheet1
    require.NoError(t, f.SetCellValue("Sheet1", "A1", "colA"))
    require.NoError(t, f.SetCellValue("Sheet1", "B1", "colB"))

    buf, err := f.WriteToBuffer()
    require.NoError(t, err)

    rows, err := ReadFirstSheet(bytes.NewReader(buf.Bytes()))
    require.NoError(t, err)
    require.Greater(t, len(rows), 0)
    require.Equal(t, "colA", rows[0][0])
    require.Equal(t, "colB", rows[0][1])
}

