package capability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/vault"
)

func TestNewUsesDefaultDescriptions(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)

	if _, err := s.NewSkill("s1", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NewAgent("a1", ""); err != nil {
		t.Fatal(err)
	}

	skill, _ := os.ReadFile(filepath.Join(dir, "skills", "s1", "SKILL.md"))
	if d := mustDesc(t, skill); !strings.Contains(d, "TODO") {
		t.Errorf("skill default description = %q", d)
	}
	agent, _ := os.ReadFile(filepath.Join(dir, "agents", "a1.md"))
	if d := mustDesc(t, agent); !strings.Contains(d, "TODO") {
		t.Errorf("agent default description = %q", d)
	}
}

func TestDescriptionWithColonStaysValidYAML(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)

	const desc = "When the user says: summarize, do X: then Y"
	if _, err := s.NewSkill("colon", desc); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "skills", "colon", "SKILL.md"))
	doc, err := vault.ParseDocument(raw)
	if err != nil {
		t.Fatalf("colon description produced invalid frontmatter: %v\n%s", err, raw)
	}
	if got, _ := doc.Get("description"); got != desc {
		t.Fatalf("description round-trip = %q, want %q", got, desc)
	}
}

func TestOpenStoreSystemClock(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), ".claude")
	s := OpenStore(dir)
	if s.Dir() != dir {
		t.Fatalf("Dir() = %q, want %q", s.Dir(), dir)
	}
	if _, err := s.NewHook(HookOptions{Name: "h", Event: "Stop"}); err != nil {
		t.Fatalf("NewHook with system clock: %v", err)
	}
}

func TestListIncludesCommands(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)
	// commands are authored elsewhere (Phase 8); list should still surface them.
	cmdPath := filepath.Join(dir, "commands", "review.md")
	if err := os.MkdirAll(filepath.Dir(cmdPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cmdPath, []byte("---\ndescription: A command\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	items, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	it, ok := findItem(items, "review")
	if !ok || it.Kind != string(KindCommand) {
		t.Fatalf("command not listed: %+v", items)
	}
}

func TestUnmergeHookPrunesEmptyEvent(t *testing.T) {
	t.Parallel()
	// Only one hook under an event; removing it should drop the event and the
	// now-empty hooks object.
	settings, _, err := MergeHook(nil, HookRegistration{Event: "Stop", Command: "only.sh"})
	if err != nil {
		t.Fatal(err)
	}
	out, changed, err := UnmergeHook(settings, "only.sh")
	if err != nil || !changed {
		t.Fatalf("UnmergeHook changed=%v err=%v", changed, err)
	}
	if strings.Contains(string(out), "Stop") || strings.Contains(string(out), "only.sh") {
		t.Fatalf("event/hook should be pruned: %s", out)
	}
}

func mustDesc(t *testing.T, content []byte) string {
	t.Helper()
	doc, err := vault.ParseDocument(content)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	d, _ := doc.Get("description")
	return d
}
