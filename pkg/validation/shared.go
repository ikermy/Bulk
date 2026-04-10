package validation

import (
	"embed"
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

//go:embed rules/*.yaml
var rulesFS embed.FS

// ValidationResult представляет результат валидации строки
// Согласно OpenAPI (InternalValidateResponse) поле errors — массив строк.
type ValidationResult struct {
	Valid bool `json:"valid"`
	// Errors содержит структурированные ошибки валидации.
	// Соответствует ТЗ §6.2: ошибки должны иметь код и сообщение.
	Errors []ValidationError `json:"errors,omitempty"`
}

// FieldValidator содержит правила валидации по полям
type FieldValidator struct {
	Rules map[string]FieldRule
}

type FieldRule struct {
	Pattern   *regexp.Regexp
	MinLength int
	MaxLength int
	Required  bool
}

// ValidationError — структурированная ошибка валидации.
// Поля соответствуют требованиям ТЗ §6.2 (Code — машинно-обрабатываемый код ошибки).
type ValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewFieldValidator загружает правила валидации для указанной ревизии.
// В соответствии с ТЗ §6.2 этот модуль является общим для Bulk и BFF.
func NewFieldValidator(revision string) *FieldValidator {
	return &FieldValidator{Rules: loadRulesForRevision(revision)}
}

// Validate проверяет отдельное поле value в соответствии с правилом для field.
// Возвращает *ValidationError (с кодом и сообщением) или nil, если поле валидно.
// Коды ошибок соответствуют ТЗ §6.2: MISSING_MANDATORY, INVALID_FORMAT, TOO_SHORT, TOO_LONG.
func (v *FieldValidator) Validate(field string, value string) *ValidationError {
	rule, exists := v.Rules[field]
	if !exists {
		return nil // Unknown field, skip validation
	}

	if rule.Required && value == "" {
		return &ValidationError{Code: "MISSING_MANDATORY", Message: fmt.Sprintf("Field %s is required", field)}
	}

	if value != "" && rule.Pattern != nil && !rule.Pattern.MatchString(value) {
		return &ValidationError{Code: "INVALID_FORMAT", Message: fmt.Sprintf("Field %s has invalid format", field)}
	}

	if rule.MinLength > 0 && len(value) < rule.MinLength {
		return &ValidationError{Code: "TOO_SHORT", Message: fmt.Sprintf("Field %s is too short", field)}
	}

	if rule.MaxLength > 0 && len(value) > rule.MaxLength {
		return &ValidationError{Code: "TOO_LONG", Message: fmt.Sprintf("Field %s is too long", field)}
	}

	return nil
}

// ValidateRow проверяет набор полей (строку) и возвращает ValidationResult.
// Проверяются только поля, представленные в строке; если необходимо — можно
// дополнительно проверять отсутствие обязательных полей по Rules.
func (v *FieldValidator) ValidateRow(row map[string]string) *ValidationResult {
	res := &ValidationResult{Valid: true, Errors: []ValidationError{}}
	for field, val := range row {
		if err := v.Validate(field, val); err != nil {
			res.Valid = false
			res.Errors = append(res.Errors, *err)
		}
	}
	return res
}

// loadRulesForRevision заглушка: возвращает набор правил для ревизии.
// В реальной системе правила могут загружаться из конфигурации, YAML или внешнего источника.
func loadRulesForRevision(revision string) map[string]FieldRule {
	// Попробуем загрузить YAML с правилами из embeded файлов rules/*.yaml
	filename := fmt.Sprintf("rules/%s.yaml", revision)
	data, err := rulesFS.ReadFile(filename)
	if err != nil {
		// fallback на встроенные правила (как раньше)
		if revision == "v1" {
			return map[string]FieldRule{
				"passportNumber": {Pattern: regexp.MustCompile(`^[A-Z0-9]{6,10}$`), MinLength: 6, MaxLength: 10, Required: true},
				"fullName":       {MinLength: 2, MaxLength: 200, Required: true},
				"birthDate":      {Pattern: regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`), Required: false},
			}
		}
		return map[string]FieldRule{}
	}

	// YAML формат: map[string]struct{pattern,minLength,maxLength,required}
	var raw map[string]struct {
		Pattern   string `yaml:"pattern"`
		MinLength int    `yaml:"minLength"`
		MaxLength int    `yaml:"maxLength"`
		Required  bool   `yaml:"required"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		// на ошибке — fallback
		return map[string]FieldRule{}
	}

	rules := make(map[string]FieldRule, len(raw))
	for k, v := range raw {
		var re *regexp.Regexp
		if v.Pattern != "" {
			re = regexp.MustCompile(v.Pattern)
		}
		rules[k] = FieldRule{Pattern: re, MinLength: v.MinLength, MaxLength: v.MaxLength, Required: v.Required}
	}
	return rules
}
