# Contributing to teambrain

Thanks for your interest in improving teambrain! This document covers how to set up, the conventions we follow, and how to get a change merged.

## Ground rules

teambrain has a strong design spine — please read it before proposing changes:

- **File over app.** Everything teambrain writes is plain text in open formats. No databases, no opaque state.
- **The tool must make itself unnecessary.** Every feature must pass the test: *uninstall teambrain and the brain still works.*
- **Deterministic core, agent for reasoning.** teambrain does file surgery; Claude Code does the thinking. teambrain makes no network calls and needs no tokens.
- **Resist scope creep.** Solve the 90% simply. Heavier ideas (a disposable cache, a headless API engine, an Obsidian plugin) are deliberately deferred; open an issue to discuss before building them.

## Development setup

```sh
git clone https://github.com/neelneelpurk/teambrain
cd teambrain
make tools     # installs the pinned golangci-lint
make ci        # the full gate set — run this before every PR
```

Requires Go 1.25+ and `git`. The Obsidian CLI is optional (its backend is covered by stub tests).

## Test-driven development

teambrain is built **red → green → refactor**. No production code without a failing test first.

- **Pyramid:** table-driven unit tests, some integration, a thin end-to-end layer via [`testscript`](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript) (`cmd/teambrain/testdata/script/*.txtar`).
- **Fakes at every boundary** — `Vault`, `Git`, `Clock`, and the command runner all have in-memory fakes. **Tests never touch the real network, a real Obsidian, or a real remote.** Git integration tests use a local bare repo.
- **Golden files** pin every generated artifact (SKILL.md, agent/command markdown, `settings.json` merges, manifests, diffs, commit messages). Regenerate with:

  ```sh
  go test ./... -update
  ```

  Always **inspect** regenerated goldens before committing — they are the spec.

## Gates (Definition of Done)

A change is done when `make ci` is green:

| Gate | Command |
|---|---|
| Formatting | `gofmt` (CI fails on unformatted files) |
| Vet | `go vet ./...` |
| Lint | `golangci-lint run` (config in `.golangci.yml`) |
| Tests + race | `go test ./... -race` |
| Coverage | every `internal/*` package ≥ 80% (`scripts/check-coverage.sh`) |

CI additionally runs the suite on Linux, macOS, and Windows.

## Package layout

| Package | Responsibility |
|---|---|
| `cmd/teambrain` | entrypoint + testscript harness |
| `internal/cli` | Cobra commands, config (Viper/XDG), the `--json` envelope, exit-code mapping |
| `internal/exit` | stable exit codes + structured errors |
| `internal/vault` | `Vault` interface, `fsdirect` backend, frontmatter, link rewriting |
| `internal/capability` | authoring (`new`), inventory (`list`), distribution (`import`/`update`/`uninstall`), `settings.json` merge, drift detection |
| `internal/manifest` | the two `.teambrain.json` schemas + read/write |
| `internal/sync` | promotion (`create`/`view`/`commit`) + the link-integrity gate |
| `internal/git` | the narrow, path-scoped git boundary |
| `internal/team` | the 1:1 team binding + force-guard |
| `internal/scaffold` | personal/team vault trees |
| `internal/skills` | embedded seed skills |
| `internal/clock` | injectable time source |

Every boundary is an interface with an in-memory fake. Keep it that way.

## Adding a command or capability

1. Write the failing test (unit + a `.txtar` for user-visible behavior).
2. Implement the deterministic core in the relevant `internal/*` package.
3. Wire the Cobra command in `internal/cli`, supporting `--json` and `--dry-run`.
4. Honor the UX contract: confirm before touching a code repo, the team vault, or `settings.json` (unless `--yes`); never make surprise writes.
5. Run `make ci`. Update goldens with `-update` and inspect them.

## Commit & PR style

- Keep commits focused; write messages that explain the *why*.
- Open a PR against `main`. The PR description should state what changed and how you tested it.
- CI must be green. New behavior needs new tests.

## Reporting bugs & requesting features

Use the [issue templates](.github/ISSUE_TEMPLATE). For security issues, see [SECURITY.md](SECURITY.md) — please do **not** open a public issue.

By contributing, you agree that your contributions are licensed under the [Apache 2.0 License](LICENSE).
