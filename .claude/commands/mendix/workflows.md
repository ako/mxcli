---
description: Pick a prepared workflow (vibe code, …) for working on this app
---

# Workflows

Run `./mxcli workflow list --json` and `./mxcli workflow status --json`.

- If a run is open, say which one and its last gate results, and ask whether to continue it.
- Otherwise ask the user which workflow to use with your question tool (`AskUserQuestion` in Claude Code), one option per workflow: its title as the label and its summary as the description. With no question tool, ask as a numbered list.

Then follow the chosen workflow's skill (its `skill` field, in `.claude/skills/<skill>/SKILL.md`), asking first for what that workflow needs — for vibe code, the change in a sentence.
