package mcp

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/neelneelpurk/teambrain/internal/obsidianapi"
)

// TestMCPEndToEndAgainstLiveObsidian is the live counterpart to
// TestMCPEndToEndAgainstMockObsidian: instead of an httptest stand-in, it drives
// the full MCP stack — the real obsidianapi HTTP client, the real tool handlers,
// the real MCP protocol — against an *actually running* Obsidian reachable
// through the Local REST API plugin. It proves the parts a mock cannot: the TLS
// handshake against the plugin's self-signed certificate, bearer auth, and that
// every endpoint's real JSON shape and search/index behavior matches what the
// tools assume.
//
// To make assertions deterministic it seeds a small "fake brain" — nested
// folders of notes with frontmatter (teambrains:/tags), headings, and
// [[wikilinks]] — into the live vault under a unique per-run "_teambrain_it/<id>"
// subtree, and deletes it on cleanup. Seeding goes through a test-only raw HTTP
// PUT/DELETE — never the obsidianapi.Client — so the shipped client and MCP tools
// stay strictly read-only (the project invariant). Unique per-run tokens keep the
// assertions robust against whatever else is already in the vault.
//
// It is opt-in: with OBSIDIAN_API_KEY unset it skips, so `make ci` stays
// hermetic. To run it against your vault, enable the "Local REST API" plugin in
// Obsidian, then:
//
//	OBSIDIAN_API_KEY=<plugin key> go test ./internal/mcp/ \
//	    -run TestMCPEndToEndAgainstLiveObsidian -v
//
// Endpoint overrides match the production server's OBSIDIAN_* contract
// (OBSIDIAN_HOST, OBSIDIAN_PORT, OBSIDIAN_PROTOCOL, OBSIDIAN_VERIFY_TLS,
// OBSIDIAN_CA_CERT); unset, they fall back to the plugin's localhost defaults.
// Setting OBSIDIAN_API_KEY is an explicit "Obsidian is running" opt-in, so a
// connection failure here is a real failure, not a skip.
func TestMCPEndToEndAgainstLiveObsidian(t *testing.T) {
	if os.Getenv("OBSIDIAN_API_KEY") == "" {
		t.Skip("set OBSIDIAN_API_KEY (from the Obsidian Local REST API plugin) to run the live integration test")
	}

	// Read path: the production, read-only client and MCP tools.
	client, err := obsidianapi.New(liveConfigFromEnv(t))
	if err != nil {
		t.Fatalf("build live Obsidian client: %v", err)
	}
	const vaultName = "personal"
	cs := connectVaults(t, map[string]obsidianapi.Client{vaultName: client}, vaultName)

	// list_vaults is pure config; it also confirms the server is up before we
	// touch the live vault.
	var lv ListVaultsOutput
	structured(t, call(t, cs, "list_vaults", nil), &lv)
	if lv.Default != vaultName || len(lv.Vaults) != 1 {
		t.Fatalf("list_vaults = %+v", lv)
	}

	// Seed a small fake brain into the live vault via raw HTTP PUT (the write path
	// the read-only client deliberately lacks), under a unique per-run subtree we
	// delete on cleanup.
	writeHC, base := liveWriteEndpoint(t)
	apiKey := os.Getenv("OBSIDIAN_API_KEY")
	sfx := randSuffix(t)
	dir := "_teambrain_it/" + sfx // unique per-run subtree
	token := "tbittoken" + sfx    // a unique word to full-text search for
	tag := "tbittag" + sfx        // a unique tag, shared by two notes
	anchor := "TbitStyle" + sfx   // a unique [[wikilink]] target note

	// The fake brain: nested folders, frontmatter, a shared tag, a cross-note
	// wikilink, and two promotion candidates routed to different teams. Vault paths
	// are dir + "/" + rel.
	adrRel := "projects/oauth-adr.md"
	styleRel := "conventions/" + anchor + ".md"
	ideaRel := "inbox/idea.md"
	dailyRel := "daily/2026-05-30.md"
	notes := []struct{ rel, content string }{
		{adrRel, fmt.Sprintf(`---
title: OAuth ADR
teambrains: [eng, design]
tags: [decision, %s]
---
# OAuth ADR

## Decision

We adopt OAuth (%s). See [[%s]] for conventions.

## Consequences

Tradeoffs apply.
`, tag, token, anchor)},
		{styleRel, "# Conventions\n\nShared team conventions, linked from the OAuth ADR.\n"},
		{ideaRel, fmt.Sprintf("---\nteambrains: [design]\n---\nA shared idea worth promoting to design. #%s\n", tag)},
		{dailyRel, fmt.Sprintf("# 2026-05-30\n\nWorked on OAuth (%s) today. #daily\n", token)},
	}
	for _, n := range notes {
		seedNote(t, writeHC, base, apiKey, dir+"/"+n.rel, n.content)
	}
	t.Cleanup(func() {
		for _, n := range notes {
			tryDelete(t, writeHC, base, apiKey, dir+"/"+n.rel)
		}
		// Deleting the files is enough: the Local REST API has no directory delete
		// (it 405s), and Obsidian prunes the now-empty subtree on its own.
	})

	// full maps a brain-relative path to the vault-relative path the tools return.
	full := func(rel string) string { return dir + "/" + rel }

	// --- Synchronous endpoints: a write is on disk immediately, so assert now. ---

	// list_notes on the brain root shows its four subfolders...
	var root ListNotesOutput
	structured(t, call(t, cs, "list_notes", map[string]any{"dir": dir}), &root)
	for _, want := range []string{"projects/", "conventions/", "inbox/", "daily/"} {
		if !contains(root.Entries, want) {
			t.Fatalf("list_notes %q = %v, missing %s", dir, root.Entries, want)
		}
	}
	// ...and listing a subfolder shows the note inside it.
	var proj ListNotesOutput
	structured(t, call(t, cs, "list_notes", map[string]any{"dir": dir + "/projects"}), &proj)
	if !contains(proj.Entries, "oauth-adr.md") {
		t.Fatalf("list_notes projects = %v, want oauth-adr.md", proj.Entries)
	}

	// read_note: whole note vs. a single heading section.
	var whole ReadNoteOutput
	structured(t, call(t, cs, "read_note", map[string]any{"path": full(adrRel)}), &whole)
	if !strings.Contains(whole.Content, token) || !strings.Contains(whole.Content, "Tradeoffs") {
		t.Fatalf("read_note whole = %q", whole.Content)
	}
	var section ReadNoteOutput
	structured(t, call(t, cs, "read_note", map[string]any{"path": full(adrRel), "heading": "Decision"}), &section)
	if !strings.Contains(section.Content, "We adopt OAuth") || strings.Contains(section.Content, "Tradeoffs") {
		t.Fatalf("read_note Decision section = %q", section.Content)
	}

	// note_outline returns the three headings, in order.
	var ol OutlineOutput
	structured(t, call(t, cs, "note_outline", map[string]any{"path": full(adrRel)}), &ol)
	if len(ol.Headings) != 3 || ol.Headings[0].Title != "OAuth ADR" || ol.Headings[2].Title != "Consequences" {
		t.Fatalf("note_outline = %+v", ol.Headings)
	}

	// read_active_note: whichever note Obsidian has open. We can't choose it, so
	// assert the round-trip when it succeeds and tolerate "no note open".
	if active := call(t, cs, "read_active_note", nil); active.IsError {
		t.Logf("read_active_note: %s (likely no note open)", firstText(active))
	} else {
		var an ActiveNoteOutput
		structured(t, active, &an)
		if an.Vault != vaultName || strings.TrimSpace(an.Content) == "" {
			t.Fatalf("read_active_note = %+v", an)
		}
	}

	// --- Index-backed endpoints: Obsidian indexes writes asynchronously, so poll. ---

	// search_brain finds the ADR by its unique token.
	eventually(t, "search to index the seeded notes", func() bool {
		var so SearchOutput
		structured(t, call(t, cs, "search_brain", map[string]any{"query": token}), &so)
		return so.Vault == vaultName && hasPath(so.Hits, full(adrRel))
	})

	// list_tags includes the unique tag shared by the ADR and the idea note.
	eventually(t, "the tag cache to include the seeded tag", func() bool {
		var lt ListTagsOutput
		structured(t, call(t, cs, "list_tags", nil), &lt)
		return hasTag(lt.Tags, tag)
	})

	// list_backlinks resolves the ADR's [[anchor]] wikilink back to the ADR alone.
	eventually(t, "the backlink scan to see the seeded link", func() bool {
		var bl BacklinksOutput
		structured(t, call(t, cs, "list_backlinks", map[string]any{"note": anchor}), &bl)
		return len(bl.Backlinks) == 1 && bl.Backlinks[0] == full(adrRel)
	})

	// promotion_candidates lists both tagged notes; the team filter narrows it —
	// the ADR is for eng+design, the idea note for design only.
	eventually(t, "the frontmatter index to expose promotion candidates", func() bool {
		var all, eng, design PromotionCandidatesOutput
		structured(t, call(t, cs, "promotion_candidates", nil), &all)
		structured(t, call(t, cs, "promotion_candidates", map[string]any{"team": "eng"}), &eng)
		structured(t, call(t, cs, "promotion_candidates", map[string]any{"team": "design"}), &design)
		return hasCandidate(all.Candidates, full(adrRel)) && hasCandidate(all.Candidates, full(ideaRel)) &&
			hasCandidate(eng.Candidates, full(adrRel)) && !hasCandidate(eng.Candidates, full(ideaRel)) &&
			hasCandidate(design.Candidates, full(adrRel)) && hasCandidate(design.Candidates, full(ideaRel))
	})

	t.Logf("live parity OK against %q: seeded a %d-note brain under %s and exercised every tool; cleaning up", vaultName, len(notes), dir)
}

