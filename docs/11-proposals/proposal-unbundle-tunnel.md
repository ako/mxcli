---
title: Remove the embedded tunnel from mxcli; ship the hub as its own binary
status: proposed
date: 2026-08-27
author: Generated with Claude Code
related:
  - 0009-tunnel-is-linux-only
  - PROPOSAL_mxcli_dev_warm_loop.md
  - PROPOSAL_hub_authentication.md
---

# Proposal: Remove the embedded tunnel from mxcli; ship the hub as its own binary

**Status:** Proposed
**Date:** 2026-08-27

## Problem statement

[ADR-0009](../13-decisions/0009-tunnel-is-linux-only.md) removed the embedded
chisel tunnel from the Windows and macOS builds, keeping it in Linux. Its
reasoning was explicit, and it rejected full removal on exactly this basis:

> **Dropping the tunnel entirely, all platforms.** Rejected: the browser preview
> from an egress-only container is a real and load-bearing feature for the Claude
> Code web workflow, and it runs on Linux **where the detection problem does not
> bite the same audience.**

**That premise has failed.** Enterprise virus scanners are flagging the Linux
binaries. Linux is not a safe harbour: mxcli's Linux builds run in dev containers,
CI runners, and managed Linux endpoints, all covered by the same EDR estate that
flags the Windows build — and a container image scan is if anything *more*
routine than a desktop scan.

The GOOS gate was the right shape for the problem as understood then. It is not
enough for the problem as it actually is. Chisel is a genuine dual-use pivoting
tool, and — as ADR-0009 says better than this proposal could — the only legitimate
fix is to stop shipping the capability where it is not wanted, never to hide it.
The gate reduced the blast radius from three platforms to one. This proposal
takes it to zero, by moving the capability into a binary that a person has to
choose to install.

### What this changes about ADR-0009

ADR-0009 is Accepted and immutable. This proposal, if accepted, is implemented by
a **new ADR that supersedes it** — per the convention in
[`docs/13-decisions/README.md`](../13-decisions/README.md). The superseding ADR
should preserve ADR-0009's reasoning wholesale: its analysis of why this is not a
false positive, why signing does not fix it, and why evading detection is off the
table are all still correct and load-bearing. Only the scope of the remedy changes.

## Measurements

Measured this session on `main` (295140db), release flags throughout
(`CGO_ENABLED=0 -trimpath -ldflags="-s -w"`), each pair built back to back from an
otherwise identical tree.

**Dependency graph.** 28 non-stdlib packages are in the Linux graph and not the
darwin one: the 9 `jpillora/chisel/*` packages, `gorilla/websocket`,
`armon/go-socks5`, the `golang.org/x/crypto/ssh` stack (plus blowfish, chacha20,
curve25519, poly1305, bcrypt_pbkdf), `golang.org/x/net/proxy` and its socks
internals, and the `jpillora/*` support libraries.

**Binary size.** To isolate chisel's cost, the build tags were flipped so that a
Linux build excluded it, and (renaming past the `_linux.go` filename constraint) a
darwin build included it:

| Build | With chisel | Without | Delta |
|-------|------------:|--------:|------:|
| linux/amd64 | 94,171,298 | 92,950,690 | **−1,220,608 (−1.30%)** |
| darwin/amd64 | 95,935,072 | 94,675,008 | **−1,260,064 (−1.31%)** |

**This corrects a number in ADR-0009.** That ADR reports the same removal as
"13.5 MB (−14.67%)" on windows/amd64 and darwin/amd64. Two independent
measurements here put it at ~1.2 MB (~1.3%), an order of magnitude smaller, and
the two platforms agree with each other. The ADR's figure could not be reproduced;
most likely it straddled unrelated churn (it warns about exactly that hazard for
the embedded-skills directory, and may not have fully controlled for it).

**This does not weaken the case — it relocates it.** Size was never the argument.
The argument is that the binary contains a tunnelling and pivoting tool, and
1.2 MB of it is exactly as detectable as 13.5 MB would be. Any version of this
work that is sold internally on "the binary gets much smaller" is being sold on a
false premise; it should be sold on capability removal, which is real.

## The transport constraint — measured, and it rules out most alternatives

Any replacement for chisel is bounded by an environmental fact that is easy to
miss and expensive to discover late:

