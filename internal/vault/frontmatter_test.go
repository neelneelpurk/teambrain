package vault

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/testutil"
)

const noteWithFrontmatter = `---
title: My Note
tags:
  - alpha
  - beta
status: draft
---
# Heading

Body text with a [[link]].
`

const noteNoFrontmatter = `# Just a body

No frontmatter here. [[other]]
`

func TestParseDocumentWithFrontmatter(t *testing.T) {
	t.Parallel()

	doc, err := ParseDocument([]byte(noteWithFrontmatter))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if !doc.HasFrontmatter {
		t.Fatal("expected HasFrontmatter=true")
	}
	if got, ok := doc.Get("title"); !ok || got != "My Note" {
		t.Fatalf("Get(title) = %q,%v; want \"My Note\",true", got, ok)
	}
	if got, ok := doc.Get("status"); !ok || got != "draft" {
		t.Fatalf("Get(status) = %q,%v", got, ok)
	}
	wantBody := "# Heading\n\nBody text with a [[link]].\n"
	if string(doc.Body) != wantBody {
		t.Fatalf("Body = %q, want %q", string(doc.Body), wantBody)
	}
	if keys := doc.Keys(); len(keys) != 3 || keys[0] != "title" || keys[2] != "status" {
		t.Fatalf("Keys() = %v, want [title tags status] in order", keys)
	}
}

func TestParseDocumentCRLF(t *testing.T) {
	t.Parallel()

	// A note authored on Windows with CRLF line endings must still parse.
	crlf := "---\r\ntitle: Win\r\nstatus: draft\r\n---\r\n# Body\r\n\r\ntext\r\n"
	doc, err := ParseDocument([]byte(crlf))
	if err != nil {
		t.Fatalf("ParseDocument(CRLF): %v", err)
	}
	if !doc.HasFrontmatter {
		t.Fatal("CRLF frontmatter not detected")
	}
	if got, _ := doc.Get("title"); got != "Win" {
		t.Fatalf("title = %q, want Win", got)
	}
	if !strings.Contains(string(doc.Body), "# Body") {
		t.Fatalf("body not preserved: %q", doc.Body)
	}
}

func TestParseDocumentNoFrontmatter(t *testing.T) {
	t.Parallel()

	doc, err := ParseDocument([]byte(noteNoFrontmatter))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if doc.HasFrontmatter {
		t.Fatal("expected HasFrontmatter=false")
	}
	if _, ok := doc.Get("title"); ok {
		t.Fatal("Get on a frontmatter-less doc should report ok=false")
	}
	if string(doc.Body) != noteNoFrontmatter {
		t.Fatalf("Body should equal the whole input")
	}
}

func TestSetPreservesOrderAndBody(t *testing.T) {
	t.Parallel()

	doc, err := ParseDocument([]byte(noteWithFrontmatter))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	// Update an existing key and append a new one.
	if err := doc.Set("status", "published"); err != nil {
		t.Fatalf("Set status: %v", err)
	}
	if err := doc.Set("promoted", true); err != nil {
		t.Fatalf("Set promoted: %v", err)
	}

	got, err := doc.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	testutil.AssertGolden(t, filepath.Join("testdata", "frontmatter", "set_existing_and_new.golden"), got)
}

func TestSetOnDocWithoutFrontmatterCreatesBlock(t *testing.T) {
	t.Parallel()

	doc, err := ParseDocument([]byte(noteNoFrontmatter))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if err := doc.Set("title", "Added"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !doc.HasFrontmatter {
		t.Fatal("Set should have created a frontmatter block")
	}
	got, err := doc.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	testutil.AssertGolden(t, filepath.Join("testdata", "frontmatter", "created_block.golden"), got)
}

func TestRenderRoundTripsUnchanged(t *testing.T) {
	t.Parallel()

	doc, err := ParseDocument([]byte(noteWithFrontmatter))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	got, err := doc.Render()
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// A parse→render round-trip with no edits must preserve the body exactly and
	// keep all keys; we pin the exact bytes with a golden.
	testutil.AssertGolden(t, filepath.Join("testdata", "frontmatter", "roundtrip.golden"), got)
}
