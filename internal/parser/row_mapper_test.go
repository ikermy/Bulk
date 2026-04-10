package parser

import (
    "testing"

    "github.com/stretchr/testify/require"
)

func TestMapRowToFields(t *testing.T) {
    row := []string{"val1", "val2", "val3"}
    m := MapRowToFields(row)
    require.Len(t, m, 3)
    require.Equal(t, "val1", m["col_0"])
    require.Equal(t, "val2", m["col_1"])
    require.Equal(t, "val3", m["col_2"])
}

