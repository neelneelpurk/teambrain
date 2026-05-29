package capability

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/neelneelpurk/teambrain/internal/clock"
	"github.com/neelneelpurk/teambrain/internal/manifest"
	"github.com/neelneelpurk/teambrain/internal/testutil"
)

func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), ".claude")
	clk := clock.NewFake(time.Date(2026, time.May, 29, 12, 0, 0, 0, time.UTC))
	return OpenStoreWithClock(dir, clk), dir
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

func findItem(items []ListItem, name string) (ListItem, bool) {
	for _, it := range items {
		if it.Name == name {
			return it, true
		}
	}
	return ListItem{}, false
}

func TestNewSkillGoldenAndDiscoverable(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)

	res, err := s.NewSkill("daily-review", "Summarize today's notes and surface open loops.")
	if err != nil {
		t.Fatalf("NewSkill: %v", err)
	}
	if res.Path != "skills/daily-review/SKILL.md" {
		t.Fatalf("unexpected path %q", res.Path)
	}
	testutil.AssertGolden(t, filepath.Join("testdata", "new", "skill.golden"),
		readFile(t, filepath.Join(dir, res.Path)))

	items, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	it, ok := findItem(items, "daily-review")
	if !ok || it.Kind != string(KindSkill) {
		t.Fatalf("skill not listed: %+v", items)
	}
	if it.Description == "" {
		t.Error("listed skill should carry its description from frontmatter")
	}
}

func TestNewAgentGoldenAndDiscoverable(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)

	res, err := s.NewAgent("researcher", "Investigate open questions across the vault.")
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	if res.Path != "agents/researcher.md" {
		t.Fatalf("unexpected path %q", res.Path)
	}
	testutil.AssertGolden(t, filepath.Join("testdata", "new", "agent.golden"),
		readFile(t, filepath.Join(dir, res.Path)))

	items, _ := s.List()
	if it, ok := findItem(items, "researcher"); !ok || it.Kind != string(KindAgent) {
		t.Fatalf("agent not listed: %+v", items)
	}
}

func TestNewHookWritesScriptSettingsAndManifest(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)

	res, err := s.NewHook(HookOptions{
		Name:    "format-go",
		Event:   "PostToolUse",
		Matcher: "Edit|Write",
	})
	if err != nil {
		t.Fatalf("NewHook: %v", err)
	}

	// Script written and executable.
	scriptPath := filepath.Join(dir, "hooks", "format-go.sh")
	testutil.AssertGolden(t, filepath.Join("testdata", "new", "hook.sh.golden"), readFile(t, scriptPath))
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(scriptPath)
		if info.Mode()&0o111 == 0 {
			t.Error("hook script should be executable")
		}
	}

	// settings.json merged.
	testutil.AssertGolden(t, filepath.Join("testdata", "new", "hook_settings.golden"),
		readFile(t, filepath.Join(dir, "settings.json")))

	// Ownership manifest records the hook with a checksum.
	man, err := manifest.LoadClaude(dir)
	if err != nil {
		t.Fatalf("LoadClaude: %v", err)
	}
	entry, ok := man.Find("format-go")
	if !ok {
		t.Fatalf("hook missing from manifest: %+v", man.Capabilities)
	}
	if entry.Type != "hook" || entry.Event != "PostToolUse" || entry.Checksum == "" {
		t.Fatalf("manifest entry wrong: %+v", entry)
	}
	if res.Changed != true {
		t.Error("first NewHook should report a change")
	}

	// Listed as a hook with its event annotated from the manifest.
	items, _ := s.List()
	if it, ok := findItem(items, "format-go"); !ok || it.Kind != string(KindHook) || it.Event != "PostToolUse" {
		t.Fatalf("hook not listed correctly: %+v", items)
	}
}

func TestNewHookIsIdempotent(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)
	opts := HookOptions{Name: "notify", Event: "Stop"}

	if _, err := s.NewHook(opts); err != nil {
		t.Fatal(err)
	}
	settingsAfterFirst := readFile(t, filepath.Join(dir, "settings.json"))

	res2, err := s.NewHook(opts)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Changed {
		t.Error("second identical NewHook should report no change")
	}
	settingsAfterSecond := readFile(t, filepath.Join(dir, "settings.json"))
	if string(settingsAfterFirst) != string(settingsAfterSecond) {
		t.Error("settings.json should be unchanged on idempotent re-run")
	}
	man, _ := manifest.LoadClaude(dir)
	if len(man.Capabilities) != 1 {
		t.Fatalf("manifest should have exactly one hook, got %d", len(man.Capabilities))
	}
}

func TestNewHookPreservesForeignSettings(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A pre-existing, foreign settings.json the user already had.
	foreign := `{
  "permissions": { "allow": ["Bash(ls:*)"] },
  "hooks": { "PreToolUse": [ { "matcher": "Bash", "hooks": [ { "type": "command", "command": "guard.sh" } ] } ] }
}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(foreign), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := s.NewHook(HookOptions{Name: "mine", Event: "PostToolUse"}); err != nil {
		t.Fatal(err)
	}
	out := string(readFile(t, filepath.Join(dir, "settings.json")))
	for _, must := range []string{"guard.sh", "permissions", "Bash(ls:*)", "mine.sh", "PreToolUse", "PostToolUse"} {
		if !strings.Contains(out, must) {
			t.Errorf("foreign settings lost %q after hook new:\n%s", must, out)
		}
	}
}

// TestListReflectsOnDiskTruth is the Phase 3 acceptance scenario: list is a live
// filesystem scan with no cache to desync.
func TestListReflectsOnDiskTruth(t *testing.T) {
	t.Parallel()
	s, dir := newStore(t)

	if _, err := s.NewHook(HookOptions{Name: "watch", Event: "Stop"}); err != nil {
		t.Fatal(err)
	}
	items, _ := s.List()
	if _, ok := findItem(items, "watch"); !ok {
		t.Fatal("hook should be listed after creation")
	}

	// Delete the script file on disk; list must no longer show it, even though
	// the manifest still records it.
	if err := os.Remove(filepath.Join(dir, "hooks", "watch.sh")); err != nil {
		t.Fatal(err)
	}
	items, _ = s.List()
	if _, ok := findItem(items, "watch"); ok {
		t.Fatal("list should reflect the deletion (no stale cache)")
	}
}

func TestNewRejectsBadNamesAndDuplicates(t *testing.T) {
	t.Parallel()
	s, _ := newStore(t)

	for _, bad := range []string{"", "../escape", "a/b", "..", "with space"} {
		if _, err := s.NewSkill(bad, "x"); err == nil {
			t.Errorf("NewSkill(%q) should be rejected", bad)
		}
	}
	if _, err := s.NewSkill("dup", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NewSkill("dup", "x"); err == nil {
		t.Error("creating a duplicate skill should error")
	}
}
