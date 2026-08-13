---
title: mxcli developer panel — ask the agent about the interaction you just performed
status: proposed
date: 2026-08-11
author: Generated with Claude Code
related:
  - PROPOSAL_mxcli_dev_warm_loop.md
  - PROPOSAL_mcp_backend.md
  - PROPOSAL_hub_authentication.md
  - proposal-solution-aware-init.md
---

# Proposal: mxcli developer panel — ask the agent about the interaction you just performed

**Status:** Proposed
**Date:** 2026-08-11

## Problem statement

`mxcli run --local` produces four runtime signals — logs, metrics, traces, and the
model catalog — and [`analyze-runtime.md`](../../.claude/skills/mendix/analyze-runtime.md)
is the procedure for joining them. That skill works. What it cannot supply is the
**correlation key**: which of those spans belong to *the thing the user just did in
the browser*.

Today that key exists only in the user's head, and reaches the agent as prose:

> "I clicked Save on the customer form maybe a minute ago and it took ages —
> can you see what happened?"

The agent then guesses at a time window, greps `runtime.log`, and hopes. The
information that would make the question precise — which widget, which page, which
XHR, at exactly which millisecond, producing exactly which span tree — was present
in the browser and thrown away.

The proposal is a small panel inside the app under development that **captures that
context at the moment of the interaction** and makes it retrievable by the agent, so
"show me the flame chart caused by pressing this button" becomes an answerable
question rather than an approximate one.

### Why now

Three pieces landed that make this tractable, none of which existed when the warm
loop started:

- `--trace-otlp` produces spans with real timestamps and parent IDs (the console
  exporter omits both, so it cannot reconstruct a call tree at all).
- The tunnel-hub reverse-proxies every app response, giving a natural injection point
  that requires nothing installed in the tester's browser.
- The hub has an authenticated owner identity (`Backend.Owner`), so a developer-only
  surface can be gated on something real.

## Design principle: the panel records, the agent pulls

The obvious design is for the panel to send a prompt straight into the Claude Code
session and render the reply inline. **This proposal deliberately does not do that.**

Pushing a prompt into a running session requires a "post a message to session X"
API. Such a surface may exist — `create_session` in the Claude Code Remote MCP
server references combining it with `send_message` and polling `list_events` — but
it is not exposed to an agent session today, and the in-agent `SendMessage` tool is
not something a web page can call. **It could not be verified while writing this
proposal, and it is outside mxcli's control.**

So v1 inverts the flow:

- The panel **records** an interaction into a store mxcli owns.
- The agent **pulls** it, on the user's ask, through an MCP tool.
- The answer appears **in the Claude Code conversation**, not in the panel.

This loses the in-page reply, which is the most striking part of the pitch. It keeps
everything else, depends on nothing unverified, and leaves the push path as a
strictly additive follow-up (see Open questions). The user still asks in natural
language — they just ask in the place they are already working, and the panel's job
is to make "the interaction I just performed" a precise referent rather than a
description.

## Architecture

Three components, each independently useful:

```
Browser (app under test)                    Dev container
┌────────────────────────────┐              ┌──────────────────────────────────┐
│  Mendix app                │              │  Mendix runtime (JVM)            │
│  ┌──────────────────────┐  │   /xas/ ───▶ │    + OTel agent ──┐              │
│  │ mxcli dev panel      │  │              │                   ▼              │
│  │  · records clicks    │  │              │  ┌───────────────────────────┐   │
│  │  · captures XHRs     │──┼── POST ─────▶│  │ mxcli run --local         │   │
│  │  · shows what it has │  │  /_mxcli/    │  │  · embedded OTLP collector│   │
│  └──────────────────────┘  │  event       │  │  · interaction store      │   │
└────────────────────────────┘              │  └─────────────┬─────────────┘   │
             ▲                              │                │ MCP             │
             │ injected by the hub          │  ┌─────────────▼─────────────┐   │
             │ (ModifyResponse)             │  │ Claude Code agent         │   │
             └──────────────────────────────┼──│  get_interactions / trace │   │
                                            │  └───────────────────────────┘   │
                                            └──────────────────────────────────┘
```

