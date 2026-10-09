---
title: mxcli vs. the TypeScript Model SDK
category: positioning
last-synced: 4e185f73
sources:
  - docs/01-project/SDK_EQUIVALENCE.md
  - README.md
---

> **Do not duplicate**: the gap-analysis tables and type-coverage numbers live in [docs/01-project/SDK_EQUIVALENCE.md](../../docs/01-project/SDK_EQUIVALENCE.md); the implemented-feature list lives in `README.md` and CLAUDE.md "Current Implementation Status"; the TypeScript SDK's own behaviour is documented at [docs.mendix.com](https://docs.mendix.com/apidocs-mxsdk/mxsdk/). Link out — do not restate.

## What this is

mxcli is a local-first, pure-Go reimplementation of the capability behind Mendix's official TypeScript Model SDK (`mendixmodelsdk`): reading and modifying the model that lives inside a Mendix `.mpr` project. It targets the same metamodel and the same on-disk format, but reaches it from a different direction.

## How it fits

The two tools solve the same problem — programmatic model manipulation — for different worlds.

**What mxcli replicates.** The metamodel itself: entities, microflows, pages, widgets, security, and the rest. Go types are generated from the same reflection data the official SDK ships, so a `DomainModels$entity` means the same thing in both.

**Where it diverges.**
- **Local, not cloud.** The TypeScript SDK is cloud-first: it connects to the Team Server, manages a working copy, and synchronises in real time. mxcli opens the `.mpr` file directly on disk — no network, no working-copy lifecycle, no live collaboration. See [[architecture/mpr-read-write]].
- **A DSL, not an object graph.** The official SDK exposes a typed JavaScript object graph mutated through a delta system. mxcli's primary surface is MDL, a SQL-shaped textual language (`create entity ...`, `show microflows ...`) executed through a backend abstraction. See [[rationale/mdl-as-sql]] and [[rationale/backend-abstraction]].
- **Pure Go, no CGO.** Built for embedding in CLI tooling and AI coding agents rather than a Node toolchain.

**What it does not aim to do.** No Team Server connectivity, no real-time sync, no delta/undo system, and only a fraction of the 52 metamodel domains. The [gap analysis](../../docs/01-project/SDK_EQUIVALENCE.md) is the canonical scorecard.

**Which tool for which job?** Reach for the official SDK when you need cloud round-trips, collaboration, or full domain coverage in a Node project. Reach for mxcli when you want to read or edit a local `.mpr` from Go, from the command line, or from an AI agent — without standing up the cloud stack.

## See also

- [docs/01-project/SDK_EQUIVALENCE.md](../../docs/01-project/SDK_EQUIVALENCE.md) — detailed gap analysis, type-coverage tables, code-generation strategy
- [[architecture/mpr-read-write]] — how mxcli opens `.mpr` files directly
- [[rationale/mdl-as-sql]] — why the surface is a SQL-shaped DSL
- [[rationale/backend-abstraction]] — how MDL execution stays decoupled from the SDK
- [[glossary]] — terminology bridge between Mendix UI, SDK, and BSON names
