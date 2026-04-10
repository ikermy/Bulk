package parser

import "strings"

// defaultColumnAliases — базовые алиасы колонок, общие для всех ревизий (ТЗ §5.3).
// Ключ: код поля AAMVA (DAC, DCS, …), значение: список алиасов заголовков XLS.
var defaultColumnAliases = map[string][]string{
	"DAC": {"first_name", "firstname", "first name", "имя", "dac"},
	"DCS": {"last_name", "lastname", "last name", "фамилия", "dcs"},
	"DBB": {"dob", "date_of_birth", "birthdate", "дата рождения", "dbb"},
	"DAG": {"address", "street", "street_address", "адрес", "dag"},
	"DAI": {"city", "город", "dai"},
	"DAJ": {"state", "штат", "daj"},
	"DAK": {"zip", "zipcode", "zip_code", "индекс", "dak"},
	"DBA": {"expiration", "exp_date", "expiry", "срок действия", "dba"},
	"DBD": {"issue_date", "issued", "дата выдачи", "dbd"},
}

// revisionAliasExtensions — дополнительные алиасы для конкретных ревизий (ТЗ §5.3).
// Добавляются к defaultColumnAliases, позволяя расширять набор под ревизию.
var revisionAliasExtensions = map[string]map[string][]string{
	// US_CA_08292017: стандартные алиасы AAMVA, расширений нет (используются defaults).
	"US_CA_08292017": {},
}

// revisionRequiredFields — обязательные поля для каждой ревизии (ТЗ §5.4, шаблон XLS).
// Для US_CA_08292017 соответствует полям шаблона из ТЗ §5.4.
var revisionRequiredFields = map[string][]string{
	"US_CA_08292017": {"DAC", "DCS", "DBB", "DAG", "DAI", "DAJ", "DAK", "DBA", "DBD"},
}

// defaultRequiredFields — поля по умолчанию, если ревизия не найдена в revisionRequiredFields.
var defaultRequiredFields = []string{"DAC", "DCS", "DBB", "DAG", "DAI", "DAJ", "DAK", "DBA", "DBD"}

// buildAliasMap возвращает итоговую карту алиасов для заданной ревизии:
// базовые алиасы, дополненные revision-специфичными расширениями (ТЗ §5.3).
func buildAliasMap(revision string) map[string][]string {
	result := make(map[string][]string, len(defaultColumnAliases))
	for code, list := range defaultColumnAliases {
		cp := make([]string, len(list))
		copy(cp, list)
		result[code] = cp
	}
	if ext, ok := revisionAliasExtensions[revision]; ok {
		for code, extra := range ext {
			result[code] = append(result[code], extra...)
		}
	}
	return result
}

// DetectColumns возвращает отображение оригинального заголовка -> кода поля
// на основе revision-специфичного набора синонимов/алиасов (ТЗ §5.3).
// Алгоритм: нормализуем заголовок (trim, toLower), сравниваем с алиасами ревизии.
func DetectColumns(headers []string, revision string) map[string]string {
	columnAliases := buildAliasMap(revision)

	res := make(map[string]string, len(headers))
	for _, h := range headers {
		norm := strings.ToLower(strings.TrimSpace(h))
		found := ""
		for code, aliases := range columnAliases {
			for _, a := range aliases {
				if norm == strings.ToLower(a) {
					found = code
					break
				}
			}
			if found != "" {
				break
			}
		}
		// fallback: если заголовок уже является кодом поля (напр., "DAC") — используем его
		if found == "" {
			up := strings.ToUpper(norm)
			if _, ok := columnAliases[up]; ok {
				found = up
			}
		}
		// пустая строка -> парсер пропустит неизвестную колонку
		res[h] = found
	}
	return res
}

// RequiredHeaders возвращает список обязательных кодов полей для заданной ревизии (ТЗ §5.4).
// Для US_CA_08292017 соответствует полям шаблона XLS из ТЗ §5.4.
// Для неизвестной ревизии возвращает набор по умолчанию.
func RequiredHeaders(revision string) []string {
	if fields, ok := revisionRequiredFields[revision]; ok {
		return fields
	}
	return defaultRequiredFields
}