### 1. Embedded OTLP collector — `run --local --trace-collect`

`--trace-otlp <endpoint>` today points at a collector the user must run themselves.
`--trace-collect` instead starts an **in-process OTLP receiver** (http/protobuf on a
loopback port), points the runtime's agent at it, and keeps a bounded ring buffer of
spans in memory.

This is the single highest-value piece and is **useful with no panel at all** — it
turns "spans go somewhere I have to set up" into "spans are queryable from mxcli",
which is what `analyze-runtime.md` currently sends people to Jaeger for. It should
ship first and alone.

Bounded by span count and age (both flag-tunable), because the default filters still
let a busy transaction produce a lot of spans, and this buffer lives in the same
process as the dev loop.

### 2. The panel

A small self-contained script: a floating toggle, a list of recent interactions, and
a copyable reference for each. It records, per interaction:

| Field | Source |
|-------|--------|
| Widget label / DOM path, page URL | click handler on `document`, capture phase |
| Timestamp window (start, end) | around the interaction's network activity |
| XHR requests fired | `fetch`/`XMLHttpRequest` wrapper, or `PerformanceObserver` |
| Mendix action name | the `/xas/` request payload |
| `traceparent` (if issued) | generated by the panel — see Correlation |

It POSTs each to `/_mxcli/event` on the app's own origin, which mxcli serves
alongside the app. It does **not** talk to the agent, hold credentials, or render
agent output in v1.

### 3. mxcli MCP server — `mxcli mcp serve`

The agent-facing surface. Exposes the recorded interactions and the collector as MCP
tools:

| Tool | Returns |
|------|---------|
| `list_interactions` | Recent interactions, newest first — label, page, time window, action |
| `get_interaction` | One interaction with its full captured detail |
| `get_trace` | The span tree for an interaction (or a raw time window), as a call tree with durations |
| `get_logs` | `runtime.log` lines within an interaction's window |

Note the direction: [`PROPOSAL_mcp_backend.md`](PROPOSAL_mcp_backend.md) makes mxcli
an MCP **client** of Studio Pro's PED server. This is the **reverse** — mxcli as an
MCP *server* the agent connects to. They share no code and do not conflict, but the
naming should be kept clearly distinct (`mcp serve` vs the `--mcp` backend flag) or
the two will be confused permanently.

## Correlation — the crux

Tying a browser interaction to its server spans is the part that decides whether this
is precise or merely suggestive. Two mechanisms, and the proposal should implement
the fallback first because it always works.

**Primary — W3C trace context.** The panel generates a `traceparent` per interaction
and attaches it to the outgoing XHR. If the runtime's OTel agent extracts inbound
trace context, every server span for that request becomes a child of the panel's
trace id, and correlation is exact.

`analyze-runtime.md` records that *outbound* propagation works — "trace context (W3C
`traceparent`) crosses app boundaries automatically over `rest call`" — which shows
the agent's context plumbing is live, and standard OTel servlet instrumentation does
extract inbound headers by default. **This has not been verified against the Mendix
runtime, and nothing should be designed around it until it has.** It is also the
first thing to test, because if it works the rest of this section is unnecessary.

Two caveats even when it works: Mendix may reject unexpected headers on `/xas/`, and
the panel adding a header changes the request the app sends — which must not alter
app behaviour.

**Fallback — time window plus action name.** Record the interaction's start/end and
the `/xas/` action, then select spans overlapping that window on the matching service
name. Approximate: concurrent activity from another tester lands in the same window.
Good enough to answer "what did this button cost", not good enough to answer it on a
busy shared preview. The panel should say which mechanism produced a given answer
rather than presenting both as equally exact.

## Panel injection — hub `ModifyResponse`

