package obsidianapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// clientFor points an HTTPClient at a test server (plain HTTP), exercising the
// real URL building and request plumbing without touching the network.
func clientFor(t *testing.T, srv *httptest.Server) *HTTPClient {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(u.Port())
	c, err := New(Config{APIKey: "secret", Protocol: "http", Host: u.Hostname(), Port: port, HTTP: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestNewRequiresAPIKey(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected an error without an API key")
	}
	if _, err := New(Config{APIKey: "k", Protocol: "ftp"}); err == nil {
		t.Fatal("expected an error for an invalid protocol")
	}
}

func TestNewDefaults(t *testing.T) {
	t.Parallel()
	c, err := New(Config{APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.base.String(); got != "https://127.0.0.1:27124" {
		t.Fatalf("base = %q, want the plugin default", got)
	}
}

func TestListFilesSendsAuthAndParses(t *testing.T) {
	t.Parallel()
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"files":["a.md","projects/"]}`)
	}))
	defer srv.Close()

	files, err := clientFor(t, srv).ListFiles(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("auth header = %q", gotAuth)
	}
	if gotPath != "/vault/" {
		t.Errorf("path = %q, want /vault/", gotPath)
	}
	if len(files) != 2 || files[0] != "a.md" {
		t.Fatalf("files = %v", files)
	}
}

func TestListFilesInDir(t *testing.T) {
	t.Parallel()
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.WriteString(w, `{"files":["adr.md"]}`)
	}))
	defer srv.Close()

	if _, err := clientFor(t, srv).ListFiles(context.Background(), "projects"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/vault/projects/" {
		t.Errorf("dir path = %q, want /vault/projects/", gotPath)
	}
}

func TestReadNote(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vault/projects/adr.md" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, "# ADR\nbody\n")
	}))
	defer srv.Close()

	got, err := clientFor(t, srv).ReadNote(context.Background(), "projects/adr.md")
	if err != nil {
		t.Fatal(err)
	}
	if got != "# ADR\nbody\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestSearchSimple(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/simple/" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if q := r.URL.Query().Get("query"); q != "oauth" {
			t.Errorf("query = %q", q)
		}
		if cl := r.URL.Query().Get("contextLength"); cl != "100" {
			t.Errorf("contextLength = %q, want default 100", cl)
		}
		_, _ = io.WriteString(w, `[{"filename":"adrs/0001.md","score":1.5,"matches":[{"context":"...oauth...","match":{"start":3,"end":8}}]}]`)
	}))
	defer srv.Close()

	res, err := clientFor(t, srv).Search(context.Background(), "oauth", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Filename != "adrs/0001.md" || len(res[0].Matches) != 1 {
		t.Fatalf("results = %+v", res)
	}
}

func TestSearchJSONLogicSendsContentType(t *testing.T) {
	t.Parallel()
	var gotCT, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, `[{"filename":"a.md","result":["eng"]}]`)
	}))
	defer srv.Close()

	q := map[string]any{"!=": []any{map[string]any{"var": "frontmatter.teambrains"}, nil}}
	res, err := clientFor(t, srv).SearchJSONLogic(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	if gotCT != "application/vnd.olrapi.jsonlogic+json" {
		t.Errorf("content-type = %q", gotCT)
	}
	if !strings.Contains(gotBody, "frontmatter.teambrains") {
		t.Errorf("body = %q", gotBody)
	}
	if len(res) != 1 || res[0].Filename != "a.md" {
		t.Fatalf("results = %+v", res)
	}
}

func TestListTags(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tags/" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"tags":[{"name":"project","count":3},{"name":"work/tasks","count":2}]}`)
	}))
	defer srv.Close()

	tags, err := clientFor(t, srv).ListTags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0].Name != "project" || tags[0].Count != 3 {
		t.Fatalf("tags = %+v", tags)
	}
}

func TestReadActiveNote(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/active/" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = io.WriteString(w, "# Active\nopen note\n")
	}))
	defer srv.Close()

	got, err := clientFor(t, srv).ReadActiveNote(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != "# Active\nopen note\n" {
		t.Fatalf("active note = %q", got)
	}
}

func TestCACertMissingFileIsError(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{APIKey: "k", CACert: "/no/such/cert.crt"}); err == nil {
		t.Fatal("a missing CA cert path should error")
	}
}

func TestErrorResponseCarriesPluginMessage(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"errorCode":40400,"message":"file does not exist"}`)
	}))
	defer srv.Close()

	_, err := clientFor(t, srv).ReadNote(context.Background(), "missing.md")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "file does not exist") {
		t.Fatalf("error should carry the plugin message: %v", err)
	}
}
