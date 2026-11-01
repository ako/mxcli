# Measuring Agent Sessions

An agent session's cost is roughly **model calls × conversation size**: every
model call re-reads the whole conversation, and every tool result stays in it.
So a call removed takes its result with it, and the bill falls with the square
of the call count. Two commands measure where a session's calls went:

| Command | Reads | Sees |
|---|---|---|
| `mxcli diag loop-report` | mxcli's own logs (`~/.mxcli/logs`) | mxcli processes: verbs, wall time, exits |
| `mxcli diag session-report` | the agent's transcript | every tool call, its result size, failures and retries, tokens |

`loop-report` cannot see the agent's Read/Edit/Grep/Skill calls, how large a
result was, or why a call happened. `session-report` can, because the
transcript records all of it.

## mxcli diag session-report

```bash
mxcli diag session-report ~/.claude/projects/<project>/<session>.jsonl
mxcli diag session-report --top 20 a.jsonl b.jsonl     # one report each, plus a combined one
mxcli diag session-report --json run/transcript.jsonl
```

Claude Code keeps transcripts in `~/.claude/projects/<project>/<session>.jsonl`,
the project being the working directory with every non-alphanumeric character
replaced by `-`. A subagent's transcript is in `<session>/subagents/`; those are
included unless `--subagents=false`, and each is costed as its own
conversation (its results are re-read by its own calls, not the main session's).

Example (a real session, abbreviated):

```text
session ea54453f (+9 subagents)  2026-09-25 00:22  wall 31h06m (active 5h16m)
model calls 1269 (main 470)  tool calls 1410  avg context 209k
tokens: cache-read 261.0M  cache-write 4.6M  output 790k  input 2.6k
tool results: 480k tokens, re-read cost 58.7M (22% of cache-read)
preamble: 42k floor (main), 53.3M paid over 1269 calls in 10 contexts (20% of context read)
tools: Bash 1222, Write 92, Edit 59, Read 12, Agent 9, …
bash 1222: other 648, build 244, mxcli 213, git 117
  mxcli 371 invocations (90 calls chained >1): exec 89, check 75, docker check 61, …
category: orientation 668 (47%), write 248 (18%), validate 123 (9%), apply 51 (4%), verify 138 (10%), diagnosis 11 (1%), retry 103 (7%), delegate 9 (1%), other 59 (4%)
costliest results (tokens × later calls):
   1   2.2k ×443  =  955k  Bash  sed -n 1139,1200p ….go
   …
failures 124  retry chains 66 (103 retry calls, 57 resolved)
    6 ok   ./bin/mxcli docker check -p …/probe/PedApp.mpr 2>&1 | [error] [CE6083] "Design property …
errors by retries (retries/chains/failures):
    6/1/6 [error] [CEN] "Design property Spacing bottomx is not supported by your theme. …
lookups: 166 file reads, 31 repeats (…/skills/fix-issue.md ×5, …); 12 doc lookups, repeated: skill fix-issue ×5, …
```

### What each line means

- **model calls** — distinct API responses. Claude Code writes one record per
  content block, each repeating the message id, so records are not calls.
- **avg context** — input + cache read + cache write per model call.
- **wall / active** — first to last record; *active* leaves out pauses over 15
  minutes, so a resumed session is not counted as one long one.
- **tool results / re-read cost** — the estimated size of every result
  (characters ÷ 4; an image width × height ÷ 750, at most 1,600), and the sum of
  size × the number of later model calls that re-read it, up to the next
  compaction. The percentage is the share of the cache-read bill that tool
  results explain; the rest is prompts, system context and the model's own
  turns.
- **preamble** — the first model call's context, which on a conversation that
  starts fresh is the fixed cost before any work: system prompt, tool
  definitions, the skill index, the project's CLAUDE.md. The preamble is
  re-read in full by every call and never amortised, so the figure that matters
  is *paid*: the floor times the calls that paid it, summed per conversation,
  because each subagent carries its own and smaller one. Its share falls as a
  session gets longer — a short task is nearly all preamble, a long build
  nearly all accumulated work — which is why a lever aimed at the preamble is
  worth least exactly where the bill is biggest.

  It is an **upper bound**. A resumed transcript, or one whose first record is
  already past a compaction, starts with prior conversation in context and
  overstates the floor by that much; when the overstatement is large enough
  that floor × calls exceeds the whole bill, the line says so instead of
  printing a share over 100%.
- **bash** — each Bash call once, under its most significant program: mxcli,
  playwright, build (make/go/npm/mx…), git, other. The mxcli line counts
  *invocations*: `exec a.mdl && docker check` is one call, two invocations.
  Verbs are named by mxcli's own command tree, as in `loop-report`; a one-shot
  is named by its statement, `-c describe`, `-c create`.

### Categories

Every call gets one category. The rules, first match wins:

| Category | Rule |
|---|---|
| retry | the call is in an error→retry chain (below) |
| write | Edit / Write; a Bash redirect into a file, `tee`, `sed -i`, `cp`, `mv`; `mxcli fmt` |
| apply | `mxcli exec`; `mxcli -c` with create/alter/drop/grant/revoke/move/rename/… |
| validate | `mxcli check`, `lint`, `docker check`, `docker build`, `report`, `diff`; make / go build |
| verify | `mxcli test`, `run`, `docker run`, `playwright …`, `screenshot`, `oql`; playwright and browser tools; `curl`; `go test` |
| diagnosis | reading or filtering a log (`.log`, `/logs/`, `.output`, `docker logs`, `journalctl`); `mxcli diag` |
| orientation | Read, Grep, Glob, Skill, ToolSearch, WebFetch; `mxcli syntax`, `help`, `describe`, `show`, `list`, `-c describe/show/list/select`; `cat`, `grep`, `ls`, `find` |
| delegate | Agent / Task (handing work to a subagent) |
| other | git, setup (`new`, `init`, `setup`), anything unmatched |

A Bash call that chains several commands takes the highest of its parts in the
order apply > verify > validate > write > diagnosis > orientation > other, so
`mxcli exec x.mdl && mxcli docker check` is an apply and `tail app.log | grep
ERROR` a diagnosis. `cd`, `echo`, `export`, `sleep` carry no category.

### Failures and retry chains

A result is a **failure** when Claude Code marked it `is_error` (for Bash, a
non-zero exit), or — for Bash, where `; echo` or `| tail` can mask the exit
code — when a line *starts* like an mxcli or test-runner error: `Error:`,
`Parse error:`, `Reference error:`, `- line N:N`, `[error]`, `FAIL`, `panic:`.
A grep hit (`file.go:12: Error:`) does not start the line and does not count.

A failure opens a **chain** on what the call worked on: the `.mdl` scripts an
mxcli command was given, the file a Read/Edit/Write touched, a file argument of
another command, or else the command itself. A later call on the same target
joins the chain and is counted as *retry*; a joining call that runs something
and succeeds *resolves* it. A chain nobody returns to for 8 calls is closed.
Chains are aggregated by their first error, with numbers, ids and paths folded
(`line 3:5` → `line N:N`), so the families show which errors cost the most
iterations.

### Lookups

Repeated reads of the same file (Read, `cat`, `head`), and repeated
documentation lookups: `mxcli syntax …`, `mxcli help …`, a Skill call, a read of
a `SKILL.md` or skill file, `CLAUDE.md` / `AGENTS.md`.

### Sharing a report

The report prints counts and short command, path and error snippets — never
prompt, thinking or tool-result text. Absolute paths are cut to their last two
elements and long token-like strings are dropped, so the output can be shared
without sharing the conversation. `--json` carries the same fields.

## Running the benchmark

`mxcli eval run` runs Claude Code headless on an eval test and reports the
session:

```bash
mxcli eval run docs/14-eval/eval-bench-001.md --version 11.15.0 -o ~/bench
```

It creates a fresh project with `mxcli new` (which runs `mxcli init`, so the
agent gets the CLAUDE.md and skills a user gets, and copies this mxcli into
it), runs `claude -p <prompt> --dangerously-skip-permissions` in it with a known
session id, copies the transcript into the run directory, writes
`session-report.txt` / `.json`, and runs the test's checks. `BENCH-001` is the
fixed brief: an order desk with four entities, three associations, overview and
edit pages, a validated microflow, two module roles, navigation and a microflow
test. Keep it unchanged, so runs before and after a change compare.

The agent needs its own credentials (a logged-in `claude`, or
`ANTHROPIC_API_KEY`). Variables a hosting Claude Code session sets for its
children (`CLAUDECODE`, `CLAUDE_CODE_*`) are removed, so a run started from
inside an agent session behaves like one started from a terminal.

To report on a run afterwards, or on several:

```bash
mxcli diag session-report ~/bench/BENCH-001-*/transcript.jsonl
```