The hub's app proxy (`cmd/mxcli/tunnelhub/server.go:135`) has a `Director` and no
`ModifyResponse` — it never touches response bodies today. Injection adds one:
rewrite `text/html` responses to include the panel's `<script>` before `</body>`.

Requirements: decompress/recompress per `Content-Encoding`; skip non-HTML entirely;
respect (and if necessary extend) whatever CSP the app sends; and stream rather than
buffer large responses.

**Alternatives considered and rejected.** A browser extension gives real devtools
integration and network capture, but needs a per-tester install and cannot be
delivered through the hub — worth revisiting as a power-user path once the data model
is proven. Injecting the panel as a **theme asset or JavaScript action** is the one to
avoid: `mxcli theme apply` is deliberately model-free so it cannot break a build, and
`theme switcher install` is called out in CLAUDE.md as the sole exception. A dev panel
written into the model would ship into production artifacts and put a debugging
surface inside the customer's app.

For a purely local loop (`run --local` with no hub), mxcli serves the panel itself on
the app port — same script, no proxy involved.

## Security

Two risks that must be designed in rather than added later.

**The panel must be gated independently of the preview.** The scenario that motivates
all of this — external testers exercising the app — is exactly the one that requires
`--require-auth=false`, because `authorizePreview` is owner-only and testers are not
the owner. That setting leaves the *preview* open, and would leave the panel open with
it. So panel injection must key on the viewer's own authenticated identity matching
`Backend.Owner`, not on whether the preview is reachable. Default off; opt in per run.

**Panel input is untrusted.** Anything the panel captures — widget labels, page
content, seeded test data, another tester's typing — flows into the agent's context.
That is a prompt-injection surface with the same shape as external webhook content.
Captured strings must be delivered to the agent clearly fenced as data, never as
instructions, and the MCP tools must return structured records rather than free prose.
Interaction capture should also exclude password fields and anything the app marks
sensitive.

## BSON structure

**Not applicable.** Nothing here reads or writes Mendix documents. The panel observes
a *running* app; the collector holds spans; the MCP server exposes both. No parser,
writer, or `$Type` involvement, so none of the storage-name hazards apply.

Worth stating explicitly because the tempting shortcut — shipping the panel as a
JavaScript action — *would* write to the model. That is rejected above.

## Proposed CLI surface

No MDL. Three flags and one subcommand:

```bash
# collector alone — useful immediately, no panel
mxcli run --local -p app.mpr --trace-collect

# panel + collector, local loop
mxcli run --local -p app.mpr --trace-collect --dev-panel

# through the hub, owner-gated
mxcli run --hub https://hub.example.com -p app.mpr --trace-collect --dev-panel

# the agent's connection (wired into .mcp.json by `mxcli init`)
mxcli mcp serve -p app.mpr
```

## Implementation plan

| File | Change |
|------|--------|
| `cmd/mxcli/docker/otelcollect.go` *(new)* | In-process OTLP/HTTP receiver; bounded span store; call-tree assembly |
| `cmd/mxcli/docker/runlocal.go` | `--trace-collect` → start collector, point `TraceOTLPEndpoint` at it |
| `cmd/mxcli/docker/localboot.go` | No change — `--trace-collect` reuses the existing OTLP wiring |
| `cmd/mxcli/devpanel/` *(new)* | Panel asset (`go:embed`), `/_mxcli/event` handler, interaction store |
| `cmd/mxcli/tunnelhub/server.go` | `ModifyResponse` on `appProxy`: owner-gated HTML injection, encoding-aware |
| `cmd/mxcli/tunnelhub/auth.go` | Panel-eligibility check, distinct from `authorizePreview` |
| `cmd/mxcli/cmd_mcp_serve.go` *(new)* | `mxcli mcp serve` — MCP server exposing the four tools |
| `cmd/mxcli/init.go` | Register the MCP server in the project's `.mcp.json` |
| `.claude/skills/mendix/analyze-runtime.md` | New section: interaction-scoped analysis |
| `docs-site/src/tools/run-local.md` | Document the flags |

