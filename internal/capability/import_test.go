package capability

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/manifest"
)

func stringsContains(s, sub string) bool { return strings.Contains(s, sub) }

// snapshot records every regular file under dir as relpath -> contents.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return out
}

// diffSnapshots returns a human description of differences, or "" if identical.
func diffSnapshots(before, after map[string]string) string {
	var diffs []string
	for path, b := range before {
		a, ok := after[path]
		switch {
		case !ok:
			diffs = append(diffs, "removed: "+path)
		case a != b:
			diffs = append(diffs, fmt.Sprintf("changed: %s\n  before=%q\n  after=%q", path, b, a))
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			diffs = append(diffs, "added: "+path)
		}
	}
	sort.Strings(diffs)
	return strings.Join(diffs, "\n")
}

// sourceVault builds a .claude store at label with a skill, an agent, and a hook.
func sourceVault(t *testing.T, label string) Source {
	t.Helper()
	dir := filepath.Join(t.TempDir(), label, ".claude")
	s := OpenStore(dir)
	if _, err := s.NewSkill("shared", "A shared skill."); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NewAgent("shared", "A shared agent."); err != nil {
		t.Fatal(err)
	}
	if _, err := s.NewHook(HookOptions{Name: "guard", Event: "PreToolUse", Matcher: "Bash"}); err != nil {
		t.Fatal(err)
	}
	return Source{Label: label, ClaudeDir: dir}
}

func targetStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "repo", ".claude")
	return OpenStore(dir), dir
}

func TestImportSkillCopiesAndRecordsOwnership(t *testing.T) {
	t.Parallel()
	src := sourceVault(t, "personal")
	s, dir := targetStore(t)

	plan, err := s.PlanImport(ImportOptions{Name: "shared", Kind: KindSkill, Sources: []Source{src}})
	if err != nil {
		t.Fatalf("PlanImport: %v", err)
	}
	res, err := s.Apply(plan)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Changed {
		t.Error("first import should report a change")
	}

	if _, err := os.Stat(filepath.Join(dir, "skills", "shared", "SKILL.md")); err != nil {
		t.Fatalf("skill not copied: %v", err)
	}
	man, _ := manifest.LoadClaude(dir)
	entry, ok := man.Find("shared")
	if !ok || entry.Type != "skill" || entry.Source != "personal" || entry.Checksum == "" || entry.Mode != "copy" {
		t.Fatalf("ownership wrong: %+v", entry)
	}

	// Re-import is idempotent.
	plan2, _ := s.PlanImport(ImportOptions{Name: "shared", Kind: KindSkill, Sources: []Source{src}})
	res2, err := s.Apply(plan2)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Changed {
		t.Error("re-import of identical content should report no change")
	}
}

func TestImportHookMergesSettingsAndPreservesForeign(t *testing.T) {
	t.Parallel()
	src := sourceVault(t, "team")
	s, dir := targetStore(t)

	// Pre-existing foreign hook in the target repo.
	foreign, _, _ := MergeHook(nil, HookRegistration{Event: "Stop", Command: "repo-local.sh"})
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), foreign, 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := s.PlanImport(ImportOptions{Name: "guard", Kind: KindHook, Sources: []Source{src}})
	if err != nil {
		t.Fatalf("PlanImport: %v", err)
	}
	if plan.ScriptPreview == "" {
		t.Error("hook plan should include a script preview for confirmation")
	}
	if plan.HookEvent != "PreToolUse" {
		t.Errorf("event = %q, want PreToolUse (read from source manifest)", plan.HookEvent)
	}
	if _, err := s.Apply(plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	settings := readFile(t, filepath.Join(dir, "settings.json"))
	s2 := string(settings)
	for _, must := range []string{"repo-local.sh", "guard.sh", "PreToolUse", "Stop"} {
		if !stringsContains(s2, must) {
			t.Errorf("settings lost %q after hook import:\n%s", must, s2)
		}
	}
	man, _ := manifest.LoadClaude(dir)
	if entry, ok := man.Find("guard"); !ok || entry.Event != "PreToolUse" {
		t.Fatalf("hook ownership wrong: %+v", entry)
	}
}

func TestImportAmbiguousRequiresFrom(t *testing.T) {
	t.Parallel()
	a := sourceVault(t, "personal")
	b := sourceVault(t, "team")
	s, _ := targetStore(t)

	// "shared" exists in both: ambiguous without --from.
	_, err := s.PlanImport(ImportOptions{Name: "shared", Kind: KindSkill, Sources: []Source{a, b}})
	if err == nil {
		t.Fatal("expected an ambiguity error")
	}

	// --from disambiguates.
	plan, err := s.PlanImport(ImportOptions{Name: "shared", Kind: KindSkill, Sources: []Source{a, b}, From: "team"})
	if err != nil {
		t.Fatalf("with From: %v", err)
	}
	if plan.SourceLabel != "team" {
		t.Fatalf("SourceLabel = %q, want team", plan.SourceLabel)
	}
}

