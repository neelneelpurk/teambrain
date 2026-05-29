package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// runRoot executes the root command with args and captured IO, returning the
// exit code and the stdout/stderr buffers.
func runRoot(t *testing.T, args ...string) (code int, stdout, stderr *bytes.Buffer) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())  // isolate from the real user config
	t.Setenv("TEAMBRAIN_VAULT_BACKEND", "fs") // deterministic regardless of a host Obsidian
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

func TestInvalidBackendFlagIsUserError(t *testing.T) {
	code, _, stderr := runRoot(t, "--vault-backend", "sqlite", "doctor")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "vault_backend") && !strings.Contains(stderr.String(), "vault-backend") {
		t.Fatalf("error should mention the backend, got %q", stderr.String())
	}
}

func TestJSONErrorEnvelopeOnStdout(t *testing.T) {
	// A precondition failure under --json should emit a structured envelope.
	code, stdout, _ := runRoot(t, "--json", "--vault-backend", "bogus", "doctor")
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
	// doctor echoes the resolved backend as JSON data; assert the flag wins.
	code, stdout, _ := runRoot(t, "--json", "--vault-backend", "fs", "doctor")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stdout=%s", code, stdout.String())
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout.String())
	}
	data, _ := env.Data.(map[string]any)
	if data["vault_backend"] != "fs" {
		t.Fatalf("vault_backend = %v, want fs", data["vault_backend"])
	}
	if data["active_backend"] != "fs" {
		t.Fatalf("active_backend = %v, want fs (flag forces fs)", data["active_backend"])
	}
}
