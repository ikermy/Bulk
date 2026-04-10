package parser

import (
	"bytes"
	"io"
	"testing"
)

// helper to create CSV-like data buffer for testing parser (CSV used as simplest)
func makeCSV(content string) io.Reader {
	return bytes.NewReader([]byte(content))
}

func TestXLSParser_MaxRows(t *testing.T) {
	p := NewXLSParser(2, 10, 1024*1024)
	// create csv with header + 3 data rows (total 4 rows)
	csv := "h1,h2\n1,1\n2,2\n3,3\n"
	_, err := p.Parse(makeCSV(csv), "rev")
	if err == nil {
		t.Fatalf("expected ErrTooManyRows, got nil")
	}
	if err != ErrTooManyRows {
		t.Fatalf("expected ErrTooManyRows, got %v", err)
	}
}

func TestXLSParser_MaxCols(t *testing.T) {
	p := NewXLSParser(10, 2, 1024*1024)
	// header has 3 columns, limit is 2
	csv := "a,b,c\n1,2,3\n"
	_, err := p.Parse(makeCSV(csv), "rev")
	if err == nil {
		t.Fatalf("expected ErrTooManyCols, got nil")
	}
	if err != ErrTooManyCols {
		t.Fatalf("expected ErrTooManyCols, got %v", err)
	}
}

func TestXLSParser_MaxFileSize(t *testing.T) {
	p := NewXLSParser(10, 10, 10) // maxFileSize = 10 bytes
	// create content larger than 10 bytes
	csv := "h1,h2\n1234567890,2\n"
	_, err := p.Parse(makeCSV(csv), "rev")
	if err == nil {
		t.Fatalf("expected ErrInvalidFileFormat due to size, got nil")
	}
}

func TestXLSParser_TrimAndMapping(t *testing.T) {
	p := NewXLSParser(10, 10, 1024*1024)
	// header contains alias for DAC in mixed case and whitespace
	csv := "  First Name  ,last name\n JOHN  , DOE \n"
	res, err := p.Parse(makeCSV(csv), "rev")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(res.Rows))
	}
	row := res.Rows[0]
	// DetectColumns maps "First Name" -> DAC and "last name" -> DCS
	if v, ok := row["DAC"]; !ok || v != "JOHN" {
		t.Fatalf("expected DAC=JOHN, got %v", row["DAC"])
	}
	if v, ok := row["DCS"]; !ok || v != "DOE" {
		t.Fatalf("expected DCS=DOE, got %v", row["DCS"])
	}
}

