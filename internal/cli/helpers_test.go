package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActiveBackend(t *testing.T) {
	t.Parallel()
	cases := []struct {
		pref     string
		detected bool
		want     string
	}{
		{"fs", false, "fs"},
		{"fs", true, "fs"},
		{"obsidian", true, "obsidian"},
		{"obsidian", false, "fs"},
		{"auto", true, "obsidian"},
		{"auto", false, "fs"},
	}
	for _, c := range cases {
		if got := activeBackend(c.pref, c.detected); got != c.want {
			t.Errorf("activeBackend(%q,%v) = %q, want %q", c.pref, c.detected, got, c.want)
		}
	}
}

func TestDetectedLabel(t *testing.T) {
	t.Parallel()
	if detectedLabel(true) != "detected" || detectedLabel(false) != "not detected" {
		t.Fatal("detectedLabel wrong")
	}
}

func TestMCPReminder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if mcpReminder(dir) == "" {
		t.Fatal("expected a reminder when no .mcp.json present")
	}
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if mcpReminder(dir) != "" {
		t.Fatal("no reminder expected when .mcp.json exists")
	}
}

func TestClaudeDirOf(t *testing.T) {
	t.Parallel()
	if got := claudeDirOf("/x"); got != filepath.Join("/x", ".claude") {
		t.Errorf("claudeDirOf(/x) = %q", got)
	}
	if got := claudeDirOf(filepath.Join("/x", ".claude")); got != filepath.Join("/x", ".claude") {
		t.Errorf("claudeDirOf already-.claude = %q", got)
	}
}

func TestShort(t *testing.T) {
	t.Parallel()
	if short("abc") != "abc" {
		t.Error("short string should be unchanged")
	}
	long := "sha256:0123456789abcdef0123456789"
	if s := short(long); len(s) >= len(long) || !strings.HasSuffix(s, "…") {
		t.Errorf("short(long) = %q", s)
	}
}

func TestIndent(t *testing.T) {
	t.Parallel()
	got := indent("a\nb", "  ")
	if got != "  a\n  b\n" {
		t.Errorf("indent = %q", got)
	}
}

func TestParseSpecs(t *testing.T) {
	t.Parallel()
	specs := parseSpecs([]string{"a.md", "src.md:dest.md"})
	if len(specs) != 2 {
		t.Fatalf("len = %d", len(specs))
	}
	if specs[0].Src != "a.md" || specs[0].Dest != "" {
		t.Errorf("spec0 = %+v", specs[0])
	}
	if specs[1].Src != "src.md" || specs[1].Dest != "dest.md" {
		t.Errorf("spec1 = %+v", specs[1])
	}
}

func TestResolvePersonalVault(t *testing.T) {
	t.Parallel()
	app := &App{Config: &Config{}}
	if got, _ := resolvePersonalVault(app, "/explicit"); got != "/explicit" {
		t.Errorf("flag should win, got %q", got)
	}
	app.Config.PersonalVault = "/from-config"
	if got, _ := resolvePersonalVault(app, ""); got != "/from-config" {
		t.Errorf("config should be used, got %q", got)
	}
	app.Config.PersonalVault = ""
	cwd, _ := os.Getwd()
	if got, _ := resolvePersonalVault(app, ""); got != cwd {
		t.Errorf("should default to cwd, got %q want %q", got, cwd)
	}
}

func TestSkillUpdateViaCLI(t *testing.T) {
	base := t.TempDir()
	vault := filepath.Join(base, "vault")
	repo := filepath.Join(base, "repo")
	makeSourceVault(t, vault)

	if code, _, e := runRoot(t, "skill", "import", "shared", "--source", vault, "--dir", repo); code != 0 {
		t.Fatalf("import: %s", e.String())
	}
	// Change the source, then update.
	srcSkill := filepath.Join(vault, ".claude", "skills", "shared", "SKILL.md")
	if err := os.WriteFile(srcSkill, []byte("---\nname: shared\ndescription: changed\n---\nnew\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runRoot(t, "skill", "update", "shared", "--source", vault, "--dir", repo)
	if code != 0 {
		t.Fatalf("update exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "updated") {
		t.Fatalf("update output = %q", stdout.String())
	}
}

func TestViewSyncHumanWithDiff(t *testing.T) {
	base := t.TempDir()
	personal := filepath.Join(base, "personal")
	team := filepath.Join(base, "team")
	for _, args := range [][]string{{"init", personal}, {"team", "init", team}, {"team", "bind", team, "--vault", personal}} {
		if code, _, e := runRoot(t, args...); code != 0 {
			t.Fatalf("setup %v: %s", args, e.String())
		}
	}
	// Existing team note + a modified personal version → produces a diff.
	if err := os.WriteFile(filepath.Join(team, "adrs", "x.md"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(personal, "projects", "x.md"), []byte("one\ntwo CHANGED\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, e := runRoot(t, "create-sync", "projects/x.md:adrs/x.md", "--vault", personal); code != 0 {
		t.Fatalf("create-sync: %s", e.String())
	}
	code, stdout, _ := runRoot(t, "view-sync", "--vault", personal)
	if code != 0 {
		t.Fatalf("view-sync exit=%d", code)
	}
	out := stdout.String()
	if !strings.Contains(out, "modified") || !strings.Contains(out, "CHANGED") {
		t.Fatalf("human view-sync should show a diff:\n%s", out)
	}
	if !strings.Contains(out, "link integrity") {
		t.Fatalf("view-sync should report link integrity:\n%s", out)
	}
}
