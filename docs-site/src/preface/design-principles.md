# Design Principles

mxcli is not a thin wrapper over the `.mpr` format. It is a deliberate bet on a
particular way of letting coding agents build Mendix applications: the agent
writes a **compact, human-readable DSL**, and mxcli's **deterministic layer**
turns that DSL into the many low-level mutations, validations, and tool calls a
real model change requires. The principles below explain the design choices that
follow from that bet, and why they differentiate mxcli from driving Studio Pro's
tools directly.

## Token efficiency

An agent authoring against mxcli emits **one larger MDL script**, not a long
back-and-forth of small, chatty tool calls. MDL itself is dense -- a module that
takes 15,000--25,000 tokens as raw model JSON is 2,000--4,000 tokens as MDL, a
5--10x reduction (see [What is MDL?](what-is-mdl.md#token-efficiency)). But the
larger win is *orchestration* overhead: one script is one agent action and one
summarized result, so there are fewer model turns, lower API cost, and faster
execution than emitting each individual document edit as its own turn. The
[MCP Backend cost analysis](../internals/mcp-backend-cost.md) traces this in
detail -- a ~20-statement script that expands to ~60 low-level tool calls still
costs the agent *one* turn when it runs through mxcli.

## Complexity belongs in the deterministic layer

A single MDL statement maps **deterministically** to whatever it takes to realise
it: multiple BSON documents on the file backend, or a fixed choreography of MCP
tool calls against a running Studio Pro. `create association`, for example,
expands to reference resolution, the write itself, and a validation pass. The LLM
never has to orchestrate that -- it states intent in MDL, and mxcli's
deterministic code carries the fan-out, ordering, and read-modify-write bookkeeping.
The agent handles the *simple* task (say what you want); mxcli handles the
*complex* one (make it valid). This keeps the hard, error-prone mechanics in code
that is tested and reproducible rather than in a probabilistic model.

## Human-readable DSL

MDL is a domain-specific language inspired by **SQL, Ada, and PL/SQL** -- SQL for
its `CREATE`/`ALTER`/`DROP`/`SHOW` verbs and set-oriented queries, Ada and PL/SQL
for its readable block structure. That familiar, structured syntax means
LLM-generated scripts are easy for a human to **review, audit, and author** --
both in documentation and in skills. A diff of an MDL script reads like a diff of
DDL, not an opaque binary or a wall of JSON. Readable code is reviewable code, and
reviewable changes are the ones a team can safely let an agent make.

## Headless agentic loop

Everything an agent needs to build and test a Mendix application is available
without ever opening Studio Pro. mxcli is a single static binary; it reads and
writes `.mpr` files directly, validates with `mxcli check` and the real Mendix
build tools, runs microflow tests in a container, and analyses the model -- all
from the command line. Studio Pro becomes an optional **visual review** surface,
not a required step in the loop. See the [Vision](vision.md) for the full roadmap
toward Studio-Pro-free development.

## Understanding large applications

mxcli builds a queryable **catalog** of project metadata and a **dependency
graph** on top of it, both exposed as ordinary SQL tables. An agent explores an
unfamiliar app the same way it would explore a database -- `SELECT` over
`CATALOG.ENTITIES`, `CATALOG.REFS`, or the `CATALOG.graph_*` views to find god
nodes, module coupling, dependency cycles, and dead code. No bespoke traversal
API to learn; complex applications become explorable through straightforward
queries. See [Graph Analysis](../tools/graph-analysis.md) and
[Catalog Use Cases](../tools/catalog-use-cases.md).

## Precise skills

Because MDL is a concrete syntax, skill files can embed **specific, executable
examples** of exactly the MDL an agent should produce -- not vague prose about how
Mendix concepts fit together. "Generate this shape of `create page` for a
master-detail view" is a copyable example; the agent adapts one worked case rather
than inferring structure from description. Concrete examples in skills yield more
reliable, accurate output and fewer retry loops.

## Power with safety

Coding agents driving mxcli are powerful -- they can generate scripts in bulk,
run automated tests, and rewrite large parts of a model. That power requires
appropriately scoped privileges. `mxcli init` provisions a **Dev Container** that
bounds exactly what the agent can see, do, and modify: it runs against the project
files and nothing else, with the toolchain pre-installed and the host environment
out of reach. Sandboxing is the default recommended way to run an agent, not an
afterthought. See [Dev Container Setup](../tools/devcontainer.md).

## Deterministic verification

`mxcli check` and `mxcli lint` let the agent verify its own output against
**deterministic rules** -- syntax, references, anti-patterns, security and
architecture policies -- with **no LLM judgement involved**. This is an explicit
design choice: correctness checks stay in the deterministic layer so the feedback
loop is fast, reproducible, and trustworthy. An agent can write MDL, get an
objective pass/fail, fix, and re-check in the same session without a human -- and
without asking a model to grade a model. Deeper gates (`mxcli docker check`,
`mxcli test`) extend the same principle to the real Mendix build and runtime.

## Longer-term: smaller local models

Generating code in a constrained DSL is inherently **simpler for an LLM** than
orchestrating a stateful sequence of tool calls -- there is one grammar to satisfy,
not an evolving protocol to reason about. That simplicity opens a path to smaller,
cheaper, and eventually **local** models for Mendix authoring. A DSL grammar can be
integrated into an LLM via **grammar-constrained decoding**, so the model can only
emit tokens that form valid MDL. The deterministic layer already absorbs the
complexity (see above); the DSL boundary is what makes constraining the model's
output tractable in the first place.
