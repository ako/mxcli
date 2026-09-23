---
title: MPR Read/Write
category: architecture
last-synced: 9ab9afa6
sources:
  - modelsdk.go
  - modelsdk/mpr/reader.go
  - modelsdk/codec/decoder.go
  - modelsdk/mpr/writer_core.go
  - modelsdk/canon/canon.go
  - modelsdk/canon/identity.go
  - docs/13-decisions/0008-identity-and-idempotence.md
---

> **Do not duplicate**: the public API surface (see `README.md` and `modelsdk.go`), specific BSON field tables (see `docs/03-development/PAGE_BSON_SERIALIZATION.md`), the write-safety rules an implementer must follow (CLAUDE.md is canonical), the decision behind conditional writes (see [ADR-0008](../../docs/13-decisions/0008-identity-and-idempotence.md)), or fix recipes (see `.claude/skills/fix-issue.md` and `/mxcli-dev:fix-issue`).

## What this is

The layer that turns a `.mpr` file on disk into typed Go model elements and back. An `.mpr` is a SQLite database whose document rows hold BSON-encoded Mendix model elements, so reading and writing is two problems stacked: SQLite access, and BSON (de)serialization of polymorphic Mendix types. This layer owns both.

## How it fits

[`modelsdk.Open`](../../modelsdk.go) returns a read-only [`Reader`](../../modelsdk/mpr/reader.go); `OpenForWriting` wraps it in a [`Writer`](../../modelsdk/mpr/writer_core.go). That nesting is deliberate — a writer *is* a reader plus mutation methods, because every safe write first reads the current state. The reader opens SQLite via the pure-Go `modernc.org/sqlite` driver (no CGO), pins a single connection to dodge lock contention, and detects the storage format.

Format detection is automatic and defensive. v1 is a single-file database; v2 (Mendix 10.18+) splits metadata from per-document `.mxunit` files under `mprcontents/`. The reader first checks for the folder, then reconciles against the actual DB schema — a `.mpr` copied without its `mprcontents/` folder would otherwise take the v1 path and fail on a `Contents` column that v2 schemas do not have.

The reason this layer is BSON-aware rather than a generic SQLite patcher is that Mendix's BSON is irregular: IDs appear as binary blobs, base64 maps, or `$ID` fields; arrays carry a leading marker (`1`, `2` or `3`) that distinguishes by-name collections from contained ones; and `$Type` discriminators select polymorphic structs. The [codec](../../modelsdk/codec/decoder.go) encodes these conventions exactly, because Studio Pro rejects any deviation — a wrong storage name, a malformed empty-array marker, or a numeric width mismatch all surface as load-time exceptions. See [[models/storage-vs-qualified-names]] and [[bug-patterns/bson-numeric-width]].

**Every write funnels through one function.** `modelsdk/mpr` exposes `UpdateRawUnit` over a private `updateUnit`, and that single choke point is what makes a cross-cutting write policy possible at all: a rule added there applies to every document type without touching a single serializer. `WriteTransaction.WriteUnit` is the second door onto the same policy — it is how `codec.Store` flushes units in a batch. The writer stages v2 file writes through a temp file and rename, so a hard-linked fixture is not clobbered through its shared inode, and bumps the `_Transaction` row Studio Pro watches for external changes.

There used to be two engines here. The hand-written `sdk/mpr` serializer was deleted once its importer count reached zero ([ADR-0004](../../docs/13-decisions/0004-full-codec-engine.md)), so reaching past the backend abstraction is now a compile error rather than a rule to remember.

**A write is conditional.** Since [ADR-0008](../../docs/13-decisions/0008-identity-and-idempotence.md), storage does not write a unit whose new content is *semantically* equal to what is stored. The comparison cannot be on bytes: a rebuild mints a fresh random `$ID` for every sub-element, so the bytes always differ and byte comparison would skip nothing. [`modelsdk/canon`](../../modelsdk/canon/canon.go) instead compares a canonical form in which each element `$ID` is replaced by its position in a containment walk — and because the set of element IDs comes from that same walk, any occurrence of one of them anywhere in the document is a reference by definition, so the comparison needs no knowledge of *which* properties hold references. That is what makes it implementable today, and why a new document type is covered without registering anything.

The consequence worth internalising is that **not writing is the safe outcome, not merely the cheap one**. Canonical equality means the two documents disagree only about which IDs they picked, and the stored ones are the IDs every pointer inside that unit already agrees with. Rewriting them is how a reverted attempt made projects unopenable. See [[models/element-identity]] for why a unit's IDs are private to it, and what has to be carried across a rebuild rather than re-minted.

## See also

- [modelsdk.go](../../modelsdk.go) — public `Open` / `OpenForWriting` entry points and constructors
- [ADR-0008](../../docs/13-decisions/0008-identity-and-idempotence.md) — the decision, the measurements, and the invariant conditional writes rest on
- [docs/03-development/PAGE_BSON_SERIALIZATION.md](../../docs/03-development/PAGE_BSON_SERIALIZATION.md) — BSON field/type tables
- [[models/element-identity]] — `$ID` vs `GUID` vs `StableId`, and the unit as identity boundary
- [[models/storage-vs-qualified-names]] — why `$Type` strings differ from SDK names
- [[bug-patterns/bson-numeric-width]] — int32/int64 serialization hazards
- [[architecture/mdl-execution]] — the backend layer that drives these writes
- [[architecture/widget-engine]] — widget BSON, which sits on top of this
