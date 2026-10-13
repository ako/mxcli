---
title: mxcli companion, slice 3 — prepared workflows held to quality gates
status: partial
date: 2026-10-13
related:
  - proposal-mxcli-companion-mod.md
  - PROPOSAL_project_brain.md
  - proposal-eval-framework.md
---

# mxcli companion, slice 3 — prepared workflows held to quality gates

Slice 3 of the [companion umbrella](proposal-mxcli-companion-mod.md). A user
working on a Mendix app with Claude Code usually wants one of a handful of
things, and each has a right way to do it that the agent does not reliably
follow on its own. This slice packages those as **prepared workflows**, offers
them in the companion's banner and as slash commands, and holds each to
**gates** that mxcli commands decide. The vibe-code workflow is built as the
prototype (status: partial); the other three are specified here and built next.

## 1. Problem

The skills that make a change good already exist — `manage-security`,
`test-microflows`, `atlas-design`, `assess-quality`, `upgrade-mendix-version`,
`assess-migration`, `bootstrap-app`, `project-brain` — but nothing sequences
them, and nothing checks that a step happened. An agent asked for "a quick
change" makes the change and stops: a new entity with no access rules, a
microflow with no test, a page with inline colours, no record of why. Each
skill is right; the gap is the **procedure** around them and the **evidence**
that it was followed.

Users also do not know these skills exist. A fresh session after `mxcli init`
offers a blank prompt.

## 2. The four workflows

| Workflow | The user brings | What it guarantees | Built from |
|---|---|---|---|
| **Vibe code** *(prototype)* | a change, in a sentence | the change lands with security, a clean build, tests and a change note | `manage-security`, `test-microflows`, `atlas-design`, `assess-quality` |
| **PRD to app** | a PRD or spec, optionally a prototype or mockup | an intent document, specs with requirements, tests per requirement, security, theme, reviews — a production-grade app, not a demo | `bootstrap-app`, `migrate-design-prototype`, `project-brain`, the vibe gates |
| **Upgrade Mendix** | the target version | tests exist *before* the upgrade (so "done" is defined), the upgrade, marketplace modules and theme moved to current versions, the same tests passing after, every step recorded | `upgrade-mendix-version`, `test-microflows`, `test-app`, marketplace commands |
| **Migrate legacy** | the old app's source code and documents | intent and specs read out of the old app, tests written from its behaviour, the Mendix app built, the tests passing as the evidence the migration is correct | `assess-migration`, `migrate-oracle-forms`, `migrate-k2-nintex`, `project-brain` |

## 3. Design

### 3.1 A workflow is a skill plus a `workflow.json`

The procedure is a skill (`.claude/skills/mendix/workflow-<id>/SKILL.md`): the
agent reads it and follows it, as with every other skill. Beside it, a
`workflow.json` declares the workflow for tools: id, title, one-line summary,
command, skill, and its **gates**.

A separate file rather than frontmatter: the Agent Skills standard keeps
frontmatter to flat string metadata, and a nested gate list there could break
other tools that read the skills.

mxcli reads the definitions it ships, then the project's
(`.ai-context/skills/`, `.claude/skills/`), the project's winning by id — so a
team can tighten or add a workflow without forking mxcli (review finding #21: a
registry that reads only its embedded copy has no extension point).

### 3.2 Gates are commands, and their results are recorded, not claimed

Each gate is decided by mxcli:

| Kind | Passes when |
|---|---|
| `intent` | the run has a one-sentence intent |
| `note` | the run has a change note |
| `command` | an mxcli command exits 0 (`docker check`, `test …`) — optionally only if a path exists (`tests/`) |
| `lint` | `mxcli lint --format json` shows no finding of the gate's rule prefixes that **was not there when the run started** |
| `score` | `mxcli report` is not below the run's starting score |

A gate is **blocking** or **reporting**. `mxcli workflow check` runs them all,
records the results in the run, and exits 1 while a blocking one fails;
`mxcli workflow close` refuses until the last check passed them. The agent
cannot call the work done, and nobody ticks a box — the brain's "derived, never
written" rule (umbrella P4) applied to procedure.

**Findings that predate the run do not block.** Measured on the first test
project: five SEC001 warnings on a template module's entities. A gate failing on
those would block every change in that project until someone fixed code the
change never touched. So a `lint` gate records the findings at `workflow start`
and blocks only on new ones; `check` reports how many were there before. The
same reasoning makes `score` compare with the run's start, not an absolute bar.

### 3.3 A run is one file in the brain

`docs/brain/workflows/<date>-<workflow>-<slug>.json`: the intent, the change
note, the baselines, and every check with its results. One file per run, as
tasks and issues are (umbrella §6.1): reviewable in a PR, conflict-free on
parallel branches, queryable across projects with DuckDB. A `docs/brain/` that
mxcli did not write is refused, as the brain itself refuses it.