### Order — each phase independently mergeable

1. **Collector** (`--trace-collect`) + `mxcli trace` CLI readout. Ships alone; removes
   the external-collector prerequisite from `analyze-runtime.md`.
2. **Verify inbound `traceparent`.** A spike, not a feature. Its outcome selects the
   correlation mechanism, so it gates phase 4's precision — but not phases 1–3.
3. **MCP server** over the collector. The agent can already answer "what did the last
   30 seconds cost" without any browser involvement.
4. **Panel + local serving**, correlation per phase 2's result.
5. **Hub injection + owner gating.** Last, because it is the piece with real blast
   radius: a bug here corrupts responses for every preview on the hub.

## Version compatibility

No MDL and no model writes, so no `sdk/versions/*.yaml` entry and no `checkFeature()`
gate.

There is one runtime dependency: tracing needs the OTel agent bundled with the Mendix
runtime, which `otelAgentJar` globs and which already fails with a clear "this runtime
may not bundle it" when absent. `--trace-collect` inherits that behaviour unchanged —
no new gate, but the minimum version that bundles the agent should be established and
recorded, since it is currently discovered only at boot.

The panel itself is version-independent: it observes the DOM and network of whatever
the app renders.

## Test plan

No `mdl-examples/` scripts — no MDL surface.

| Test | Asserts |
|------|---------|
| `TestOTLPCollector_AssemblesCallTree` | Spans with parent IDs → correct tree, durations, ordering |
| `TestOTLPCollector_Bounded` | Ring buffer evicts by count and age; memory stays bounded under a span flood |
| `TestDevPanel_EventStore` | Interactions recorded, retrievable newest-first, capped |
| `TestDevPanel_ExcludesSensitiveFields` | Password/marked-sensitive inputs never captured |
| `TestHubInject_HTMLOnly` | Injects into `text/html`; leaves JSON, JS, images, and `/xas/` responses byte-identical |
| `TestHubInject_Encoding` | gzip in → gzip out, body still valid |
| `TestHubInject_OwnerGated` | Non-owner and anonymous viewers get an uninjected response, even with `--require-auth=false` |
| `TestMCPServer_Tools` | Each tool's schema and payload shape; captured strings fenced as data |
| Integration (`-tags integration`) | `run --local --trace-collect`, drive a page with Playwright, assert the interaction's span tree names the expected microflow |

The last one matters most and is the only honest proof: per the
"verified at the layer the symptom lives in" rule, correlation is a *render-time plus
runtime* property. A unit test over a synthetic span set proves the tree-builder, not
that the panel's window actually selects the right spans in a live app. The existing
`run --local --screenshot` Playwright machinery already provides the driver.

## Open questions

1. **Does the Mendix runtime extract inbound `traceparent`?** Phase 2 answers it. If
   no, correlation stays window-based and the panel must be honest about precision.
2. **In-panel answers.** The push API is the missing half. If a supported
   "post to session X" surface is confirmed, the panel gains a text box and renders
   replies — purely additive, no redesign, because the capture and MCP layers are
   unchanged. Explicitly deferred, not designed around.
3. **Multi-tester attribution.** With several testers on one preview, whose
   interactions does the owner's panel show? Simplest v1: only the owner's own
   browser records anything. A shared feed is more useful for "what did the tester
   just hit" and needs a per-viewer identity the panel does not have today.
4. **Solution-wide view.** With [`proposal-solution-aware-init`](proposal-solution-aware-init.md)
   and warm-loop slice 5, one interaction may span two apps. `--trace-service` gives
   each app a distinct name and trace context already propagates over `rest call`, so
   a cross-app tree looks reachable — but the collector would need to receive from
   several runtimes, and it is currently per-`run --local`.
5. **Where does the interaction store live?** In-memory dies with the dev loop; a file
   under `.mxcli/` survives restarts and lets the agent look back at yesterday. The
   latter is probably right, and needs the same sensitive-data care as capture.
