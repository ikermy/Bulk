package xls

import (
    "bytes"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
    "github.com/xuri/excelize/v2"
)

func TestReadFirstSheet_CSV(t *testing.T) {
    csv := "a,b,c\n1,2,3\n"
    rows, err := ReadFirstSheet(bytes.NewReader([]byte(csv)))
    require.NoError(t, err)
    require.Len(t, rows, 2)
    require.Equal(t, []string{"a", "b", "c"}, rows[0])
}

func TestReadFirstSheet_XLSX(t *testing.T) {
    f := excelize.NewFile()
    sheet := f.GetSheetName(0)
    _ = f.SetCellValue(sheet, "A1", "hello")
    _ = f.SetCellValue(sheet, "B1", "world")
    buf, err := f.WriteToBuffer()
    require.NoError(t, err)
    rows, err := ReadFirstSheet(bytes.NewReader(buf.Bytes()))
    require.NoError(t, err)
    require.GreaterOrEqual(t, len(rows), 1)
    require.GreaterOrEqual(t, len(rows[0]), 2)
    require.Equal(t, "hello", rows[0][0])
    require.Equal(t, "world", rows[0][1])
}

func TestReadFirstSheet_OLE_Error(t *testing.T) {
    // create bytes that start with OLE signature but are not valid .xls content
    data := []byte{0xD0, 0xCF, 0x11, 0xE0}
    // pad some data
    pad := make([]byte, 100)
    for i := range pad {
        pad[i] = byte(i % 256)
    }
    data = append(data, pad...)

    // call ReadFirstSheet: extrame/xls.Open likely fails on invalid content — we expect an error
    _, err := ReadFirstSheet(bytes.NewReader(data))
    require.Error(t, err)
}

// Ensure tests run quickly even on slower CI
func init() {
    // excelize may use time.Now internally; ensure deterministic small sleep tolerances
    _ = time.Now()
}

func TestReadFirstSheet_XLSX_NoSheets(t *testing.T) {
    f := excelize.NewFile()
    // remove default sheet to simulate zero sheets
    sheet := f.GetSheetName(0)
    _ = f.DeleteSheet(sheet)
    buf, err := f.WriteToBuffer()
    require.NoError(t, err)
    rows, err := ReadFirstSheet(bytes.NewReader(buf.Bytes()))
    require.NoError(t, err)
    // expect empty rows slice when no sheets
    require.Len(t, rows, 0)
}

func TestReadFirstSheet_InvalidPK_LeadsToError(t *testing.T) {
    // create data that starts with PK but is not a valid zip archive
    data := []byte{'P', 'K', 0, 1, 2, 3, 4}
    _, err := ReadFirstSheet(bytes.NewReader(data))
    require.Error(t, err)
}

func TestReadFirstSheet_CSV_BadFormat(t *testing.T) {
    // unmatched quote should produce csv parse error
    data := "a,b\n\"bad\n"
    _, err := ReadFirstSheet(bytes.NewReader([]byte(data)))
    require.Error(t, err)
}



