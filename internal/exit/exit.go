// Package exit defines teambrain's stable, documented process exit codes and a
// rich error type that carries one. The CLI's top-level handler maps any error
// to a code via CodeOf, so commands signal intent simply by returning the right
// constructor (Userf, Preconditionf, Externalf).
package exit

import (
	"errors"
	"fmt"
)

// Code is a process exit code with stable semantics. These values are part of
// teambrain's public contract (see the CLI UX section of the docs) and must not
// be renumbered.
type Code int

const (
	// OK signals success.
	OK Code = 0
	// User signals a user or validation error (bad flag, missing argument,
	// invalid input). The fault is in the invocation.
	User Code = 1
	// Precondition signals that the environment is not ready for the command
	// (e.g. no team vault bound, not inside a vault).
	Precondition Code = 2
	// External signals a failure in an external system teambrain shells out to
	// (git, the Obsidian CLI) rather than in teambrain itself.
	External Code = 3
)

// String renders the code's name for logs and tests.
func (c Code) String() string {
	switch c {
	case OK:
		return "ok"
	case User:
		return "user"
	case Precondition:
		return "precondition"
	case External:
		return "external"
	default:
		return fmt.Sprintf("code(%d)", int(c))
	}
}

// Error is teambrain's structured error. It pairs a human message with an exit
// Code, an optional actionable Hint ("the next step"), and an optional wrapped
// cause. It satisfies the standard errors.Is/As/Unwrap contracts.
type Error struct {
	Code Code
	Msg  string
	Hint string
	Err  error
}

// Error implements the error interface. When a cause is wrapped, it is appended
// after a colon so the full chain reads naturally.
func (e *Error) Error() string {
	if e.Err != nil {
		return e.Msg + ": " + e.Err.Error()
	}
	return e.Msg
}

// Unwrap exposes the wrapped cause for errors.Is/As.
func (e *Error) Unwrap() error { return e.Err }

// Wrap attaches an underlying cause and returns the receiver for chaining.
func (e *Error) Wrap(cause error) *Error {
	e.Err = cause
	return e
}

// WithHint attaches an actionable next step and returns the receiver.
func (e *Error) WithHint(hint string) *Error {
	e.Hint = hint
	return e
}

func newf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// Userf builds a User (exit 1) error.
func Userf(format string, args ...any) *Error { return newf(User, format, args...) }

// Preconditionf builds a Precondition (exit 2) error.
func Preconditionf(format string, args ...any) *Error { return newf(Precondition, format, args...) }

// Externalf builds an External (exit 3) error.
func Externalf(format string, args ...any) *Error { return newf(External, format, args...) }

// CodeOf extracts the exit Code from an error chain. A nil error is OK; an
// *Error (anywhere in the chain) yields its Code; any other non-nil error
// defaults to User, the generic failure bucket.
func CodeOf(err error) Code {
	if err == nil {
		return OK
	}
	var te *Error
	if errors.As(err, &te) {
		return te.Code
	}
	return User
}
