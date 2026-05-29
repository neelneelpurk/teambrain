// Package obsidianapi is a small, read-only client for the Obsidian Local REST
// API community plugin (github.com/coddingtonbear/obsidian-local-rest-api). It
// exposes just the endpoints the teambrain MCP needs to drive retrieval over a
// *running* Obsidian vault. Retrieval stays Obsidian's job; this is only a thin
// HTTP bridge to the live app, never a reimplementation of search.
package obsidianapi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Defaults match the Local REST API plugin's out-of-the-box configuration.
const (
	DefaultProtocol = "https"
	DefaultHost     = "127.0.0.1"
	DefaultPort     = 27124
)

// Config configures a Client. APIKey is required; the rest default to the
// plugin's standard localhost HTTPS endpoint with its self-signed certificate.
// JSON tags let a Config be read straight from a per-vault config file.
type Config struct {
	APIKey    string `json:"api_key"`
	Protocol  string `json:"protocol,omitempty"` // "https" (default) or "http"
	Host      string `json:"host,omitempty"`
	Port      int    `json:"port,omitempty"`
	VerifyTLS bool   `json:"verify_tls,omitempty"` // verify the server cert (the plugin ships a self-signed one, so default false)
	// CACert is an optional path to the plugin's self-signed certificate
	// (GET /obsidian-local-rest-api.crt). When set, TLS is verified against it.
	CACert string       `json:"ca_cert,omitempty"`
	HTTP   *http.Client `json:"-"` // optional injected client (tests); built from the above when nil
}

// SimpleSearchResult is one hit from POST /search/simple/.
type SimpleSearchResult struct {
	Filename string        `json:"filename"`
	Score    float64       `json:"score"`
	Matches  []SearchMatch `json:"matches"`
}

// SearchMatch is a single in-file match with its surrounding context.
type SearchMatch struct {
	Context string `json:"context"`
	Match   struct {
		Start int `json:"start"`
		End   int `json:"end"`
	} `json:"match"`
}

// AdvancedResult is one hit from POST /search/ (Dataview or JsonLogic), where
// Result is whatever the query evaluated to for that file.
type AdvancedResult struct {
	Filename string          `json:"filename"`
	Result   json.RawMessage `json:"result"`
}

// Tag is a vault tag with how many times it is used (from GET /tags/).
type Tag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// Client is the read-only surface of the Local REST API that the MCP drives.
// It is an interface so the MCP tools can be tested against a fake.
type Client interface {
	ListFiles(ctx context.Context, dir string) ([]string, error)
	ReadNote(ctx context.Context, path string) (string, error)
	ReadActiveNote(ctx context.Context) (string, error)
	Search(ctx context.Context, query string, contextLength int) ([]SimpleSearchResult, error)
	SearchJSONLogic(ctx context.Context, query any) ([]AdvancedResult, error)
	ListTags(ctx context.Context) ([]Tag, error)
}

// HTTPClient talks to a live Obsidian via the Local REST API plugin.
type HTTPClient struct {
	apiKey string
	base   *url.URL
	http   *http.Client
}

// New builds an HTTPClient from cfg, applying the plugin defaults.
func New(cfg Config) (*HTTPClient, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("obsidian api key is required")
	}
	protocol := cfg.Protocol
	if protocol == "" {
		protocol = DefaultProtocol
	}
	if protocol != "http" && protocol != "https" {
		return nil, fmt.Errorf("invalid protocol %q: want http or https", protocol)
	}
	host := cfg.Host
	if host == "" {
		host = DefaultHost
	}
	port := cfg.Port
	if port == 0 {
		port = DefaultPort
	}

	hc := cfg.HTTP
	if hc == nil {
		tr := &http.Transport{}
		if protocol == "https" {
			tlsCfg, err := tlsConfigFor(cfg)
			if err != nil {
				return nil, err
			}
			tr.TLSClientConfig = tlsCfg
		}
		hc = &http.Client{Timeout: 10 * time.Second, Transport: tr}
	}

	return &HTTPClient{
		apiKey: cfg.APIKey,
		base:   &url.URL{Scheme: protocol, Host: host + ":" + strconv.Itoa(port)},
		http:   hc,
	}, nil
}

