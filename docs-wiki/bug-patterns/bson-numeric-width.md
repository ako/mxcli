---
title: BSON Numeric Width Mismatches
category: bug-pattern
last-synced: 4e185f73
sources:
  - .claude/skills/fix-issue/findings/
  - sdk/mpr/parser.go
---

> **Do not duplicate**: the per-field fix recipes live in the `fix-issue/findings/*.jsonl` records (issues #583, #585) and the `extractInt` helper signature lives in `sdk/mpr/parser.go`. This page describes the pattern only.

## What this is

A family of silent read-side bugs where a numeric BSON field comes back as `0` (or `unlimited`) even though Studio Pro clearly shows a real value. The field is present in the `.mpr`, but mxcli's parser never sees it because of a width mismatch between how the value was written and how the parser tries to read it.

## How it fits

Studio Pro writes integer properties at whatever width it chooses — often `int64` — but Go's type switch is exact: `raw["X"].(int32)` only matches if the stored value is literally an `int32`. When the widths disagree, the type assertion fails, and instead of erroring it returns the zero value. The bug is invisible at parse time and only surfaces downstream, when `describe` or a catalog query reports `Length`, `MinOccurs`, `MaxLength`, `FractionDigits` or similar as `0`.

The tell-tale: a numeric field reads `0`/`unlimited` while Studio Pro shows a non-zero value, and the field's read path contains a narrow `.(int32)` (or `.(int64)`) assertion. The class recurs because every new numeric field invites a fresh hand-written assertion, and the failure is silent rather than loud.

The canonical fix is the width-agnostic `extractInt` helper in [`sdk/mpr/parser.go`](../../sdk/mpr/parser.go), which accepts `int32`/`int64`/`int`/`float64`. The per-field recipe — including how to sweep for stray assertions and preserve non-zero defaults for absent fields — is in the symptom table.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the per-instance fix recipes for this pattern
- [[architecture/mpr-read-write]] — how the BSON reader/parser layer is structured
- [[models/storage-vs-qualified-names]] — the other major class of read/write mismatch
