// Package validation содержит общие компоненты валидации, разделяемые между Bulk Service и BFF.
// Согласно ТЗ §6.2: "Этот модуль используется и в Bulk Service, и в BFF".
package validation

import (
	"context"

	pkgval "github.com/ikermy/Bulk/pkg/validation"
)

// FieldRule — алиас pkg/validation.FieldRule.
// Общий тип для Bulk Service и BFF согласно ТЗ §6.2.
type FieldRule = pkgval.FieldRule

// FieldValidator — алиас pkg/validation.FieldValidator.
// Общий тип для Bulk Service и BFF согласно ТЗ §6.2.
type FieldValidator = pkgval.FieldValidator

// NewFieldValidator загружает правила валидации для указанной ревизии.
// Делегирует в pkg/validation.NewFieldValidator → loadRulesForRevision(revision).
// Соответствует ТЗ §6.2: Rules: loadRulesForRevision(revision).
func NewFieldValidator(revision string) *FieldValidator {
	return pkgval.NewFieldValidator(revision)
}

// LocalValidator выполняет локальную валидацию строки по правилам ревизии.
// Реализует ТЗ §6.1 "Local" режим: проверка структуры и формата полей
// на стороне Bulk Service без обращения к BFF (быстрая предварительная проверка).
//
// Порядок проверок согласно ТЗ §6.1 п.4:
//   - Проверяет обязательность полей и соответствие форматов (паттерны, длины).
//   - Выполняется до вызова BFF, позволяя сразу отклонить синтаксически невалидные строки.
type LocalValidator struct {
	fv *FieldValidator
}

// NewLocalValidator создаёт LocalValidator с правилами для указанной ревизии.
// Использует NewFieldValidator(revision) для загрузки revision-специфичных правил.
func NewLocalValidator(revision string) *LocalValidator {
	return &LocalValidator{fv: NewFieldValidator(revision)}
}

// ValidateRow выполняет локальную валидацию набора полей (строки).
// Сигнатура совпадает с BFFValidator.ValidateRow для взаимозаменяемости.
// Ошибка (второй возврат) всегда nil — локальная валидация не делает сетевых вызовов.
func (v *LocalValidator) ValidateRow(_ context.Context, row map[string]string, _ string) (*pkgval.ValidationResult, error) {
	// revision уже применена при создании LocalValidator через NewLocalValidator(revision),
	// поэтому повторно не требуется — правила уже загружены.
	return v.fv.ValidateRow(row), nil
}
