package parser

import (
    "errors"
    "fmt"
)

// ErrInvalidHeaders is a sentinel error representing invalid/missing headers.
var ErrInvalidHeaders = errors.New("invalid headers")

// InvalidHeadersError carries a list of missing required header codes.
type InvalidHeadersError struct {
    Missing []string
}

func (e *InvalidHeadersError) Error() string {
    return fmt.Sprintf("invalid headers: missing %v", e.Missing)
}

// Is allows errors.Is(err, ErrInvalidHeaders) to succeed for InvalidHeadersError.
func (e *InvalidHeadersError) Is(target error) bool {
    return target == ErrInvalidHeaders
}

// NewInvalidHeadersError returns a structured InvalidHeadersError when missing is non-empty,
// otherwise returns the sentinel ErrInvalidHeaders.
func NewInvalidHeadersError(missing []string) error {
    if len(missing) == 0 {
        return ErrInvalidHeaders
    }
    return &InvalidHeadersError{Missing: missing}
}

