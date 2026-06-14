# MCP Backend: How a Script Maps to Tool Calls

mxcli can apply an MDL script in two ways:

- **File (MPR) backend** — the default. mxcli opens the `.mpr` directly, mutates the
  BSON in process, and writes it back to disk. No network, no running Studio Pro.
- **MCP backend** (`--mcp`) — mxcli connects to a *running* Studio Pro's embedded
  MCP server ("PED") and applies each change through its tools, so edits land in the
  live, open model with no save/reload cycle.

The two backends produce the same model, but they have **very different cost
profiles**. The file backend turns one statement into one in-process mutation —
effectively free. The MCP backend turns one statement into *one or more* JSON-RPC
tool calls to Studio Pro. This page explains that mapping for domain-model scripts
and what it means for latency, token usage, and an LLM's context window.

> The numbers below trace the worked example
> [`mdl-examples/doctype-tests/01-mcp-domain-model-examples.mdl`](https://github.com/mendixlabs/mxcli/blob/main/mdl-examples/doctype-tests/01-mcp-domain-model-examples.mdl)
> — a 25-statement domain-model script that stays entirely inside the MCP
> authoring surface. To see the surface for your connected version, run
> `mxcli mcp capabilities -p app.mpr --mcp`.

## The mapping

Each MDL statement expands into a small, fixed *choreography* of PED tool calls.
The choreography — not the statement count — drives the cost.

| MDL statement | PED tool calls | Calls |
|---|---|---|
| `create module` | `ped_create_module` | 1 |
| `create enumeration` | `ped_create_document` + `ped_check_errors` | 2 |
| `create entity` (incl. `extends`, non-persistent) | `ped_update_document` (add) + `ped_check_errors` | 2 |
| `create association` | `ped_update_document` (add) + `ped_check_errors` | 2 |
| `create view entity` | create source-doc + set `/oql` + entity add + check | 4 |
| `alter entity` (add / rename / set documentation) | 2× `ped_read_document` (locate + read live attrs) + `ped_update_document` + `ped_check_errors` | 4 |
| `show` / `describe` of a **just-modified** module | `ped_read_document` (entities + associations) + an enrichment read for attribute types | 2 |
| `show enumerations` (and other reads of unchanged docs) | served from the local `.mpr` | 0 |
| one-time `ped_get_schema` per element type (entity, attribute, association, view source) | fetched once per session, then cached | 4 total |
| `initialize` handshake | once per connection | 1 |

For the 25-statement example that works out to **≈ 62 tool calls** on the happy
path: ~47 writes-plus-validation, ~10 read-backs, 4 schema fetches, 1 handshake.

### Three things inflate the count

1. **Validation runs after every write.** `ped_check_errors` fires once per write
   statement, so **~17 of the 62 calls are validation alone**. This keeps the live
   model provably consistent at each step, but it is not free.
2. **`ALTER` is read-modify-write.** Because PED cannot change an attribute's type
   in place, mxcli first *reads* the live entity (to tell a rename from a drop+add,
   and to leave untouched attributes alone), then writes. Every `ALTER ENTITY` is
   ~4 calls even when it changes one thing.
3. **Reading a module you just wrote costs 2 calls, not 0.** After any write, that
   module's on-disk `.mpr` is stale, so `SHOW`/`DESCRIBE` of it is reconstructed
   live from Studio Pro (a structure read plus an enrichment read for attribute
   types) instead of being served locally.

## What this means for performance

Every tool call is a synchronous JSON-RPC round trip to Studio Pro, executed
**sequentially**. Wall-clock time is therefore roughly:

```
total ≈ (number of tool calls) × (round-trip latency)
```

So latency scales with the *choreography-expanded* call count, not the number of
statements. The practical levers:

- **Batch writes into one script run**, not many small invocations — the
  `initialize` handshake and schema fetches are paid once per connection.
- **Drop trailing `SHOW`/`DESCRIBE` verification** from production scripts. In the
  example, the eight read-back lines account for ~10 of the 62 calls; removing them
  cuts roughly a sixth of the traffic.
- **The file backend has zero round trips.** If Studio Pro doesn't need to be open
  — e.g. CI, bulk generation, or scripted migrations — the file backend applies the
  same script far faster. Reserve `--mcp` for when live, validated, no-reload edits
  in an open model are the point.

## What this means for token cost and LLM context

This is the part that matters most when an **AI agent** is driving mxcli, and it
hinges on a single question: *who makes the tool calls?*

### Agent → mxcli (efficient)

When an agent runs `mxcli exec script.mdl -p app.mpr --mcp` (via the shell, the
REPL, or an editor command), the agent emits **one** action — the MDL script — and
receives **one** summarized result. The ~62 PED tool calls, and their often
verbose, schema-laden JSON responses, happen *inside the mxcli process*. They never
enter the model's context window, and they don't each cost a model turn.

What lands in context:

- the MDL script — compact and declarative (the example is ~150 readable lines), and
- mxcli's textual summary of the run.

That is a few thousand tokens, regardless of how many PED round trips it took.

### Agent → Studio Pro MCP directly (expensive)

An agent connected straight to Studio Pro's MCP server has no such amortization. To
build the same domain model it must issue each of the ~62 calls **as its own tool
call**:

- every request *and* every response (schemas, validation output, read-backs) is
  written into the context window, and
- each call is a separate model turn — so the model "thinks" ~62 times instead of
  once, multiplying both latency and per-turn token cost.

The same work that is one declarative script for the mxcli path becomes dozens of
imperative, stateful tool calls whose intermediate results accumulate in context.

### The takeaway

```
                     calls in LLM context   model turns   tokens in context
Agent → mxcli (--mcp)          0                  1              low (script + summary)
Agent → Studio Pro MCP        ~62               ~62             high (every request + result)
```

mxcli's MCP backend is, in effect, a **compression layer over the PED tool surface**:
the LLM works in compact, reviewable MDL while mxcli absorbs the chatty,
validation-heavy, read-modify-write round trips on the model's behalf. For bulk
authoring this is the difference between one cheap turn and dozens of expensive
ones — and it is why, even though a script "costs" ~62 tool calls, running it
*through mxcli* keeps that cost out of the model's context.

## See also

- [Connecting to a Running Studio Pro (MCP)](../tools/mcp-connect.md) — how to set up
  the connection and run scripts against a live model.
- [`mxcli mcp capabilities`](../reference/capabilities.md) — what the MCP backend can
  author against your connected Studio Pro version.
- [System Architecture](architecture.md) — the backend abstraction that lets the
  same MDL target either the file or the MCP backend.
- [Storage Names vs Qualified Names](storage-names.md) — why the BSON the file
  backend writes differs from the names in the SDK docs.
