package parser

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/ikermy/Bulk/pkg/xls"
)

type ParseResult struct {
	Rows []map[string]string
}

type XLSParser struct {
	MaxRows     int
	MaxCols     int
	MaxFileSize int64
}

func NewXLSParser(maxRows, maxCols int, maxFileSize int64) *XLSParser {
	return &XLSParser{MaxRows: maxRows, MaxCols: maxCols, MaxFileSize: maxFileSize}
}

var (
	ErrInvalidFileFormat = errors.New("invalid file format")
	ErrEmptyFile         = errors.New("empty file")
	ErrTooManyRows       = errors.New("too many rows")
	ErrTooManyCols       = errors.New("too many columns")
)

// mapColumns delegates header auto-detection to package-level detector
func (p *XLSParser) mapColumns(headers []string, revision string) map[string]string {
	return DetectColumns(headers, revision)
}

// Parse reads the first sheet, enforces limits and returns rows mapped by field codes
func (p *XLSParser) Parse(r io.Reader, revision string) (*ParseResult, error) {
	// Read into buffer to support size check and reuse reader for detection
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return nil, err
	}
	if p.MaxFileSize > 0 && int64(buf.Len()) > p.MaxFileSize {
		return nil, ErrInvalidFileFormat
	}

	rows, err := xls.ReadFirstSheet(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, ErrInvalidFileFormat
	}

	// initialize with zero-length slice (capacity hint 0 is redundant)
	res := &ParseResult{Rows: make([]map[string]string, 0)}
	if len(rows) == 0 {
		return nil, ErrEmptyFile
	}

	headers := rows[0]
	// enforce max cols on header
	if p.MaxCols > 0 && len(headers) > p.MaxCols {
		return nil, ErrTooManyCols
	}

	// check total rows: rows includes header, so data rows = len(rows)-1
	if p.MaxRows > 0 && len(rows)-1 > p.MaxRows {
		return nil, ErrTooManyRows
	}

	columnMap := p.mapColumns(headers, revision)

	// check that at least one header was recognized; otherwise treat as invalid headers
	recognized := 0
	for _, h := range headers {
		if columnMap[h] != "" {
			recognized++
		}
	}
	if recognized == 0 {
		return nil, NewInvalidHeadersError(nil)
	}

	for i, row := range rows[1:] {
		rowData := make(map[string]string)
		// iterate over headers up to MaxCols
		for colIdx := 0; colIdx < len(headers); colIdx++ {
			if p.MaxCols > 0 && colIdx >= p.MaxCols {
				break
			}
			header := headers[colIdx]
			if colIdx < len(row) {
				fieldCode := columnMap[header]
				if fieldCode != "" {
					rowData[fieldCode] = strings.TrimSpace(row[colIdx])
				}
			}
		}
		// Excel row number (headers +1) -> first data row is 2
		rowData["_row"] = strconv.Itoa(i + 2)
		res.Rows = append(res.Rows, rowData)
	}

	return res, nil
}
