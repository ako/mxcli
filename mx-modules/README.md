# mx-modules

Marketplace module `.mpk`s the doctype integration gate imports (via
`mx module-import`) before running the examples that depend on them. The mapping
of script → module(s), and the per-Mendix-version selection, lives in
`scriptModuleDeps` in `mdl/executor/roundtrip_doctype_test.go`.

## Driver JARs are stripped to stay under GitHub's 100 MB file limit

`mx check` validates the **model** (Java action signatures, entities, microflows)
— it does not run the bundled JDBC drivers. Those drivers live in the `.mpk`'s
`vendorlib/` and can be huge: the pristine **External Database Connector 6.3.0**
is 106 MB, almost all of it a single `vendorlib/snowflake-jdbc-*.jar` (~100 MB),
which pushes the file over GitHub's hard 100 MB per-file limit.

So the bundled `ExternalDatabaseConnector-v6.3.0.mpk` here has that driver removed:

```bash
zip -d ExternalDatabaseConnector-v6.3.0.mpk "vendorlib/snowflake-jdbc-*.jar"
```

The slimmed module still imports and `mx check`s cleanly (verified on Mendix
11.12). **If you re-download this module from the marketplace, strip the large
`vendorlib/` driver JAR(s) again** before committing, or the push will be rejected
for exceeding the file-size limit. Keep the filename version suffix accurate to
the source release.

| MPK | Used for | Notes |
|-----|----------|-------|
| `ExternalDatabaseConnector-v6.2.3.mpk` | Mendix `..11.11` | full module (95 MB, under the limit) |
| `ExternalDatabaseConnector-v6.3.0.mpk` | Mendix `11.12+` | snowflake JDBC driver stripped (106 MB → 21 MB) |
| `BusinessEvents_3.12.0.mpk` | all versions | — |