> **The transport must be TLS on port 443, spoken to a proxy that terminates and
> re-inspects it.**

A Claude Code container — the environment the preview feature exists to serve —
has egress only through a TLS-terminating MITM proxy (hence its CA bundle at
`/root/.ccr/ca-bundle.crt` and a matching Java truststore). Measured in one:

| Check | Result |
|-------|--------|
| `ssh` client present | **Absent** — no `ssh`, no `autossh` |
| Direct TCP to `github.com:22` | **Blocked** |
| Proxy `CONNECT …:22` | `200 Connection Established` — **meaningless, see below** |
| SSH banner from a real sshd (`github.com:22`) | **Never arrives**; a TLS attempt gets `Connection reset by peer` |
| HTTPS through the proxy | **Works** |

Two things make this readable rather than ambiguous, and both are needed:

- **The `200` is not evidence.** `CONNECT nonexistent.invalid:22` *also* returns
  `200 Connection Established` — the proxy answers optimistically without
  connecting upstream. Anyone testing reachability by CONNECT status alone will
  conclude SSH works, and be wrong.
- **The banner is the evidence.** An sshd speaks *first*, sending `SSH-2.0-…` the
  moment the socket opens. Nothing arrives. The `:443` control is silent too, but
  that is correct — TLS clients speak first — which is exactly why both controls
  are required to interpret either.

Independently, the proxy declares `gitSshRewrite: true`: the platform already
rewrites git SSH URLs to HTTPS because SSH does not work there.

**This is why chisel was chosen and why it works.** It is WebSocket over genuine
TLS on 443, so a TLS-terminating proxy can relay it like any other HTTPS traffic —
which is what `proxyForURL` in `docker/tunnel.go` exists for. Nothing about SSH
survives that path.

The constraint is therefore a design input for anything that replaces the
transport, and it eliminates a whole family of otherwise-attractive answers before
any code is written.

## Alternatives, if a separate binary does not satisfy the policy

This proposal's answer — move the capability into an opt-in binary — is the
cheapest one that keeps the feature. If the policy that flagged the Linux builds
objects to the *capability* rather than to what ships unasked, it will not be
enough. The alternatives, with the transport constraint applied:

