package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/capability"
)

func TestDoctorReportsBackend(t *testing.T) {
	code, stdout, _ := runRoot(t, "--json", "doctor")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	var env Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	data, _ := env.Data.(map[string]any)
	if data["healthy"] != true {
		t.Fatalf("clean dir should be healthy, got %v", data)
	}
	if data["active_backend"] != "fs" {
		t.Fatalf("active_backend = %v, want fs", data["active_backend"])
	}
}

func TestDoctorDetectsTamper(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")

	// Source vault with a hook, imported into the repo.
	srcDir := filepath.Join(base, "vault", ".claude")
	srcStore := capability.OpenStore(srcDir)
	if _, err := srcStore.NewHook(capability.HookOptions{Name: "guard", Event: "PreToolUse"}); err != nil {
		t.Fatal(err)
	}
	store := capability.OpenStore(filepath.Join(repo, ".claude"))
	plan, err := store.PlanImport(capability.ImportOptions{
		Name: "guard", Kind: capability.KindHook,
		Sources: []capability.Source{{Label: "v", ClaudeDir: srcDir}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Apply(plan); err != nil {
		t.Fatal(err)
	}

	// Tamper with the imported script.
	if err := os.WriteFile(filepath.Join(repo, ".claude", "hooks", "guard.sh"), []byte("evil"), 0o755); err != nil {
		t.Fatal(err)
	}

	code, stdout, _ := runRoot(t, "--json", "doctor", "--dir", repo)
	if code != 0 {
		t.Fatalf("doctor exit=%d", code)
	}
	if !strings.Contains(stdout.String(), `"healthy": false`) {
		t.Fatalf("doctor should report unhealthy:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "guard") {
		t.Fatalf("doctor should name the drifted capability:\n%s", stdout.String())
	}
}

func TestDoctorReportsRetrievalPath(t *testing.T) {
	code, stdout, _ := runRoot(t, "--json", "doctor")
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	// The retrieval path is host-dependent (obsidian may be installed), but the
	// field must always be reported.
	if !strings.Contains(stdout.String(), `"retrieval"`) {
		t.Fatalf("doctor JSON should report a retrieval path:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"retrieval_available"`) {
		t.Fatalf("doctor JSON should report retrieval_available:\n%s", stdout.String())
	}
}

func TestInitWarnsWhenObsidianAbsent(t *testing.T) {
	// Force Obsidian absent: empty PATH + isolated home (no ~/.claude.json).
	t.Setenv("PATH", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	dir := filepath.Join(t.TempDir(), "brain")
	code, _, stderr := runRoot(t, "init", dir)
	if code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(stderr.String(), "retrieval is unavailable") {
		t.Fatalf("init should warn loudly that retrieval needs Obsidian, stderr=%q", stderr.String())
	}
}

func TestCommandNewAndList(t *testing.T) {
	dir := t.TempDir()
	if code, _, e := runRoot(t, "command", "new", "triage", "--description", "Triage inbox", "--dir", dir); code != 0 {
		t.Fatalf("command new: %s", e.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "commands", "triage.md")); err != nil {
		t.Fatalf("command not written: %v", err)
	}
	code, stdout, _ := runRoot(t, "--json", "command", "list", "--dir", dir)
	if code != 0 || !strings.Contains(stdout.String(), "triage") {
		t.Fatalf("command list = %d\n%s", code, stdout.String())
	}
}
