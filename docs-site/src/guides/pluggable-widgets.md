# Pluggable Widgets Across Mendix Versions

Modern Mendix UI is built from **pluggable widgets** — DataGrid 2, Combo box, Gallery,
the data-grid filters, and any widget you install from the Marketplace. mxcli can create
and configure these widgets in a page, and it does so in a way that stays correct as the
installed widget versions change from project to project. This guide explains how that
works and how to inspect what mxcli has discovered about a widget.

## The problem: "the definition of this widget has changed" (CE0463)

Every pluggable-widget instance stored in an `.mpr` embeds a copy of the widget's
**definition** — the full list of properties, their types, default values, captions,
categories, and declared order. When you open the app, Studio Pro (and `mx check`)
compares that embedded definition against the widget package (`.mpk`) actually installed
in the project. If they differ in any way, you get:

> **CE0463** — "The definition of this widget has changed. Update this widget by
> right-clicking it and selecting 'Update widget'…"

Different Mendix versions — and different Marketplace releases of the same widget — ship
slightly different definitions: a property is added, an enumeration option is renamed, a
category moves, the property order changes. A tool that emitted one fixed definition would
trip CE0463 the moment the project's widget version didn't match.

## How mxcli stays version-correct

mxcli never hard-codes one widget shape. It reconciles a known-good template against the
widget package **installed in your project**, so the definition it writes matches the
version you actually have — no `mx update-widgets` step required.

1. **Embedded template.** mxcli ships a known-good template for each built-in widget
   (extracted from Studio Pro). This provides the correct nested BSON structure that is
   hard to build from scratch.

2. **Reconcile against the project `.mpk`.** When you pass a project (`-p`), mxcli finds
   the widget's `.mpk` in the project's `widgets/` folder, parses its definition, and
   updates the template to match: it adds and removes properties, rewrites enumeration
   option sets, fixes categories/captions/defaults, fills in per-property metadata, and —
   importantly — **re-orders the properties to the package's declared order**, including
   the system properties (Label / Visibility / Editability) at their real position. This
   is exactly what Studio Pro's own "Update widget" does, derived generically from the
   package with no widget-specific code.

3. **Apply the widget's own dynamic rules.** Which properties a widget shows depends on
   its *configuration* — a Combo box in enumeration mode hides the association properties;
   a DataGrid 2 with selection off hides the selection labels. That logic lives in each
   widget's compiled `editorConfig.js`. mxcli statically extracts those **dynamic property
   rules** and applies them, so a freshly-created widget carries exactly the defaults
   Studio Pro would give it.

The result: widgets created by mxcli open cleanly across Mendix 10.x and 11.x with the
bundled widget packages, and against Marketplace-updated packages, without manual
"Update widgets" fix-ups.

> For the internal mechanics (template extraction, BSON cross-references, the augment
> pipeline), see [Widget Template System](../internals/widget-templates.md) and
> [Pluggable Widget Engine](../internals/widget-engine.md).

## Inspecting the discovered widget format

Two commands let you see exactly what mxcli knows about the widgets available to a project.

### List available widgets

```bash
mxcli widget list                 # built-in widget definitions
mxcli widget list -p app.mpr      # also loads project-specific definitions
```

### Describe one widget

`mxcli widget describe` shows a widget's **expected properties** and its **dynamic property
rules**. Name the widget by its MDL keyword (`COMBOBOX`, `DATAGRID2`, `GALLERY`, …) or its
full widget id.

```bash
# mxcli's built-in view of the widget:
mxcli widget describe COMBOBOX

# the version-accurate view from the package installed in the project:
mxcli widget describe COMBOBOX -p app.mpr

# machine-readable:
mxcli widget describe DATAGRID2 -p app.mpr --format json
```

With `-p`, the properties and rules come from the widget package actually installed in the
project (`widgets/*.mpk`) — the version-accurate "discovered" format, and the only way to
inspect a Marketplace widget mxcli has no built-in knowledge of. Without `-p`, they come
from mxcli's embedded template.

Example (abridged):

```text
Widget: Combo box (combobox)
  ID:      com.mendix.widget.web.combobox.Combobox
  Version: 2.5.0
  Kind:    pluggable
  Source:  project .mpk

Properties (58):
  source                 enumeration required default=context {context|database|static}  (General::Data source)
  optionsSourceType      enumeration required default=association {association|enumeration|boolean}  (General::Data source)
  attributeEnumeration   attribute required  (General::Data source)
  …
  selectAllButtonCaption textTemplate required  (General::Multiple-selection (reference set))
  Label                  system  [system]
  Visibility             system  [system]
  Editability            system  [system]
  customEditability      enumeration required default=default {default|never|conditionally}  (General::Editability)
  …

Dynamic property rules (10):
  itemSelectionMethod       hidden when itemSelection = "None"
  keepSelection             hidden when itemSelection ≠ "Multi"
  loadMoreButtonCaption     hidden when pagination ≠ "loadMore"
  …
  — 7 of 23 editor hide-rules recognized
```

The property list is in the widget's **declared order** (system properties appear where the
package declares them, not appended at the end), each with its type, whether it's required,
its default, enumeration options, and category. Object-list widgets (e.g. DataGrid 2
columns) show their nested item properties indented. The dynamic rules read as
"property *X* is hidden when *Y*", and the trailing coverage line reports how many of the
widget's editor hide-rules mxcli recognized.

### `--format json`

Both the properties and the rules are available as JSON for scripting:

```json
{
  "widgetId": "com.mendix.widget.web.combobox.Combobox",
  "version": "2.5.0",
  "source": "project .mpk",
  "properties": [
    { "key": "source", "type": "enumeration", "required": true, "default": "context",
      "enum": ["context", "database", "static"], "category": "General::Data source" }
  ],
  "dynamicRules": [
    { "property": "loadMoreButtonCaption", "hiddenWhen": "pagination ≠ \"loadMore\"" }
  ]
}
```

## Marketplace and custom widgets

`mxcli widget describe -p app.mpr <widget-id>` works for **any** widget installed in the
project, including Marketplace and custom widgets mxcli has no embedded template for — it
reads the definition and editor rules straight from the installed `.mpk`. To teach the
page builder to author a custom widget from MDL, extract a definition for it first:

```bash
mxcli widget extract --mpk widgets/MyWidget.mpk    # writes a .def.json
mxcli widget init -p app.mpr                        # extract defs for all project widgets
```

See [Marketplace Content](marketplace.md) for downloading and installing packages.
