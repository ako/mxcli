---
title: Microflow Download File Statement
status: draft
---

# Microflow Download File Statement

Status: Draft

## Summary

Add MDL syntax for Mendix `Microflows$DownloadFileAction`:

```mdl
download file $GeneratedReport;
download file $GeneratedReport show in browser;
```

The feature is intentionally small: it exposes an existing Studio Pro microflow
activity without adding new semantics.

## Motivation

Projects that already contain download-file actions should survive
describe/exec/describe without losing the activity or falling back to an
unsupported-action comment. The statement also gives users a straightforward way
to author file downloads when the file document variable already exists.

## Syntax

```antlr
downloadFileStatement
    : DOWNLOAD FILE_KW VARIABLE (SHOW IN BROWSER)? onErrorClause? SEMICOLON
    ;
```

Examples:

```mdl
download file $GeneratedExport;
download file $GeneratedReport show in browser on error rollback;
```

## Semantics

- The operand must be a variable containing a `System.FileDocument`.
- `show in browser` maps to Studio Pro's `ShowInBrowser` flag.
- Error handling follows the normal microflow action `on error ...` forms.

## Tests And Examples

- Parser/visitor coverage for both normal and `show in browser` forms.
- Builder/writer coverage for `DownloadFileAction`.
- Example script: `mdl-examples/bug-tests/333-microflow-download-file.mdl`.
