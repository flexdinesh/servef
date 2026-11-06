package app

import (
	"errors"

	"github.com/flexdinesh/srcz/pkg/contract"
)

// Error is an operation's failure: its kind, which a transport maps (HTTP:
// a status), an optional code naming a failure clients handle specially,
// and a user-facing message.
type Error struct {
	Kind    contract.ErrorKind
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

// Response is the error's transport body.
func (e *Error) Response() contract.ErrorResponse {
	return contract.ErrorResponse{Error: e.Message, Code: e.Code}
}

func fail(kind contract.ErrorKind, message string) *Error {
	return &Error{Kind: kind, Message: message}
}

// AsError returns err's *Error, or an internal one with err's message.
func AsError(err error) *Error {
	var failure *Error
	if errors.As(err, &failure) {
		return failure
	}
	return fail(contract.KindInternal, err.Error())
}
