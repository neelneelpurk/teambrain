package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/neelneelpurk/teambrain/internal/capability"
	"github.com/neelneelpurk/teambrain/internal/vault"
)

// newDoctorCommand returns `teambrain doctor`: it reports the active vault
// backend, checks teambrain-owned capabilities for checksum drift (tamper
// detection), and — read-only — reminds you to configure an Obsidian MCP for
// retrieval if none is detected.
func newDoctorCommand() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose the teambrain environment",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appFrom(cmd.Context())

			detected := vault.DetectObsidian()
			active := activeBackend(app.Config.VaultBackend, detected)

			drift, err := capability.OpenStore(filepath.Join(dir, ".claude")).CheckDrift()
			if err != nil {
				return err
			}
			mcp := mcpReminder(dir)

			data := map[string]any{
				"vault_backend":     app.Config.VaultBackend,
				"obsidian_detected": detected,
				"active_backend":    active,
				"healthy":           len(drift) == 0,
				"drift":             drift,
			}
			if mcp != "" {
				data["mcp_reminder"] = mcp
			}

			return app.Emit("doctor", data, func(w io.Writer) {
				fmt.Fprintf(w, "vault backend: %s (active: %s)\n", app.Config.VaultBackend, active)
				fmt.Fprintf(w, "obsidian CLI:  %s\n", detectedLabel(detected))
				if len(drift) == 0 {
					fmt.Fprintln(w, "ownership:     OK (no checksum drift)")
				} else {
					fmt.Fprintf(w, "ownership:     %d capability(ies) drifted — possible tamper:\n", len(drift))
					for _, d := range drift {
						fmt.Fprintf(w, "  - %s (%s): %s\n", d.Name, d.File, d.Reason)
					}
				}
				if mcp != "" {
					fmt.Fprintf(w, "retrieval:     %s\n", mcp)
				}
			})
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "directory whose .claude to inspect for drift")
	return cmd
}

// activeBackend reports which backend would actually be used given the
// configured preference and whether Obsidian was detected.
func activeBackend(pref string, detected bool) string {
	switch pref {
	case string(vault.BackendObsidian):
		if detected {
			return "obsidian"
		}
		return "fs"
	case string(vault.BackendAuto):
		if detected {
			return "obsidian"
		}
		return "fs"
	default:
		return "fs"
	}
}

func detectedLabel(detected bool) string {
	if detected {
		return "detected"
	}
	return "not detected"
}

// mcpReminder returns a read-only reminder when no MCP config is found in dir.
func mcpReminder(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, ".mcp.json")); err == nil {
		return ""
	}
	return "no .mcp.json found; configure an Obsidian MCP in Claude Code for retrieval (teambrain does not manage it)"
}
