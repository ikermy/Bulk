package parser

import "strconv"

// MapRowToFields placeholder for row to JSON mapping logic
func MapRowToFields(row []string) map[string]string {
	res := make(map[string]string)
	for i, v := range row {
		res["col_"+strconv.Itoa(i)] = v
	}
	return res
}
