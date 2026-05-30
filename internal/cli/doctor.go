package cli

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/neelneelpurk/teambrain/internal/capability"
)

// newDoctorCommand returns `teambrain doctor`: it checks teambrain-owned
// capabilities for checksum drift (tamper detection) and — read-only — reports
// the brain-retrieval path, reminding you to configure an Obsidian MCP if none
// is available.
func newDoctorCommand() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose the teambrain environment",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appFrom(cmd.Context())

			drift, err := capability.OpenStore(filepath.Join(dir, ".claude")).CheckDrift()
			if err != nil {
				return err
			}
			retrieval, retrievalOK := retrievalStatus(dir)

			data := map[string]any{
				"healthy":             len(drift) == 0,
				"drift":               drift,
				"retrieval":           retrieval,
				"retrieval_available": retrievalOK,
			}

			return app.Emit("doctor", data, func(w io.Writer) {
				if len(drift) == 0 {
					fmt.Fprintln(w, "ownership:  OK (no checksum drift)")
				} else {
					fmt.Fprintf(w, "ownership:  %d capability(ies) drifted — possible tamper:\n", len(drift))
					for _, d := range drift {
						fmt.Fprintf(w, "  - %s (%s): %s\n", d.Name, d.File, d.Reason)
					}
				}
				if retrievalOK {
					fmt.Fprintf(w, "retrieval:  %s (live)\n", retrieval)
				} else {
					fmt.Fprintf(w, "retrieval:  UNAVAILABLE — %s\n", retrievalSetupHint)
				}
			})
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "directory whose .claude to inspect for drift")
	return cmd
}
