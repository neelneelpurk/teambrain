---
name: synthesize-notes
description: Distill raw notes, a meeting, or a long thread into decisions, action items, and open questions. Use when the user has a pile of unstructured notes and wants the signal extracted.
---

# Synthesize notes

Raw notes are input; the output is what a busy reader needs: what was decided,
what to do, and what's still unknown. Compress aggressively.

## Connect it to the brain (Obsidian CLI)

Run `obsidian --help`, then use the Obsidian CLI to search the vault for related
notes and follow backlinks, so the synthesis links into what's already there
instead of floating alone. (No Obsidian CLI? See the `search-brain` skill.)

## Method

1. **Read everything once** before writing anything. Hold the whole picture.
2. **Extract, don't summarize.** Pull out the three things that matter:
   - **Decisions** — what was settled, and by whom if it matters.
   - **Action items** — owner, what, and (if stated) by when. One line each.
   - **Open questions** — what's unresolved and who needs to resolve it.
3. **Drop the rest.** Side chatter, restated context, and throat-clearing don't
   make the cut. If everything seems important, you haven't synthesized yet.
4. **Preserve links and names** verbatim — they're how the notes connect to the
   rest of the brain.

## Output

```markdown
## Decisions
- ...

## Action items
- [ ] @owner — action

## Open questions
- ...
```

Lead with decisions and actions; questions last. If there are no decisions, say
so plainly — that itself is a finding.
