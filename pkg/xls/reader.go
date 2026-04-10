package xls

import (
	"bytes"
	"encoding/csv"
	"io"
	"os"

	extxls "github.com/extrame/xls"
	"github.com/xuri/excelize/v2"
)

// ReadFirstSheet reads the first sheet of an XLSX, XLS or CSV content from reader
// It tries to detect format by content (simple heuristic) and returns rows as [][]string
//
// Соответствие ТЗ §5.1 (Обработка XLS):
//   - Поддерживаются форматы .xlsx (Excel 2007+) и .csv (CSV) — реализовано через
//     github.com/xuri/excelize/v2 и encoding/csv соответственно.
//   - Дополнительно добавлена поддержка старого бинарного формата .xls (Excel 97-2003)
//     через библиотеку github.com/extrame/xls. Для парсинга .xls мы временно
//     записываем поток во временный файл и передаём путь в библиотеку (требование API
//     extrame/xls). Это обеспечивает полное соответствие ТЗ §5.1 по поддерживаемым форматам.
func ReadFirstSheet(r io.Reader) ([][]string, error) {

	// read into buffer
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, err
	}
	data := buf.Bytes()

	// quick heuristic: if it contains "PK" at start, treat as xlsx (zip archive)
	if len(data) >= 2 && data[0] == 'P' && data[1] == 'K' {
		f, err := excelize.OpenReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return [][]string{}, nil
		}
		rows, err := f.GetRows(sheets[0])
		if err != nil {
			return nil, err
		}
		return rows, nil
	}

	// detect OLE Compound File signature for old .xls (D0 CF 11 E0)
	if len(data) >= 4 && data[0] == 0xD0 && data[1] == 0xCF && data[2] == 0x11 && data[3] == 0xE0 {
		// extrame/xls requires a filename; write buffer to temp file
		tmp, err := os.CreateTemp("", "bulk-*.xls")
		if err != nil {
			return nil, err
		}
		tmpName := tmp.Name()
		defer func() { _ = os.Remove(tmpName) }()
		if _, err := tmp.Write(data); err != nil {
			_ = tmp.Close()
			return nil, err
		}
		if err := tmp.Close(); err != nil {
			return nil, err
		}

		// Delegated to readXLSFile helper to allow tests to override .xls parsing without
		// requiring a real .xls binary fixture.
		return readXLSFile(tmpName)
	}

	// fallback: CSV
	cr := csv.NewReader(bytes.NewReader(data))
	records, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	return records, nil
}

// readXLSFile reads an .xls file by path and returns rows. It is a variable so tests
// can replace it to simulate .xls parsing without needing a real .xls binary.
var readXLSFile = func(path string) ([][]string, error) {
	wb, err := extxls.Open(path, "utf-8")
	if err != nil {
		return nil, err
	}
	if wb == nil || wb.NumSheets() == 0 {
		return [][]string{}, nil
	}
	sheet := wb.GetSheet(0)
	if sheet == nil {
		return [][]string{}, nil
	}
	var rows [][]string
	for i := 0; i <= int(sheet.MaxRow); i++ {
		r := sheet.Row(i)
		var cols []string
		for j := 0; j < r.LastCol(); j++ {
			cols = append(cols, r.Col(j))
		}
		rows = append(rows, cols)
	}
	return rows, nil
}
