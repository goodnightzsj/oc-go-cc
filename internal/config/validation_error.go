package config

import (
	"errors"
	"fmt"
)

// ValidationError carries a stable field/code for settings clients. Err keeps
// CLI diagnostics; the GUI must never expose arbitrary diagnostic text.
type ValidationError struct {
	Field string
	Code  string
	Err   error
}

func (e *ValidationError) Error() string { return e.Err.Error() }
func (e *ValidationError) Unwrap() error { return e.Err }

func invalid(field, code, format string, args ...any) error {
	return &ValidationError{Field: field, Code: code, Err: fmt.Errorf(format, args...)}
}

// AtField prefixes a nested validation path without losing its machine code.
func AtField(prefix string, err error) error {
	var validation *ValidationError
	if errors.As(err, &validation) {
		return &ValidationError{Field: prefix + "." + validation.Field, Code: validation.Code, Err: fmt.Errorf("%s.%w", prefix, err)}
	}
	return fmt.Errorf("%s: %w", prefix, err)
}
