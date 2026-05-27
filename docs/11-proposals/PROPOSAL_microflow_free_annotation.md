---
title: Microflow Free Annotation
status: draft
---

# Microflow Free Annotation

Status: Draft

## Summary

Add explicit round-trip support for free-floating microflow annotations that are
serialized next to an activity in MDL but are not visually attached to that
activity in the Mendix model.

## Motivation

Mendix stores some visual notes as standalone annotations. During `describe`,
those notes can appear immediately before an activity because the activity is
the next stable textual anchor. If the parser treats every preceding
`@annotation` as activity metadata, `exec` rewrites the note into an attached
annotation flow and changes the diagram.

## Semantics

`@annotation 'text'` is treated as a free annotation when both conditions hold:

1. It appears before any activity-binding metadata for the same statement.
2. A later activity-binding annotation follows before the activity statement.

Activity-binding metadata is currently `@position`, `@caption`, `@color`,
`@excluded`, or `@anchor`.

Example:

```mdl
@annotation 'section header'
@position(100, 200)
log info node 'Audit' 'starting';
```

The note remains free-floating. By contrast, an annotation after `@position` is
still attached to the activity:

```mdl
@position(100, 200)
@annotation 'activity note'
log info node 'Audit' 'starting';
```

## Tests And Examples

- `mdl-examples/bug-tests/319-microflow-free-annotation.mdl` documents the
  supported syntax.
- Parser tests cover both order-sensitive cases.
- Builder tests verify that the free annotation is emitted as a standalone
  annotation and not attached to the activity.

## Open Questions

- Should free annotation binding use textual order only, or should it also
  consider visual proximity in the microflow diagram?
- Should MDL grow an explicit keyword for free annotations to avoid relying on
  order-sensitive disambiguation?
