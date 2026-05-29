package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/exit"
)

func TestSuccessEnvelope(t *testing.T) {
	t.Parallel()

	env := SuccessEnvelope("skill.list", map[string]any{"count": 2}, "obsidian not detected")

	if !env.OK {
		t.Fatal("success envelope should have OK=true")
	}
	if env.Command != "skill.list" {
		t.Fatalf("Command = %q, want skill.list", env.Command)
	}
	if env.Error != nil {
		t.Fatalf("success envelope should have no Error, got %+v", env.Error)
	}
	if len(env.Warnings) != 1 || env.Warnings[0] != "obsidian not detected" {
		t.Fatalf("Warnings = %v, want one warning", env.Warnings)
	}
}

func TestErrorEnvelopeDerivesCodeAndKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode int
		wantKind string
		wantHint string
	}{
		{
			name:     "precondition with hint",
			err:      exit.Preconditionf("no team bound").WithHint("run `teambrain team bind`"),
			wantCode: int(exit.Precondition),
			wantKind: "precondition",
			wantHint: "run `teambrain team bind`",
		},
		{
			name:     "external",
			err:      exit.Externalf("git push failed"),
			wantCode: int(exit.External),
			wantKind: "external",
		},
		{
			name:     "plain error defaults to user",
			err:      errorsNew("disk full"),
			wantCode: int(exit.User),
			wantKind: "user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			env := ErrorEnvelope("sync.commit", tt.err)
			if env.OK {
				t.Fatal("error envelope should have OK=false")
			}
			if env.Error == nil {
				t.Fatal("error envelope must carry an Error")
			}
			if env.Error.Code != tt.wantCode {
				t.Fatalf("Code = %d, want %d", env.Error.Code, tt.wantCode)
			}
			if env.Error.Kind != tt.wantKind {
				t.Fatalf("Kind = %q, want %q", env.Error.Kind, tt.wantKind)
			}
			if env.Error.Hint != tt.wantHint {
				t.Fatalf("Hint = %q, want %q", env.Error.Hint, tt.wantHint)
			}
		})
	}
}

func TestWriteJSONIsStableAndNewlineTerminated(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	env := SuccessEnvelope("init", map[string]string{"path": "/vault"})
	if err := WriteJSON(&buf, env); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	out := buf.String()
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("JSON output must end with a newline, got %q", out)
	}

	// It must round-trip as valid JSON.
	var got Envelope
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if !got.OK || got.Command != "init" {
		t.Fatalf("round-tripped envelope mismatch: %+v", got)
	}
}

func errorsNew(s string) error { return &simpleErr{s} }

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }
