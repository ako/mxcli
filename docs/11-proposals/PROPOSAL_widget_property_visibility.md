---
title: Widget Property Conditional Visibility
status: draft
---

# Widget Property Conditional Visibility

Status: Draft

## Summary

Drive `TextTemplate` (and other conditional) widget property serialization
based on per-property visibility rules sourced from each pluggable widget's
`editorConfig.js`. When a property is hidden under the widget's current
configuration (e.g. `type === 'expression'`), emit `TextTemplate: null`; when
visible, emit the populated default `Forms$ClientTemplate`. Eliminates CE0463
for widgets whose active property set depends on enum/boolean toggles.

## Motivation

Two CE0463 cases observed against Mendix 11.9 `mx check`:

| Page | Widget | Symptom |
|---|---|---|
| `PW.P_PW02_Properties` | VideoPlayer `video1` with `type: 'expression'` | We emit populated `ClientTemplate` for `videoUrl` and `posterUrl` (hidden when `type='expression'`). Studio Pro's *Update widget* sets both to `null`. |
| `PW.P_PW08_MultiSlot` | Timeline `timelineCustom1` with `customVisualization: true` | We emit `TextTemplate: null` for `title`, `description`, `timeIndication` (always visible). Studio Pro's *Update widget* populates them with an empty `ClientTemplate`. |

Both fail with `CE0463 "definition of this widget has changed"` until the user
right-clicks → *Update widget*. The two cases are opposite directions but stem
from the same gap: the serializer has no way to know whether a property is
*active* in the widget's current configuration.

The condition isn't declared anywhere mxcli currently reads. `widget.xml` carries
only `<property key="..." type="..." />` declarations; the visibility rules live
in the widget's compiled `editorConfig.js` as imperative JavaScript:

```js
exports.getProperties = function (e, t, r) {
  "dynamic" === e.type     && F.hidePropertiesIn(t, e, ["urlExpression", "posterExpression"]);
  "expression" === e.type  && F.hidePropertiesIn(t, e, ["videoUrl", "posterUrl"]);
  "aspectRatio" === e.heightUnit
    ? F.hidePropertyIn(t, e, "height")
    : F.hidePropertyIn(t, e, "heightAspectRatio");
  ...
};
```

`hidePropertiesIn` and `hidePropertyIn` come from Mendix's published
`PluggableWidgetUtils` helper (`{hidePropertyIn, hidePropertiesIn,
hideNestedPropertiesIn, changePropertyIn}`). Roughly 80% of pluggable widgets
on the marketplace follow this pattern.

## Proposed approach

Add a static-analysis pass over `editorConfig.js` that extracts visibility
rules into a structured `propertyVisibility[]` field in each widget's
`.def.json`. At BSON serialization time, evaluate each rule against the
widget's current property values and decide whether to emit the populated
`ClientTemplate` or `null`.

### `.def.json` schema extension

```json
{
  "widgetId": "com.mendix.widget.web.videoplayer.VideoPlayer",
  "mdlName": "VIDEOPLAYER",
  "propertyMappings": [...],
  "propertyVisibility": [
    {
      "propertyKey": "urlExpression",
      "hiddenWhen": { "propertyKey": "type", "operator": "eq", "value": "dynamic" }
    },
    {
      "propertyKey": "videoUrl",
      "hiddenWhen": { "propertyKey": "type", "operator": "eq", "value": "expression" }
    },
    {
      "propertyKey": "height",
      "hiddenWhen": { "propertyKey": "heightUnit", "operator": "eq", "value": "aspectRatio" }
    },
    {
      "propertyKey": "heightAspectRatio",
      "hiddenWhen": { "propertyKey": "heightUnit", "operator": "ne", "value": "aspectRatio" }
    }
  ]
}
```

The rule grammar covers the dominant patterns observed in marketplace widgets:

- `eq` / `ne` against a primitive value
- `truthy` (property is set / non-empty)
- `falsy` (property unset / empty / false)
- `and` / `or` (compose two or more sub-conditions)

Rules that don't fit the grammar (custom helpers, nested array logic, runtime
data-dependent checks) are emitted as `"hiddenWhen": "unsupported"` and trigger
a fall-back to existing behavior.

### Static JS extraction

A new package `sdk/widgets/visibility/` houses the analyzer:

