package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/neelneelpurk/teambrain/internal/git"
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
		return "", err
	}
	return cwd, nil
}

// buildPromoter constructs a Promoter from the personal vault and every bound
// team vault (1:n). Remote-only teams (no local path) are skipped with a warning
// since promotion needs a local working tree.
func buildPromoter(app *App, vaultFlag string) (*sync.Promoter, error) {
	personalPath, err := resolvePersonalVault(app, vaultFlag)
	if err != nil {
		return nil, err
	}
	root, err := loadPersonalRoot(personalPath)
	if err != nil {
		return nil, err
	}

	backend := vault.Backend(app.Config.VaultBackend)
	personal, err := vault.Open(backend, personalPath, vault.DetectObsidian, app.Warn)
	if err != nil {
		return nil, err
	}

	var targets []sync.TeamTarget
	for _, b := range root.Teams {
		if b.Path == "" {
			app.Warn("team %q is bound by remote only; clone it locally and rebind with a path to promote", b.Name)
			continue
		}
		tv, err := vault.Open(backend, b.Path, vault.DetectObsidian, app.Warn)
		if err != nil {
			return nil, err
		}
		targets = append(targets, sync.TeamTarget{Name: b.Name, Vault: tv})
	}
	return sync.NewPromoter(personal, targets, git.NewShell()), nil
}

func newCreateSyncCommand() *cobra.Command {
	var vaultFlag string
	cmd := &cobra.Command{
		Use:   "create-sync [path]...",
		Short: "Stage tagged notes for promotion to their team brains",
		Long: `Stage notes for promotion. Each note routes itself by listing team names in
its teambrains: frontmatter property; a note can target several teams. With no
paths, create-sync scans the whole vault for tagged notes; with paths, it stages
just those. Notes are copied into _sync/<team>/ with frontmatter normalized.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appFrom(cmd.Context())
			p, err := buildPromoter(app, vaultFlag)
			if err != nil {
				return err
			}
			res, err := p.CreateSync(args, app.Config.DryRun)
			if err != nil {
				return err
			}
			for _, w := range res.Warnings {
				app.Warn("%s", w)
			}
			return app.Emit("create-sync", res, func(w io.Writer) {
				verb := "staged"
				if app.Config.DryRun {
					verb = "would stage"
				}
				fmt.Fprintf(w, "%s %d note→team route(s):\n", verb, len(res.Staged))
				for _, it := range res.Staged {
					fmt.Fprintf(w, "  %s → %s/%s\n", it.Src, it.Team, it.Dest)
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
		Short: "Preview each team's staged payload with a diff and link-integrity report",
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
				if len(view.Teams) == 0 {
					fmt.Fprintln(w, "nothing staged")
					return
				}
				for _, tv := range view.Teams {
					fmt.Fprintf(w, "── team: %s ──\n", tv.Team)
					for _, it := range tv.Items {
						fmt.Fprintf(w, "%-10s %s\n", it.Status, it.Dest)
						if it.Diff != "" {
							fmt.Fprint(w, indent(it.Diff, "    "))
						}
					}
					if len(tv.LinkIssues) == 0 {
						fmt.Fprintln(w, "link integrity: OK")
					} else {
						fmt.Fprintf(w, "link integrity: %d unresolved link(s) — will dangle in %s:\n", len(tv.LinkIssues), tv.Team)
						for _, li := range tv.LinkIssues {
							fmt.Fprintf(w, "  %s → [[%s]]\n", li.Note, li.Link)
						}
					}
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
		Short: "Promote each team's staged payload into its vault and commit it",
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
				for _, tc := range res.Teams {
					fmt.Fprintf(w, "%s %d note(s) to %s:\n", verb, len(tc.Committed), tc.Team)
					for _, d := range tc.Committed {
						fmt.Fprintf(w, "  + %s\n", d)
					}
					if tc.Pushed {
						fmt.Fprintf(w, "  pushed to the %s remote\n", tc.Team)
					}
				}
			})
		},
	}
	cmd.Flags().StringVar(&vaultFlag, "vault", "", "personal vault path (default: current directory)")
	cmd.Flags().StringVar(&message, "message", "", "commit message (default: templated per team)")
	cmd.Flags().BoolVar(&push, "push", false, "push each team to its remote after committing")
	return cmd
}

// indent prefixes every line of s with prefix.
func indent(s, prefix string) string {
	var b strings.Builder
	for _, ln := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString(prefix)
		b.WriteString(ln)
		b.WriteString("\n")
	}
	return b.String()
}
