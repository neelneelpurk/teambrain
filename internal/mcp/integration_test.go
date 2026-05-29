package mcp

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/neelneelpurk/teambrain/internal/obsidianapi"
	"github.com/neelneelpurk/teambrain/internal/vault"
)

// TestMCPEndToEndAgainstMockObsidian drives the full MCP stack — the real
// obsidianapi HTTP client, the real tool handlers, and the real MCP protocol —
// against a mock Local REST API serving the testdata/vault fixtures. It is the
// closest hermetic stand-in for a running Obsidian: every tool is exercised
// over the wire and asserted against known vault content.
func TestMCPEndToEndAgainstMockObsidian(t *testing.T) {
	t.Parallel()
	const token = "test-key"
	vaultDir := filepath.Join("testdata", "vault")

	srv := httptest.NewServer(mockObsidian(t, vaultDir, token))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(u.Port())
	client, err := obsidianapi.New(obsidianapi.Config{
		APIKey: token, Protocol: "http", Host: u.Hostname(), Port: port, HTTP: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	cs := connectVaults(t, map[string]obsidianapi.Client{"personal": client}, "personal")

	// list_vaults
	var lv ListVaultsOutput
	structured(t, call(t, cs, "list_vaults", nil), &lv)
	if lv.Default != "personal" || len(lv.Vaults) != 1 {
		t.Fatalf("list_vaults = %+v", lv)
	}

	// search_brain — finds the ADR that mentions OAuth
	var so SearchOutput
	structured(t, call(t, cs, "search_brain", map[string]any{"query": "OAuth"}), &so)
	if so.Vault != "personal" || !hasPath(so.Hits, "projects/oauth-adr.md") {
		t.Fatalf("search_brain = %+v", so)
	}

	// read_note with a heading — returns only that section
	var rn ReadNoteOutput
	structured(t, call(t, cs, "read_note", map[string]any{"path": "projects/oauth-adr.md", "heading": "Decision"}), &rn)
	if !strings.Contains(rn.Content, "We adopt OAuth") || strings.Contains(rn.Content, "Tradeoffs") {
		t.Fatalf("read_note section = %q", rn.Content)
	}

	// note_outline — the ADR's heading structure
	var ol OutlineOutput
	structured(t, call(t, cs, "note_outline", map[string]any{"path": "projects/oauth-adr.md"}), &ol)
	if len(ol.Headings) != 3 || ol.Headings[0].Title != "OAuth ADR" || ol.Headings[2].Title != "Consequences" {
		t.Fatalf("note_outline = %+v", ol.Headings)
	}

	// list_backlinks — the ADR links to conventions/style
	var bl BacklinksOutput
	structured(t, call(t, cs, "list_backlinks", map[string]any{"note": "conventions/style"}), &bl)
	if len(bl.Backlinks) != 1 || bl.Backlinks[0] != "projects/oauth-adr.md" {
		t.Fatalf("list_backlinks = %+v", bl)
	}

	// list_notes — the four top-level folders
	var ln ListNotesOutput
	structured(t, call(t, cs, "list_notes", nil), &ln)
	if !equalStrings(ln.Entries, []string{"conventions/", "daily/", "inbox/", "projects/"}) {
		t.Fatalf("list_notes root = %+v", ln.Entries)
	}

	// list_tags — frontmatter and inline tags, with counts
	var lt ListTagsOutput
	structured(t, call(t, cs, "list_tags", nil), &lt)
	for _, want := range []string{"decision", "daily", "idea"} {
		if !hasTag(lt.Tags, want) {
			t.Fatalf("list_tags missing %q: %+v", want, lt.Tags)
		}
	}

	// promotion_candidates — both tagged notes, sorted by path
	var pc PromotionCandidatesOutput
	structured(t, call(t, cs, "promotion_candidates", nil), &pc)
	if len(pc.Candidates) != 2 || pc.Candidates[0].Path != "inbox/idea.md" {
		t.Fatalf("promotion_candidates = %+v", pc.Candidates)
	}
	// filtered to one team
	var pcd PromotionCandidatesOutput
	structured(t, call(t, cs, "promotion_candidates", map[string]any{"team": "design"}), &pcd)
	if len(pcd.Candidates) != 1 || pcd.Candidates[0].Path != "inbox/idea.md" {
		t.Fatalf("promotion_candidates(design) = %+v", pcd.Candidates)
	}

	// read_active_note — the daily note designated active by the mock
	var an ActiveNoteOutput
	structured(t, call(t, cs, "read_active_note", nil), &an)
	if !strings.Contains(an.Content, "Worked on OAuth") {
		t.Fatalf("read_active_note = %q", an.Content)
	}
}

// mockObsidian emulates the subset of the Obsidian Local REST API that the MCP
// tools use, serving the markdown vault at vaultDir and requiring a bearer token.
func mockObsidian(t *testing.T, vaultDir, token string) http.Handler {
	t.Helper()
	mux := http.NewServeMux()

	authed := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+token {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"errorCode":40101,"message":"Authorization required."}`)
				return
			}
			next(w, r)
		}
	}

	mux.HandleFunc("/vault/", authed(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/vault/")
		if rel == "" || strings.HasSuffix(rel, "/") {
			entries, err := listDir(vaultDir, strings.TrimSuffix(rel, "/"))
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"files": entries})
			return
		}
		data, err := os.ReadFile(filepath.Join(vaultDir, filepath.FromSlash(rel)))
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"errorCode":40400,"message":"file does not exist"}`)
			return
		}
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write(data)
	}))

	mux.HandleFunc("/search/simple/", authed(func(w http.ResponseWriter, r *http.Request) {
		query := strings.ToLower(r.URL.Query().Get("query"))
		results := []map[string]any{}
		_ = walkNotes(vaultDir, func(rel string, content []byte) {
			if query != "" && strings.Contains(strings.ToLower(string(content)), query) {
				results = append(results, map[string]any{
					"filename": rel, "score": 1.0,
					"matches": []map[string]any{{"context": snippet(string(content), query)}},
				})
			}
		})
		_ = json.NewEncoder(w).Encode(results)
	}))

	mux.HandleFunc("/search/", authed(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var q map[string]any
		_ = json.Unmarshal(body, &q)
		results := []map[string]any{}
		switch {
		case isVarQuery(q, "frontmatter.teambrains"):
			_ = walkNotes(vaultDir, func(rel string, content []byte) {
				doc, err := vault.ParseDocument(content)
				if err != nil {
					return
				}
				if teams := doc.GetList("teambrains"); len(teams) > 0 {
					b, _ := json.Marshal(teams)
					results = append(results, map[string]any{"filename": rel, "result": json.RawMessage(b)})
				}
			})
		default:
			if sub, ok := inSubstring(q); ok {
				_ = walkNotes(vaultDir, func(rel string, content []byte) {
					if strings.Contains(string(content), sub) {
						results = append(results, map[string]any{"filename": rel, "result": true})
					}
				})
			}
		}
		_ = json.NewEncoder(w).Encode(results)
	}))

	mux.HandleFunc("/tags/", authed(func(w http.ResponseWriter, _ *http.Request) {
		counts := map[string]int{}
		inline := regexp.MustCompile(`#([\w/-]+)`)
		_ = walkNotes(vaultDir, func(_ string, content []byte) {
			if doc, err := vault.ParseDocument(content); err == nil {
				for _, tg := range doc.GetList("tags") {
					counts[tg]++
				}
			}
			for _, m := range inline.FindAllStringSubmatch(string(content), -1) {
				counts[m[1]]++
			}
		})
		tags := []map[string]any{}
		for name, c := range counts {
			tags = append(tags, map[string]any{"name": name, "count": c})
		}
		sort.Slice(tags, func(i, j int) bool { return tags[i]["name"].(string) < tags[j]["name"].(string) })
		_ = json.NewEncoder(w).Encode(map[string]any{"tags": tags})
	}))

	mux.HandleFunc("/active/", authed(func(w http.ResponseWriter, _ *http.Request) {
		data, err := os.ReadFile(filepath.Join(vaultDir, "daily", "2026-05-29.md"))
		if err != nil {
			http.Error(w, "no active note", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write(data)
	}))

	return mux
}

func listDir(root, sub string) ([]string, error) {
	ents, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(sub)))
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ents))
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name()+"/")
		} else {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

func walkNotes(root string, fn func(rel string, content []byte)) error {
	return fs.WalkDir(os.DirFS(root), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		data, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if readErr != nil {
			return readErr
		}
		fn(p, data)
		return nil
	})
}

func snippet(content, lowerQuery string) string {
	i := strings.Index(strings.ToLower(content), lowerQuery)
	if i < 0 {
		return ""
	}
	start := i - 20
	if start < 0 {
		start = 0
	}
	end := i + len(lowerQuery) + 20
	if end > len(content) {
		end = len(content)
	}
	return content[start:end]
}

func isVarQuery(q map[string]any, name string) bool {
	v, ok := q["var"].(string)
	return ok && v == name
}

func inSubstring(q map[string]any) (string, bool) {
	arr, ok := q["in"].([]any)
	if !ok || len(arr) == 0 {
		return "", false
	}
	s, ok := arr[0].(string)
	return s, ok
}

func hasPath(hits []SearchHit, path string) bool {
	for _, h := range hits {
		if h.Path == path {
			return true
		}
	}
	return false
}

func hasTag(tags []TagItem, name string) bool {
	for _, tg := range tags {
		if tg.Name == name {
			return true
		}
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