| Alternative | Verdict |
|-------------|---------|
| **Bring your own approved tunnel** (`ssh -R`, from the developer's own OpenSSH) | **Ruled out** for Claude Code containers by the measurement above. It may work on a corporate laptop with an approved SSH client, but supporting both environments means two transports — worse than one. |
| **Hosted runner** — the app runs on the hosted side; the developer pushes a build | **Unaffected by the constraint**, because it needs no transport at all. Strongest endpoint story: nothing listens, tunnels, or accepts inbound. Costs compute + a database per preview, and changes the product from "your local app, previewed" to "your app, deployed" — worth building only if it keeps the warm loop (push once, then `reload_model` over the admin channel). |
| **A narrow HTTP-only forwarder** we write ourselves | The constraint *specifies* its design: HTTPS/WebSocket on 443 — chisel's wire shape without the SSH and SOCKS payload. Legitimate **only** as capability reduction (it genuinely cannot pivot), never as signature reduction; being wire-identical to chisel while claiming to differ sits close to the line ADR-0009 draws. Detection outcome is unknown, so this costs real work to discover whether it helps. |
| **Cloudflare Tunnel / Tailscale Funnel** | Untested, and the most promising untested option: both are built for HTTPS-only egress, so the constraint does not obviously bite, and both are frequently already allow-listed. Worth probing before building anything bespoke. |

The hub itself survives all four. Its seam is already at the transport boundary:
chisel appears only in the control server at `ControlAddr` and in whatever listens
on a backend's `ReversePort`, while Host routing, per-subdomain autocert, the
registry, GitHub auth, hub keys, the session log and the admin overview are
transport-agnostic — as `control_other.go` already proves by compiling and testing
without it. Swapping transports is a contained change, not a rewrite.

## Proposed design

### One new binary, both ends

A second main package, `cmd/mxcli-hub`, containing everything that speaks the
tunnel — the relay *and* the client — as the user asked. Today's split across two
mxcli surfaces (`mxcli tunnel-hub` for the server, `mxcli run --hub` for the
client) collapses into one tool with two verbs:

```bash
# the relay (was: mxcli tunnel-hub)
mxcli-hub serve --domain example.com [--github-oauth-client-id …] [--keys-file …]

# the client (was: the tunnelling half of mxcli run --hub)
mxcli-hub expose --port 8080 --hub https://hub.example.com [--project X --branch Y]

# hub credentials (was: mxcli auth hub …)
mxcli-hub auth login --token <pat>
mxcli-hub auth status
```

The code moves nearly intact. What ships in `mxcli-hub`:

| From | Lines | Contents |
|------|------:|----------|
| `cmd/mxcli/tunnelhub/` (+`audit/`) | ~4,700 | Registry, registration API, Host-routing front, autocert, admin overview, GitHub auth, hub keys, session log |
| `cmd/mxcli/hubauth/` | ~360 | Hub key resolution (`MXCLI_HUB_KEY` → `~/.mxcli/auth.json`) |
| `cmd/mxcli/docker/tunnel*.go` | ~450 | Chisel client, proxy resolution honouring `NO_PROXY` |
| `cmd/mxcli/docker/hubclient.go` | ~280 | Registration, heartbeat, re-register-on-hub-restart |
| `cmd/mxcli/cmd_tunnelhub.go`, `cmd_auth_hub.go` | ~350 | The two command trees |

This is a favourable move because the hub code is already almost free-standing.
Its only imports from the rest of the repo are its own `audit` subpackage and
`internal/auth` (981 lines, the credential store); `tunnel.go` and `hubclient.go`
import nothing from mxcli at all.

### What leaves mxcli

Every `--hub*` flag on `mxcli run`, the `tunnel-hub` and `auth hub` command trees,
and the two build-tag seams ADR-0009 introduced (`tunnel_linux.go` /
`tunnel_other.go`, `control_linux.go` / `control_other.go`). Those seams exist only
to keep chisel out of some builds; with chisel out of every mxcli build they are
dead weight, and removing them pays back the maintenance obligation ADR-0009
listed as a negative consequence.

### The `ApplicationRootUrl` problem — the one real design difficulty

`run --hub` registers with the hub **before boot**, precisely so the runtime can
start with `ApplicationRootUrl` set to the assigned URL; without it the SPA and the
`originURI` cookie misbehave under the public origin
(`docker/runlocal.go`, the slice-3 wiring). Split the binaries and mxcli no longer
knows that URL.

The clean answer is a small, **generic** flag:

```bash
mxcli run --local -p app.mpr --app-root-url https://myapp.hub.example.com
```

`--app-root-url` sets the existing `LocalRuntimeOptions.ApplicationRootUrl` and
carries no tunnel code whatsoever. It is independently useful — any reverse proxy,
ngrok, or cloudflared setup needs the same thing, and mxcli has no way to express
it today.

The URL is knowable in advance because the hub's subdomain is deterministic from
prefix/project/branch (`tunnelhub/slug.go`, `baseSlug`), so `mxcli-hub` can print
it without anything running:

```bash
URL=$(mxcli-hub url --hub https://hub.example.com --project app --branch main)
mxcli run --local -p app.mpr --app-root-url "$URL" &
mxcli-hub expose --port 8080 --hub https://hub.example.com
```

Three commands where there was one. That is the honest cost of the split, and it
is the same shape as the `mxcli tunnel` gap identified when we looked at running
the app in Docker Desktop — a tunnel that attaches to a port someone else is
serving, rather than one welded to the boot path.

### Distribution — and the line ADR-0009 drew

`mxcli-hub` gets its own release artifacts per platform, alongside mxcli's.

**mxcli must never download or execute `mxcli-hub`.** ADR-0009 rejected "shell out
to an external chisel binary" partly because mxcli would then be *fetching and
running* a flagged tunnelling binary at run time — "a stronger EDR signal than
linking it, and a supply-chain question besides". That objection survives this
proposal intact and constrains it: the whole benefit is that installing a tunnel
becomes a deliberate human act. If mxcli auto-fetches the hub binary on
`--hub`, we have moved the bytes and kept the problem. So:

- no auto-download, no `exec` of `mxcli-hub` from mxcli;
- `mxcli setup mxcli` does not gain a hub variant;
- the dev container template does not install it by default.

Whether `mxcli-hub` ships for all platforms or Linux only is an open question
below. The argument for all platforms is that a deliberate download is exactly the
consent that makes this acceptable; the argument for Linux only is that it is
where the hub client and server actually run.

### Telling users where it went

A removed flag should not read as `unknown flag: --hub`. Keep `--hub` and
`tunnel-hub` registered for one or two releases as **deprecation shims that
contain no tunnel code** — they print where the capability moved and exit
non-zero. The risk to weigh: a registered flag advertises a capability the binary
does not have, which is mildly confusing but far kinder than a bare parse error.
Remove the shims on a named release.

### The guard, inverted

`scripts/check-tunnel-deps.sh` currently fails if chisel appears in a **non-Linux**
graph. It becomes a two-way guard:

- **`./cmd/mxcli`** — the forbidden list must not appear on **any** platform,
  Linux included. This is the guard that matters; without it the import returns
  the next time someone edits hub-adjacent code.
- **`./cmd/mxcli-hub`** — chisel is *expected*; its absence means the split
  silently broke the feature.

## Go module boundary — recommended, with its cost

Both binaries in one Go module means **`go.mod` still requires chisel**. ADR-0009
flagged this as Neutral, and for a link-time gate it was. It is not neutral here:
an enterprise scanning mxcli's dependency manifest — SCA, SBOM, `go list -m all`,
Dependabot — still sees chisel, and a policy that blocks the *dependency* is not
satisfied by a binary that does not link it. That is a real variant of the
complaint that started this.

Splitting the hub into its own module (`./hub`, module
`github.com/mendixlabs/mxcli/hub`) removes chisel from mxcli's module graph
entirely. `go install` of mxcli would never fetch it, and mxcli's SBOM would not
list it.

The cost is real and should not be waved away: two modules mean a second
`go.mod`/`go.sum` to keep in step, a `go.work` for local development, release and
CI plumbing per module, and a decision about `internal/auth` — which both need
(981 lines) and which, being `internal/`, is *not importable across a module
boundary*. That last point is the concrete blocker: the shared credential store
must either move to a non-internal package, be duplicated, or be extracted into a
small third module.

**Recommendation: single module first, separate module as a fast follow.** The
binary split is what answers the reported problem and can ship without the module
question; doing both at once entangles a straightforward code move with a
build-system change. But the module split should be planned, not deferred
indefinitely — a dependency-manifest complaint is a plausible next report.

## BSON structure

**Not applicable.** Nothing here touches Mendix documents. This is a build and
packaging change plus a code move; no parser, writer, or `$Type` is involved.

## Proposed MDL syntax

**None.** No grammar, AST, visitor, or executor changes. The MDL surface is
untouched.

## Implementation plan

| File | Change |
|------|--------|
| `cmd/mxcli-hub/main.go` *(new)* | Cobra root; `serve`, `expose`, `url`, `auth` |
| `cmd/mxcli-hub/tunnelhub/` | Moved from `cmd/mxcli/tunnelhub/` (+`audit/`), unchanged |
| `cmd/mxcli-hub/hubauth/` | Moved from `cmd/mxcli/hubauth/`, unchanged |
| `cmd/mxcli-hub/tunnel.go`, `hubclient.go` | Moved from `cmd/mxcli/docker/`; build tags dropped (this binary is the tunnel) |
| `cmd/mxcli/docker/runlocal.go` | Strip hub registration/heartbeat/tunnel (~55 hub references); keep `ApplicationRootUrl`, now fed by the new flag |
| `cmd/mxcli/cmd_run.go` | Remove `--hub*` (~45 references); add `--app-root-url`; `--hub` becomes a shim pointing at `mxcli-hub` |
| `cmd/mxcli/cmd_tunnelhub.go`, `cmd_auth_hub.go` | Deleted (shim retained briefly for `tunnel-hub`) |
| `cmd/mxcli/docker/tunnel*.go` | Deleted, both seam halves and their tests |
| `scripts/check-tunnel-deps.sh` | All platforms for `./cmd/mxcli`; require for `./cmd/mxcli-hub` |
| `Makefile`, `.github/workflows/release.yml` | Parallel per-platform targets + release artifacts for `mxcli-hub` |
| `docs/13-decisions/00NN-*.md` *(new)* | ADR superseding ADR-0009 |
| `docs-site/src/tools/run-local.md`, `.claude/skills/mendix/run-local.md` | The three-command flow; `--app-root-url` |
| `CLAUDE.md` | The hub bullet in the implemented-features list now names a separate binary |

### Order

1. **`--app-root-url` on `run --local`.** Independently useful, no tunnel
   involvement, and it unblocks the split. Mergeable alone.
2. **`cmd/mxcli-hub` with the code moved**, both binaries building, no mxcli
   behaviour change yet. Largest diff, near-zero risk — it is a move.
3. **Cut the tunnel out of mxcli**: flags, seams, dependencies, shims.
4. **Guard inverted, release plumbing, docs, superseding ADR.**
5. *(Fast follow)* Module split, `internal/auth` extraction.

Steps 2 and 3 are deliberately separate so that the commit which removes the
capability is small and reviewable on its own.

## Version compatibility

No Mendix version dependency; nothing here reads or writes model content. No
`sdk/versions/*.yaml` entry, no `checkFeature()` gate.

The user-facing compatibility break is the CLI surface: `mxcli run --hub` and
`mxcli tunnel-hub` stop working, and anyone using them must install a second
binary and adopt the three-command flow. That warrants a **minor version bump and
a prominent CHANGELOG entry**, not a patch release.

## Test plan

The hub's existing tests move with the code and must keep passing unchanged —
that is the main evidence the move was faithful. `tunnelhub` is well covered
already (registry, API, keys, auth, sessions, server, plus the in-process
integration test).

| Test | Asserts |
|------|---------|
| `scripts/check-tunnel-deps.sh` (extended) | Forbidden modules absent from `./cmd/mxcli` on **linux**, darwin, and windows; present in `./cmd/mxcli-hub` |
| Binary-inspection mode of the same script | A built `mxcli` contains no chisel string literals on any platform (the Linux binary carries ~450 today) |
| `TestRunLocal_AppRootURL` | `--app-root-url` reaches `LocalRuntimeOptions.ApplicationRootUrl`; absent by default |
| `TestHubURLIsDeterministic` | `mxcli-hub url` matches the subdomain `serve` would assign for the same identity — the two must not drift, or `--app-root-url` is silently wrong |
| Moved `tunnelhub` suites | Pass unchanged in their new location |
| Integration (`-tags integration`) | `mxcli-hub serve` + `mxcli-hub expose` against a local HTTP server end to end, replacing the current in-process tunnel test |

The last one matters most: today's coverage tests the tunnel *inside* one process.
After the split the two ends are separate binaries, so the interesting failure —
they no longer agree on registration, ports, or auth — is only visible across a
real process boundary.

## Open questions

1. **Which platforms does `mxcli-hub` ship for?** All (a deliberate download is
   the consent that makes it acceptable) or Linux only (where it actually runs)?
   Leaning all-platforms, since a macOS developer wanting a preview is a real case
   and the whole point is that nobody gets it unasked.
2. **Naming.** `mxcli-hub` reads as "part of mxcli", which is accurate and helps
   discovery, but also ties the flagged binary to the mxcli name in any EDR
   report. A distinct name would decouple the reputations at the cost of
   discoverability. Note the line from ADR-0009: renaming to *hide* a dependency is
   off the table; naming a genuinely separate product is not the same act, but the
   distinction is worth stating deliberately rather than stumbling into.
3. **Do the hub's own binaries get flagged, and does that matter?** Almost
   certainly yes, and probably not — the tool is opt-in and honestly described.
   But if the hub is meant to be installable on managed endpoints, this whole
   exercise only relocates the ticket. **This is the question that decides whether
   the proposal is right at all**, and it should be answered with whoever reported
   the Linux detections **before** any of this is built.

   What to ask them, because the answer selects from the alternatives table above:
   does the policy object to *unapproved binaries* (→ a hosted runner, or one of
   their own approved tools), to *tunnelling as a capability* (→ hosted runner
   only), to *chisel specifically* (→ this proposal suffices), or to *dev machines
   accepting inbound traffic at all* (→ hosted runner only)? A separate question
   again if the objection is chisel in the **dependency manifest** rather than in
   the binary — that one needs the module split, not the binary split.
4. **How long do the deprecation shims live?** One release, two, or none?
5. **Does anything else in the repo assume `run --hub` exists?** The devcontainer
   template, the bootstrap prompt, and the session-grouping work in the hub
   overview all reference the integrated flow; each needs an audit pass in step 4.
