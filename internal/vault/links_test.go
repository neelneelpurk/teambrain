package vault

import (
	"path/filepath"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/testutil"
)

func TestRewriteLinksForMove(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		content      string
		oldRel       string
		newRel       string
		wantContent  string
		wantRewrites int
		wantIssues   int
	}{
		{
			name:         "bare rename",
			content:      "see [[old]] for details",
			oldRel:       "old",
			newRel:       "new",
			wantContent:  "see [[new]] for details",
			wantRewrites: 1,
		},
		{
			name:         "full path move",
			content:      "see [[projects/old]] now",
			oldRel:       "projects/old",
			newRel:       "archive/new",
			wantContent:  "see [[archive/new]] now",
			wantRewrites: 1,
		},
		{
			name:         "preserves alias",
			content:      "[[old|the old note]]",
			oldRel:       "old",
			newRel:       "new",
			wantContent:  "[[new|the old note]]",
			wantRewrites: 1,
		},
		{
			name:         "preserves heading",
			content:      "[[old#Section]]",
			oldRel:       "old",
			newRel:       "new",
			wantContent:  "[[new#Section]]",
			wantRewrites: 1,
		},
		{
			name:         "preserves heading and alias",
			content:      "[[old#Section|nick]]",
			oldRel:       "old",
			newRel:       "new",
			wantContent:  "[[new#Section|nick]]",
			wantRewrites: 1,
		},
		{
			name:        "embed is unsupported: left intact and reported",
			content:     "![[old]]",
			oldRel:      "old",
			newRel:      "new",
			wantContent: "![[old]]",
			wantIssues:  1,
		},
		{
			name:        "block reference is unsupported: left intact and reported",
			content:     "[[old#^abc123]]",
			oldRel:      "old",
			newRel:      "new",
			wantContent: "[[old#^abc123]]",
			wantIssues:  1,
		},
		{
			name:        "unrelated link untouched",
			content:     "[[somethingelse]] and [[old-friend]]",
			oldRel:      "old",
			newRel:      "new",
			wantContent: "[[somethingelse]] and [[old-friend]]",
		},
		{
			name:         "folder-only move: bare link unchanged, full path rewritten",
			content:      "bare [[old]] and full [[projects/old]]",
			oldRel:       "projects/old",
			newRel:       "archive/old",
			wantContent:  "bare [[old]] and full [[archive/old]]",
			wantRewrites: 1,
		},
		{
			name:        "strips .md in target when matching",
			content:     "[[old.md]]",
			oldRel:      "old",
			newRel:      "new",
			wantContent: "[[new]]",
			// .md is normalized away on match; replacement uses bare new name.
			wantRewrites: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, rewrites, issues := rewriteLinksForMove([]byte(tt.content), tt.oldRel, tt.newRel)
			if string(out) != tt.wantContent {
				t.Errorf("content = %q, want %q", string(out), tt.wantContent)
			}
			if len(rewrites) != tt.wantRewrites {
				t.Errorf("rewrites = %d (%v), want %d", len(rewrites), rewrites, tt.wantRewrites)
			}
			if len(issues) != tt.wantIssues {
				t.Errorf("issues = %d (%v), want %d", len(issues), issues, tt.wantIssues)
			}
		})
	}
}

func TestRewriteLinksGoldenFixture(t *testing.T) {
	t.Parallel()

	const fixture = `---
title: Referencing note
---
# Links

A normal link: [[projects/old]].
With alias: [[projects/old|Old Project]].
With heading: [[projects/old#Goals]].
An embed (unsupported): ![[projects/old]].
A block ref (unsupported): [[projects/old#^block1]].
An unrelated link: [[areas/health]].
`
	out, _, issues := rewriteLinksForMove([]byte(fixture), "projects/old", "archive/old-project")
	testutil.AssertGolden(t, filepath.Join("testdata", "links", "rewritten.golden"), out)

	if len(issues) != 2 {
		t.Fatalf("expected 2 unsupported-construct issues (embed + block ref), got %d: %v", len(issues), issues)
	}
}

func TestUnresolvedLinks(t *testing.T) {
	t.Parallel()

	content := []byte("links: [[exists]] [[missing]] [[also-missing#h]] ![[embedmissing]]")
	known := map[string]bool{"exists": true}

	got := UnresolvedLinks(content, known)
	want := []string{"missing", "also-missing", "embedmissing"}
	if len(got) != len(want) {
		t.Fatalf("UnresolvedLinks = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("UnresolvedLinks[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
