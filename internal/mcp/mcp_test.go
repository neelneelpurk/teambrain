package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/neelneelpurk/teambrain/internal/obsidianapi"
)

// fakeClient is an in-memory obsidianapi.Client for hermetic tests — no network.
type fakeClient struct {
	files     map[string][]string
	notes     map[string]string
	active    string
	search    []obsidianapi.SimpleSearchResult
	jsonlogic []obsidianapi.AdvancedResult
	tags      []obsidianapi.Tag
	err       error
}

func (f *fakeClient) ListFiles(_ context.Context, dir string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.files[dir], nil
}

func (f *fakeClient) ReadNote(_ context.Context, path string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	c, ok := f.notes[path]
	if !ok {
		return "", errors.New("file does not exist")
	}
	return c, nil
}

func (f *fakeClient) ReadActiveNote(_ context.Context) (string, error) {
	return f.active, f.err
}

func (f *fakeClient) Search(_ context.Context, _ string, _ int) ([]obsidianapi.SimpleSearchResult, error) {
	return f.search, f.err
}

func (f *fakeClient) SearchJSONLogic(_ context.Context, _ any) ([]obsidianapi.AdvancedResult, error) {
	return f.jsonlogic, f.err
}

func (f *fakeClient) ListTags(_ context.Context) ([]obsidianapi.Tag, error) {
	return f.tags, f.err
}

// connect wires a single fake as the "default" vault.
func connect(t *testing.T, fc obsidianapi.Client) *mcp.ClientSession {
	return connectVaults(t, map[string]obsidianapi.Client{"default": fc}, "default")
}

func connectVaults(t *testing.T, vaults map[string]obsidianapi.Client, def string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	srv := NewServer(vaults, def, "test")
	st, ct := mcp.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	cli := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := cli.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	return res
}

