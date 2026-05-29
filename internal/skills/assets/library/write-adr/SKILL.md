---
name: write-adr
description: Capture an architecture decision as a short, durable record (context, decision, consequences). Use when the user makes or revisits a technical decision worth remembering, or asks to write an ADR.
---

# Write an architecture decision record

An ADR records *why*, so the next person (often future-you) doesn't relitigate
a settled decision or repeat a discarded one. Keep it to one page.

> Find related notes and prior ADRs first via the Obsidian MCP/CLI (see the
> `search-brain` skill) — don't re-decide something already on record.

## Structure

```markdown
---
title: ADR-NNNN: <short decision title>
status: proposed | accepted | superseded
date: <YYYY-MM-DD>
---

## Context
The forces at play: constraints, requirements, what made this a decision and
not an obvious default. State them neutrally.

## Decision
What we chose, in one or two sentences. Active voice: "We will use X."

## Alternatives considered
Each real option and the one-line reason it lost. This is the most valuable
section — it shows the decision was reasoned, not arbitrary.

## Consequences
What becomes easier, what becomes harder, and what we now have to live with.
Include the bad parts honestly.
```

## Rules

- One decision per ADR. Number them; never edit an accepted one — supersede it
  with a new ADR that links back.
- Write for someone with context but not the meeting. No insider shorthand.
- If you can't articulate a discarded alternative, you haven't decided yet —
  you've just picked.

In a team brain, ADRs live in `adrs/`. Promote them with `teambrain create-sync`.
