// Package apperr defines the small set of error kinds the domain layer returns
// and the HTTP layer maps to status codes. It knows nothing about HTTP or SQL.
package apperr

import "time"

// Kind classifies an Error so the API layer can pick a status code in one place.
type Kind int

const (
	KindInvalid         Kind = iota + 1 // caller supplied bad input
	KindNotFound                        // referenced entity does not exist
	KindConflict                        // request is valid but violates current state
	KindTooLarge                        // request exceeds a size limit
	KindUnauthenticated                 // no valid credentials / session
	KindForbidden                       // understood, but not allowed
	KindRateLimited                     // too many attempts
	KindUnsupportedType                 // unsupported request content type
)

// Error is a domain error with a stable machine-readable Code and a message
// that is safe to show to API clients.
type Error struct {
	Kind       Kind
	Code       string
	Message    string
	RetryAfter time.Duration // optional: how long a rate-limited client should wait
}

func (e *Error) Error() string { return e.Message }

func Invalid(code, msg string) *Error   { return &Error{Kind: KindInvalid, Code: code, Message: msg} }
func NotFound(code, msg string) *Error  { return &Error{Kind: KindNotFound, Code: code, Message: msg} }
func Conflict(code, msg string) *Error  { return &Error{Kind: KindConflict, Code: code, Message: msg} }
func TooLarge(code, msg string) *Error  { return &Error{Kind: KindTooLarge, Code: code, Message: msg} }
func Forbidden(code, msg string) *Error { return &Error{Kind: KindForbidden, Code: code, Message: msg} }
func UnsupportedType(code, msg string) *Error {
	return &Error{Kind: KindUnsupportedType, Code: code, Message: msg}
}
func Unauthenticated(code, msg string) *Error {
	return &Error{Kind: KindUnauthenticated, Code: code, Message: msg}
}

// RateLimited is a 429; retryAfter becomes the Retry-After response header.
func RateLimited(code, msg string, retryAfter time.Duration) *Error {
	return &Error{Kind: KindRateLimited, Code: code, Message: msg, RetryAfter: retryAfter}
}