- `parser.go` — uses a third-party JS AST library (e.g.
  [`github.com/dop251/goja`](https://github.com/dop251/goja) parser, headless
  parse-only mode) to produce an AST from the minified
  `*.editorConfig.js`.
- `extract.go` — pattern-matches three known helpers
  (`hidePropertyIn`, `hidePropertiesIn`, `hideNestedPropertiesIn`) inside
  `exports.getProperties = function (e, t, r) { ... }`. Walks the
  conditional expression in front of each call and emits a structured
  `VisibilityCondition`.
- Recognized AST patterns:
  - `BinaryExpr {Left:Literal, Op:===, Right:MemberExpr(e.<key>)} && CallExpr(hidePropertyIn|hidePropertiesIn)`
  - `MemberExpr(e.<key>) === Literal && ...` (mirror order)
  - `MemberExpr(e.<key>) && ...` → truthy
  - `!MemberExpr(e.<key>) && ...` → falsy
  - Ternary: `cond ? hideA : hideB`
  - Top-level `&&` chains of the above
- Unsupported nodes are logged at `widget init` time and produce no rule.

### Runtime evaluation

`sdk/widgets/visibility/eval.go`:

```go
func IsHidden(def *WidgetDefinition, propKey string, values map[string]any) bool
```

The serializer calls `IsHidden` for each property; if `true`, emit
`TextTemplate: null`; if `false` and the property's `Type` is
`TextTemplate`, emit a populated default `Forms$ClientTemplate` (from the
embedded template's `Translations` defaults).

### Wiring

1. **Extraction** — invoke during `mxcli widget init`
   (`RefreshWidgetDefinitions`) and during build-time embedded-template
   regeneration. Stored under `propertyVisibility[]` in
   `<projectDir>/.mxcli/widgets/<widget>.def.json` and
   `sdk/widgets/definitions/<widget>.def.json`.
2. **Loading** — `WidgetDefinition` (in `mdl/executor/widget_engine.go`)
   gains a new field `PropertyVisibility []VisibilityRule`. Backward-
   compatible: existing `.def.json` files without the field continue to
   produce the current behavior.
3. **Serialization** — `serializeWidgetValueForRawType`
   (`sdk/mpr/writer_widgets_custom.go:160`) gains a definition-aware
   `TextTemplate` decision. Same for `augment.go`'s
   `createDefaultWidgetValue`.
4. **MPR write path** — `mdl/backend/mpr/widget_builder.go`
   `createDefaultWidgetValue` (the path fixed in `4ea402c2` for
   object-list items) extends to also consult visibility rules at the
   top-level property layer.

## Affected widgets

A sample sweep of vanilla Mendix 11.9 widget `editorConfig.js` files using
`grep "hidePropertyIn\|hidePropertiesIn"`:

| Widget | # rules | Notable triggers |
|---|---:|---|
| VideoPlayer | 2 | `type`, `heightUnit` |
| Datagrid | 8+ | `pagingEnabled`, `selectionMode`, `columnsFilterable` |
| Gallery | 6+ | `selectionMode`, `pagination` |
| DatagridTextFilter | 3 | `defaultValue` (truthy) |
| Maps | 4+ | `mapProvider`, `markerSourceType` |
| Accordion | 2 | `expandBehavior` |
| Tooltip | 3+ | `renderMethod` |
| Timeline | 0 (always visible) | — |

Timeline's CE0463 isn't caused by hidden properties — it's caused by the
serializer hardcoding `TextTemplate: nil` even for always-visible TextTemplate
properties. The same proposal addresses both directions:

- "Hidden" properties → `null`
- "Visible and TextTemplate-typed and unset" → populated empty `ClientTemplate`
  from the schema defaults
- "Visible and other-type" → primitive/expression/attribute as today

## Backward compatibility

- `.def.json` without `propertyVisibility` → serializer behaves as today.
  Existing fixtures unaffected.
- A new `propertyVisibility: "unsupported"` value lets the extractor signal
  "this widget has rules we couldn't parse" without breaking serialization.
  The serializer treats unsupported as `false` (assume visible) for safety.
- `mxcli widget init` reruns extraction whenever the .mpk version changes
  (same drift-detection path as the existing `.def.json` refresh).

## Tests and examples

- Unit tests for the JS extractor: feed minified VideoPlayer / Datagrid /
  Gallery / Timeline `editorConfig.js` fixtures, assert expected
  `VisibilityCondition` output.
- Unit tests for `IsHidden(values)` covering each operator (eq, ne, truthy,
  falsy, and, or).
- Integration tests: existing roundtrip pages
  `mdl-examples/doctype-tests/30-pluggable-widget-examples.mdl` and
  `32-pluggable-widget-object-lists-v010.mdl` extended with
  `PW02_Properties` (`type='expression'`) and `PW08_MultiSlot`
  (`customVisualization='true'`) variants. Both must pass `mx check` with
  zero CE0463 against vanilla Mendix 11.9.
- Reference fixtures `mx-test-projects/test5-app` `P_PW02_Properties_2`
  and `P_PW08_MultiSlot_2` (Studio Pro–updated copies of the failing pages)
  used as BSON baselines for the diff harness.

## Open questions

- **JS parser dependency** — Adding goja (or a Go-native JS parser) is a
  non-trivial dependency. Alternative: hand-roll a minimal AST walker for the
  subset of patterns above. Same code complexity, no third-party dep.
- **`editorConfig.js` minification variability** — Different Mendix widget
  versions ship with different minifier output. The extractor needs to be
  tolerant of identifier renaming (`e`/`t`/`r` are not stable). Mitigation:
  match on the call-site shape `hidePropertyIn(<args>)` rather than identifier
  names, and identify `e.<key>` member expressions by their position in the
  call chain.
- **Conditional in nested objects/lists** — `hideNestedPropertiesIn` walks
  into object-list items. Pattern extraction is the same, but the runtime
  eval needs to know it's evaluating per-item. Defer to a follow-up: this
  proposal covers top-level properties only; nested rules are recorded but
  not yet evaluated.
- **Non-equality conditions** — A small minority of widgets use string regex,
  arithmetic, or runtime data lookups. Recommend marking as `unsupported` and
  documenting per-widget exceptions in `sdk/widgets/visibility/README.md`.
- **`changePropertyIn`** — Beyond hide/show, some widgets *change* a property's
  caption or required-ness via the same helper module. Out of scope; documented
  as a follow-up.

## Phasing

1. **Phase 1 — Datapath (no extractor)**: hand-author
   `propertyVisibility[]` for VideoPlayer + Timeline as a proof-of-concept.
   Wire serializer through. Verify CE0463 eliminated on `test5-app`.
2. **Phase 2 — Extractor**: build JS AST walker. Regenerate
   `propertyVisibility[]` for all `sdk/widgets/definitions/*.def.json` from
   their bundled `.mpk` `editorConfig.js`. Add the
   `mdl-examples/doctype-tests/30-…/32-…` `type='expression'` /
   `customVisualization='true'` variants.
3. **Phase 3 — Object-list rules**: extend visibility to
   `hideNestedPropertiesIn`, used by Accordion groups and DataGrid columns.

Phase 1 alone closes the two reported CE0463 cases.
