// Package apperr defines the small set of error kinds the domain layer returns
// and the HTTP layer maps to status codes. It knows nothing about HTTP or SQL.
package apperr

// Kind classifies an Error so the API layer can pick a status code in one place.
type Kind int

const (
	KindInvalid  Kind = iota + 1 // caller supplied bad input
	KindNotFound                 // referenced entity does not exist
	KindConflict                 // request is valid but violates current state
	KindTooLarge                 // request exceeds a size limit
)

// Error is a domain error with a stable machine-readable Code and a message
// that is safe to show to API clients.
type Error struct {
	Kind    Kind
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

func Invalid(code, msg string) *Error  { return &Error{KindInvalid, code, msg} }
func NotFound(code, msg string) *Error { return &Error{KindNotFound, code, msg} }
func Conflict(code, msg string) *Error { return &Error{KindConflict, code, msg} }
func TooLarge(code, msg string) *Error { return &Error{KindTooLarge, code, msg} }
