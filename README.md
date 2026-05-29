# teambrain

> Two Obsidian vaults — a personal brain and a team brain — that Claude Code can read, extend, and share between. The brain is plain files; teambrain is a small, optional tool that does the few things files and git can't do safely on their own.

[![CI](https://github.com/neelneelpurk/teambrain/actions/workflows/ci.yml/badge.svg)](https://github.com/neelneelpurk/teambrain/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/neelneelpurk/teambrain.svg)](https://pkg.go.dev/github.com/neelneelpurk/teambrain)
[![Go Report Card](https://goreportcard.com/badge/github.com/neelneelpurk/teambrain)](https://goreportcard.com/report/github.com/neelneelpurk/teambrain)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

---

## What it is

`teambrain` manages two plain-text knowledge vaults for use with [Claude Code](https://claude.com/claude-code):

- a **personal brain** — private, yours alone;
- a **team brain** — shared with your team.

Both are normal [Obsidian](https://obsidian.md) vaults and normal git repositories. teambrain only adds the handful of operations that the filesystem and git cannot do safely by hand:

1. **Authoring** Claude Code capabilities (skills, agents, hooks, slash commands) with correct placement and valid frontmatter.
2. **Distributing** those capabilities into code repositories — copying the files **and** merging a hook into `settings.json` without clobbering foreign content, with ownership and checksums tracked so they can be cleanly removed.
3. **Promoting** notes from your personal brain to the team brain — an explicit copy gated by a **link-integrity check** so you never publish a note with links that dangle in the team vault.

Everything else is just files, `git`, and Obsidian.

## Why "file over app"

teambrain follows Steph Ango's (kepano's) *file over app* philosophy and Obsidian's principles of **privacy, longevity, extensibility**:

- **Everything is plain text** in open formats: Markdown notes, YAML frontmatter, plain-JSON `.teambrain.json` manifests. No opaque state, no database.
- **Files are the source of truth.** `list` is a live filesystem scan — there is no cache to desync.
- **The tool is optional.** Uninstall teambrain and you still have two Obsidian vaults, two git repos, and plain `.claude/` capabilities. Nothing is locked in. *That is the test every feature must pass.*
- **Deterministic core, agent for reasoning.** teambrain does file surgery; Claude Code does the thinking, expressed as embedded skills.
- **Degrade gracefully.** No Obsidian CLI? Use the filesystem. No agent? Deterministic output.

## Install

```sh
# With Go 1.25+
go install github.com/neelneelpurk/teambrain/cmd/teambrain@latest

# Or build from source
git clone https://github.com/neelneelpurk/teambrain
cd teambrain
make build      # produces ./bin/teambrain
```

Pre-built binaries for Linux, macOS, and Windows are attached to each [release](https://github.com/neelneelpurk/teambrain/releases).

## Quickstart

```sh
# 1. Scaffold your two vaults.
teambrain init ~/personal-brain
teambrain team init ~/team-brain

# 2. Make the team vault a git repo (your team shares this).
git -C ~/team-brain init && git -C ~/team-brain add -A && git -C ~/team-brain commit -m "init"

# 3. Bind your personal brain to the one team brain (1:1).
cd ~/personal-brain
teambrain team bind ~/team-brain
teambrain team status

# 4. Author a capability in your brain, with Claude Code's help.
teambrain skill new daily-review --description "Summarize today's notes and surface open loops"

# 5. Distribute a capability into a code repo (copies files + merges settings.json).
cd ~/code/my-service
teambrain hook import format-on-save --source ~/personal-brain   # shows the script, asks to confirm

# 6. Promote a note to the team brain — reviewed before it lands.
cd ~/personal-brain
teambrain create-sync projects/adr-0001.md:adrs/0001.md
teambrain view-sync          # shows a diff AND a link-integrity report
teambrain commit-sync --push # copies into the team vault, commits ONLY those files, pushes
```

See the **[User Guide](USERGUIDE.md)** for the full walkthrough.

## Architecture

```
~/personal-brain/             # Vault 1 — private (its own git repo, or no remote)
├── CLAUDE.md
├── .claude/                  # skills/ agents/ hooks/ commands/  + .teambrain.json (ownership)
├── _sync/                    # staging for promotion to the team (gitignored)
├── inbox/ daily/ projects/ areas/ resources/
└── .teambrain.json           # vault role + the 1:1 team binding

~/team-brain/                 # Vault 2 — shared (its own git repo)
├── CLAUDE.md
├── .claude/                  # skills/ agents/ hooks/ commands/  + .teambrain.json
└── adrs/ design-docs/ runbooks/ conventions/ mocs/
```

Two plain vaults, two plain git repos — no submodules, no symlinks. Links don't cross vaults; promotion is an explicit copy. That boundary is a feature: it keeps the separation clean and removes dangling cross-vault links entirely.

## Command reference

| Command | Purpose |
|---|---|
| `teambrain init [--here\|<path>]` | Scaffold (or repair) a personal-brain vault |
| `teambrain team init <path>` | Scaffold a team-brain vault |
| `teambrain team bind <path\|remote> [--force]` | Bind this personal vault to its one team vault |
| `teambrain team status` | Report the binding and the team vault's git state |
| `teambrain {skill,agent,hook,command} new <name>` | Author a new capability |
| `teambrain {skill,agent,hook,command} list` | List capabilities (live filesystem scan) |
| `teambrain {skill,agent,hook,command} import <name> --source <vault>` | Copy a capability into this repo's `.claude` |
| `teambrain {skill,agent,hook,command} update <name>` | Refresh an installed capability from its source |
| `teambrain {skill,agent,hook,command} uninstall <name>` | Remove a teambrain-owned capability |
| `teambrain create-sync <path[:dest]>...` | Stage notes for promotion |
| `teambrain view-sync` | Preview the payload with a diff and link-integrity report |
| `teambrain commit-sync [--push] [--message <m>]` | Promote into the team vault and commit those files |
| `teambrain doctor` | Report the active backend and check for capability tamper |

**Global flags:** `--vault-backend fs\|obsidian\|auto` · `--json` · `--dry-run` · `--yes` · `--verbose/-v` · `--quiet` · `--no-color`

### Exit codes

Stable and documented, so scripts can branch on them:

| Code | Meaning |
|---|---|
| `0` | success |
| `1` | user / validation error |
| `2` | precondition not met (e.g. no team bound) |
| `3` | external failure (git, Obsidian) |

### JSON for scripting

Every command accepts `--json` and emits a stable envelope. Human output is never meant to be parsed:

```sh
teambrain --json skill list | jq '.data.capabilities[].name'
```

```json
{
  "ok": true,
  "command": "skill.list",
  "data": { "capabilities": [ { "name": "daily-review", "kind": "skill", "path": "skills/daily-review/SKILL.md" } ] }
}
```

## Safety properties

- **`settings.json` is never clobbered.** Hook registration is a typed read-modify-write that preserves every foreign key and unknown field; only your entry is added.
- **Hooks run code, so import shows the script and confirms** (`--yes` to bypass) and never auto-runs it. `doctor` flags checksum drift (tamper detection).
- **Promotion stages by explicit path only** — never `git add -A` — and commits only the promoted files, tolerating an otherwise-dirty tree.
- **Nothing is written outside a vault.** Path containment is enforced on every write.
- **`uninstall` is exact.** It removes only teambrain-owned files and the matching `settings.json` entry, leaving the repo byte-identical otherwise.

## Backends

- **`fs` (default, always available)** reads and writes vault files directly.
- **`obsidian`** routes operations through the [Obsidian CLI](https://help.obsidian.md), whose decisive advantage is a **link-preserving move** using Obsidian's own resolver.
- **`auto`** picks `obsidian` when its CLI is detected, otherwise `fs`, and logs the choice. If you write directly to a live vault while Obsidian is running, teambrain warns about the desync risk.

> Retrieval is intentionally **out of scope**: configure your own Obsidian MCP in Claude Code. `doctor` will, read-only, remind you if none is detected.

## Development

```sh
make tools      # install golangci-lint (pinned)
make ci         # fmt-check + vet + lint + race tests + coverage gate (mirrors CI)
make test       # fast unit tests
make cover-html # open an HTML coverage report
```

teambrain is built test-first (red → green → refactor) with fakes at every boundary, golden files for every generated artifact, and a thin end-to-end layer via [`testscript`](https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript). See **[CONTRIBUTING.md](CONTRIBUTING.md)**.

## License

[Apache 2.0](LICENSE). See [NOTICE](NOTICE).

> The *file over app* framing follows Steph Ango's publicly stated philosophy; the application to this design is the authors', not a quote from him.
