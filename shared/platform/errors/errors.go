package errors

import (
	"errors"
	"fmt"
)

// Error types for different categories of errors
const (
	ErrorTypeValidation = "validation"
	ErrorTypeNotFound   = "not_found"
	ErrorTypeConflict   = "conflict"
	ErrorTypeInternal   = "internal"
	ErrorTypeExternal   = "external"
)

// AppError represents an application error with type and context
type AppError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Err     error  `json:"-"` // Original error, not serialized
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s", e.Message, e.Err.Error())
	}
	return e.Message
}

// Unwrap returns the wrapped error
func (e *AppError) Unwrap() error {
	return e.Err
}

// Is checks if the error matches the target
func (e *AppError) Is(target error) bool {
	if target == nil {
		return false
	}
	
	if appErr, ok := target.(*AppError); ok {
		return e.Type == appErr.Type
	}
	
	return errors.Is(e.Err, target)
}

// NewValidation creates a new validation error
func NewValidation(message string) *AppError {
	return &AppError{
		Type:    ErrorTypeValidation,
		Message: message,
	}
}

// NewNotFound creates a new not found error
func NewNotFound(message string) *AppError {
	return &AppError{
		Type:    ErrorTypeNotFound,
		Message: message,
	}
}

// NewConflict creates a new conflict error
func NewConflict(message string) *AppError {
	return &AppError{
		Type:    ErrorTypeConflict,
		Message: message,
	}
}

// NewInternal creates a new internal error
func NewInternal(message string) *AppError {
	return &AppError{
		Type:    ErrorTypeInternal,
		Message: message,
	}
}

// NewExternal creates a new external service error
func NewExternal(message string) *AppError {
	return &AppError{
		Type:    ErrorTypeExternal,
		Message: message,
	}
}

// Wrap wraps an existing error with a message
func Wrap(err error, message string) *AppError {
	if err == nil {
		return nil
	}
	
	// If it's already an AppError, preserve the type
	if appErr, ok := err.(*AppError); ok {
		return &AppError{
			Type:    appErr.Type,
			Message: message,
			Err:     err,
		}
	}
	
	// Default to internal error for unknown errors
	return &AppError{
		Type:    ErrorTypeInternal,
		Message: message,
		Err:     err,
	}
}

// Type checking functions

// IsValidation checks if error is a validation error
func IsValidation(err error) bool {
	return hasErrorType(err, ErrorTypeValidation)
}

// IsNotFound checks if error is a not found error
func IsNotFound(err error) bool {
	return hasErrorType(err, ErrorTypeNotFound)
}

// IsConflict checks if error is a conflict error
func IsConflict(err error) bool {
	return hasErrorType(err, ErrorTypeConflict)
}

// IsInternal checks if error is an internal error
func IsInternal(err error) bool {
	return hasErrorType(err, ErrorTypeInternal)
}

// IsExternal checks if error is an external service error
func IsExternal(err error) bool {
	return hasErrorType(err, ErrorTypeExternal)
}

// hasErrorType checks if the error has the specified type
func hasErrorType(err error, errorType string) bool {
	if err == nil {
		return false
	}
	
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type == errorType
	}
	
	return false
}

// GetErrorType returns the error type, or "unknown" if not an AppError
func GetErrorType(err error) string {
	if err == nil {
		return ""
	}
	
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Type
	}
	
	return "unknown"
}