# v1 (SQLite) vs v2 (mprcontents/)

Mendix uses two MPR file format versions. The library auto-detects and handles both.

## MPR v1 (Mendix < 10.18)

A single `.mpr` SQLite database file containing all model data.

### Storage

- `Unit` table: one row per document — identity, containment, conflict markers,
  content hash, **and the BSON blob itself in the `Contents` column**
- `_MetaData` table: Mendix product/build version and schema hash

There is no separate contents table. `SELECT Contents FROM Unit WHERE UnitID = ?`
is the read; `UPDATE Unit SET Contents = ? WHERE UnitID = ?` is the write. See
[MPR File Format](./mpr-format.md) for the full column list.

### Characteristics

- Self-contained single file
- Larger file size (entire project in one database)
- Binary diffs make Git versioning difficult
- All documents read/written through SQLite queries

### Reading

```go
// Auto-detected by modelsdk.Open()
reader, _ := modelsdk.Open("/path/to/project.mpr")
// reader.Version() returns 1
```

## MPR v2 (Mendix >= 10.18)

An `.mpr` metadata file plus a `mprcontents/` folder with individual document files.

### Storage

- `.mpr` file: SQLite with the same `Unit` table as v1 **minus the `Contents`
  column**, plus a `_Transaction` table holding `LastTransactionID`
- `mprcontents/` folder: individual `.mxunit` files containing BSON

Each `.mxunit` file holds the raw BSON for one document. It is named by the
document's `UnitID` rendered in .NET GUID byte order, and lives two directories
deep — sharded by the first two and next two hex characters of that name.

### Directory Layout

```
project/
├── app.mpr                    # SQLite metadata
└── mprcontents/
    ├── bf/
    │   └── 10/
    │       └── bf10dffa-61ff-4288-8a63-d53ae4522615.mxunit
    ├── 2a/
    │   └── 3b/
    │       └── 2a3b4c5d-....mxunit
    └── ...
```

A flat `mprcontents/<UUID>.mxunit` will not be found: the two shard directories
are part of the path.

### Characteristics

- Better for Git versioning -- individual files change independently
- Smaller diffs when modifying a single document
- Parallel read access to different documents
- Requires both the `.mpr` file and the `mprcontents/` folder

### Reading

```go
// Auto-detected by modelsdk.Open()
reader, _ := modelsdk.Open("/path/to/project.mpr")
// reader.Version() returns 2
```

## Format Detection

Detection is by **directory**, with a schema check as the reconciliation step
(`modelsdk/mpr/reader.go`):

| Step | Check | Result |
|------|-------|--------|
| 1 | `mprcontents/` exists next to the `.mpr` and is a directory | v2 |
| 2 | otherwise | v1 |
| 3 | if step 2 said v1 but `Unit` has **no `Contents` column** | v2 after all |

Step 3 exists because the folder check fails for a `.mpr` copied away from its
`mprcontents/`. The schema is the ground truth there: `Unit.Contents` is
present in v1 and absent in v2, which is the one column that distinguishes the
two. Opening such a project **for writing** is refused rather than guessed at,
since the SQLite rows would have no files behind them.

`_MetaData._FormatVersion` is a second, independent signal — present and equal
to `2` in v2, absent in v1 — which mxcli does not currently use.

This detection is automatic -- callers of `Open()` and `OpenForWriting()` do not need to specify the format.

## Comparison

| Feature | v1 | v2 |
|---------|----|----|
| Mendix version | < 10.18 | >= 10.18 |
| File structure | Single `.mpr` | `.mpr` + `mprcontents/` |
| Tables | `Unit`, `_MetaData` | `Unit`, `_MetaData`, `_Transaction` |
| Document storage | `Unit.Contents` blob in SQLite | Individual `.mxunit` files |
| Git friendliness | Poor (binary diffs) | Good (per-document files) |
| File size | Larger single file | Distributed across files |
| Read performance | Single DB query | File I/O per document |
| Write granularity | Full DB transaction | Per-file write |

## Writing Behavior

When writing with `OpenForWriting()`:

- **v1**: `UPDATE Unit SET Contents = ? WHERE UnitID = ?` — the blob goes into
  the row it belongs to
- **v2**: the BSON is written to a temp file and renamed into place at
  `mprcontents/<XX>/<YY>/<UUID>.mxunit`, then SQLite is updated with the new
  `Unit.ContentsHash` (base64 SHA-256 of the contents) and a fresh
  `_Transaction.LastTransactionID`

The writer handles both formats transparently.
