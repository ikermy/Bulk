package xls

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadFirstSheet_XLS_Mock(t *testing.T) {
	// Байты с OLE Compound File сигнатурой — запускают ветку .xls в ReadFirstSheet
	data := []byte{0xD0, 0xCF, 0x11, 0xE0, 0x01, 0x02}

	orig := readXLSFile
	defer func() { readXLSFile = orig }()

	// Подставляем фиктивный парсер, возвращающий предсказуемые строки
	readXLSFile = func(path string) ([][]string, error) {
		return [][]string{
			{"A1", "B1"},
			{"A2", "B2"},
		}, nil
	}

	rows, err := ReadFirstSheet(bytes.NewReader(data))
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "A1", rows[0][0])
	require.Equal(t, "B2", rows[1][1])
}