// liveConfigFromEnv builds the read client's endpoint config from the same
// OBSIDIAN_* variables the production teambrain-mcp server reads, so the live
// test and the real server agree on configuration.
func liveConfigFromEnv(t *testing.T) obsidianapi.Config {
	t.Helper()
	cfg := obsidianapi.Config{
		APIKey:    os.Getenv("OBSIDIAN_API_KEY"),
		Protocol:  os.Getenv("OBSIDIAN_PROTOCOL"),
		Host:      os.Getenv("OBSIDIAN_HOST"),
		CACert:    os.Getenv("OBSIDIAN_CA_CERT"),
		VerifyTLS: os.Getenv("OBSIDIAN_VERIFY_TLS") == "true",
	}
	if p := os.Getenv("OBSIDIAN_PORT"); p != "" {
		port, err := strconv.Atoi(p)
		if err != nil {
			t.Fatalf("invalid OBSIDIAN_PORT %q: %v", p, err)
		}
		cfg.Port = port
	}
	return cfg
}

// liveWriteEndpoint builds the base URL and HTTP client used only for test
// seeding. It resolves the same endpoint defaults as the read client but is kept
// separate on purpose: writes never go through obsidianapi, so the shipped
// retrieval surface stays read-only.
func liveWriteEndpoint(t *testing.T) (*http.Client, *url.URL) {
	t.Helper()
	cfg := liveConfigFromEnv(t)
	protocol := cfg.Protocol
	if protocol == "" {
		protocol = obsidianapi.DefaultProtocol
	}
	host := cfg.Host
	if host == "" {
		host = obsidianapi.DefaultHost
	}
	port := cfg.Port
	if port == 0 {
		port = obsidianapi.DefaultPort
	}
	base := &url.URL{Scheme: protocol, Host: host + ":" + strconv.Itoa(port)}
	// The plugin serves a self-signed certificate on localhost; the read client
	// skips verification by default for the same reason. This is test seeding to
	// a local app only (gosec is excluded for _test.go).
	hc := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12},
	}}
	return hc, base
}

