package vault

import (
	"path"
	"regexp"
	"strings"
)

// wikilinkRe matches an Obsidian wikilink with an optional embed prefix:
// [[target]], [[target#heading|alias]], or ![[target]]. The inner text may not
// contain brackets or newlines, which keeps matching to a single link.
var wikilinkRe = regexp.MustCompile(`!?\[\[[^\[\]\n]+\]\]`)

// wikilink is a parsed link. frag includes its leading '#'; alias includes its
// leading '|'. Both are empty when absent.
type wikilink struct {
	raw    string
	embed  bool
	target string
	frag   string
	alias  string
}

// LinkRewrite records a single link that changed during a move.
type LinkRewrite struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

// LinkIssue records a link that references the moved note but falls outside the
// safely-rewritable subset, so it was left intact for manual review.
type LinkIssue struct {
	Link   string `json:"link"`
	Reason string `json:"reason"`
}

// parseWikilink decomposes a full match such as "![[a/b#h|alias]]".
func parseWikilink(m string) wikilink {
	wl := wikilink{raw: m}
	s := m
	if strings.HasPrefix(s, "!") {
		wl.embed = true
		s = s[1:]
	}
	s = strings.TrimPrefix(s, "[[")
	s = strings.TrimSuffix(s, "]]")
	if i := strings.Index(s, "|"); i >= 0 {
		wl.alias = s[i:]
		s = s[:i]
	}
	if i := strings.Index(s, "#"); i >= 0 {
		wl.frag = s[i:]
		s = s[:i]
	}
	wl.target = s
	return wl
}

// normalizeTarget trims surrounding space and a trailing ".md" so that
// [[note]], [[note.md]] and [[ note ]] compare equal.
func normalizeTarget(t string) string {
	t = strings.TrimSpace(t)
	t = strings.TrimSuffix(t, ".md")
	return t
}

// rewriteLinksForMove rewrites links in content that reference the note moving
// from oldRel to newRel (both vault-relative, extension optional). It returns
// the new content, the rewrites it made, and issues for references it
// deliberately left intact (embeds and block references).
//
// The supported subset is intentionally narrow and documented: plain links by
// full relative path or by bare basename, optionally carrying a heading and/or
// alias. Anything else is reported rather than mangled.
func rewriteLinksForMove(content []byte, oldRel, newRel string) ([]byte, []LinkRewrite, []LinkIssue) {
	oldRel = normalizeTarget(oldRel)
	newRel = normalizeTarget(newRel)
	oldBase := path.Base(oldRel)
	newBase := path.Base(newRel)

	var (
		rewrites []LinkRewrite
		issues   []LinkIssue
	)

	out := wikilinkRe.ReplaceAllStringFunc(string(content), func(m string) string {
		wl := parseWikilink(m)
		target := normalizeTarget(wl.target)

		matchesFull := target == oldRel
		matchesBase := !strings.Contains(target, "/") && target == oldBase
		if !matchesFull && !matchesBase {
			return m // links to other notes are none of our business
		}

		if wl.embed {
			issues = append(issues, LinkIssue{Link: m, Reason: "embed of the moved note; update manually"})
			return m
		}
		if strings.HasPrefix(wl.frag, "#^") {
			issues = append(issues, LinkIssue{Link: m, Reason: "block reference to the moved note; update manually"})
			return m
		}

		newTarget := newBase
		if matchesFull {
			newTarget = newRel
		}
		if newTarget == target {
			return m // e.g. folder-only move referenced by bare name: still resolves
		}

		replacement := "[[" + newTarget + wl.frag + wl.alias + "]]"
		rewrites = append(rewrites, LinkRewrite{Before: m, After: replacement})
		return replacement
	})

	return []byte(out), rewrites, issues
}

// UnresolvedLinks returns the distinct wikilink targets in content that are not
// present in known, in first-appearance order. A target resolves if either its
// normalized form or its basename is a key in known. Used by the promotion
// link-integrity check.
func UnresolvedLinks(content []byte, known map[string]bool) []string {
	var out []string
	seen := make(map[string]bool)

	for _, m := range wikilinkRe.FindAllString(string(content), -1) {
		target := normalizeTarget(parseWikilink(m).target)
		if target == "" {
			continue
		}
		if known[target] || known[path.Base(target)] {
			continue
		}
		if seen[target] {
			continue
		}
		seen[target] = true
		out = append(out, target)
	}
	return out
}