// tlsConfigFor builds the TLS config: pin to the plugin's certificate when
// CACert is given, verify against system roots when VerifyTLS is set, else skip
// verification (the plugin's default certificate is self-signed on localhost).
func tlsConfigFor(cfg Config) (*tls.Config, error) {
	if cfg.CACert != "" {
		pem, err := os.ReadFile(cfg.CACert)
		if err != nil {
			return nil, fmt.Errorf("read ca cert %q: %w", cfg.CACert, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificates found in %q", cfg.CACert)
		}
		return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, nil
	}
	if cfg.VerifyTLS {
		return &tls.Config{MinVersion: tls.VersionTLS12}, nil
	}
	// G402: the Obsidian Local REST API plugin serves a self-signed certificate
	// on localhost; verification is opt-in via VerifyTLS or CACert.
	return &tls.Config{InsecureSkipVerify: true}, nil //nolint:gosec // see comment above
}

// ListFiles lists the entries directly under dir (vault root when dir is empty).
// Subdirectory entries come back with a trailing slash, as the plugin returns them.
func (c *HTTPClient) ListFiles(ctx context.Context, dir string) ([]string, error) {
	p := "/vault/"
	if d := strings.Trim(dir, "/"); d != "" {
		p += d + "/"
	}
	var body struct {
		Files []string `json:"files"`
	}
	if err := c.doJSON(ctx, http.MethodGet, p, nil, "", nil, &body); err != nil {
		return nil, err
	}
	return body.Files, nil
}

// ReadNote returns the raw Markdown of a note at a vault-relative path.
func (c *HTTPClient) ReadNote(ctx context.Context, path string) (string, error) {
	clean := strings.TrimPrefix(path, "/")
	return c.doText(ctx, http.MethodGet, "/vault/"+clean, nil)
}

// ReadActiveNote returns the raw Markdown of the note currently open in Obsidian.
func (c *HTTPClient) ReadActiveNote(ctx context.Context) (string, error) {
	return c.doText(ctx, http.MethodGet, "/active/", nil)
}

// ListTags returns every tag in the vault with its usage count.
func (c *HTTPClient) ListTags(ctx context.Context) ([]Tag, error) {
	var body struct {
		Tags []Tag `json:"tags"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/tags/", nil, "", nil, &body); err != nil {
		return nil, err
	}
	return body.Tags, nil
}

// Search runs the plugin's simple full-text search, returning matches with
// contextLength characters of surrounding context.
func (c *HTTPClient) Search(ctx context.Context, query string, contextLength int) ([]SimpleSearchResult, error) {
	if contextLength <= 0 {
		contextLength = 100
	}
	q := url.Values{"query": {query}, "contextLength": {strconv.Itoa(contextLength)}}
	var out []SimpleSearchResult
	if err := c.doJSON(ctx, http.MethodPost, "/search/simple/", q, "", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SearchJSONLogic runs an advanced JsonLogic query against each file's metadata
// (including frontmatter), returning the files for which it is truthy.
func (c *HTTPClient) SearchJSONLogic(ctx context.Context, query any) ([]AdvancedResult, error) {
	payload, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	var out []AdvancedResult
	if err := c.doJSON(ctx, http.MethodPost, "/search/", nil, "application/vnd.olrapi.jsonlogic+json", payload, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *HTTPClient) doText(ctx context.Context, method, path string, query url.Values) (string, error) {
	data, err := c.do(ctx, method, path, query, "", nil)
	return string(data), err
}

func (c *HTTPClient) doJSON(ctx context.Context, method, path string, query url.Values, contentType string, body []byte, out any) error {
	data, err := c.do(ctx, method, path, query, contentType, body)
	if err != nil {
		return err
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response from %s: %w", path, err)
	}
	return nil
}

// do issues a request and returns the response body, closing it. Non-2xx
// responses are turned into an error carrying the plugin's message when present.
func (c *HTTPClient) do(ctx context.Context, method, path string, query url.Values, contentType string, body []byte) ([]byte, error) {
	u := *c.base
	u.Path = path
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("obsidian request failed (is Obsidian running with the Local REST API plugin?): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, apiError(resp.StatusCode, data)
	}
	return data, nil
}

// apiError extracts the plugin's structured error message when present.
func apiError(status int, data []byte) error {
	var e struct {
		ErrorCode int    `json:"errorCode"`
		Message   string `json:"message"`
	}
	if json.Unmarshal(data, &e) == nil && e.Message != "" {
		return fmt.Errorf("obsidian rest api error %d: %s (HTTP %d)", e.ErrorCode, e.Message, status)
	}
	return fmt.Errorf("obsidian rest api returned HTTP %d", status)
}
