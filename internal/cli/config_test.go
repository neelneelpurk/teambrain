package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig points XDG_CONFIG_HOME at a temp dir and drops a config.yaml with
// the given body into the teambrain config dir. It returns the config dir.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	dir := filepath.Join(xdg, "teambrain")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if body != "" {
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}
	return dir
}

func TestConfigDefaults(t *testing.T) {
	// No file, no env: every value falls back to its compiled-in default.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := LoadConfig(NewViper())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.PersonalVault != "" {
		t.Fatalf("PersonalVault default = %q, want empty", cfg.PersonalVault)
	}
	if cfg.JSON || cfg.DryRun || cfg.Yes || cfg.Verbose || cfg.Quiet {
		t.Fatalf("boolean defaults should all be false, got %+v", cfg)
	}
}

func TestConfigFileOverridesDefault(t *testing.T) {
	writeConfig(t, "personal_vault: /from/file\n")

	cfg, err := LoadConfig(NewViper())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.PersonalVault != "/from/file" {
		t.Fatalf("PersonalVault = %q, want %q (file should beat default)", cfg.PersonalVault, "/from/file")
	}
}

func TestConfigEnvOverridesFile(t *testing.T) {
	writeConfig(t, "personal_vault: /from/file\n")
	t.Setenv("TEAMBRAIN_PERSONAL_VAULT", "/from/env")

	cfg, err := LoadConfig(NewViper())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.PersonalVault != "/from/env" {
		t.Fatalf("PersonalVault = %q, want %q (env should beat file)", cfg.PersonalVault, "/from/env")
	}
}

func TestConfigDirHonorsXDG(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if got, want := ConfigDir(), filepath.Join(xdg, "teambrain"); got != want {
		t.Fatalf("ConfigDir() = %q, want %q", got, want)
	}
}
