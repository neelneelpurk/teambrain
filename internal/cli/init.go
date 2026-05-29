package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/neelneelpurk/teambrain/internal/exit"
	"github.com/neelneelpurk/teambrain/internal/scaffold"
	"github.com/neelneelpurk/teambrain/internal/vault"
)

func newInitCommand() *cobra.Command {
	var here bool

	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Scaffold (or repair) a personal-brain vault",
		Long: `Create a personal-brain vault at the given path (or, with --here, in the
current directory): the content folders, the .claude capability folders, the
seeded create-teambrain-* scaffolding skills, and the plain-JSON manifests.

init is safe to re-run: it only creates missing files, so it never overwrites
your edits or the team binding. A re-run on an intact vault is a no-op; a re-run
on a vault missing a file repairs just that file.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appFrom(cmd.Context())

			root, err := resolveInitPath(here, args)
			if err != nil {
				return err
			}
			v, err := vault.NewFSDirect(root)
			if err != nil {
				return exit.Userf("resolve vault path %q: %v", root, err)
			}

			res, err := scaffold.PersonalVault(v, app.Config.DryRun)
			if err != nil {
				return exit.Externalf("scaffold vault: %v", err)
			}

			// Brain retrieval requires Obsidian (MCP or CLI). Surface the
			// dependency loudly at init so it isn't discovered only at query time.
			if _, ok := retrievalStatus(root); !ok {
				app.Warn("brain retrieval is unavailable: %s", retrievalSetupHint)
			}

			return app.Emit("init", res, func(w io.Writer) {
				verb := "created"
				if app.Config.DryRun {
					verb = "would create"
				}
				fmt.Fprintf(w, "personal-brain vault at %s\n", res.Root)
				fmt.Fprintf(w, "%s %d file(s); %d already present\n", verb, len(res.Created), len(res.Existing))
				if len(res.Created) > 0 && (app.Config.Verbose || app.Config.DryRun) {
					for _, f := range res.Created {
						fmt.Fprintf(w, "  + %s\n", f)
					}
				}
			})
		},
	}

	cmd.Flags().BoolVar(&here, "here", false, "initialize in the current directory")
	return cmd
}

// resolveInitPath determines the vault root from --here or a single positional
// argument. Exactly one must be provided.
func resolveInitPath(here bool, args []string) (string, error) {
	switch {
	case here && len(args) == 1:
		return "", exit.Userf("pass either --here or a path, not both")
	case here:
		cwd, err := os.Getwd()
		if err != nil {
			return "", exit.Externalf("determine current directory: %v", err)
		}
		return cwd, nil
	case len(args) == 1:
		return args[0], nil
	default:
		return "", exit.Userf("specify a vault path or pass --here").
			WithHint("e.g. `teambrain init ~/personal-brain` or `teambrain init --here`")
	}
}
