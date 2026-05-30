---
name: promote-to-team
description: Promote notes from this personal brain to a bound team brain — decide what is team-worthy, route each note by its teambrains frontmatter property, resolve links that would dangle in the team vault, then commit. Use when the user wants to share, publish, sync, or push notes to their team.
---

# Promote notes to a team brain

Promotion copies vetted notes from the personal brain into a shared **team
brain**. teambrain owns the deterministic file surgery and git; you own the
judgment — *which* notes are team-worthy, where they land, and how to resolve
links that would dangle once a note leaves the personal vault.

A note routes itself: it lists its target teams in a `teambrains:` frontmatter
property (with an optional `teambrain_dest:` path override). One note can target
several teams at once.

## 0. Preconditions

Run `teambrain team status` to see the bound teams and their names. If the user
wants a team that isn't bound yet, stop and have them bind it first:

```sh
teambrain team bind <path|remote> --name <name>
```

Promotion needs a team with a local working tree; a remote-only binding is
skipped with a warning until it is cloned and rebound by path.

## 1. Decide what to promote (the reasoning)

- If the user named specific notes, use those.
- Otherwise, use the **search-brain** skill (Obsidian MCP or CLI) to find the
  candidates — recent decisions, ADRs, runbooks, or conventions worth sharing.
  Search the live vault; don't guess from filenames.
- For each note the user agrees to share, make sure its frontmatter names the
  right team(s): `teambrains: [<name>, ...]`. Add `teambrain_dest:` only when the
  team path should differ from the note's own path. Confirm before editing a
  note's frontmatter.

## 2. Stage

```sh
teambrain create-sync <path>...   # stage specific notes (additive)
teambrain create-sync             # or rebuild the whole set from frontmatter
```

Staging copies each note into `_sync/<team>/`, strips the routing properties
from the promoted copy, and leaves the originals untouched. A whole-vault scan
rebuilds the set, so an untagged note's stale copy is cleared automatically.

## 3. Review — and resolve dangling links (the gate)

```sh
teambrain view-sync
```

This shows, per team, a diff of each note plus a **link-integrity report**. A
link resolves if its target already exists in that team vault or is part of the
same payload. For every link flagged as dangling, decide *with the user*:

- **also-tag the target** — add the linked note to its own `teambrains:` and
  re-stage it, so it travels along (best when the target is itself shareable);
- **inline** the referenced content into the note;
- **edit the link** to point at something the team already has, or drop it;
- **accept it** — only deliberately, via `--force` at commit time.

Re-run `create-sync` + `view-sync` until the report is clean (or the user has
chosen to force specific dangles knowingly).

## 4. Commit

```sh
teambrain commit-sync          # writes to the team repo(s); confirms first
teambrain commit-sync --push   # also push to each team's remote
teambrain commit-sync --force  # promote despite dangling links (deliberate)
```

commit-sync writes to **shared** repositories, so it shows what it will do and
asks before committing — answer the prompt, or pass `--yes` when you have
already reviewed. It commits only the promoted paths, tolerating an
otherwise-dirty tree, and clears the staging on success.

## Principles

- Promotion is deliberate and one-directional: personal → team, an explicit copy.
- Never publish a dangling link by accident — resolve it, or force it knowingly.
- Keep the personal originals intact; only the promoted copy is normalized.
