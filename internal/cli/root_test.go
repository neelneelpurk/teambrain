package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// runRoot executes the root command with args and captured IO, returning the
// exit code and the stdout/stderr buffers.
func runRoot(t *testing.T, args ...string) (code int, stdout, stderr *bytes.Buffer) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // isolate from the real user config
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	io := IO{In: strings.NewReader(""), Out: stdout, Err: stderr}
	code = Execute(io, BuildInfo{Version: "1.2.3", Commit: "abc1234", Date: "2026-05-29"}, args)
	return code, stdout, stderr
}

func TestVersionFlag(t *testing.T) {
	code, stdout, _ := runRoot(t, "--version")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout.String(), "1.2.3") {
		t.Fatalf("version output %q does not contain the version", stdout.String())
	}
	if !strings.Contains(stdout.String(), "teambrain") {
		t.Fatalf("version output %q does not name the tool", stdout.String())
	}
}

func TestHelpFlag(t *testing.T) {
	code, stdout, _ := runRoot(t, "--help")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	out := stdout.String()
	for _, want := range []string{"teambrain", "Usage:", "Flags:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help output missing %q:\n%s", want, out)
		}
	}
}

func TestUnknownCommandIsUserError(t *testing.T) {
	code, _, stderr := runRoot(t, "frobnicate")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (user error)", code)
	}
	if !strings.Contains(strings.ToLower(stderr.String()), "error") {
		t.Fatalf("expected an error message on stderr, got %q", stderr.String())
	}
}

func TestJSONErrorEnvelopeOnStdout(t *testing.T) {
	// A user error under --json should emit a structured envelope on stdout.
	// `init` with no path and no --here is the simplest such error.
	code, stdout, _ := runRoot(t, "--json", "init")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\n%s", err, stdout.String())
	}
	if env.OK {
		t.Fatal("error envelope should have ok=false")
	}
	if env.Error == nil || env.Error.Code != 1 {
		t.Fatalf("expected error code 1, got %+v", env.Error)
	}
}

func TestPersistentFlagsResolveIntoConfig(t *testing.T) {
	// A persistent flag must resolve into Config and change behavior: --dry-run
	// makes init report what it "would create" instead of writing.
	dir := filepath.Join(t.TempDir(), "brain")
	code, stdout, _ := runRoot(t, "init", "--dry-run", dir)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout=%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "would create") {
		t.Fatalf("--dry-run should resolve into Config and yield a dry-run report, got:\n%s", stdout.String())
	}
}
