# CLAUDE.md

Working guide for Claude Code on the **teambrain** codebase. (This is for hacking
on the tool itself — not to be confused with the vault `CLAUDE.md` files that
`teambrain init` generates, which live in `internal/scaffold/scaffold.go`.)

## What this is

A single static Go CLI that manages two kinds of Obsidian vaults — a personal
brain and one or more team brains — for use with Claude Code. Philosophy:
**file over app**. Everything teambrain writes is plain text (Markdown, YAML
frontmatter, plain-JSON `.teambrain.json` manifests). The deterministic core
does file/git surgery; Claude Code does the reasoning via embedded skills.
There is **no database, no LLM/network calls, and no embeddings** in the core.

Prime directive: every feature must pass *"uninstall teambrain and the vaults
still work in Obsidian + git + Claude Code."*

- Module: `github.com/neelneelpurk/teambrain` · Go 1.25 · Apache-2.0.

## Commands

```sh
make ci          # THE GATE — run before every commit/push. Must be green.
make build       # -> ./bin/teambrain
make test        # fast unit tests
make race        # go test ./... -race
make cover-check # per-package internal/* coverage floor (>=80%)
make lint        # golangci-lint (v2; `make tools` installs the pinned version)
go test ./... -update   # regenerate golden files — ALWAYS inspect the diff after
```

CI (`.github/workflows/ci.yml`) runs the same gates on Linux, macOS, **and
Windows**. Use `gh run view <id> --log` to read failures; don't assume local ==
CI (see Gotchas).

## Definition of Done (gates)

`make ci` must pass: `gofmt` clean · `go vet` · `golangci-lint` 0 issues ·
`go test ./... -race` · every `internal/*` package ≥ 80% coverage. New behavior
needs new tests.

## How we build: strict TDD

- **Red → green → refactor.** No production code without a failing test first.
- **Fakes at every boundary** — `vault.FakeVault`, `git.Fake`, `clock.Fake`. Tests
  never touch the real network, a real Obsidian, or a real remote. Git
  integration tests use a local bare repo in `t.TempDir()`.
- **Golden files** pin every generated artifact (SKILL.md, manifests,
  `settings.json` merges, diffs, commit messages) via `internal/testutil` +
  `-update`. Regenerate with `-update`, then **read the result** — goldens are
  the spec.
- **Thin end-to-end** via `rogpeppe/go-internal/testscript`:
  `cmd/teambrain/testdata/script/*.txtar`.

## Package map

| Package | Responsibility |
|---|---|
| `cmd/teambrain` | entrypoint + testscript harness |
| `internal/cli` | Cobra commands, config (Viper/XDG), `--json` envelope, exit-code mapping |
| `internal/exit` | stable exit codes + structured errors |
| `internal/vault` | `Vault` iface; `fsdirect` backend; frontmatter; wikilinks |
| `internal/capability` | author (`new`), list, distribute (`import`/`update`/`uninstall`), `settings.json` merge, drift detection |
| `internal/manifest` | the two `.teambrain.json` schemas (root + `.claude` ownership) |
| `internal/sync` | tag-routed promotion (`create`/`view`/`commit`) + link-integrity gate |
| `internal/git` | narrow, **path-scoped** git boundary |
| `internal/team` | named 1:n team bindings |
| `internal/scaffold` | personal/team vault trees |
| `internal/skills` | embedded seed + library skills (`//go:embed assets`) |
| `internal/clock` | injectable time source |

Every boundary is an interface with an in-memory fake. Keep it that way.

## Conventions (match these)

- **Output:** return results via `app.Emit(command, data, human func(io.Writer))`.
  It emits the JSON envelope under `--json`, else calls the human renderer.
  Never hand-roll JSON or print results ad hoc.
- **Errors/exit codes:** `internal/exit` — `Userf` (1, bad input), `Preconditionf`
  (2, env not ready), `Externalf` (3, git/Obsidian). Add a `.WithHint(...)`.
  The top level maps any error via `exit.CodeOf`.
- **Time:** inject `clock.Clock`; use `clock.NewFake` in tests so timestamps are
  deterministic and gold-able.
- **Mutations** honor `--dry-run` and confirm before touching a code repo, a
  team vault, or `settings.json` unless `--yes`. No surprise writes.

## Invariants — do not break

- **Path containment:** never read/write outside a vault. `vault.FSDirect`
  rejects escapes (`ErrOutsideVault`); route file ops through it.
- **`settings.json` is merged, never clobbered:** `capability` does a typed
  read-modify-write preserving foreign keys/unknown fields. Removal unmerges
  only our entry.
- **Git is path-scoped:** `Add(dir, paths)` + `Commit(dir, msg, paths)` only —
  never `git add -A` or `git add .`. Promotion tolerates a dirty tree.
- **Retrieval is Obsidian's job.** teambrain ships **no** search engine/index.
  The embedded `search-brain` skill drives the Obsidian MCP → Obsidian CLI;
  `doctor` reports the retrieval path; `init` warns if neither is present.
- **1:n promotion routing:** a note declares targets in its `teambrains: [..]`
  frontmatter (optional `teambrain_dest:` override, else same relative path).
  `create-sync` stages per team into `_sync/<team>/`, **stripping the routing
  props** from the promoted copy; `commit-sync` fans out per team.
- **Embedded skills:** `internal/skills/assets/{scaffold,library}/<name>/SKILL.md`.
  All are seeded by `init`; `library/` is also the `skill catalog`/`skill add`
  set. Every library skill must drive the Obsidian CLI to find files —
  `TestLibrarySkillsDriveObsidianCLI` enforces it.
- **Manifests are authoritative plain JSON.** Any cache is derived/disposable.

## Gotchas (these have bitten us)

- **Line endings:** golden comparisons are byte-exact and CI runs on Windows.
  `.gitattributes` forces `eol=lf` — keep it; never commit CRLF fixtures.
- **Host Obsidian skews retrieval *reporting* (not writes):** vault access is
  always direct `fs`, so a host `obsidian` on `PATH` can't hijack reads/writes.
  But `doctor`/`init` detect that CLI for the *retrieval* path, so a test that
  asserts retrieval status must isolate `PATH` (`t.Setenv("PATH", t.TempDir())`)
  — see `TestRetrievalStatus` / `TestInitWarnsWhenObsidianAbsent`.
- **Coverage script** (`scripts/check-coverage.sh`) gates only `ok … coverage:`
  lines; the no-test helper `internal/testutil` is intentionally not gated.
- **CRLF/path tests:** compare paths against `filepath.Abs(...)`, not Unix
  literals — `Abs` is OS-specific on Windows (`D:\...`).

## Adding a command

1. Write the failing test (unit + a `.txtar` for user-visible behavior).
2. Implement the deterministic core in the relevant `internal/*` package.
3. Wire the Cobra command in `internal/cli`, register it in `root.go`, support
   `--json` and `--dry-run`, and confirm before risky writes.
4. `make ci`; regenerate goldens with `-update` and inspect them.

See `CONTRIBUTING.md` for more, `README.md`/`USERGUIDE.md` for product behavior.