### 3.4 CLI

```
mxcli workflow list [--json]
mxcli workflow start <workflow> "<intent>"      # records the baselines
mxcli workflow check [run] [--json]             # exits 1 while a blocking gate fails
mxcli workflow note [run] "<change note>"
mxcli workflow close [run]                      # refused while blocked
mxcli workflow status [--json]                  # open runs and their last results
```

`[run]` defaults to the newest open run.

### 3.5 Entry points: commands first, the banner second

- **Slash commands** — `/workflow-vibe <change>` and `/workflows` (pick one) —
  shipped by `init` like the other commands. They work in every Claude Code
  surface, including cloud and web sessions where project mods do not load
  (measured 2026-10-11).
- **The companion's banner** offers the workflows `mxcli workflow list`
  reports: on the welcome band in a project's first session, then as a compact
  "Workflows:" row, hideable for the session. A button **fills the prompt** with
  the command (`/workflow-vibe `) rather than submitting it: every workflow needs
  the user's own words next.
- **While a run is open** the band shows its intent and gates —
  `Vibe code · Add a status to orders · ✓ security  ✗ tests  ! quality` — read
  from `mxcli workflow status`, refreshed when the agent runs `mxcli workflow`.

## 4. The vibe-code prototype

Built in this slice's first PR:

| Gate | Kind | Blocks |
|---|---|---|
| Intent | intent | yes |
| Security | lint, `SEC*` and `MPR007`, new findings only | yes |
| Builds | command, `docker check` | yes |
| Tests | command, `test tests/ --local --db-type hsqldb`, requires `tests/` | yes |
| Quality | score, `report` | no |
| Change note | note | yes |

Theme has no gate yet — there is no theme check to run — so the skill asks for
Atlas classes and no inline styles, and for the note to say what was done.

**Measured end to end** on a copy of `local-test-app` (Mendix 11): `start`
recorded a 95.1 quality baseline and 5 pre-existing security findings; `check`
ran all six gates in 11 s and blocked on Tests (no `tests/`) and Change note;
`close` refused with the blocking gates named.

### Decisions taken (each open to the user)

1. **Security, Builds and Tests block; Quality reports.** A change that adds a
   security hole, breaks the build or ships untested logic is not done. A score
   dip is worth a look, not a stop.
2. **A vibe change gets a short note, not a proposal** — intent, what changed,
   which gates passed. Full proposals belong to PRD to app.
3. **Names**: `/workflow-vibe`, `/workflows`, `mxcli workflow`.

## 5. The other three, next

- **PRD to app** adds gates the brain slices provide: an intent document exists
  (umbrella slice 4), every requirement has a test (slice 5), and the vibe gates
  per delivered slice.
- **Upgrade Mendix** needs a gate kind that compares **before and after**: the
  test results from before the upgrade, and the same tests after it. Its first
  stage is to make tests exist when they do not, since "done" is "the same tests
  pass".
- **Migrate legacy** needs the same before/after comparison, with "before" being
  the old app's behaviour written down as tests.

Each will declare **stages** (ordered groups of gates) once a workflow has more
than one checkpoint; the vibe loop is a single stage.

## 6. Measuring whether it helps

The eval harness (`mxcli eval`) can run a BENCH brief twice — with and without
the vibe workflow — and compare check scores, `mxcli report` and turns. A
workflow that adds turns without raising quality is not worth its cost; this is
how to find out before building the other three.

## 7. Test plan

- `cmd/mxcli/workflow`: definitions load and a project overrides by id; each
  gate kind decides from its command and the run; lint gates block only on new
  findings; a run never checked cannot close; one file per run; a foreign
  `docs/brain/` is refused.
- `cmd/mxcli`: every shipped workflow loads from the embedded skills, has its
  skill, and its gates call mxcli commands and flags that exist (shown to fail
  on a renamed flag).
- Mod: the band offers the workflows mxcli lists on terminal and desktop, a
  button fills the prompt, an open run replaces the offers with its gate line,
  `mxcli workflow` from the agent refreshes it, the offers hide for the session.
- By hand: run `/workflow-vibe` on a real change in a fresh project.

## 8. Open questions

1. **"Workflow" collides with Mendix workflows** (the document type, `create
   workflow` in MDL). In the CLI and the docs the two can be confused; a
   different word (`playbook`, `recipe`) for these is cheap now and expensive
   later.
2. **Gates that need a running app** (`test --local`) boot a runtime: seconds to
   a minute. Run them on every check, or only at `close`?
3. **A theme gate**: is there a check worth writing (inline styles, colours
   outside the palette — the theme-styling skill names both)?
4. **Stages and resumption** for the long workflows: a PRD-to-app run spans
   sessions; how the banner shows "stage 3 of 6" and how a new session picks it
   up.