// seedNote creates or overwrites a fixture note in the live vault via PUT.
func seedNote(t *testing.T, hc *http.Client, base *url.URL, apiKey, vaultPath, content string) {
	t.Helper()
	if err := vaultWrite(hc, base, apiKey, http.MethodPut, vaultPath, content); err != nil {
		t.Fatalf("seed %q: %v", vaultPath, err)
	}
}

// tryDelete removes a seeded note, best-effort: cleanup must not turn a passing
// test red, so a failure is logged rather than fatal.
func tryDelete(t *testing.T, hc *http.Client, base *url.URL, apiKey, vaultPath string) {
	t.Helper()
	if err := vaultWrite(hc, base, apiKey, http.MethodDelete, vaultPath, ""); err != nil {
		t.Logf("cleanup: delete %q: %v (a leftover may remain in the test vault)", vaultPath, err)
	}
}

// vaultWrite issues a single mutating request to the Local REST API. A 404 on
// DELETE is treated as success (already gone).
func vaultWrite(hc *http.Client, base *url.URL, apiKey, method, vaultPath, body string) error {
	u := *base
	u.Path = "/vault/" + vaultPath
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, u.String(), r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if method == http.MethodPut {
		req.Header.Set("Content-Type", "text/markdown")
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if method == http.MethodDelete && resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// eventually polls cond until it holds or a budget expires, for endpoints whose
// answer depends on Obsidian's asynchronous indexing of the seeded notes.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	const budget = 20 * time.Second
	deadline := time.Now().Add(budget)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s", budget, what)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// randSuffix returns a short random hex string, so each run's fixtures get a
// unique namespace and unique tokens.
func randSuffix(t *testing.T) string {
	t.Helper()
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("random suffix: %v", err)
	}
	return hex.EncodeToString(b[:])
}

// hasCandidate reports whether a promotion candidate with the given path is present.
func hasCandidate(cands []PromotionCandidate, path string) bool {
	for _, c := range cands {
		if c.Path == path {
			return true
		}
	}
	return false
}
