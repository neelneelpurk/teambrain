package cli

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/neelneelpurk/teambrain/internal/exit"
	"github.com/neelneelpurk/teambrain/internal/git"
	"github.com/neelneelpurk/teambrain/internal/manifest"
	"github.com/neelneelpurk/teambrain/internal/scaffold"
	"github.com/neelneelpurk/teambrain/internal/team"
	"github.com/neelneelpurk/teambrain/internal/vault"
)

func newTeamCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "team",
		Short: "Create, bind, and inspect team vaults (1:n)",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newTeamInitCommand(), newTeamBindCommand(), newTeamUnbindCommand(), newTeamStatusCommand())
	return cmd
}

func newTeamInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init <path>",
		Short: "Scaffold a team-brain vault",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appFrom(cmd.Context())
			v, err := vault.NewFSDirect(args[0])
			if err != nil {
				return exit.Userf("resolve team path %q: %v", args[0], err)
			}
			res, err := scaffold.TeamVault(v, app.Config.DryRun)
			if err != nil {
				return exit.Externalf("scaffold team vault: %v", err)
			}
			return app.Emit("team.init", res, func(w io.Writer) {
				verb := "created"
				if app.Config.DryRun {
					verb = "would create"
				}
				fmt.Fprintf(w, "team-brain vault at %s\n", res.Root)
				fmt.Fprintf(w, "%s %d file(s); %d already present\n", verb, len(res.Created), len(res.Existing))
			})
		},
	}
}

func newTeamBindCommand() *cobra.Command {
	var vaultFlag, name string
	var force bool
	cmd := &cobra.Command{
		Use:   "bind <path|remote>",
		Short: "Bind this personal vault to a (named) team vault",
		Long: `Register a team vault under a name. A personal vault may bind to several
teams (1:n); notes route to one or more of them via their teambrains: frontmatter
property. The name defaults to the target's final path segment (or repo name);
override it with --name. Rebinding an existing name to a different target needs
--force.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appFrom(cmd.Context())
			personalPath, err := resolvePersonalVault(app, vaultFlag)
			if err != nil {
				return err
			}
			root, err := loadPersonalRoot(personalPath)
			if err != nil {
				return err
			}
			teamName := name
			if teamName == "" {
				teamName = team.DeriveName(args[0])
			}
			if err := team.Bind(root, teamName, args[0], time.Now().UTC().Format(time.RFC3339), force); err != nil {
				return err
			}
			if !app.Config.DryRun {
				if err := manifest.SaveRoot(personalPath, root); err != nil {
					return err
				}
			}
			bound, _ := root.Team(teamName)
			return app.Emit("team.bind", bound, func(w io.Writer) {
				fmt.Fprintf(w, "bound team %q -> %s\n", teamName, team.Describe(bound))
			})
		},
	}
	cmd.Flags().StringVar(&vaultFlag, "vault", "", "personal vault path (default: current directory)")
	cmd.Flags().StringVar(&name, "name", "", "team name notes reference in teambrains: (default: derived from the target)")
	cmd.Flags().BoolVar(&force, "force", false, "rebind the name even if it points elsewhere")
	return cmd
}

func newTeamUnbindCommand() *cobra.Command {
	var vaultFlag string
	cmd := &cobra.Command{
		Use:   "unbind <name>",
		Short: "Remove a team binding by name",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := appFrom(cmd.Context())
			personalPath, err := resolvePersonalVault(app, vaultFlag)
			if err != nil {
				return err
			}
			root, err := loadPersonalRoot(personalPath)
			if err != nil {
				return err
			}
			if err := team.Unbind(root, args[0]); err != nil {
				return err
			}
			if !app.Config.DryRun {
				if err := manifest.SaveRoot(personalPath, root); err != nil {
					return err
				}
			}
			return app.Emit("team.unbind", map[string]any{"name": args[0]}, func(w io.Writer) {
				fmt.Fprintf(w, "unbound team %q\n", args[0])
			})
		},
	}
	cmd.Flags().StringVar(&vaultFlag, "vault", "", "personal vault path (default: current directory)")
	return cmd
}

type teamStatus struct {
	Name      string `json:"name"`
	Path      string `json:"path,omitempty"`
	Remote    string `json:"remote,omitempty"`
	Exists    bool   `json:"exists"`
	IsGitRepo bool   `json:"is_git_repo"`
	BoundAt   string `json:"bound_at,omitempty"`
}

func newTeamStatusCommand() *cobra.Command {
	var vaultFlag string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "List bound team vaults and their git state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := appFrom(cmd.Context())
			personalPath, err := resolvePersonalVault(app, vaultFlag)
			if err != nil {
				return err
			}
			root, err := loadPersonalRoot(personalPath)
			if err != nil {
				return err
			}

			g := git.NewShell()
			teams := make([]teamStatus, 0, len(root.Teams))
			for _, b := range root.Teams {
				st := teamStatus{Name: b.Name, Path: b.Path, Remote: b.Remote, BoundAt: b.BoundAt}
				if b.Path != "" {
					_, statErr := os.Stat(b.Path)
					st.Exists = statErr == nil
					st.IsGitRepo = g.IsRepo(b.Path)
				}
				teams = append(teams, st)
			}

			return app.Emit("team.status", map[string]any{
				"vault": personalPath,
				"bound": root.IsBound(),
				"teams": teams,
			}, func(w io.Writer) {
				if len(teams) == 0 {
					fmt.Fprintln(w, "no team vaults bound")
					fmt.Fprintln(w, "bind one with `teambrain team bind <path|remote> --name <name>`")
					return
				}
				for _, t := range teams {
					target := t.Remote
					if target == "" {
						target = t.Path
					}
					fmt.Fprintf(w, "%-16s %s\n", t.Name, target)
					if t.Path != "" {
						fmt.Fprintf(w, "  exists: %t   git repo: %t\n", t.Exists, t.IsGitRepo)
					}
				}
			})
		},
	}
	cmd.Flags().StringVar(&vaultFlag, "vault", "", "personal vault path (default: current directory)")
	return cmd
}

// loadPersonalRoot loads the personal vault's root manifest, mapping a missing
// vault to a precondition error.
func loadPersonalRoot(personalPath string) (*manifest.Root, error) {
	root, err := manifest.LoadRoot(personalPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, exit.Preconditionf("%s is not a teambrain vault", personalPath).
				WithHint("run `teambrain init` there first")
		}
		return nil, err
	}
	return root, nil
}
