package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestBuildInfoString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		b    BuildInfo
		want string
	}{
		{"full", BuildInfo{Version: "1.2.3", Commit: "abc1234", Date: "2026-05-29"}, "1.2.3 (abc1234, 2026-05-29)"},
		{"commit only", BuildInfo{Version: "1.2.3", Commit: "abc1234"}, "1.2.3 (abc1234)"},
		{"version only", BuildInfo{Version: "1.2.3"}, "1.2.3"},
		{"empty falls back to dev", BuildInfo{}, "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.b.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppWarnHumanGoesToStderr(t *testing.T) {
	t.Parallel()

	var out, errb bytes.Buffer
	app := &App{IO: IO{Out: &out, Err: &errb}, Config: &Config{}}
	app.Warn("link %q is unresolved", "[[ghost]]")

	if got := errb.String(); !strings.Contains(got, `warning: link "[[ghost]]" is unresolved`) {
		t.Fatalf("stderr = %q, want the warning", got)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty in human warn mode, got %q", out.String())
	}
	if w := app.Warnings(); len(w) != 1 || w[0] != `link "[[ghost]]" is unresolved` {
		t.Fatalf("Warnings() = %v, want one recorded warning", w)
	}
}

func TestAppWarnQuietSuppressesStderrButRecords(t *testing.T) {
	t.Parallel()

	var errb bytes.Buffer
	app := &App{IO: IO{Err: &errb}, Config: &Config{Quiet: true}}
	app.Warn("noisy")

	if errb.Len() != 0 {
		t.Fatalf("quiet mode must not print to stderr, got %q", errb.String())
	}
	if len(app.Warnings()) != 1 {
		t.Fatalf("warning should still be recorded under --quiet")
	}
}

func TestAppEmitJSONFoldsWarnings(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	app := &App{IO: IO{Out: &out}, Config: &Config{JSON: true}}
	app.Warn("heads up")

	if err := app.Emit("skill.list", map[string]string{"k": "v"}, func(io.Writer) {
		t.Fatal("human renderer must not run under --json")
	}); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if !env.OK || env.Command != "skill.list" {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	if len(env.Warnings) != 1 || env.Warnings[0] != "heads up" {
		t.Fatalf("warnings not folded into envelope: %v", env.Warnings)
	}
}

func TestAppEmitHumanInvokesRenderer(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	app := &App{IO: IO{Out: &out}, Config: &Config{}}
	if err := app.Emit("doctor", nil, func(w io.Writer) {
		fmt.Fprint(w, "all good")
	}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if out.String() != "all good" {
		t.Fatalf("human output = %q, want %q", out.String(), "all good")
	}
}

func TestNewRootCommandIsConstructed(t *testing.T) {
	t.Parallel()

	cmd := NewRootCommand(IO{In: strings.NewReader(""), Out: io.Discard, Err: io.Discard}, BuildInfo{Version: "x"})
	if cmd == nil || cmd.Use != "teambrain" {
		t.Fatalf("NewRootCommand did not return the root command")
	}

	var doctor bool
	for _, c := range cmd.Commands() {
		if c.Name() == "doctor" {
			doctor = true
			if got := commandID(c); got != "doctor" {
				t.Fatalf("commandID(doctor) = %q, want doctor", got)
			}
		}
	}
	if !doctor {
		t.Fatal("doctor command not registered on root")
	}
}
