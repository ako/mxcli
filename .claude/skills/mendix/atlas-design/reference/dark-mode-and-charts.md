# Charts, dark mode and widget overrides

Supporting reference for [atlas-design](../SKILL.md).

## Charts — a dataviz-grade theme for the Mendix chart widgets

Out of the box the chart widgets (Column / Bar / Area / Pie / Line) render **raw
Plotly defaults**: one flat colour, a floating mode-bar, wide margins, heavy
gridlines, a white paper background. That is the single biggest "not a real product"
tell. Three Plotly hooks — barely used by generated apps — turn them into designed
charts. All three are **plain JSON strings** (no Mendix expression quoting).

| Property | Plotly layer | Use it for |
|---|---|---|
| `customLayout` | `layout` | transparent `paper_bgcolor` + `plot_bgcolor`, system font, `#8a94a6` ticks, tight `margin`, faint `gridcolor`, `zeroline:false` / `showline:false`, dark `hoverlabel` |
| `customConfigurations` | `config` | `{"displayModeBar":false,"responsive":true}` — removes the floating toolbar |
| `customSeriesOptions` (per series; chart-level on Pie) | trace | brand colour, `marker.cornerradius` (rounded bars), `line.shape:"spline"` + translucent `fillcolor` (area), Pie colour array + white inside labels |

**The key trick — transparent background = theme-agnostic charts.** Set
`paper_bgcolor` and `plot_bgcolor` to `rgba(0,0,0,0)`; the plot inherits whatever
panel it sits on, so **one config is correct in both light and dark** with zero
per-theme CSS. Pair it with a neutral tick colour (`#8a94a6`) that reads on either
background. Always kill the white paper **and** the mode-bar — the two ugliest
defaults.

Ready-made `customLayout` (transparent, themed):
```json
{
  "paper_bgcolor": "rgba(0,0,0,0)",
  "plot_bgcolor": "rgba(0,0,0,0)",
  "font": { "family": "system-ui, -apple-system, 'Segoe UI', sans-serif", "color": "#8a94a6" },
  "margin": { "t": 8, "r": 8, "b": 32, "l": 40 },
  "xaxis": { "gridcolor": "rgba(138,148,166,0.15)", "zeroline": false, "showline": false },
  "yaxis": { "gridcolor": "rgba(138,148,166,0.15)", "zeroline": false, "showline": false },
  "hoverlabel": { "bgcolor": "#1a2129", "font": { "color": "#ffffff" } }
}
```

`customConfigurations` (kill the mode-bar): `{ "displayModeBar": false, "responsive": true }`

`customSeriesOptions` per type:
```jsonc
// Column / Bar — brand colour + rounded corners
{ "marker": { "color": "#2b5170", "cornerradius": 6 } }
// Area — spline curve + translucent fill
{ "line": { "shape": "spline", "color": "#2b5170" }, "fill": "tozeroy", "fillcolor": "rgba(43,81,112,0.15)" }
// Pie (chart-level) — colour array + white inside labels
{ "marker": { "colors": ["#2b5170", "#4a7a5c", "#c9a227", "#a13a2c"] }, "insidetextfont": { "color": "#ffffff" } }
```

Swap the hex values for your brand palette (the same values you set in the Layer-1
scaffold). The generic `dataviz` skill is the HTML/React analogue of this — same
"kill the defaults, one theme-agnostic config, brand the series" philosophy.

**Chart gotchas** are in the [gotchas catalog](#gotchas-catalog). All chart types
(incl. Line/Bubble/Heatmap/TimeSeries) are MDL-authorable today — see
`mdl-examples/doctype-tests/34-chart-widget-examples.mdl` and `custom-widgets`.

---
## Optional dark-mode Atlas-widget overrides

Paste into `main.scss` (Layer 2), after the `@import`s. Replace the token
placeholders with your dark palette. Popovers/modals render at `<body>`, so the
popover + modal block must **not** be scoped to your app class — keep it global.

```scss
// --- Dark palette tokens (TODO: set these) ----------------------------------
$dk-surface: #1a2129;   // panel / row background
$dk-surface-2: #232c37; // header / chip background
$dk-ink:     #e6ebf1;   // primary text
$dk-ink-mut: #9aa6b4;   // muted text
$dk-border:  #2f3a47;   // hairline

// Wrap in the media query for a dual-theme app; DELETE the @media line (and its
// closing brace) for a committed dark-only app to make these unconditional.
@media (prefers-color-scheme: dark) {

  // Form controls: text input / textarea / combobox field
  .form-control,
  .mx-textarea textarea,
  .form-control input {
    background: $dk-surface; color: $dk-ink; border-color: $dk-border;
  }

  // Datagrid: rows, headers, filter chips
  .mx-datagrid table, .mx-datagrid tr, .mx-datagrid th, .mx-datagrid td {
    background: $dk-surface; color: $dk-ink; border-color: $dk-border;
  }
  .filter-selector-button {
    background: $dk-surface-2; color: $dk-ink; border-color: $dk-border;
  }

  // Datagrid dropdown filter: kill the hardcoded white scroll-fade gradient
  .widget-dropdown-filter-menu {
    background-image: none; background-color: $dk-surface;
  }
  .widget-dropdown-filter-menu * { color: $dk-ink; }

  // Accordion / Fieldset
  .mx-groupbox, .mx-groupbox-header, fieldset, legend {
    background: $dk-surface; color: $dk-ink; border-color: $dk-border;
  }

  // TreeNode: expanded child rows carry a WHITE card bg — let the panel show through
  .mx-treenode, .mx-treenode .mx-treenode-content {
    background: transparent; color: $dk-ink;
  }
}

// Popovers / modals render at <body> — theme these GLOBALLY (unscoped).
// Combobox / tooltip / dropdown-filter popovers and edit popups (.mx-window /
// .modal-content) live outside your app class, so a scoped selector misses them.
.mx-window-content, .modal-content, .mx-window-header, .mx-tooltip, .mx-combobox-menu {
  background: $dk-surface; color: $dk-ink; border-color: $dk-border;
}
.mx-window-content .form-control, .modal-content .form-control {
  background: $dk-surface-2; color: $dk-ink; border-color: $dk-border;
}
.mx-window .btn-default, .modal-content .btn-default {
  background: $dk-surface-2; color: $dk-ink; border-color: $dk-border;
}
// Charts: DON'T style them here — use the transparent customLayout (above).
```

---
