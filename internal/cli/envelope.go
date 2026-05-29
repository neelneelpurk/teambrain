package cli

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/neelneelpurk/teambrain/internal/exit"
)

// Envelope is the stable JSON shape teambrain emits under --json. Skills and
// scripts consume this; human-formatted output is never meant to be parsed.
type Envelope struct {
	OK       bool       `json:"ok"`
	Command  string     `json:"command,omitempty"`
	Data     any        `json:"data,omitempty"`
	Warnings []string   `json:"warnings,omitempty"`
	Error    *ErrorInfo `json:"error,omitempty"`
}

// ErrorInfo is the machine-readable error payload inside an Envelope.
type ErrorInfo struct {
	Code    int    `json:"code"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// SuccessEnvelope builds an OK envelope carrying data and any warnings.
func SuccessEnvelope(command string, data any, warnings ...string) Envelope {
	return Envelope{OK: true, Command: command, Data: data, Warnings: warnings}
}

// ErrorEnvelope builds a failure envelope, deriving the exit code, kind, and
// hint from the error chain.
func ErrorEnvelope(command string, err error) Envelope {
	code := exit.CodeOf(err)
	info := &ErrorInfo{
		Code:    int(code),
		Kind:    code.String(),
		Message: err.Error(),
	}
	var te *exit.Error
	if errors.As(err, &te) {
		info.Hint = te.Hint
		info.Message = te.Error()
	}
	return Envelope{OK: false, Command: command, Error: info}
}

// WriteJSON marshals env as indented JSON terminated by a newline.
func WriteJSON(w io.Writer, env Envelope) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(env)
}