func firstText(res *mcp.CallToolResult) string {
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func structured(t *testing.T, res *mcp.CallToolResult, out any) {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
}

func TestServerExposesTeambrainTools(t *testing.T) {
	t.Parallel()
	cs := connect(t, &fakeClient{})
	res, err := cs.ListTools(context.Background(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = true
	}
	for _, want := range []string{"list_vaults", "search_brain", "read_note", "read_active_note", "note_outline", "list_backlinks", "list_notes", "list_tags", "promotion_candidates"} {
		if !got[want] {
			t.Errorf("missing tool %q (have %v)", want, got)
		}
	}
}

func TestListVaultsAndRouting(t *testing.T) {
	t.Parallel()
	personal := &fakeClient{search: []obsidianapi.SimpleSearchResult{{Filename: "personal-hit.md"}}}
	eng := &fakeClient{search: []obsidianapi.SimpleSearchResult{{Filename: "eng-hit.md"}}}
	cs := connectVaults(t, map[string]obsidianapi.Client{"personal": personal, "eng": eng}, "personal")

	// list_vaults reports both and the default.
	var lv ListVaultsOutput
	structured(t, call(t, cs, "list_vaults", nil), &lv)
	if lv.Default != "personal" || len(lv.Vaults) != 2 {
		t.Fatalf("list_vaults = %+v", lv)
	}

	// Omitting vault hits the default; naming one routes there.
	var def SearchOutput
	structured(t, call(t, cs, "search_brain", map[string]any{"query": "x"}), &def)
	if def.Vault != "personal" || def.Hits[0].Path != "personal-hit.md" {
		t.Fatalf("default route = %+v", def)
	}
	var routed SearchOutput
	structured(t, call(t, cs, "search_brain", map[string]any{"query": "x", "vault": "eng"}), &routed)
	if routed.Vault != "eng" || routed.Hits[0].Path != "eng-hit.md" {
		t.Fatalf("eng route = %+v", routed)
	}
}

func TestUnknownVaultIsToolError(t *testing.T) {
	t.Parallel()
	cs := connect(t, &fakeClient{})
	res := call(t, cs, "search_brain", map[string]any{"query": "x", "vault": "ghost"})
	if !res.IsError || !strings.Contains(firstText(res), "ghost") {
		t.Fatalf("unknown vault should be a tool error naming it: %q", firstText(res))
	}
}

func TestListTags(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{tags: []obsidianapi.Tag{{Name: "project", Count: 3}, {Name: "work", Count: 1}}}
	var out ListTagsOutput
	structured(t, call(t, connect(t, fc), "list_tags", nil), &out)
	if len(out.Tags) != 2 || out.Tags[0].Name != "project" || out.Tags[0].Count != 3 {
		t.Fatalf("tags = %+v", out.Tags)
	}
}

func TestReadActiveNote(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{active: "# Open\nthe note I'm looking at\n"}
	res := call(t, connect(t, fc), "read_active_note", nil)
	var out ActiveNoteOutput
	structured(t, res, &out)
	if !strings.Contains(out.Content, "looking at") || out.Vault != "default" {
		t.Fatalf("active note = %+v", out)
	}
}

func TestSearchBrain(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{search: []obsidianapi.SimpleSearchResult{
		{Filename: "adrs/0001.md", Score: 2.0, Matches: []obsidianapi.SearchMatch{{Context: "...oauth flow..."}}},
	}}
	res := call(t, connect(t, fc), "search_brain", map[string]any{"query": "oauth"})
	if res.IsError {
		t.Fatalf("unexpected error result: %s", firstText(res))
	}
	if !strings.Contains(firstText(res), "adrs/0001.md") {
		t.Fatalf("text = %q", firstText(res))
	}
	var out SearchOutput
	structured(t, res, &out)
	if len(out.Hits) != 1 || out.Hits[0].Path != "adrs/0001.md" || len(out.Hits[0].Snippets) != 1 {
		t.Fatalf("hits = %+v", out.Hits)
	}
}

func TestSearchEmptyQueryIsToolError(t *testing.T) {
	t.Parallel()
	res := call(t, connect(t, &fakeClient{}), "search_brain", map[string]any{"query": "  "})
	if !res.IsError {
		t.Fatal("empty query should be a tool error")
	}
}

func TestReadNoteWholeAndSection(t *testing.T) {
	t.Parallel()
	note := "# Title\nintro\n\n## Decision\nwe chose oauth\n\n## Consequences\ntradeoffs\n"
	cs := connect(t, &fakeClient{notes: map[string]string{"adrs/0001.md": note}})

	whole := call(t, cs, "read_note", map[string]any{"path": "adrs/0001.md"})
	if !strings.Contains(firstText(whole), "Consequences") {
		t.Fatalf("whole-note read missing content: %q", firstText(whole))
	}

	section := call(t, cs, "read_note", map[string]any{"path": "adrs/0001.md", "heading": "Decision"})
	var out ReadNoteOutput
	structured(t, section, &out)
	if !strings.Contains(out.Content, "we chose oauth") || strings.Contains(out.Content, "tradeoffs") {
		t.Fatalf("section should be just the Decision block, got %q", out.Content)
	}
}

func TestReadNoteMissingHeadingIsToolError(t *testing.T) {
	t.Parallel()
	cs := connect(t, &fakeClient{notes: map[string]string{"n.md": "# A\nx\n"}})
	res := call(t, cs, "read_note", map[string]any{"path": "n.md", "heading": "Nope"})
	if !res.IsError {
		t.Fatal("a missing heading should be a tool error")
	}
}

func TestReadNoteMissingFileIsToolError(t *testing.T) {
	t.Parallel()
	res := call(t, connect(t, &fakeClient{}), "read_note", map[string]any{"path": "ghost.md"})
	if !res.IsError {
		t.Fatal("a missing file should be a tool error")
	}
}

func TestOutline(t *testing.T) {
	t.Parallel()
	note := "# Title\n```\n# not a heading\n```\n## Section A\ntext\n### Sub\n"
	cs := connect(t, &fakeClient{notes: map[string]string{"n.md": note}})
	res := call(t, cs, "note_outline", map[string]any{"path": "n.md"})
	var out OutlineOutput
	structured(t, res, &out)
	if len(out.Headings) != 3 {
		t.Fatalf("expected 3 headings (fenced # ignored), got %+v", out.Headings)
	}
	if out.Headings[0].Title != "Title" || out.Headings[2].Level != 3 {
		t.Fatalf("headings = %+v", out.Headings)
	}
}

func TestBacklinks(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{jsonlogic: []obsidianapi.AdvancedResult{
		{Filename: "projects/b.md"}, {Filename: "projects/a.md"},
	}}
	res := call(t, connect(t, fc), "list_backlinks", map[string]any{"note": "decision.md"})
	var out BacklinksOutput
	structured(t, res, &out)
	if out.Note != "decision" {
		t.Errorf("note name should drop .md, got %q", out.Note)
	}
	if len(out.Backlinks) != 2 || out.Backlinks[0] != "projects/a.md" {
		t.Fatalf("backlinks should be sorted: %v", out.Backlinks)
	}
}

func TestPromotionCandidatesFiltersByTeam(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{jsonlogic: []obsidianapi.AdvancedResult{
		{Filename: "z.md", Result: json.RawMessage(`["eng","design"]`)},
		{Filename: "a.md", Result: json.RawMessage(`"eng"`)},
		{Filename: "other.md", Result: json.RawMessage(`["design"]`)},
	}}
	cs := connect(t, fc)

	all := call(t, cs, "promotion_candidates", nil)
	var allOut PromotionCandidatesOutput
	structured(t, all, &allOut)
	if len(allOut.Candidates) != 3 || allOut.Candidates[0].Path != "a.md" {
		t.Fatalf("all candidates (sorted) = %+v", allOut.Candidates)
	}

	eng := call(t, cs, "promotion_candidates", map[string]any{"team": "eng"})
	var engOut PromotionCandidatesOutput
	structured(t, eng, &engOut)
	if len(engOut.Candidates) != 2 {
		t.Fatalf("eng candidates = %+v", engOut.Candidates)
	}
	for _, c := range engOut.Candidates {
		if !contains(c.Teams, "eng") {
			t.Errorf("candidate %s not tagged eng: %v", c.Path, c.Teams)
		}
	}
}

func TestListNotes(t *testing.T) {
	t.Parallel()
	fc := &fakeClient{files: map[string][]string{"": {"a.md", "projects/"}}}
	res := call(t, connect(t, fc), "list_notes", nil)
	var out ListNotesOutput
	structured(t, res, &out)
	if len(out.Entries) != 2 {
		t.Fatalf("entries = %v", out.Entries)
	}
}

func TestClientErrorBecomesToolError(t *testing.T) {
	t.Parallel()
	cs := connect(t, &fakeClient{err: errors.New("connection refused")})
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"search_brain", map[string]any{"query": "x"}},
		{"list_notes", nil},
		{"list_backlinks", map[string]any{"note": "x"}},
		{"promotion_candidates", nil},
	} {
		res := call(t, cs, tc.name, tc.args)
		if !res.IsError {
			t.Errorf("%s should surface a tool error when the client fails", tc.name)
		}
	}
}

func TestDecodeTeams(t *testing.T) {
	t.Parallel()
	if got := decodeTeams(json.RawMessage(`["a","b"]`)); len(got) != 2 {
		t.Errorf("list = %v", got)
	}
	if got := decodeTeams(json.RawMessage(`"solo"`)); len(got) != 1 || got[0] != "solo" {
		t.Errorf("scalar = %v", got)
	}
	if got := decodeTeams(json.RawMessage(`null`)); got != nil {
		t.Errorf("null = %v", got)
	}
}

func TestExtractSectionStopsAtSameLevel(t *testing.T) {
	t.Parallel()
	content := "# A\nintro\n## B\nbee\n## C\ncee\n"
	got, ok := extractSection(content, "B")
	if !ok || !strings.Contains(got, "bee") || strings.Contains(got, "cee") {
		t.Fatalf("section B = %q (ok=%v)", got, ok)
	}
	if _, ok := extractSection(content, "missing"); ok {
		t.Error("missing heading should report not found")
	}
}
