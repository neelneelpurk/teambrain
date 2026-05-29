# teambrain User Guide

This guide walks through every teambrain workflow end to end. If you just want the elevator pitch and a command table, see the [README](README.md).

## Table of contents

1. [Concepts](#concepts)
2. [Installation](#installation)
3. [Setting up your vaults](#setting-up-your-vaults)
4. [Binding a team vault](#binding-a-team-vault)
5. [Authoring capabilities](#authoring-capabilities)
6. [Distributing capabilities into code repos](#distributing-capabilities-into-code-repos)
7. [Promoting notes to the team brain](#promoting-notes-to-the-team-brain)
8. [Hooks and safety](#hooks-and-safety)
9. [Health checks with doctor](#health-checks-with-doctor)
10. [Backends: fs vs Obsidian](#backends-fs-vs-obsidian)
11. [Scripting with `--json`](#scripting-with---json)
12. [Configuration](#configuration)
13. [Uninstalling teambrain](#uninstalling-teambrain)
14. [Troubleshooting](#troubleshooting)

---

## Concepts

| Term | Meaning |
|---|---|
| **Personal brain** | Your private Obsidian vault + git repo. Where you think and author. |
| **Team brain** | A shared Obsidian vault + git repo. Where vetted notes land for the team. |
| **Capability** | A Claude Code extension: a **skill**, **agent**, **hook**, or slash **command**, living under `.claude/`. |
| **Manifest** | A plain-JSON `.teambrain.json`. There are two kinds: the **root** manifest (vault role + team binding) and the **`.claude` ownership** manifest (what teambrain installed, with checksums). |
| **Promotion** | Copying a note from your personal brain to the team brain, gated by a link-integrity check. |

The golden rule: **files are the source of truth.** teambrain never holds authoritative state outside the vaults. Delete the tool and everything still works in Obsidian, git, and Claude Code.

## Installation

```sh
go install github.com/neelneelpurk/teambrain/cmd/teambrain@latest
teambrain --version
```

Requires Go 1.25+ to build. Pre-built binaries are on the [releases page](https://github.com/neelneelpurk/teambrain/releases). `git` is required for promotion; the Obsidian CLI is optional.

## Setting up your vaults

Create a personal brain:

```sh
teambrain init ~/personal-brain
```

This scaffolds:

```
~/personal-brain/
├── CLAUDE.md                         # orientation for Claude Code
├── .gitignore                        # ignores _sync/ and the optional cache
├── .teambrain.json                   # vault role = personal, team unbound
├── .claude/
│   ├── .teambrain.json               # ownership manifest (empty)
│   ├── skills/
│   │   ├── create-teambrain-skill/SKILL.md
│   │   ├── create-teambrain-agent/SKILL.md
│   │   └── create-teambrain-hook/SKILL.md
│   ├── agents/ hooks/ commands/
├── inbox/ daily/ projects/ areas/ resources/
```

`init` is **safe to re-run**: it only creates missing files, so it never overwrites your edits or your team binding. Re-running on an intact vault is a no-op; re-running on a vault missing a file repairs just that file.

Use `--here` to initialize the current directory, and `--dry-run` to preview:

```sh
mkdir ~/brain && cd ~/brain && teambrain init --here
teambrain --dry-run init ~/another-brain   # prints the plan, writes nothing
```

Create a team brain the same way (note the different folders — `adrs/`, `design-docs/`, `runbooks/`, `conventions/`, `mocs/`):

```sh
teambrain team init ~/team-brain
```

## Binding a team vault

A personal brain points at exactly **one** team brain. The binding is a single field in `~/personal-brain/.teambrain.json` — no central registry.

```sh
cd ~/personal-brain
teambrain team bind ~/team-brain          # bind to a local path
# or
teambrain team bind git@github.com:acme/team-brain.git   # bind to a remote
```

Check it:

```sh
teambrain team status
# team: /Users/you/team-brain
# exists: true
# git repo: true
```

Rebinding to a **different** team is refused unless you pass `--force`, so the link never changes by accident:

```sh
teambrain team bind ~/other-team           # error: a team vault is already bound
teambrain team bind ~/other-team --force   # ok
```

> All `team` and sync commands accept `--vault <path>` to target a personal vault other than the current directory, or set `personal_vault` in config.

## Authoring capabilities

teambrain places capability files deterministically with valid frontmatter; you (with Claude Code) write the content. Each vault is seeded with `create-teambrain-*` skills that drive this from inside Claude Code.

```sh
# A skill: .claude/skills/<name>/SKILL.md
teambrain skill new daily-review --description "Summarize today's notes and surface open loops"

# An agent: .claude/agents/<name>.md
teambrain agent new researcher --description "Investigate open questions across the vault"

# A slash command: .claude/commands/<name>.md
teambrain command new triage --description "Triage the inbox into projects and areas"

# A hook: .claude/hooks/<name>.sh + a settings.json entry + ownership
teambrain hook new format-go --event PostToolUse --matcher "Edit|Write"
```

List what exists — this is a **live filesystem scan**, so it always reflects disk:

```sh
teambrain skill list
teambrain --json hook list   # includes the event, read from the manifest
```

Delete a capability file and it simply disappears from `list`. There is no cache to desync.

> All authoring/listing commands take `--dir <path>` to operate on a `.claude` folder other than the one in the current directory.

## Distributing capabilities into code repos

Capabilities authored in a brain can be installed into any code repository's `.claude/`. Run the command **inside the target repo** ("act where you are").

```sh
cd ~/code/my-service

# Copy a skill from your personal brain.
teambrain skill import daily-review --source ~/personal-brain

# Hooks run code, so import shows the script and asks before installing.
teambrain hook import format-go --source ~/personal-brain
#   About to import hook "format-go" from "personal-brain".
#   Event: PostToolUse
#   This script will run on that event. Review it:
#   ----------------------------------------
#   #!/usr/bin/env bash
#   ...
#   ----------------------------------------
#   Import this hook? [y/N]:
```

Pass `--yes` to skip the prompt (e.g. in automation). Use `--mode link` to symlink instead of copy (skipped on Windows).

If a capability name exists in **more than one** source vault, import refuses to guess:

```sh
teambrain skill import shared --source ~/personal-brain --source ~/team-brain
#   error: skill "shared" is ambiguous; found in personal-brain, team-brain
#   Hint: disambiguate with --from <personal-brain|team-brain>

teambrain skill import shared --source ~/personal-brain --source ~/team-brain --from team-brain
```

teambrain records ownership (with a checksum) in the repo's `.claude/.teambrain.json`. To refresh after the source changes, or to remove cleanly:

```sh
teambrain skill update daily-review --source ~/personal-brain
teambrain skill uninstall daily-review
```

`uninstall` removes **only** teambrain-owned files and the matching `settings.json` entry — your own files and foreign hooks are never touched. When the last owned capability is removed, the ownership manifest is deleted too, leaving no teambrain artifact behind.

### Configuring a default source

So you don't have to pass `--source` every time, set your personal vault in config; its bound team vault is then a source too:

```sh
mkdir -p ~/.config/teambrain
printf 'personal_vault: ~/personal-brain\n' > ~/.config/teambrain/config.yaml
cd ~/code/my-service
teambrain skill import daily-review      # searches personal + team automatically
```

## Promoting notes to the team brain

Promotion is a deliberate three-step flow with a safety gate.

**1. Stage** the notes you want to share. The destination mirrors where they'll live in the team vault (`src:dest`, or just `src` to keep the same path):

```sh
cd ~/personal-brain
teambrain create-sync projects/adr-0001.md:adrs/0001.md runbooks/deploy.md
```

This copies the notes into `~/personal-brain/_sync/` (gitignored), normalizing their frontmatter. **Originals are untouched.**

**2. Review** before anything is published:

```sh
teambrain view-sync
```

`view-sync` shows, for each staged note, whether it's **new** or **modified** in the team vault (with a diff), and a **link-integrity report**:

```
new        adrs/0001.md
    + ---
    + title: ADR 1
    + ---
    + relates to [[secret-research]]

link integrity: 1 unresolved link(s) — these will dangle in the team vault:
  adrs/0001.md → [[secret-research]]
```

A link resolves if its target already exists in the team vault **or** is itself part of this promotion. Anything else is flagged so you can also-stage it, inline it, or leave it — your call, before it lands.

**3. Commit** into the team vault:

```sh
teambrain commit-sync               # copy + commit those files
teambrain commit-sync --push        # also push to the team remote
teambrain commit-sync --message "promote auth ADR"
```

`commit-sync` copies `_sync/` into the team vault, **stages and commits only those paths** (a dirty tree is fine — your teammates' uncommitted work is left alone), optionally pushes, and clears `_sync/`. Use `--dry-run` to see exactly what would be committed without writing anything.

## Hooks and safety

Hooks are the one capability that executes code, so teambrain treats them carefully:

- **`hook new`** writes the script, merges the registration into `settings.json` with a **typed read-modify-write** that preserves every foreign hook and unknown field, and records a checksum.
- **`hook import`** shows the script and confirms before installing; it never runs the hook.
- **`doctor`** recomputes checksums and flags any drift — your early warning that an installed hook (or any owned capability) was modified.
- **`hook uninstall`** unmerges exactly your entry from `settings.json`, pruning emptied groups, and leaves foreign content intact.

## Health checks with doctor

```sh
teambrain doctor
# vault backend: auto (active: fs)
# obsidian CLI:  not detected
# ownership:     OK (no checksum drift)
# retrieval:     no .mcp.json found; configure an Obsidian MCP in Claude Code for retrieval
```

`doctor` inspects the `.claude` in the current directory (override with `--dir`). Under `--json` it returns a `healthy` boolean and a `drift` array you can gate CI on:

```sh
teambrain --json doctor --dir ~/code/my-service | jq '.data.healthy'
```

## Backends: fs vs Obsidian

teambrain works fully without Obsidian. The optional `obsidian` backend adds a **link-preserving move** (using Obsidian's resolver instead of teambrain's documented subset) and richer queries.

```sh
teambrain --vault-backend fs   ...   # always works
teambrain --vault-backend obsidian ...   # requires the Obsidian CLI; falls back to fs with a warning
teambrain --vault-backend auto ...       # obsidian if detected, else fs (the default)
```

If you choose `fs` while Obsidian is running on the same vault, teambrain warns that direct writes may desync the live app.

The filesystem link rewriter supports a documented subset of wikilinks: `[[note]]`, `[[note|alias]]`, `[[note#heading]]`, and full-path forms. Embeds (`![[note]]`) and block references (`[[note#^id]]`) to a moved note are **reported, not rewritten** — teambrain warns rather than risk mangling them.

## Scripting with `--json`

Every read and mutation supports `--json`, returning a stable envelope:

```json
{ "ok": true, "command": "create-sync", "data": { /* ... */ }, "warnings": [] }
```

On error:

```json
{ "ok": false, "command": "skill.import", "error": { "code": 1, "kind": "user", "message": "...", "hint": "..." } }
```

Combine with the documented exit codes (`0/1/2/3`) to build robust automation. Human output is never meant to be parsed.

## Configuration

Resolution order (highest first): **command-line flag → environment variable → config file → default.**

- **Config file:** `$XDG_CONFIG_HOME/teambrain/config.yaml` (or `~/.config/teambrain/config.yaml`).
- **Environment:** `TEAMBRAIN_<KEY>`, e.g. `TEAMBRAIN_VAULT_BACKEND=fs`.

| Key | Default | Meaning |
|---|---|---|
| `vault_backend` | `auto` | `fs`, `obsidian`, or `auto` |
| `personal_vault` | _(unset)_ | default personal vault for `import`/sync source resolution |
| `json` | `false` | always emit JSON |
| `dry_run` / `yes` / `verbose` / `quiet` / `no_color` | `false` | global behavior toggles |

## Uninstalling teambrain

This is the design's acid test. Remove the binary and you are left with:

- two normal Obsidian vaults you can open and edit;
- two normal git repositories;
- plain `.claude/` skills, agents, hooks, and commands that Claude Code reads directly;
- human-readable `.teambrain.json` manifests you can delete or keep.

Nothing is locked in.

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| `is not a teambrain vault` (exit 2) | Run `teambrain init` in that directory, or pass `--vault <path>`. |
| `no team vault bound` (exit 2) | Run `teambrain team bind <path\|remote>`. |
| `team vault is not a git repository` (exit 3) | `git init` the team vault (and add a remote for `--push`). |
| `is ambiguous; found in ...` (exit 1) | Pass `--from <label>` to pick a source. |
| `not a teambrain-owned capability` (exit 1) | You can only `uninstall` what teambrain installed; check `… list`. |
| `doctor` reports drift | An owned file changed since install — re-`update` it, or investigate possible tampering. |
| Permission error reading `~/Downloads` etc. on macOS | Grant your terminal Files & Folders / Full Disk Access in System Settings. |
