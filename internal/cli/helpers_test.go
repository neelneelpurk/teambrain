package cli

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestRetrievalStatus(t *testing.T) {
	// Isolate from a host Obsidian CLI and the real ~/.claude.json.
	t.Setenv("PATH", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows home
	dir := t.TempDir()

	// Neither CLI nor an Obsidian MCP -> unavailable.
	if path, ok := retrievalStatus(dir); ok || path != retrievalNone {
		t.Fatalf("expected unavailable, got %q,%v", path, ok)
	}

	// An obsidian-named MCP server in .mcp.json -> obsidian-mcp.
	writeFile(t, filepath.Join(dir, ".mcp.json"), `{"mcpServers":{"obsidian":{"command":"x"}}}`)
	if path, ok := retrievalStatus(dir); !ok || path != retrievalMCP {
		t.Fatalf("expected obsidian-mcp, got %q,%v", path, ok)
	}

	// A non-obsidian MCP server does not count.
	writeFile(t, filepath.Join(dir, ".mcp.json"), `{"mcpServers":{"github":{"command":"x"}}}`)
	if path, ok := retrievalStatus(dir); ok || path != retrievalNone {
		t.Fatalf("non-obsidian MCP should not count, got %q,%v", path, ok)
	}

	// The Obsidian CLI on PATH -> obsidian-cli (POSIX exec bit; skip on Windows).
	if runtime.GOOS != "windows" {
		bin := t.TempDir()
		if err := os.WriteFile(filepath.Join(bin, "obsidian"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin)
		if path, ok := retrievalStatus(dir); !ok || path != retrievalCLI {
			t.Fatalf("expected obsidian-cli, got %q,%v", path, ok)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
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
	for _, args := range [][]string{{"init", personal}, {"team", "init", team}, {"team", "bind", team, "--name", "eng", "--vault", personal}} {
		if code, _, e := runRoot(t, args...); code != 0 {
			t.Fatalf("setup %v: %s", args, e.String())
		}
	}
	// Existing team note + a modified, tagged personal version at the same path
	// (via teambrain_dest) → produces a diff.
	if err := os.WriteFile(filepath.Join(team, "adrs", "x.md"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	note := "---\nteambrains: [eng]\nteambrain_dest: adrs/x.md\n---\none\ntwo CHANGED\n"
	if err := os.WriteFile(filepath.Join(personal, "projects", "x.md"), []byte(note), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, e := runRoot(t, "create-sync", "projects/x.md", "--vault", personal); code != 0 {
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
