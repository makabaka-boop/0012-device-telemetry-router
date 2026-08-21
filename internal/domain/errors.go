// Package domain provides domain errors mapped to unified API error
// envelopes by the HTTP layer.
package domain

import "fmt"

// Error is a typed domain error carrying an API error code and status.
type Error struct {
	Code    string
	Message string
	Details any
	Status  int
}

func (e *Error) Error() string {
	if e.Details == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Details)
}

// NewError builds a domain error with the given code and HTTP status.
func NewError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// WithDetails attaches structured details to the error.
func (e *Error) WithDetails(d any) *Error {
	e.Details = d
	return e
}

var (
	ErrInvalidField     = NewError(422, "INVALID_FIELD", "invalid field")
	ErrChecksumMismatch = NewError(422, "CHECKSUM_MISMATCH", "checksum mismatch")
	ErrDeviceNotFound   = NewError(404, "DEVICE_NOT_FOUND", "device not found")
	ErrDeviceConflict   = NewError(409, "DEVICE_CONFLICT", "device already exists")
	ErrDeviceInactive   = NewError(422, "DEVICE_INACTIVE", "device is not active")
	ErrDuplicateMessage = NewError(200, "DUPLICATE_MESSAGE", "duplicate message")
	ErrRuleConflict     = NewError(409, "RULE_CONFLICT", "rule conflict")
	ErrRuleNotFound     = NewError(404, "RULE_NOT_FOUND", "rule not found")
	ErrUnknownMetric    = NewError(422, "UNKNOWN_METRIC", "unknown metric")
	ErrTSOutOfRange     = NewError(422, "TS_OUT_OF_RANGE", "timestamp out of range")
	ErrEventNotFound    = NewError(404, "EVENT_NOT_FOUND", "event not found")
)