func TestImportNotFound(t *testing.T) {
	t.Parallel()
	src := sourceVault(t, "personal")
	s, _ := targetStore(t)
	if _, err := s.PlanImport(ImportOptions{Name: "ghost", Kind: KindSkill, Sources: []Source{src}}); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestImportLinkMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require privileges on Windows")
	}
	t.Parallel()
	src := sourceVault(t, "personal")
	s, dir := targetStore(t)

	plan, err := s.PlanImport(ImportOptions{Name: "shared", Kind: KindAgent, Sources: []Source{src}, Mode: "link"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(plan); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "agents", "shared.md")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected a symlink in link mode")
	}
	man, _ := manifest.LoadClaude(dir)
	if entry, _ := man.Find("shared"); entry.Mode != "link" {
		t.Fatalf("mode = %q, want link", entry.Mode)
	}
}

func TestUninstallRemovesOnlyOwned(t *testing.T) {
	t.Parallel()
	src := sourceVault(t, "personal")
	s, dir := targetStore(t)

	// A foreign skill the user wrote directly (not owned by teambrain).
	foreignSkill := filepath.Join(dir, "skills", "mine", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(foreignSkill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(foreignSkill, []byte("---\nname: mine\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, _ := s.PlanImport(ImportOptions{Name: "shared", Kind: KindSkill, Sources: []Source{src}})
	if _, err := s.Apply(plan); err != nil {
		t.Fatal(err)
	}

	res, err := s.Uninstall("shared")
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !res.Changed {
		t.Error("uninstall should report a change")
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "shared", "SKILL.md")); !os.IsNotExist(err) {
		t.Error("owned skill should be removed")
	}
	if _, err := os.Stat(foreignSkill); err != nil {
		t.Error("foreign skill must be untouched")
	}

	// Uninstalling something not owned is an error.
	if _, err := s.Uninstall("mine"); err == nil {
		t.Error("uninstalling a non-owned capability should error")
	}
}

// TestImportUninstallByteIdentical is the Phase 4 acceptance scenario.
func TestImportUninstallByteIdentical(t *testing.T) {
	t.Parallel()
	src := sourceVault(t, "personal")
	s, dir := targetStore(t)

	// Establish a realistic pre-existing repo: a foreign skill and a foreign
	// hook in canonical settings.json.
	if err := os.MkdirAll(filepath.Join(dir, "skills", "mine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "mine", "SKILL.md"), []byte("---\nname: mine\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	foreignSettings, _, _ := MergeHook(nil, HookRegistration{Event: "Stop", Matcher: "", Command: "repo-local.sh"})
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), foreignSettings, 0o644); err != nil {
		t.Fatal(err)
	}

	before := snapshot(t, dir)

	// Import a skill and a hook, then uninstall both.
	for _, kind := range []Kind{KindSkill, KindHook} {
		name := "shared"
		if kind == KindHook {
			name = "guard"
		}
		plan, err := s.PlanImport(ImportOptions{Name: name, Kind: kind, Sources: []Source{src}})
		if err != nil {
			t.Fatalf("PlanImport %s: %v", kind, err)
		}
		if _, err := s.Apply(plan); err != nil {
			t.Fatalf("Apply %s: %v", kind, err)
		}
	}
	if _, err := s.Uninstall("shared"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Uninstall("guard"); err != nil {
		t.Fatal(err)
	}

	after := snapshot(t, dir)
	if diff := diffSnapshots(before, after); diff != "" {
		t.Fatalf("repo not byte-identical after import→uninstall:\n%s", diff)
	}
}

func TestUpdateDetectsDrift(t *testing.T) {
	t.Parallel()
	src := sourceVault(t, "personal")
	s, dir := targetStore(t)

	plan, _ := s.PlanImport(ImportOptions{Name: "shared", Kind: KindSkill, Sources: []Source{src}})
	if _, err := s.Apply(plan); err != nil {
		t.Fatal(err)
	}

	// Change the source skill, then update.
	srcSkill := filepath.Join(src.ClaudeDir, "skills", "shared", "SKILL.md")
	if err := os.WriteFile(srcSkill, []byte("---\nname: shared\ndescription: changed\n---\nnew body\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := s.Update("shared", []Source{src})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !res.Changed {
		t.Error("update should detect the drifted source")
	}
	got := readFile(t, filepath.Join(dir, "skills", "shared", "SKILL.md"))
	if !stringsContains(string(got), "new body") {
		t.Error("update should have refreshed the imported file")
	}
}
