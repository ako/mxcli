---
title: Built-in file database for run --local (HSQLDB)
status: proposed
date: 2026-09-21
---

# Proposal: Built-in file database for `run --local` (HSQLDB)

**Status:** Proposed
**Date:** 2026-09-21
**Depends on:** the Windows process-helper fix (PR #1147) — this builds on that branch.

`mxcli run --local` can only boot against PostgreSQL: `runlocal.go` hard-codes
`DatabaseType = "PostgreSQL"`, there is no `--db-type`, a TCP reachability check
runs before boot, and `--ensure-db` refuses anything else. That makes the tool
unusable without a database server, which is exactly the friction when handing a
build to a customer who only wants to click a `.exe` and see their app.

Mendix ships an embedded, file-based database (HSQLDB) with the runtime, and
Studio Pro's own local run defaults to it. This proposal exposes it through
`mxcli run --local` so an app can boot with **no external database at all**, its
data in a local file.

## Research findings

Everything below was read out of the Mendix 11.12.2 runtime on this machine, not
assumed.

1. **The runtime supports it.** `runtime/pad/etc/example.conf` lists `HSQLDB` as a
   valid `DatabaseType` (`HSQLDB, MYSQL, ORACLE, POSTGRESQL, SAPHANA, SQLSERVER`),
   and documents `DatabaseJdbcUrl` as a value that *"overrides the other database
   connection settings"*.
2. **The driver is bundled.** `runtime/bundles/org.hsqldb.hsqldb.2.7.4.jar` ships
   with the runtime; no extra download is needed.
3. **The runtime builds the file URL itself.** Decompiling
   `com.mendix.datastorage-connectionbus.jar`:
   - `HsqldbDataStoreConfigurator` contains `jdbc:hsqldb:file:`,
     `builtInDatabasePath`, `hsqldb/`, `ifexists=true`, `databaseName`;
   - `MainDatabaseConfiguration` contains `data/database`.
   So with only `DatabaseType=HSQLDB` + `DatabaseName=<name>` (and empty host /
   user / password), the runtime derives
   `jdbc:hsqldb:file:<BasePath>/data/database/hsqldb/<name>;ifexists=true`.
   `BasePath` is the deployment directory `run --local` already sets.
4. **Mendix marks it development-only.** `HSQLDB.yaml.hbs` states the setup is
   *"intended for local testing only"*. This feature is for local dev / demo, not
   production — and the docs must say so.
5. **mxcli's current behaviour.** `runlocal.go` defaults `o.DB.Type` to
   `"PostgreSQL"`; `cmd_run.go` builds `DBConfig` from flags and never sets
   `Type`; the pre-boot `pingTCP` and `EnsureDatabase` are PostgreSQL-only.

## Design

### CLI

- New flag on `run`: `--db-type postgresql|hsqldb` (case-insensitive), default
  `postgresql`. `postgresql` keeps today's behaviour exactly.
- With `--db-type hsqldb`:
  - `--db-host`, `--db-user`, `--db-password` are refused (an explicit value is a
    user error, not silently ignored) with a message naming the flag.
  - `--db-name` is honoured (the HSQLDB file base name); default stays the
    project-derived name.
  - `--ensure-db` is refused: there is nothing to provision.

### Boot config

`DBConfig.Type` is taken from the flag instead of being forced to PostgreSQL.
For HSQLDB the runtime config carries `DatabaseType="HSQLDB"`, `DatabaseHost=""`,
`DatabaseUserName=""`, `DatabasePassword=""`, `DatabaseName=<name>`. `mxcli`
does **not** set `DatabaseJdbcUrl`, so the runtime's own path rule applies.

### Pre-boot reachability

The `pingTCP` check is skipped for HSQLDB (there is no host:port). For PostgreSQL
it is unchanged.

### Where the data lands

By the runtime's own rule the files go to
`<project>/deployment/data/database/hsqldb/<name>.*`. mxcli keeps that default
(least surprise, matches Studio Pro) and documents it; no `DatabaseJdbcUrl` is
set and no `--db-path` flag is added.

Accepted consequence: `deployment/` is a build output, so the database lives with
the rest of the local run artifacts. This is fine for the feature's purpose — a
local, disposable dev/demo database — and is stated in the docs. Anyone who needs
durable data uses a real database.

## Testing

Unit (any OS, no runtime needed):

- `DBConfig` → runtime config mapping: HSQLDB yields empty host/user/password and
  the right type; PostgreSQL is byte-for-byte unchanged.
- flag parsing: `--db-type hsqldb` (both cases), an invalid value is refused,
  and `hsqldb` + `--db-host` / `--ensure-db` are refused.
- the reachability check is skipped for HSQLDB.

Integration (Linux, reuses the existing `make test-integration` mxbuild + runtime
cache):

- boot a blank project with `run --local --db-type hsqldb`; assert HTTP 200;
- assert the HSQLDB files exist under
  `<project>/deployment/data/database/hsqldb/`.

Windows: the package must compile and the unit tests must pass (the existing
`windows-process-regression` job pattern); a full Windows boot integration is
optional and may be added as its own job.

## Scope

- In: `postgresql` and `hsqldb` only. Other Mendix database types are out of
  scope.
- Out: production use, multi-node, `--ensure-db` for HSQLDB, `--db-jdbc-url`
  passthrough.

## Rollout

- One PR on top of #1147: `feat: run --local with the built-in HSQLDB database`.
- CHANGELOG entry under `[Unreleased] / Added`.
- Docs: `--db-type` in the `run` help; a note that HSQLDB is for local testing
  only and where its files live.
