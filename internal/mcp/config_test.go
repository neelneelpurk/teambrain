package mcp

import (
	"testing"

	"github.com/neelneelpurk/teambrain/internal/obsidianapi"
)

func TestParseConfigMultiVault(t *testing.T) {
	t.Parallel()
	data := []byte(`{"default":"personal","vaults":{
		"personal":{"api_key":"k1","port":27124},
		"eng":{"api_key":"k2","port":27125}
	}}`)
	c, err := ParseConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if c.Default != "personal" || len(c.Vaults) != 2 {
		t.Fatalf("config = %+v", c)
	}
	clients, def, err := c.Build()
	if err != nil {
		t.Fatal(err)
	}
	if def != "personal" || len(clients) != 2 {
		t.Fatalf("built %d clients, default %q", len(clients), def)
	}
}

func TestParseConfigInfersSingleDefault(t *testing.T) {
	t.Parallel()
	c, err := ParseConfig([]byte(`{"vaults":{"solo":{"api_key":"k"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Default != "solo" {
		t.Fatalf("default should be inferred, got %q", c.Default)
	}
}

func TestParseConfigErrors(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"bad json":            `{`,
		"no vaults":           `{"vaults":{}}`,
		"ambiguous default":   `{"vaults":{"a":{"api_key":"k"},"b":{"api_key":"k"}}}`,
		"default not present": `{"default":"x","vaults":{"a":{"api_key":"k"}}}`,
	}
	for name, data := range cases {
		if _, err := ParseConfig([]byte(data)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestBuildRejectsVaultMissingAPIKey(t *testing.T) {
	t.Parallel()
	c := &Config{Default: "a", Vaults: map[string]obsidianapi.Config{"a": {}}}
	if _, _, err := c.Build(); err == nil {
		t.Fatal("a vault without an API key should fail to build")
	}
}

func TestSingleVault(t *testing.T) {
	t.Parallel()
	c := SingleVault("default", obsidianapi.Config{APIKey: "k"})
	if c.Default != "default" || len(c.Vaults) != 1 {
		t.Fatalf("SingleVault = %+v", c)
	}
}
