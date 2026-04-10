package validation

import (
	"testing"
)

func TestValidate_FieldRules(t *testing.T) {
	v := NewFieldValidator("v1")

	// missing mandatory
	if err := v.Validate("passportNumber", ""); err == nil {
		t.Fatalf("expected error for missing mandatory")
	} else if err.Code != "MISSING_MANDATORY" {
		t.Fatalf("expected MISSING_MANDATORY, got %v", err.Code)
	}

	// invalid format
	if err := v.Validate("passportNumber", "abc123"); err == nil {
		t.Fatalf("expected error for invalid format")
	} else if err.Code != "INVALID_FORMAT" {
		t.Fatalf("expected INVALID_FORMAT, got %v", err.Code)
	}

	// valid passport
	if err := v.Validate("passportNumber", "ABC1234"); err != nil {
		t.Fatalf("expected valid passport, got %v", err)
	}

	// fullName too short
	if err := v.Validate("fullName", "A"); err == nil {
		t.Fatalf("expected TOO_SHORT error")
	} else if err.Code != "TOO_SHORT" {
		t.Fatalf("expected TOO_SHORT, got %v", err.Code)
	}

	// fullName valid
	if err := v.Validate("fullName", "John Doe"); err != nil {
		t.Fatalf("expected valid fullName, got %v", err)
	}
}

func TestValidateRow_Aggregation(t *testing.T) {
	v := NewFieldValidator("v1")
	row := map[string]string{"passportNumber": "abc", "fullName": "J"}
	res := v.ValidateRow(row)
	if res.Valid {
		t.Fatalf("expected invalid row, got valid")
	}
	if len(res.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(res.Errors))
	}
}
