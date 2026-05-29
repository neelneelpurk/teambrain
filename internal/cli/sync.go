package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neelneelpurk/teambrain/internal/exit"
	"github.com/neelneelpurk/teambrain/internal/git"
	"github.com/neelneelpurk/teambrain/internal/manifest"
	"github.com/neelneelpurk/teambrain/internal/sync"
	"github.com/neelneelpurk/teambrain/internal/vault"
)

// resolvePersonalVault determines the personal vault path: an explicit flag, the
// configured personal_vault, or the current directory.
func resolvePersonalVault(app *App, flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if app.Config.PersonalVault != "" {
		return app.Config.PersonalVault, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", exit.Externalf("determine current directory: %v", err)
	}
	return cwd, nil
}

// buildPromoter constructs a Promoter from the personal vault and its bound team
// vault. The personal vault must be initialized.
func buildPromoter(app *App, vaultFlag string) (*sync.Promoter, error) {
	personalPath, err := resolvePersonalVault(app, vaultFlag)
	if err != nil {
		return nil, err
	}
	root, err := manifest.LoadRoot(personalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, exit.Preconditionf("%s is not a teambrain vault", personalPath).
				WithHint("run `teambrain init` there first")
		}
		return nil, err
	}
	backend := vault.Backend(app.Config.VaultBackend)
	personal, err := vault.Open(backend, personalPath, vault.DetectObsidian, app.Warn)
	if err != nil {
		return nil, err
	}
	var team vault.Vault
	if root.IsBound() && root.Team.Path != "" {
		t, err := vault.Open(backend, root.Team.Path, vault.DetectObsidian, app.Warn)
		if err != nil {
			return nil, err
		}
		team = t
	}
	return sync.NewPromoter(personal, team, git.NewShell()), nil
}

func parseSpecs(args []string) []sync.Spec {
	specs := make([]sync.Spec, 0, len(args))
	for _, a := range args {
		if i := strings.Index(a, ":"); i >= 0 {
			specs = append(specs, sync.Spec{Src: a[:i], Dest: a[i+1:]})
		} else {
			specs = append(specs, sync.Spec{Src: a})
		}
	}
	return specs
}

func newCreateSyncCommand() *cobra.Command {
	var vaultFlag string
	cmd := &cobra.Command{
		Use:   "create-sync <path[:dest]>...",
		Short: "Stage notes for promotion to the team brain",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appFrom(cmd.Context())
			p, err := buildPromoter(app, vaultFlag)
			if err != nil {
				return err
			}
			res, err := p.CreateSync(parseSpecs(args), app.Config.DryRun)
			if err != nil {
				return err
			}
			return app.Emit("create-sync", res, func(w io.Writer) {
				verb := "staged"
				if app.Config.DryRun {
					verb = "would stage"
				}
				fmt.Fprintf(w, "%s %d note(s) for promotion:\n", verb, len(res.Staged))
				for _, it := range res.Staged {
					fmt.Fprintf(w, "  %s → %s\n", it.Src, it.Dest)
				}
				fmt.Fprintln(w, "review with `teambrain view-sync`, then `teambrain commit-sync`")
			})
		},
	}
	cmd.Flags().StringVar(&vaultFlag, "vault", "", "personal vault path (default: current directory)")
	return cmd
}

func newViewSyncCommand() *cobra.Command {
	var vaultFlag string
	cmd := &cobra.Command{
		Use:   "view-sync",
		Short: "Preview the staged payload with a diff and link-integrity report",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appFrom(cmd.Context())
			p, err := buildPromoter(app, vaultFlag)
			if err != nil {
				return err
			}
			view, err := p.ViewSync()
			if err != nil {
				return err
			}
			return app.Emit("view-sync", view, func(w io.Writer) {
				if len(view.Items) == 0 {
					fmt.Fprintln(w, "nothing staged")
					return
				}
				for _, it := range view.Items {
					fmt.Fprintf(w, "%-10s %s\n", it.Status, it.Dest)
					if it.Diff != "" {
						fmt.Fprint(w, indent(it.Diff, "    "))
					}
				}
				if len(view.LinkIssues) == 0 {
					fmt.Fprintln(w, "\nlink integrity: OK")
					return
				}
				fmt.Fprintf(w, "\nlink integrity: %d unresolved link(s) — these will dangle in the team vault:\n", len(view.LinkIssues))
				for _, li := range view.LinkIssues {
					fmt.Fprintf(w, "  %s → [[%s]]\n", li.Note, li.Link)
				}
			})
		},
	}
	cmd.Flags().StringVar(&vaultFlag, "vault", "", "personal vault path (default: current directory)")
	return cmd
}

func newCommitSyncCommand() *cobra.Command {
	var vaultFlag, message string
	var push bool
	cmd := &cobra.Command{
		Use:   "commit-sync",
		Short: "Promote the staged payload into the team vault and commit it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appFrom(cmd.Context())
			p, err := buildPromoter(app, vaultFlag)
			if err != nil {
				return err
			}
			res, err := p.CommitSync(sync.CommitOptions{Message: message, Push: push, DryRun: app.Config.DryRun})
			if err != nil {
				return err
			}
			return app.Emit("commit-sync", res, func(w io.Writer) {
				verb := "committed"
				if app.Config.DryRun {
					verb = "would commit"
				}
				fmt.Fprintf(w, "%s %d note(s) to the team vault:\n", verb, len(res.Committed))
				for _, d := range res.Committed {
					fmt.Fprintf(w, "  + %s\n", d)
				}
				if res.Pushed {
					fmt.Fprintln(w, "pushed to the team remote")
				}
			})
		},
	}
	cmd.Flags().StringVar(&vaultFlag, "vault", "", "personal vault path (default: current directory)")
	cmd.Flags().StringVar(&message, "message", "", "commit message (default: templated)")
	cmd.Flags().BoolVar(&push, "push", false, "push to the team remote after committing")
	return cmd
}

// indent prefixes every non-empty line of s with prefix.
func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	var b strings.Builder
	for _, ln := range lines {
		b.WriteString(prefix)
		b.WriteString(ln)
		b.WriteString("\n")
	}
	return b.String()
}
