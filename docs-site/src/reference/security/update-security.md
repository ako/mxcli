# UPDATE SECURITY

Bring entity access rules back into sync with their domain model — the headless
equivalent of Studio Pro's **Update security** button in the domain model editor.

## Syntax

```sql
UPDATE SECURITY;
UPDATE SECURITY <module>;
UPDATE SECURITY IN <module>;
```

With no module, every module in the project is reconciled. The two scoped forms
are the same thing; `IN` is optional.

## What it does

An access rule names the members it governs. When the entity gains an attribute
or an association, every rule on it needs an entry for the new member — and when
a member goes, its entries have to go with it. A rule that has fallen behind is
what Mendix reports as:

```
[error] [CE0066] "Entity access is out of date. Please update security by
        clicking the 'Update security' button in the domain model editor."
        at Domain model of module 'MyModule'
```

`UPDATE SECURITY` adds what is missing, removes what is stale, and downgrades
write rights on calculated attributes. It reports what it changed:

```
Reconciled 3 access rule(s) in module Administration
```

A project whose rules already cover their entities is **not written to** — the
command says `All entity access rules are up to date` and leaves the model alone.

`System` is skipped: its entities are the platform's and its access rules are
not the project's to rewrite. Naming it explicitly is an error rather than a
silent no-op.

## When you need it

Rarely, for models mxcli writes. Every mxcli write path reconciles as it writes —
`GRANT`, `ALTER ENTITY`, `CREATE ASSOCIATION`, and the finalize step after any
script — so an MDL script leaves the rules correct.

It is for a model that arrived from somewhere else:

- a module imported or updated **outside Studio Pro**, whose author never pressed
  Update security, so the package ships rules that do not cover every member;
- a hand-edited or merged `.mpr`;
- a project last touched by a Mendix version whose conversion added members.

`mxcli marketplace install` and `mxcli marketplace update` run this for the module
they copy in, and report the count — a transplant copies the incoming units
verbatim, so without it the package's rules reach the project unchecked
(mendixlabs/mxcli#1085).

## Examples

```sql
-- After a headless module install or update
UPDATE SECURITY UserCommons;

-- Every module in the project
UPDATE SECURITY;
```

From the shell, without writing a script:

```bash
mxcli -p app.mpr -c "update security UserCommons"
```

## Notes

- Reconciling a marketplace module's rules changes that module, so it shows up in
  `mxcli diff-local` and `mxcli marketplace diff` reads it as a local edit. That
  is unavoidable — it is the same change Studio Pro's button makes — and it only
  happens when the rules were genuinely incomplete.
- The rules of an entity whose generalization lives in **another module** cannot
  be fully checked here: those inherited members are neither added nor removed,
  but existing entries for them are preserved rather than pruned.

## See also

- [GRANT](grant.md) — author entity access rules
- [REVOKE](revoke.md) — remove them
- [Error messages](../../appendixes/error-messages.md) — CE0066
