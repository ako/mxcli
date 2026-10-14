# `GUID` is the database's identity — method and results

Measurement for
[`PROPOSAL_marketplace_module_upgrade.md` §8](../../PROPOSAL_marketplace_module_upgrade.md),
run 2026-08-11 on Mendix **11.12.1**.

The question: §4 showed Studio Pro's module update renumbers every `$ID` and
preserves every `GUID`. That makes `GUID` the only candidate carrier of database
identity — but it is inference until something is measured against a real
database.

## Why this is measurable here and not in Studio Pro

The lever is a single BSON value in the stored model, and mxcli can write one
where Studio Pro offers no such control. `GUID` is safe to edit in isolation
because **nothing points at it** — unlike an `$ID`, which is a pointer target and
cannot be rewritten without rewriting every reference in the same pass
(ADR-0008).

## Setup

```bash
mxcli run --local --setup --ensure-db -p E2E.mpr   # local PostgreSQL + runtime
mxcli run --local -p E2E.mpr                       # boot; runtime creates the schema
```

Subject: a blank 11.12.1 app, `Administration` 4.3.2, entity `Account`
(persistent, so it gets a table).

## The runtime writes its own identity map

```sql
select id, entity_name, table_name from mendixsystem$entity
 where entity_name = 'Administration.Account';
-- b16e49ea-91df-4caa-aed8-6ba4c4e133c5 | Administration.Account | administration$account

select a.id, a.attribute_name from mendixsystem$attribute a
  join mendixsystem$entity e on a.entity_id = e.id
 where e.entity_name = 'Administration.Account';
-- aac00d66-7cc1-4def-a8d6-8b81fa1f5477 | FullName
-- f9c5f2aa-6ab9-4e62-9cbc-950405335377 | Email
```

Those are the model's own `GUID`s. `Account` stores the bytes
`ea 49 6e b1 df 91 aa 4c ae d8 6b a4 c4 e1 33 c5`; undoing the .NET field order
(first three groups little-endian) gives `b16e49ea-91df-4caa-aed8-6ba4c4e133c5`.
`FullName` decodes to `aac00d66-…` the same way. **Byte-identical, not
correlated.**

Note `b16e49ea…` is also the `GUID` recorded in §4 — a different project, a
different Mendix version. The `GUID` belongs to the *published module*.

## Runs

| Run | Model change | `mendixsystem$entity.id` | `administration$account` |
|---|---|---|---|
| 1 | — (baseline) | `b16e49ea-…` | 1 row inserted |
| 2 | `GUID` → `a0a1a2a3…` | `a3a2a1a0-…` | **0 rows** |
| 3 | `GUID` restored | `b16e49ea-…` | table recreated, empty |
| 4 | none (**control**) | `b16e49ea-…` | **row survives** |

Run 2 changed nothing else: same entity name, same table name, same attributes.
Run 4 is what makes run 2 readable — an unchanged reboot preserves the row, so
the loss came from the identity change and not from restarting.

## Conclusion

The database keys entities *and their attributes* on the model's `GUID`. Studio
Pro's update preserves exactly that, so `$ID` renumbering is irrelevant to data
safety, and a `GUID`-preserving replace is data-safe at the level of element
identity.

## What this does not establish

- **Not that the upgrade is harmless.** An element the new version deletes still
  loses its column or table. That is a schema decision, not an identity failure —
  and §4's silently destroyed local edit is untouched by any of this.
- **Not the DDL text.** The runtime logs the command count (596 cold, 38 on the
  identity change) but not the statements at INFO level. The *outcome* was
  measured instead, which answers the question asked more directly than the
  statements would.
- **Not associations.** Association `GUID`s were observed in the model but their
  join-table identity was not exercised.

## Associations — measured 2026-10-14

Run on Mendix **11.6.6** (the vendored `testdata/expr-checker` app, PostgreSQL,
`mxcli run --local`), closing the gap above. Subject: a cross-module
association `MyFirstModule.Config_Account` from a local `Config` entity to
`Administration.Account` (Reference, column storage), with its `GUID` set to
`15 02 15 02 aa bb cc dd 11 22 33 44 55 66 77 88` — distinct from its `$ID`, as
a Studio Pro-authored element's is (an mxcli-created one has `GUID == $ID`,
which would hide a re-mint).

The runtime keys it the same way as an entity:

```sql
select id, table_name, child_column_name from mendixsystem$association
 where association_name = 'MyFirstModule.Config_Account';
-- 02150215-bbaa-ddcc-1122-334455667788 | myfirstmodule$config | myfirstmodule$config_account
```

— the model's `GUID` with the .NET field order undone. The column carries a real
foreign key (`frn_myfirstmodule$config_myfirstmodule$config_account`), so the
seed is a real `Account` (4242) and a `Config` row linking to it.

| Run | Model change | `mendixsystem$association.id` | `config_account` of the seeded row |
|---|---|---|---|
| 1 | — (baseline, seeded) | `02150215-…` | 4242 |
| 2 | none (**control**) | `02150215-…` | **4242** |
| 3 | `marketplace update` of Administration, association carried across (`$ID` and `GUID` kept) | `02150215-…` | **4242** |
| 4 | association dropped and re-created from MDL — what replaying `--save-edits` output does | `64fef841-…` (new `GUID`) | **NULL** |

Run 4 keeps the `Config` row and loses the link: the runtime treats the
re-created association as a new one. So an association's `GUID` is its
database identity exactly as an entity's is, and re-creating one from MDL is
not a rescue for its data. This is why `PerformUpdate` puts back the stored
element (`RestoreDependentAssociations`) instead of relying on a replay.
