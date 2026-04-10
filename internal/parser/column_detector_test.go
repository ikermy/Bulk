package parser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDetectColumns_BasicAliases проверяет маппинг заголовков XLS на коды полей AAMVA.
// Ревизия US_CA_08292017 используется согласно ТЗ §18.2 (TestXLSParser_Parse).
func TestDetectColumns_BasicAliases(t *testing.T) {
	headers := []string{"First_Name", "Lastname", "dob", "unknown"}
	res := DetectColumns(headers, "US_CA_08292017")
	require.Equal(t, "DAC", res["First_Name"])
	require.Equal(t, "DCS", res["Lastname"])
	require.Equal(t, "DBB", res["dob"])
	require.Equal(t, "", res["unknown"])
}

// TestDetectColumns_DirectCodes проверяет fallback: заголовок == код поля (напр., "DAC").
func TestDetectColumns_DirectCodes(t *testing.T) {
	headers := []string{"DAC", "DCS", "DBB", "DAG", "DAI", "DAJ", "DAK", "DBA", "DBD"}
	res := DetectColumns(headers, "US_CA_08292017")
	for _, h := range headers {
		require.Equal(t, h, res[h], "direct code %s should map to itself", h)
	}
}

// TestDetectColumns_UnknownRevisionFallback проверяет, что неизвестная ревизия
// использует базовые алиасы (defaultColumnAliases).
func TestDetectColumns_UnknownRevisionFallback(t *testing.T) {
	headers := []string{"first_name", "last_name"}
	res := DetectColumns(headers, "UNKNOWN_REV")
	require.Equal(t, "DAC", res["first_name"])
	require.Equal(t, "DCS", res["last_name"])
}

// TestRequiredHeaders_US_CA_08292017 проверяет список обязательных полей для главной
// ревизии из ТЗ §5.4 (шаблон XLS): DAC, DCS, DBB, DAG, DAI, DAJ, DAK, DBA, DBD.
func TestRequiredHeaders_US_CA_08292017(t *testing.T) {
	req := RequiredHeaders("US_CA_08292017")
	required := []string{"DAC", "DCS", "DBB", "DAG", "DAI", "DAJ", "DAK", "DBA", "DBD"}
	for _, field := range required {
		require.Contains(t, req, field)
	}
	require.Len(t, req, len(required))
}

// TestRequiredHeaders_OrderAndContents проверяет набор по умолчанию для неизвестной ревизии.
func TestRequiredHeaders_OrderAndContents(t *testing.T) {
	req := RequiredHeaders("v1")
	require.Contains(t, req, "DAC")
	require.Contains(t, req, "DCS")
	require.Contains(t, req, "DBB")
}
