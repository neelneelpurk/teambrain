---
name: search-brain
description: Find relevant notes in the vault before answering anything that depends on personal or team knowledge. Retrieval is done through Obsidian (MCP or CLI), which must be present. Use whenever a question references past decisions, people, projects, runbooks, or vault notes — search first, don't guess.
---

# Search the brain

The vault is the source of truth, and **Obsidian is how you read it**. Retrieve
before reasoning — don't answer from memory or guess from filenames.

## Choose the retrieval tool (required — in priority order)

1. **Obsidian MCP** — if Obsidian MCP tools are connected this session, use them
   first. They query the live app: the best search, backlinks, outline, and link
   resolution available.
2. **Obsidian CLI** — else if `obsidian` is on your PATH, use it. Run
   `obsidian --help` for the exact commands (the CLI is evolving), then search
   the vault, read notes, and list backlinks to pull context.
3. **Neither present? STOP.** Tell the user that brain retrieval needs the
   Obsidian CLI or an Obsidian MCP, and how to set one up. Do **not** fall back to
   blind `grep`/`glob` over the vault — that is exactly the low-signal guessing
   this skill exists to prevent. (`teambrain doctor` reports which path is active.)

## Method

1. **Decompose** the question into 2–4 keyword queries — entities and terms, not
   prose sentences.
2. **Search** with the chosen Obsidian tool to find the candidate notes.
3. **Read only what you need.** A note is a table of contents; pull the specific
   note or heading that answers the query. Follow **backlinks** to widen only when
   a result is incomplete.
4. **Re-search** when results are weak — reformulate, don't fetch blindly. Stop as
   soon as you have enough; over-fetching pollutes the context window.

## Cite

Every claim drawn from the vault cites its source note (and heading when known),
so the user can verify it. If the vault is silent on something, **say so** — never
invent a note or a fact.
