---
title: mxcli companion — a Claude Code mod that shows the project, its brain, and how they connect
status: draft
date: 2026-10-11
related:
  - PROPOSAL_project_brain.md
  - journey-architecture-viz.md
  - PROPOSAL_multi_project_tree.md
  - proposal-eval-framework.md
  - PROPOSAL_vscode_visualizations.md
---

# mxcli companion — a Claude Code mod that shows the project, its brain, and how they connect

This is an **umbrella proposal**. It describes the whole idea, fixes the
principles every part must follow, and splits the work into slices that can each
get their own proposal. Only the first slice — the **project tree with MDL
source** — is specified in enough detail to build. It is chosen first because it
needs no new mxcli capability and so tests the delivery mechanism (a mod shipped
by `mxcli init`) on its own.

## 1. Problem

A user building a Mendix app with Claude Code and mxcli works through a
transcript. Everything the agent builds — modules, entities, microflows, pages —
and everything that explains it — intent, specs, decisions, tests — is invisible
unless the user asks for it in words, and reads the answer as text that scrolls
away. Concretely:

- **There is no persistent picture of the app.** "What did it build?" is a
  question, not something the user can glance at. Studio Pro has an App Explorer;
  a Claude Code session has none.
- **Starting is a blank prompt.** A fresh repo after `mxcli init` gives the user a
  cursor. The procedures that make a good start exist (`bootstrap-app`, the
  brain's slices) but the user has to know to ask for them; today that is done by
  pasting an "empty-repo seed prompt".
- **The reasoning is not connected to the result.** The brain records
  requirements anchored to model elements, and `mxcli test` runs tests, but
  nothing connects a spec to the tests that prove it or to the documents that
  implement it. Nothing shows the user whether what they asked for is built,
  tested, and passing.
- **Explaining an app to a newcomer** is done with hand-made decks (sketch-deck
  journey slides: persona → screens → logic → data → external per step). They are
  good documentation but are made by hand, and nothing keeps them in step with
  the model.

Claude Code can now load **mods**: plugins of function hooks that draw panes,
bands above the prompt, and status entries, run host commands, and submit
prompts. That makes a persistent, interactive view next to the agent possible
without leaving the session.

## 2. What exists today (verified)

The UI is new; nearly all the data it needs already exists.

| Need | Existing piece | Notes |
|---|---|---|
| Project tree | `mxcli project-tree -p app.mpr` (`cmd/mxcli/project_tree.go`) | JSON `[{label, type, qualifiedName?, children?}]`; ~60 node types; modules, folders, domain model, security, navigation, settings. Built for IDE integrations; used by `vscode-mdl` and the TUI |
| MDL of a document | `mxcli describe -p app.mpr [<type>] <qn> --json` | `{name, type, mdl, executable}`. Type is optional: it is resolved from the catalog, then the live reader |
| Prior art for tree → source | `vscode-mdl/src/projectTreeProvider.ts`, `mdlContentProvider.ts` | Same flow: spawn `project-tree`, click → `describe`, with a type fallback list |
| Change detection | `cmd/mxcli/tui/watcher.go` | fsnotify on `.mpr` and `mprcontents/` subdirs, 500 ms debounce |
| Diagrams | `describe --format elk` / `mermaid` | **Not SVG** — layout and rendering happen in a browser (elkjs / mermaid from a CDN). Microflow objects and entities do carry stored canvas positions |
| Brain | `mxcli brain …` (`cmd/mxcli/brain/`), `docs/brain/` | Markdown, committed. Decisions, requirements (slices, forward anchors), open questions. `--json` on every verb. Progress derived from anchors resolving |
| Tests | `mxcli test` (`.test.mdl` / `.test.md`), `mxcli playwright verify` | Console + JUnit with `--junit`. **No link from a test to a requirement; no default results file** |
| Acceptance checks | `mxcli eval` check kinds (`entity_exists`, `page_has_widget`, `tests_pass`, …) | Per-check pass/fail in `score.json` |
| Shipping into a project | `mxcli init` writes `.claude/skills/`, commands, `settings.json`, a SessionStart bootstrap hook | Embedded via go:embed; refreshed with `init --sync-skills` |

And on the Claude Code side (checked against this build's mod API and the
plugin-loading docs):

| Capability | How |
|---|---|
| Ship with the project, no marketplace | A plugin folder under the project's **`.claude/skills/<name>/`** containing `.claude-plugin/plugin.json` is loaded in place (origin `@skills-dir`), enabled by default, **after the workspace trust dialog**. It loads only when the session is started from the repo root, and desktop worktrees load the main checkout's copy |
| Pane / band / status / toast | `$.ui.open` + `ui.render` on `Pane`; `AbovePrompt`; `$.ui.status`; `$.ui.toast` |
| Elements | `Box`, `Text`, `Button`, `Select`, `Input`, `Code` (syntax-highlighted, line numbers), `Markdown`; `Svg` on desktop/VS Code/mobile only, **max 131,072 characters**, no clicks inside |
| Run mxcli | `$.process.run(argv)` (one-shot, 30 s default, 4 MiB stdout); `$.process.spawn` (streaming, long-lived) |
| Start a workflow from a button | `$.prompt.submit({ text })` starts a turn as if typed; `$.prompt.fill` drafts it |
| React to the agent's work | `tool.call` hook sees every Bash/Edit call and its result |
| Greeting | **Not** `initialUserMessage` (that works in `-p` mode only). A `session.start` hook draws an `AbovePrompt` band, or opens a pane — but a pane opened unasked only takes its place from 144 terminal columns |

## 3. The idea

One mod, shipped by `mxcli init`, called the **mxcli companion**. It has six
views, which the slices in §5 build one at a time:

1. **Start** — a band above the prompt in a fresh or idle session, offering
   prepared workflows: *New app from a PRD*, *Change the app via a proposal*,
   *Explore this app*. A button starts the workflow; the workflow's skill does the
   interviewing.
2. **Project** — the app's structure as a tree. Select a document to see its MDL
   source; for microflows and domain models, a diagram as well.
3. **Brain** — the project's documentation as the brain holds it: intent,
   specs with their requirements, plans, decisions, open questions, reviews — per
   slice, laid out by stage, with links into the project view.
4. **Verification** — which tests prove which spec, and whether they passed in
   the last run; which requirements are built (anchors resolve) but untested, and
   which are tested but not built.
5. **Inbox** — records an extractor proposed from the conversation
   (requirements, test intents, tasks, issues, open questions), waiting for the
   user to accept, edit or drop them (§5.8).
6. **Journeys** — user journeys held in the brain, drawn as journey maps
   (persona → screens → logic → data → external per step) whose cards link to
   the documents they name. They serve as a map of the app. The same data can be
   exported as a sketch-deck for documentation (§5.7).

## 4. Principles

These hold for every slice. Each is a decision the slice proposals inherit, not
reopen.

**P1 — The mod draws; mxcli knows.** Every view reads a `--json` command. The mod
never parses an MPR, a brain Markdown file, or a deck. This keeps all the logic
in Go, where it is tested and where `vscode-mdl` and the TUI can use the same
outputs. A view that needs data mxcli cannot provide gets a new mxcli command,
not mod code.

**P2 — The brain holds all project documentation.** Intent, PRDs, specs, plans,
decisions, open questions, reviews and journeys all live in the brain, so there
is one store to read, one place to check, and one format the agent and the mod
process. A second documentation home (`docs/specs/`, a deck folder) would be a
second thing to keep in step. See §6 for what this changes in the brain's design.

**P3 — Links are anchors, and anchors are checked.** Every link from
documentation to the app is a brain anchor (`@Module.Doc[.Member]`), checked
against the catalog. A spec that names a renamed microflow, a journey card
pointing at a deleted page, or a test naming an unknown requirement is reported
by `brain check`, not discovered by a reader.

**P4 — State is derived, never written.** "Built" means the anchors resolve;
"tested" means a test names the requirement; "passing" means the last results
file says so. No status column, no checkbox, no figure in prose. This is the
brain proposal's A6, applied to the whole companion.

*The one stated exception:* a **task's or issue's status** (open, done) cannot
be computed from the model, so it is written — on the record itself, and nowhere
else (§6.1). Requirements keep their derived status; a requirement never gains a
status field because tasks have one.

**P5 — It works on every surface, degrading where it must.** The terminal has no
`Svg`, mobile no `Select`. Each view has a text form that works everywhere; a
picture is added where the surface can draw one.

## 5. Slices

Each slice is usable on its own. The order is by dependency and by what each one
proves.

| # | Slice | New mxcli capability | Depends on |
|---|---|---|---|
| 1 | Project tree + MDL source, shipped by `init` | none (one small fix: describe keywords) | — |
| 2 | A long-lived mxcli the mod talks to: requests over a Unix socket, change events on stdout | `mxcli serve --socket`, a shared file watcher | 1 |
| 3 | Start band + workflow launcher | workflow frontmatter on skills, `mxcli workflow list --json`, new user workflow skills | 1 (delivery) |
| 4 | Brain as the documentation store + Brain view | new brain document kinds and links (§6) | 1 |
| 5 | Test ↔ spec traceability + Verification view | `@requirement` on tests, JSON results file, coverage in `brain check` | 4 |
| 6 | Diagrams in the Project view | `describe --format svg` for microflows, nanoflows, domain models | 1 |
| 7 | Journeys + sketch-deck export | journey documents in the brain, `mxcli journey …` | 4, 6 (renderer) |
| 8 | Conversation → structure: an extractor that turns the conversation into staged brain records | provenance on `brain capture`, new record kinds (test intent, task, issue) | 4, 5 |

Slice 1 is specified in §7. The rest are sketched below with what is known and
what each needs to decide.

### 5.2 A long-lived mxcli

Specified in its own proposal: [proposal-mxcli-companion-serve.md](proposal-mxcli-companion-serve.md).
Measured on Evora, a click costs about a second as its own process and under
0.2 s inside a warm one, and slice 1 can only guess at changes from tool calls.
The mod cannot write to a spawned child's stdin after the first text, so the
server answers HTTP on a Unix socket (`$.http.fetch` takes a `socketPath`) and
reports changes on stdout. It answers umbrella open questions 3 and 4.

### 5.3 Start band and workflows

Specified in its own proposal, with the vibe-code workflow built as the
prototype: [proposal-mxcli-companion-workflows.md](proposal-mxcli-companion-workflows.md).
The sketch below is the starting point it refined: workflows got a
`workflow.json` beside their skill instead of frontmatter, and gates.

**Where workflows live:** as skills in `.claude/skills/mendix/`, shipped by
`init` like every other skill, so one file is both the agent's procedure and the
thing the UI offers. A skill becomes a workflow by declaring it in frontmatter:

```yaml
workflow:
  title: New app from a PRD
  stages: [intent, spec, plan, build, test, review]
  offer: fresh        # fresh | always | never — when the Start band shows it
```

`mxcli workflow list --json` returns the declared workflows (P1: the mod does not
read skill files). A button calls `$.prompt.submit` with a prompt that invokes the
skill. While a workflow runs, the band shows its current stage, derived from the
brain (P4), not from the conversation.

**What exists:** `bootstrap-app` covers most of *new app* (interview, `mxcli new`,
requirements as slices, propose the model before building). **What does not:**
any user-facing *PRD*, *change via proposal*, or *plan → review* workflow; those
exist only as contributor commands (`mxcli-dev:proposal`, `adr-new`, `review`)
and are not shipped. Writing those skills is the bulk of this slice.

**Greeting:** a fresh project (no `docs/brain/`, or an empty model) shows the
Start band on `session.start`. It replaces pasting the empty-repo seed prompt.

### 5.4 Brain as the documentation store

Specified in §6, because it amends an existing proposal. The Brain view is a
per-slice board across stages (intent → spec → plan → built → tested →
reviewed), read from `brain plan --json` / `brain show --json`, with each
anchor linking into the Project view.

### 5.5 Test ↔ spec traceability

- **Link:** a `@requirement <id>` annotation in the `.test.mdl` javadoc block
  (`testrunner/parser.go`), and the same in a header comment of
  `tests/verify-*.test.sh`. A test may name several.
- **Results:** `mxcli test` and `mxcli playwright verify` always write
  `.mxcli/test-results/latest.json` (gitignored) — per test: id, file, kind,
  requirement ids, outcome, duration, message. JUnit stays as an extra.
- **Coverage:** `brain check` reports requirements with no test, and tests naming
  unknown requirements. `brain plan --json` gains `tested` / `passing` per
  requirement, derived from the annotation and the results file (P4).
- **Acceptance checks:** a spec's acceptance criteria may be written as the eval
  framework's typed checks (`entity_exists`, `page_has_widget`, …), so part of
  "is this spec met" is machine-checked without writing a test. Whether to reuse
  `evalrunner/checks.go` directly is for the slice proposal.
- **View:** spec → requirements → tests → last outcome, and requirement →
  anchors → built. A table, not a picture: it needs to scroll and be read.
- **Refresh:** a `tool.call` hook that sees the agent run `mxcli test` or
  `playwright verify` reloads the view when the call returns.

### 5.6 Diagrams (`describe --format svg`)

The mod cannot run elkjs or mermaid (no DOM), so the SVG must come finished from
mxcli. For **microflows, nanoflows and domain models** the model stores canvas
positions, so a Go renderer can draw at Studio Pro's own coordinates with no
layout engine — the result looks like Studio Pro's canvas. Page wireframes and
the system overview have no positions and need a simple layout; they are later
work. The 131,072-character cap means compact output (no per-element styles, short
paths). VS Code, the TUI and `mxcli serve` could use the same renderer and drop
their CDN dependency, which is why this is its own slice and not mod code.

### 5.7 Journeys and sketch-deck export

**Journeys are brain documents** (P2): a persona (a user role), steps, and per
step the cards in each layer, each card carrying a label, a note, a status
(AI / plain logic / scripted / not in the demo / offline — the deck's legend) and
**anchors** to the documents it names (P3). `brain check` reports stale cards.
`mxcli journey suggest` can propose cards for a step from the catalog's refs
(page → microflows it calls → entities they touch → REST clients), for the agent
or user to accept; detecting whole journeys automatically was tried and rejected
(`docs/plans/architecture-diagram-plan.md`: cyclic navigation, per-role paths and
popups make it noise), so journeys are authored.

**In the mod**, a journey is drawn from the brain data (P1: `mxcli journey show
--json`), not cut out of a deck. Cards cannot be clicked inside an SVG, so the
steps and cards are also listed beside the picture, and selecting a card opens
its documents in the Project view; a selected card is highlighted by injecting a
CSS rule into the SVG.

**Sketch-deck export** turns the brain's journeys (plus a title slide and an
overview) into a sketch-deck HTML file as user documentation. The format was
read from an existing deck (`mxcli-agent-dx-presentation.html`): one HTML file,
one large SVG, one `<g>` layer per slide with `data-sd-slide="<n>"` and
`data-sd-frame="x y w h"` giving the camera view, steps revealed with
`data-sd-step`, fonts from Google Fonts, and a script for the camera. Each
journey slide in that deck is 90–145 KB, which is one more reason the mod draws
from data rather than from the deck: a slide is near or over the `Svg` cap.
Whether export is an mxcli command or the sketch-deck skill reading
`mxcli journey show --json` is for the slice proposal; P1 says either way the
data comes from mxcli.

### 5.8 Conversation → structure

Requirements, tests, tasks and issues are mostly *said* before they are written
down: "inspectors log a defect on a tablet", "from severity 2 up, after
maintenance approves", "photos are a must, five per defect is enough", "the SAP
sync sometimes fails at night". Today they stay in the transcript unless someone
remembers to capture them — the brain proposal's A11 names exactly this gap ("a
curation step with no trigger stops"). This slice adds the trigger: an extractor
that follows the conversation and proposes structured records.

**It reuses the brain's existing pipeline unchanged in shape.** The extractor
*captures*, the human *promotes* — the same split the brain already has between
`staged.jsonl` and the committed files, and the reason promotion was made a
human step in the first place. Nothing the extractor produces becomes project
documentation without a person accepting it.

**Mechanism (all in the mod's reach today):**

1. On `turn.complete`, the mod runs `$.model.fork` — a tool-less completion over
   the session's **own** transcript, served from the prompt cache — with an
   extraction prompt: *list the requirements, test cases, tasks, issues and open
   questions stated since the last extraction, as JSON, each with the turn it
   came from.* Cheaper alternatives for quiet turns: a `$.model.classify` gate
   ("did this turn state anything new?") before the fork, or extraction only on
   idle / on a button.
2. Each record goes to `mxcli brain capture … --json` (P1: the mod does not write
   brain files), with its kind, slice guess, anchors the model proposed, and
   **provenance** (session id and message id). mxcli de-duplicates against what
   is already staged or promoted and validates the anchors.
3. The Brain view gains an **Inbox**: staged records grouped by kind, each with
   *Accept* (`brain promote`), *Edit* (fills the prompt box via `$.prompt.fill`
   with a correction request), and *Drop*. Accepting places the record in the
   structure — slice → feature → record — which is the third column of the
   "conversation to structure" picture.

**"And back."** Provenance makes the link two-way: a promoted record shows the
turn it came from, and a record can be sent back into the conversation as work
("implement REQ *Up to 5 photos per defect*", "write the test for *Severity 1
creates no order*") with `$.prompt.submit`.

**Record kinds.** The picture uses REQ, TEST, TASK, ISSUE and TODO, grouped into
phases (MVP, Phase 2, Backlog) and features (Defect capture, Work orders).
Against the brain:

| Picture | Brain | Status |
|---|---|---|
| Phase | slice | exists |
| Feature | spec (a document, §6) | slice 4 |
| REQ | requirement entry | exists |
| Open question (implied: "should a defect create a work order?") | open question | exists |
| TEST | **test intent**: a planned test naming a requirement; satisfied when a real test carries `@requirement` for it (§5.5) | new |
| TASK / TODO | **task**: work that is not a requirement (check an integration, sketch a form) | new — a brain record, synced out (§6.1) |
| ISSUE | **issue**: a known defect or risk, anchored to what it affects | new — a brain record, synced out (§6.1) |

A test intent is the useful new kind: it is a TEST card before any test file
exists, and §5.5's coverage check turns it into "tested" once a test names the
requirement — derived, not ticked (P4).

**Risks the slice proposal must answer:** cost per turn (measure fork cost on a
real session before choosing per-turn vs on-idle); duplicate and contradictory
extractions as a conversation revises itself ("five photos" then "ten"); the
extractor inventing requirements nobody stated (provenance makes every record
checkable against its source turn, and the eval framework can score extraction
against a scripted conversation); and whether extraction also runs on a
**pasted** source — a PRD, meeting notes — which is the same extraction over a
document instead of the transcript, and the natural first step of the *New app
from a PRD* workflow (§5.3).

## 6. Amendment to the project brain

P2 changes the brain's scope. This section states the change so the brain
proposal can be amended rather than contradicted.

**What it was:** a small, capped, always-loaded store of the "negative space" —
only what cannot be derived from the model. Its §3 explains why the caps are
"the single most important thing in it": a store loaded every session that grows
without bound stops being read.

**What survives unchanged:**
- **Negative space.** Specs, intent, plans, journeys and reviews are all things
  the model cannot tell you. None of them may transcribe the model; they
  reference it by anchors. A spec that lists an entity's attributes is wrong in
  the same way a decision that lists them is.
- **Caps on what is always loaded.** `project.md` and the module shards keep
  their caps and stay the session-start reading.

**What changes:** the brain gains a second tier, **documents**, alongside the
existing **entries**:

| | Entries (today) | Documents (new) |
|---|---|---|
| Kinds | decision, requirement, open question | intent / PRD, spec, plan, journey, review |
| Size | one paragraph | pages |
| Loaded | every session (capped) | on demand: `brain brief`, the mod, a workflow step |
| Cap | per shard, enforced by `promote` | none on size; `brain show` reports size |
| Anchors | yes, checked per kind | yes; a document's requirements are entries |
| Location | `project.md`, `modules/`, `plan/<slice>.md` | `docs/brain/<kind>/<id>.md`, with frontmatter (id, kind, slice, links) |

Requirements stay entries, and a spec **contains** them: the spec document is the
prose and reasoning, its requirements are the checkable, anchored lines that
`brain plan` counts and that tests name (§5.5). That keeps the existing
"built = anchors resolve" mechanism intact and gives tests something small and
stable to point at.

**Open for the slice-4 proposal:** whether documents go through `capture` /
`promote` (human promotion was deliberate for entries; a 3-page spec written in a
workflow may need a different gate), how a review links to the spec and change
it reviews, and whether a PRD and an intent are one kind or two.

### 6.1 Tasks, issues and test intents: one JSON file per record

**The brain is the source of truth for tasks and issues, not a tracker.** Not
every user has GitHub; some use Jira, Azure DevOps, Linear, or nothing. So the
record lives in the repo, and a sync adapter mirrors it to whatever the user
uses — the reverse of making one tracker the home and the brain a cache.

**Format — decided: option (a).** The three new kinds are stored as **one JSON
file per record**; the existing kinds stay as they are.

| Kind | Stored as | Why |
|---|---|---|
| decision, requirement, open question | Markdown shards, unchanged | always loaded, capped, reviewed as prose in a PR |
| task, issue, test intent | `docs/brain/{tasks,issues,tests}/<id>.json`, one file each | queryable across repos, conflict-free on parallel branches |
| documents (§6) | `docs/brain/<kind>/<id>.md` with frontmatter | pages of prose |

One file per record, not one shared `.jsonl`, because this repo has measured
the alternative: the bug findings began as a single appended file, parallel fix
branches conflicted on it constantly, and `merge=union` let 256 duplicates through
unnoticed. They were split into one file per record
(`.claude/skills/fix-issue/findings/<area>/<date>-<slug>.json`) and are queried
with DuckDB. Each branch now adds its own files and nothing conflicts. The
same holds for a project where the agent captures tasks on several branches.

Whether decisions and requirements should later move to the same format — with
`mxcli brain show` rendering the Markdown view — is left until the document tier
settles its format. Moving them costs the prose review the brain proposal values;
not moving them leaves those kinds out of cross-repo queries.

**The record** (fields, not final):

```json
{
  "schema_version": 1,
  "id": "T-20261011-check-sap-assets",
  "kind": "task",
  "title": "Check SAP asset integration",
  "body": "The asset list lives in SAP; confirm the consumed service covers it.",
  "slice": "02-sap-integration",
  "status": "open",
  "anchors": ["@SapIntegration.RC_Assets"],
  "source": { "session": "…", "message": "…" },
  "external": { "system": "jira", "id": "QE-142", "url": "https://…" },
  "created": "2026-10-11"
}
```

`status` is the P4 exception, written here only. `source` is the provenance from
§5.8. `external` is set by sync and never by hand. The schema is a contract
checked by `brain check` (and a `make`-style check in mxcli's own CI), with
`schema_version` so a cross-repo query survives projects on different mxcli
versions.

**Sync.** `mxcli brain sync --to <system>` with one adapter per system
(GitHub, GitLab, Jira, Azure DevOps, Linear to start; the adapter interface is
for the slice proposal). `external` makes re-sync update the same ticket instead
of creating a duplicate. **Ownership, to avoid two-way conflicts:** the brain
owns content (title, body, anchors, slice); the external system owns workflow
state (status, assignee) once a record is synced, and sync pulls it back into
`status`. A record with no `external` is owned entirely by the brain. Sync is
opt-in and never runs without being asked.

**Overview across projects.** Because the records are plain files in each repo,
an overview of every open task is a DuckDB query over local checkouts — no
server, no tracker:

```sql
select regexp_extract(filename, 'repos/([^/]+)/', 1) as project,
       kind, title, status, slice
from read_json_auto('~/repos/*/docs/brain/{tasks,issues}/*.json', filename = true)
where status <> 'done'
order by project, slice;
```

Caveat: it reads checkouts, so it is as fresh as each repo's last pull. A thin
`mxcli brain overview <dirs…>` can `git fetch` first and read the default branch
with `git show`, giving the same result without touching working trees; it can
also feed a cross-project view in the companion later.

## 7. Slice 1 — project tree and MDL source

### 7.1 Scope

- A pane, **Project**, opened by the `/mxcli` command (and by a button on the
  status entry). It shows the tree from `mxcli project-tree`, expandable, and
  below or beside it the MDL source of the selected document.
- Selecting a document runs `mxcli describe --json` and shows the `mdl` field in a
  `Code` element.
- The tree reloads when the agent changes the model, and on a Refresh button.
- Shipped by `mxcli init` and refreshed by `init --sync-skills`.

Out of scope for slice 1: editing MDL in the pane (the agent edits), diagrams,
the brain, the Start band, a long-lived mxcli process.

### 7.2 Design

**Locating the project.** In order: a mod option (`userConfig.mprPath`), else the
single `*.mpr` in the session's directory, else one level down (the same rule
`init` uses for solution repos). Several candidates → a `Select` in the pane.

**Data.** `$.process.run(["mxcli", "project-tree", "-p", mpr])` and
`$.process.run(["mxcli", "describe", "-p", mpr, type, qn, "--json"])`, with
`MXCLI_QUIET=1`. The mxcli binary is found as `./mxcli` (where the bootstrap
script puts it) or on `PATH`. One process per request is acceptable for slice 1;
§7.4 measures whether it stays acceptable.

**Drawing the tree.** A row per visible node: indent, a disclosure `Button`
(`plain`) for nodes with children, the label, and the type dimmed. Expanded
nodes and the selection live in `$.state` atoms so a reload keeps them. Only
the rows that fit `e.props.bodyColumns` × the viewport are drawn — a tree is held
to 100,000 characters of text per drawing, which a large project's full tree
would exceed. Folders and category nodes expand; document nodes select.

**Drawing the source.** `Code` with `startLine: 1`. MDL has no highlighter
grammar yet; slice 1 draws it with `language: "sql"` as the closest fit, and a
contributed MDL grammar is an open question (§10). Documents above the drawing
limit are cut by the engine with a note; acceptable for slice 1.

**Node types with no MDL.** `project-tree` emits nodes that `describe` cannot
answer: container nodes (folder, domainmodel as a whole, category, security) and
a few document types. Containers show their children as a list instead of
source. For document types, the mod sends the tree's `type` and mxcli resolves it
(P1) — which needs the fix in §7.3.

**Refresh.** A `tool.call` hook, after the call returns, reloads the tree when the
call was a Bash command running `mxcli` with `exec`, `-c`, `new`, `rename`, or
`marketplace`, or an Edit/Write under `mprcontents/` or of an `.mpr`; debounced,
so a burst of agent calls reloads once. Changes made outside the session (Studio
Pro) are picked up by the Refresh button. A watcher is slice-later work (§10).

**Surfaces.** Terminal (fullscreen docks the pane beside the transcript; the main
screen opens it inline) and desktop. Both draw from the same tree; the test kit
runs the UI tests on both.

### 7.3 Changes to mxcli

| File | Change |
|---|---|
| `mods/companion/` (new) | The mod source: `.claude-plugin/plugin.json`, `hooks/hooks.json`, `hooks/register.tsx`, `types/index.d.ts`, `*.test.ts`. Kept out of `.claude/skills/mendix/` so the skills sync (`rsync --delete`, mirrored into `.ai-context/`) neither deletes it nor ships it to non-Claude tools |
| `Makefile` | `sync-mods` target copying `mods/companion/` into an embed dir; part of `sync-all` |
| `cmd/mxcli/skills_content.go` | Embed the mod directory |
| `cmd/mxcli/init.go`, `init_tooling_sync.go` | Write the mod to `.claude/skills/mxcli-companion/` for the `claude` tool only; refresh it in `--sync-skills`, including stale-file removal inside that folder only |
| `cmd/mxcli/cmd_describe.go` | Accept every type `project-tree` emits that the executor can describe (today JavaScript actions and others have no CLI keyword), and return a clear "no MDL for this type" error for the rest, so the mod needs no fallback list |
| `cmd/mxcli/project_tree_test.go` | A test that every node type `project-tree` emits is either describable or on an explicit no-MDL list, so the two cannot drift again |

No BSON or MDL syntax changes. No change to the project file is ever made by the
mod.

### 7.4 Hypotheses slice 1 tests

Slice 1 exists as much to answer these as to deliver the tree. Each has the
measurement that settles it.

1. **A mod shipped in `.claude/skills/` loads in a fresh session after trust**,
   in the terminal and in the desktop Code tab, with no install step. Measure on
   a fresh `mxcli init` repo, both surfaces; record what the user sees.
2. **One process per request is fast enough.** Time `project-tree` and `describe`
   on a small app and on a large one (Evora-sized). If `project-tree` exceeds a
   couple of seconds or its output nears `$.process.run`'s 4 MiB cap, slice 2
   adds a long-lived mxcli process (`$.process.spawn`) before going further.
3. **Refresh from `tool.call` catches the agent's changes.** Build a small module
   through the agent and count missed reloads.
4. **The tree is usable in a docked pane** at typical widths, with keyboard and
   mouse.

### 7.5 Test plan

- **Mod:** `claude plugin validate mods/companion`; `claude plugin test` with
  `$.process` mocked: tree renders on `terminal` and `desktop`; expanding,
  selecting and refreshing; a `describe` error shows a message, not a blank
  pane; a `tool.call` running `mxcli exec` triggers one reload, a `git status`
  none. Each test reverted against a stubbed hook to show it can fail.
- **mxcli:** the drift test in §7.3; `describe` keyword tests for the newly
  accepted types; `init` and `init --sync-skills` integration tests that the mod
  lands in `.claude/skills/mxcli-companion/` for `--tool claude` and not for other
  tools, and that a sync leaves user files in `.claude/skills/` alone.
- **By hand, recorded in the PR:** hypotheses 1–4 on a fresh repo, terminal and
  desktop.

## 8. Version compatibility

- **Mendix:** none of slice 1 depends on the Mendix version beyond what
  `project-tree` and `describe` already support.
- **Claude Code:** mods need a build that supports function-hook plugins and
  `@skills-dir` loading. What an older Claude Code does with the folder is
  **unmeasured** — slice 1 must check it does no harm (hypothesis 1), and `init`
  should say which version the companion needs.
- **Other tools:** the companion is Claude Code only. Every other tool keeps
  what `init` gives it today; the `--json` commands the mod uses are available to
  any of them.

## 9. Alternatives considered

- **Extend `mxcli tui` instead.** It already browses the project and previews
  MDL, and has an agent socket. But it is a separate terminal program; the point
  here is a view *inside* the agent session that reacts to what the agent does.
  The TUI and the mod should share mxcli's `--json` outputs (P1), not code.
- **Ship via a marketplace** (`extraKnownMarketplaces` + `enabledPlugins` in
  project settings). Needs a GitHub or git source and an install step per user;
  `@skills-dir` needs neither. A marketplace remains an option for users who want
  the companion in repos not made by `init`.
- **Specs outside the brain** (`docs/specs/`). Rejected by P2: two stores to keep
  in step, two places for the agent to look.
- **Cut journey slides out of an existing deck** for the mod. Rejected: the deck
  is output, not source; slides are near the `Svg` cap; and the cards would need
  anchors added by hand to a drawing.

## 10. Open questions

1. **Name and command.** "companion", `/mxcli`? The folder name in
   `.claude/skills/` must not collide with a skill.
2. **MDL highlighting.** Contribute an MDL grammar to the `Code` element (the API
   allows plugin-contributed grammars), or draw as SQL? The VS Code extension has
   a TextMate grammar that may be reusable.
3. **When does a long-lived mxcli process become necessary** *(answered: slice 2, §5.2)* — slice 2, or only if
   hypothesis 2 fails? Candidates: a `serve --stdio` JSON-RPC mode, custom requests
   on `mxcli lsp`, or a headless TUI agent socket. Any of them would serve VS Code
   too.
4. **Changes from outside the session** *(answered: slice 2's watcher, §5.2)* (Studio Pro open alongside): add a watcher
   (a streaming `mxcli watch` the mod spawns) or keep the Refresh button?
5. **Settings merge in `init`.** `init` overwrites `.claude/settings.json`. If any
   slice needs a setting (e.g. `enabledPlugins` to let users turn the companion
   off), `init` must merge instead.
6. **Brain documents and promotion** (§6): does a workflow-written spec go through
   `promote`?
7. **Multiple apps in one repo** (`PROPOSAL_multi_project_tree.md`): slice 1 picks
   one `.mpr`; does the Project view need a switcher from the start?
8. **Decisions and requirements as JSON too** (§6.1): revisit once the document
   tier's format is settled.
9. **Sync adapters** (§6.1): which systems first, how credentials are held, and
   whether sync pulls new tickets created outside the brain back in as records.
