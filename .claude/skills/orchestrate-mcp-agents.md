# Orchestrate Multiple Agents Editing One Live Studio Pro (MCP backend)

This skill is the procedure for coordinating several agents that concurrently
change one Mendix model through a single Studio Pro MCP ("PED") server (the
`mxcli --mcp` backend). It exists because the server multiplexes clients but
backs onto **one in-memory model** — so safety comes from *how you partition the
work*, not from any locking the server provides.

Background and rationale: [`docs/11-proposals/PROPOSAL_mcp_backend.md`](../../docs/11-proposals/PROPOSAL_mcp_backend.md)
(section "Multi-agent orchestration"); transport/tool facts in
[`docs/03-development/PED_MCP_CAPABILITIES.md`](../../docs/03-development/PED_MCP_CAPABILITIES.md).

## When to Use This Skill

Use when you are about to fan out **more than one agent** that *writes* to the
same live Studio Pro via the MCP backend (e.g. generating several modules,
microflows, or pages in parallel). For a single writer, you don't need it. For
read-only fan-out, you don't need it (reads are always safe).

## The one rule that matters: the unit of isolation is the DOCUMENT

Every PED write tool acts on exactly one named document
(`ped_create_document`, `ped_update_document`, `ped_check_errors`). Therefore:

- **Two agents on different documents are isolated and safe.**
- **Two agents on the same document race** — last-write-wins per element, and
  positional add/remove (`/entities/N`, `remove index N`) corrupts under
  interleaving (the "re-read after every mutation" rule can't hold across two
  writers).

A module's **whole domain model is a single document**, so entity/association
writes partition by **module**, not by entity.

## Procedure

### 1. Build the work list and its dependency graph
- List every document to be created/changed.
- Draw the cross-document reference edges. PED resolves references **by name
  against the live model at write time**, so a referenced document must already
  exist when the referencing write runs. Common edges:
  - enumeration → entity that has an attribute of that enum
  - view-entity source document → the view entity
  - microflow / nanoflow / java action → the microflow that calls it
  - entity / module → anything referencing it

### 2. Partition into disjoint write sets, one per agent
- Group by **document**; for domain models, group by **module**.
- Each agent owns a disjoint set. No two agents share a document.
- Anything that genuinely must be co-edited (a shared module's domain model,
  Navigation, project Security) goes to **one** owner agent — do not split it.

### 3. Order by dependency, fan out within a layer
- Topologically sort the dependency edges. Create dependencies **before**
  dependents.
- Within a dependency layer (no edges between them), agents run in parallel.
- **The orchestrator owns the graph.** With the hybrid mxcli backend, agents
  read existence from the local `.mpr` (last-saved) and only see their *own*
  in-session writes — they cannot see another agent's *unsaved* work. So the
  orchestrator must (a) sequence dependency layers and (b) tell a dependent that
  its dependency exists, rather than expecting the dependent to discover it.

### 4. Make every write idempotent (timeout-safe)
- A slow create can return `-32000 Request timed out` **even though it
  succeeded** (Studio Pro appears to serialise work on its UI thread).
- On timeout: do **not** blind-retry. Re-read / `ped_find_document` to check
  whether the write landed; treat "already exists" as success.
- Prefer check-then-act for every create.

### 5. Validate at a quiescent point
- `ped_check_errors` validates the live model *as it is now*, including other
  agents' half-finished edits. Don't treat transient cross-agent errors as your
  failure.
- Run the authoritative validation after a dependency layer completes (a
  quiescent point), scoped to the documents that layer produced.

### 6. One human save at the end
- There is **no flush/save tool**. All agents accumulate into one unsaved
  in-memory model. A human saves once in Studio Pro when the run is done.
- No agent can durably "commit" its slice independently; plan the run as a single
  unit of work that ends with a save.

## Anti-patterns (these cause corruption or false failures)

- Two agents adding entities to the **same module** in parallel.
- Splitting Navigation, project Settings, or Security across agents.
- Blind-retrying a timed-out `ped_create_document` (→ duplicate-document error).
- A dependent agent relying on the **local reader** to see a dependency another
  agent just created but hasn't saved (it won't — route that check through MCP or
  have the orchestrator assert it).
- Running `ped_check_errors` mid-run and aborting on another agent's transient
  error.

## Quick checklist

- [ ] Work list + dependency edges drawn
- [ ] Disjoint document/module assignment per agent
- [ ] Dependencies created before dependents (layered)
- [ ] Shared documents (Navigation/Security/shared module) given to one owner
- [ ] Writes are check-then-act / idempotent
- [ ] Validation deferred to layer-quiescent points
- [ ] Single human save at the end
