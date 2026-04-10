package parser

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewInvalidHeadersError_Empty(t *testing.T) {
	err := NewInvalidHeadersError([]string{})
	require.Equal(t, ErrInvalidHeaders, err)
}

func TestNewInvalidHeadersError_NonEmpty(t *testing.T) {
	missing := []string{"A", "B"}
	err := NewInvalidHeadersError(missing)
	// should be our structured type
	var ihe *InvalidHeadersError
	require.True(t, errors.As(err, &ihe))
	require.Equal(t, missing, ihe.Missing)
	// Error() should contain the missing slice
	require.Contains(t, err.Error(), "missing")
	// errors.Is should match the sentinel
	require.True(t, errors.Is(err, ErrInvalidHeaders))
}
