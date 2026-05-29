package vault

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// installObsidianStub writes a stub `obsidian` onto PATH that returns canned
// JSON per verb and logs its arguments. It returns the path to the args log.
func installObsidianStub(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("stub obsidian uses a POSIX shell script")
	}
	dir := t.TempDir()
	logFile := filepath.Join(dir, "args.log")

	script := `#!/bin/sh
echo "$@" >> "` + logFile + `"
case "$1" in
  read)       echo '{"content":"hello from obsidian"}' ;;
  create)     echo '{"ok":true}' ;;
  move)       echo '{"rewrites":[{"before":"[[old]]","after":"[[new]]"}],"files_touched":["daily/n.md"]}' ;;
  search)     echo '{"results":[{"path":"x.md","score":0.9}]}' ;;
  backlinks)  echo '{"backlinks":["a.md","b.md"]}' ;;
  outline)    echo '{"headings":["# Title","## Section"]}' ;;
  unresolved) echo '{"unresolved":["ghost","phantom"]}' ;;
  property)   echo '{"ok":true}' ;;
  *)          echo '{}' ;;
esac
`
	stub := filepath.Join(dir, "obsidian")
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logFile
}

func TestObsidianReadParsesContent(t *testing.T) {
	logFile := installObsidianStub(t)
	o, err := NewObsidianCLI("/vault")
	if err != nil {
		t.Fatal(err)
	}
	got, err := o.Read("notes/a.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != "hello from obsidian" {
		t.Fatalf("Read = %q", got)
	}
	assertLogged(t, logFile, "read", "--path", "notes/a.md", "--json")
}

func TestObsidianWriteSendsCreate(t *testing.T) {
	logFile := installObsidianStub(t)
	o, _ := NewObsidianCLI("/vault")
	if err := o.Write("notes/a.md", []byte("body")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	assertLogged(t, logFile, "create", "--path", "notes/a.md")
}

func TestObsidianMoveIsLinkPreserving(t *testing.T) {
	logFile := installObsidianStub(t)
	o, _ := NewObsidianCLI("/vault")
	report, err := o.Move("projects/old.md", "archive/new.md")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if len(report.Rewrites) != 1 || report.Rewrites[0].After != "[[new]]" {
		t.Fatalf("move report = %+v", report)
	}
	if len(report.FilesTouched) != 1 {
		t.Fatalf("files touched = %v", report.FilesTouched)
	}
	assertLogged(t, logFile, "move", "--from", "projects/old.md", "--to", "archive/new.md")
}

func TestObsidianQueryVerbs(t *testing.T) {
	logFile := installObsidianStub(t)
	o, _ := NewObsidianCLI("/vault")

	if r, err := o.Search("query terms"); err != nil || len(r) != 1 || r[0].Path != "x.md" {
		t.Fatalf("Search = %v, %v", r, err)
	}
	if b, err := o.Backlinks("a.md"); err != nil || len(b) != 2 {
		t.Fatalf("Backlinks = %v, %v", b, err)
	}
	if h, err := o.Outline("a.md"); err != nil || len(h) != 2 {
		t.Fatalf("Outline = %v, %v", h, err)
	}
	if u, err := o.Unresolved(); err != nil || len(u) != 2 || u[0] != "ghost" {
		t.Fatalf("Unresolved = %v, %v", u, err)
	}
	if err := o.SetProperty("a.md", "status", "done"); err != nil {
		t.Fatalf("SetProperty: %v", err)
	}

	assertLogged(t, logFile, "search", "--query", "query terms")
	assertLogged(t, logFile, "unresolved", "--json")
	assertLogged(t, logFile, "property", "set", "--key", "status", "--value", "done")
}

// inheritedFromFSDirect proves the obsidian backend still satisfies the full
// Vault interface (the unimproved methods come from the embedded FSDirect).
func TestObsidianSatisfiesVault(t *testing.T) {
	t.Parallel()
	o, err := NewObsidianCLI(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var _ Vault = o
	if o.Backend() != "obsidian" {
		t.Fatalf("Backend() = %q", o.Backend())
	}
}

func assertLogged(t *testing.T, logFile string, wantArgs ...string) {
	t.Helper()
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("read stub log: %v", err)
	}
	log := string(data)
	for _, want := range wantArgs {
		if !strings.Contains(log, want) {
			t.Errorf("stub log missing %q\nlog:\n%s", want, log)
		}
	}
}
