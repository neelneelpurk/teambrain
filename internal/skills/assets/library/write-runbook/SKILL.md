---
name: write-runbook
description: Write an operational runbook for a service or procedure that a tired on-call engineer can follow at 3am. Use when documenting how to operate, deploy, or recover something.
---

# Write a runbook

A runbook is for someone under stress who did not write the system. Optimize for
copy-pasteable, unambiguous steps — not prose.

## Find related runbooks first

Use the **search-brain** skill to search the vault for existing runbooks and
notes about this system — reuse established steps, escalation paths, and naming
instead of duplicating them. Follow backlinks to the service's other docs.
search-brain queries the live vault through Obsidian (MCP preferred, else the
CLI), so you build on what's actually there.

## Structure

```markdown
---
title: Runbook: <service / procedure>
owner: <team>
last_reviewed: <YYYY-MM-DD>
---

## When to use this
The exact symptom, alert, or task this runbook addresses.

## Prerequisites
Access, tools, and env vars needed before you start. Link them.

## Steps
1. Numbered, imperative, one action each. Include the literal command.
2. After a risky step, say how to confirm it worked (and what "worked" looks like).

## Rollback
How to undo, and the trigger for deciding to.

## Escalation
Who/what to page if this doesn't resolve it, and after how long.
```

## Rules

- Every command is exact and copy-pasteable. No "configure the thing."
- State the expected output of checks, so the reader knows if they're on track.
- Note destructive steps loudly **before** the command, not after.
- Date it and name an owner. An unreviewed runbook is a liability, not an asset.

In a team brain, runbooks live in `runbooks/`.
