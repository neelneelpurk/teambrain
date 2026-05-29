package exit

import (
	"errors"
	"fmt"
	"testing"
)

func TestCodeOf(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")

	tests := []struct {
		name string
		err  error
		want Code
	}{
		{name: "nil is OK", err: nil, want: OK},
		{name: "plain error defaults to User", err: sentinel, want: User},
		{name: "user error", err: Userf("bad input"), want: User},
		{name: "precondition error", err: Preconditionf("no team bound"), want: Precondition},
		{name: "external error", err: Externalf("git failed"), want: External},
		{name: "wrapped *Error keeps code", err: fmt.Errorf("ctx: %w", Externalf("git failed")), want: External},
		{name: "double-wrapped *Error keeps code", err: fmt.Errorf("a: %w", fmt.Errorf("b: %w", Preconditionf("x"))), want: Precondition},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := CodeOf(tt.err); got != tt.want {
				t.Fatalf("CodeOf(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestErrorMessage(t *testing.T) {
	t.Parallel()

	t.Run("message only", func(t *testing.T) {
		t.Parallel()
		e := Userf("cannot read %q", "file.md")
		if got, want := e.Error(), `cannot read "file.md"`; got != want {
			t.Fatalf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("wraps cause", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("permission denied")
		e := Externalf("git push failed").Wrap(cause)
		if got, want := e.Error(), "git push failed: permission denied"; got != want {
			t.Fatalf("Error() = %q, want %q", got, want)
		}
		if !errors.Is(e, cause) {
			t.Fatalf("errors.Is could not find wrapped cause")
		}
	})

	t.Run("carries hint", func(t *testing.T) {
		t.Parallel()
		e := Preconditionf("no team bound").WithHint("run `teambrain team bind <path>`")
		if e.Hint != "run `teambrain team bind <path>`" {
			t.Fatalf("Hint = %q, want the bind hint", e.Hint)
		}
		if e.Code != Precondition {
			t.Fatalf("Code = %d, want %d", e.Code, Precondition)
		}
	})
}

func TestCodeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code Code
		want string
	}{
		{OK, "ok"},
		{User, "user"},
		{Precondition, "precondition"},
		{External, "external"},
		{Code(99), "code(99)"},
	}
	for _, tt := range tests {
		if got := tt.code.String(); got != tt.want {
			t.Errorf("Code(%d).String() = %q, want %q", int(tt.code), got, tt.want)
		}
	}
}

func TestErrorAsTarget(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("wrap: %w", Userf("nope"))

	var te *Error
	if !errors.As(err, &te) {
		t.Fatalf("errors.As failed to extract *Error")
	}
	if te.Code != User {
		t.Fatalf("extracted Code = %d, want %d", te.Code, User)
	}
}
