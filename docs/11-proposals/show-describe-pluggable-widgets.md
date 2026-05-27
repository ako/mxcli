---
title: SHOW/DESCRIBE Support for Missing Pluggable (React) Widgets
status: proposed
---

# Proposal: SHOW/DESCRIBE Support for Missing Pluggable (React) Widgets

## Overview

**Scope:** Improve DESCRIBE PAGE/SNIPPET output for pluggable widgets that currently fall through to generic formatting
**Priority:** High — pluggable widgets account for 2,711 instances across the 3 test projects but only 7 of ~30+ widget types get detailed MDL output

## Current State

### What works today (7 widget types with detailed DESCRIBE)

| MDL Keyword | Widget ID | Properties Extracted |
|-------------|-----------|---------------------|
| `combobox` | `com.mendix.widget.web.combobox.Combobox` | Attribute, DataSource (association mode), CaptionAttribute |
| `DATAGRID2` | `com.mendix.widget.web.datagrid.Datagrid` | DataSource, XPath, Sort, Columns (attribute, alignment, sortable, resizable), ControlBar, Selection, Paging |
| `gallery` | `com.mendix.widget.web.gallery.Gallery` | DataSource, Selection, Filter widgets, Content template |
| `textfilter` | `...datagridtextfilter.DatagridTextFilter` | Attributes, FilterType |
| `numberfilter` | `...datagridnumberfilter.DatagridNumberFilter` | Attributes, FilterType |
| `dropdownfilter` | `...datagriddropdownfilter.DatagridDropdownFilter` | Attributes, FilterType |
| `datefilter` | `...datagriddatefilter.DatagridDateFilter` | Attributes, FilterType |

### What falls through to generic formatting

The `else` branch in `cmd_pages_describe_output.go` (line 424) handles all other CustomWidgets. It extracts the last segment of the widget ID, uppercases it, and shows only:
- `label` (from Caption)
- `attribute` (from Content)
- Appearance (Class/Style)

This means widgets like Image, Tooltip, Charts, Badge, etc. appear in DESCRIBE output but lose all their meaningful properties.

## Pluggable Widgets Found in Test Projects

Analysis of 3 real Mendix projects (EnquiriesManagement, Evora-FactoryManagement, LatoProductInventory):

### Tier 1 — High usage, Mendix-core widgets (should get dedicated formatting)

| Widget | Total Count | Widget ID | Key Properties to Extract |
|--------|-------------|-----------|--------------------------|
| **Image** | 784 | `com.mendix.widget.web.image.Image` | DataSource (static/dynamic), ImageUrl, DefaultImage, Width, Height, OnClick action, AlternativeText |
| **Tooltip** | 172 | `com.mendix.widget.web.tooltip.Tooltip` | Content (child widgets), Position (top/bottom/left/right), TriggerOn (hover/click), RenderMethod |
| **Badge** | 80 | `com.mendix.widget.native.badge.Badge` + `...custom.badge.Badge` | Value (expression/attribute), Type (badge/label), OnClick action |
| **PopupMenu** | 24 | `com.mendix.widget.web.popupmenu.PopupMenu` | Trigger widget, Menu items (caption, action, icon), Position |
| **Events** | 21 | `com.mendix.widget.web.events.Events` | OnLoad action, OnLoadDelay, Timer interval |
| **Timeline** | 29 | `com.mendix.widget.web.timeline.Timeline` | DataSource, Title, Description, Time, Icon, GroupBy |
| **Accordion** | 13 | `com.mendix.widget.web.accordion.Accordion` | Groups (header, content widgets), Collapsible, ExpandedIndex |
| **HTMLElement** | 11 | `com.mendix.widget.web.htmlelement.HTMLElement` | Tag, Content (expression/child widgets), Attributes (key/value pairs) |
| **SelectionHelper** | 4 | `com.mendix.widget.web.selectionhelper.SelectionHelper` | DataSource, SelectionMethod, OnChange action |

### Tier 2 — Charts (common pattern, batch-implementable)

| Widget | Total Count | Widget ID | Key Properties to Extract |
|--------|-------------|-----------|--------------------------|
| **LineChart** | 31 | `com.mendix.widget.web.linechart.LineChart` | DataSource, XAxis, YAxis, Series (attribute, color, line style) |
| **ColumnChart** | 16 | `com.mendix.widget.web.columnchart.ColumnChart` | DataSource, XAxis, YAxis, Series (attribute, color) |
| **BarChart** | 9 | `com.mendix.widget.web.barchart.BarChart` | DataSource, XAxis, YAxis, Series |
| **PieChart** | 7 | `com.mendix.widget.web.piechart.PieChart` | DataSource, ValueAttribute, SliceCaption, Colors |
| **BubbleChart** | 4 | `com.mendix.widget.web.bubblechart.BubbleChart` | DataSource, XAxis, YAxis, BubbleSize |
| **AreaChart** | 2 | `com.mendix.widget.web.areachart.AreaChart` | DataSource, XAxis, YAxis, Series |
| **TimeSeries** | 2 | `com.mendix.widget.web.timeseries.TimeSeries` | DataSource, XAxis, YAxis |
| **CustomChart** | 4 | `com.mendix.widget.web.customchart.CustomChart` | DataSource, JSON config |

All chart widgets share a common property pattern (DataSource, axes, series). A single `extractChartProperties()` helper can cover most of them.

### Tier 3 — Utility widgets

| Widget | Total Count | Widget ID | Key Properties to Extract |
|--------|-------------|-----------|--------------------------|
| **Switch** | 17 | `com.mendix.widget.custom.switch.Switch` | Attribute (Boolean), Label, Editable |
| **ProgressBar** | 8 | `com.mendix.widget.custom.progressbar.ProgressBar` | Value (expression/attribute), MinValue, MaxValue, Label |
| **ProgressCircle** | 6 | `com.mendix.widget.custom.progresscircle.ProgressCircle` | Value, MinValue, MaxValue |
| **Slider** | 8 | `com.mendix.widget.custom.slider.Slider` | Value (attribute), MinValue, MaxValue, Step |
| **Markdown** | 7 | `com.mendix.widget.web.markdown.Markdown` | Content (attribute/expression), SanitizeContent |
| **RichText** | 2 | `com.mendix.widget.custom.richtext.RichText` | Value (attribute), ReadOnly, Toolbar options |
| **Maps** | 11 | `com.mendix.widget.custom.Maps.Maps` + native | DataSource, Latitude, Longitude, Markers, Zoom |
| **TreeView** | 8 | `com.mendix.widget.web.treeView.TreeView` | DataSource, Children association, Caption, Icon |
| **LanguageSelector** | 3 | `com.mendix.widget.web.languageselector.LanguageSelector` | Position, Trigger |
| **BarcodeScanner** | 1 | `com.mendix.widget.web.barcodescanner.BarcodeScanner` | OnDetect action |
| **DocumentViewer** | 1 | `com.mendix.widget.web.documentviewer.DocumentViewer` | File entity, Height |

### Not in scope — Native-only and 3rd-party widgets

Native-only widgets (FloatingActionButton, BottomSheet, SafeAreaView, ListViewSwipe, IntroScreen, BackgroundImage, Carousel, Animation) and third-party marketplace widgets (JavaScriptSnippet, Drawer, KeyboardShortcut, 3D Viewer, TreeList, OrgChart) are out of scope. They fall through to the generic formatter which shows the uppercased widget name — acceptable for niche widgets.

## Proposed DESCRIBE Output

### Image (784 instances — most used pluggable widget)

Current output:
```
image image1;
```

Proposed output:
```
image image1 datasource: dynamic from MyModule.Product/image, width: 200, height: 150,
  AlternativeText: $currentObject/Name, onclick: call_nanoflow MyModule.ShowImageFull;
```

For static image:
```
image image1 ImageUrl: 'https://example.com/logo.png', width: 100;
```

### Tooltip (172 instances)

Current output:
```
tooltip tooltip1;
```

Proposed output:
```
tooltip tooltip1 position: top, TriggerOn: hover {
  TRIGGER {
    actionbutton button1 caption: 'Info';
  }
  content {
    text text1 content: 'This field is required';
  }
};
```

### Badge (80 instances)

Current output:
```
BADGE badge1;
```

Proposed output:
```
BADGE badge1 value: $currentObject/count, type: badge, onclick: show_page MyModule.Detail;
```

### Chart widgets (shared pattern)

Current output:
```
LINECHART lineChart1;
```

Proposed output:
```
LINECHART lineChart1
  datasource: database from MyModule.SalesData
  XAxis: Month
  YAxis: Revenue
  SERIES series1 attribute: Revenue, Color: '#3498db', LineStyle: straight;
```

### Accordion (13 instances)

Current output:
```
ACCORDION accordion1;
```

Proposed output:
```
ACCORDION accordion1 Collapsible: Yes {
  GROUP 'General Information' {
    textbox name1 attribute: $currentObject/Name;
    textbox email1 attribute: $currentObject/Email;
  }
  GROUP 'Address' Expanded: Yes {
    textbox street1 attribute: $currentObject/Street;
    textbox city1 attribute: $currentObject/City;
  }
};
```

### HTMLElement (11 instances)

Current output:
```
HTMLELEMENT htmlElement1;
```

Proposed output:
```
HTMLELEMENT htmlElement1 Tag: 'div', attributes: [data-testid='container', role='alert'] {
  text text1 content: $currentObject/message;
};
```

### Switch (17 instances)

Current output:
```
SWITCH switch1;
```

Proposed output:
```
SWITCH switch1 attribute: $currentObject/IsActive, editable: Yes;
```

### ProgressBar (8 instances)

Current output:
```
PROGRESSBAR progressBar1;
```

Proposed output:
```
PROGRESSBAR progressBar1 value: $currentObject/Progress, MaxValue: 100, label: '%{value}%';
```

## Implementation Approach

### Architecture: Property Extraction Framework

Rather than writing ad-hoc extraction code for each widget, introduce a lightweight property extraction framework in `cmd_pages_describe_pluggable.go`:

```go
// extractPluggableWidgetProperties extracts known properties from any CustomWidget.
// returns a list of key=value strings for MDL output, plus child widget groups.
func (e *Executor) extractPluggableWidgetProperties(w map[string]any, widgettype string) (
    props []string, childGroups []namedWidgetGroup) {

    switch widgettype {
    case "image":
        return e.extractImageProperties(w)
    case "tooltip":
        return e.extractTooltipProperties(w)
    case "BADGE":
        return e.extractBadgeProperties(w)
    // ... chart widgets share extractChartProperties()
    case "LINECHART", "COLUMNCHART", "BARCHART", "AREACHART", "TIMESERIES":
        return e.extractChartProperties(w)
    case "PIECHART":
        return e.extractPieChartProperties(w)
    case "ACCORDION":
        return e.extractAccordionProperties(w)
    // ... etc
    default:
        return nil, nil // fall through to generic
    }
}
```

### Shared helpers (already exist, reusable)

The following helpers in `cmd_pages_describe_pluggable.go` already handle the complex property bag format:

| Helper | Purpose |
|--------|---------|
| `extractCustomWidgetPropertyString()` | Get a string property by key |
| `extractCustomWidgetPropertyAttributes()` | Get attribute references |
| `extractCustomWidgetAttribute()` | Get the primary attribute binding |
| `extractCustomWidgetPropertyExpression()` | Get expression values |
| `parseCustomWidgetDataSource()` | Parse DataSource property (database/microflow/nanoflow) |
| `extractTextTemplateParameters()` | Extract text template expressions |

### Implementation phases

**Phase 1 — Image widget** (784 instances, most impactful single widget)
- Extract: DataSource type (static URL / dynamic entity), Width, Height, AlternativeText, OnClick action
- ~60 lines in `extractImageProperties()`

**Phase 2 — Container widgets** (Tooltip, Accordion, HTMLElement)
- These have child widget groups that need recursive formatting
- Tooltip: trigger + content groups
- Accordion: groups with headers + content
- HTMLElement: tag, attributes, children
- ~120 lines total

**Phase 3 — Simple value widgets** (Badge, Switch, ProgressBar, ProgressCircle, Slider)
- Single-value widgets with straightforward property extraction
- Share a pattern: Value/Attribute binding + a few options
- ~80 lines total

**Phase 4 — Chart widgets** (LineChart, ColumnChart, BarChart, PieChart, etc.)
- All share DataSource + axis + series pattern
- One `extractChartProperties()` helper covers 7 chart types
- ~100 lines

**Phase 5 — Remaining widgets** (PopupMenu, Events, Timeline, Markdown, RichText, Maps, TreeView, SelectionHelper)
- Varied complexity, implement based on demand
- ~150 lines total

### Wire into existing DESCRIBE formatter

The `else` branch in `cmd_pages_describe_output.go` line 424 changes from:

```go
} else {
    // current generic handling
}
```

To:

```go
} else {
    // Try widget-specific extraction first
    extraProps, childGroups := e.extractPluggableWidgetProperties(rawWidget, widgettype)
    if len(extraProps) > 0 || len(childGroups) > 0 {
        // use extracted properties
        props = append(props, extraProps...)
        // ... format with child groups if present
    }
    // Fall through to generic for unknown widgets (unchanged)
}
```

### No grammar/AST/visitor changes needed

This is purely a DESCRIBE output improvement. The DESCRIBE PAGE command already handles CustomWidgets — we're just making the property extraction richer for more widget types. No new MDL keywords, grammar rules, or AST types are required.

## SHOW WIDGETS Enhancement (optional)

The existing `show widgets` command already lists all widget instances including pluggable ones. A useful enhancement would be filtering by widget category:

```
show widgets where widgettype like '%chart%'     -- all chart widgets
show widgets where widgettype = 'IMAGE'           -- all Image widgets
show widgets where widgettype like '%filter%'     -- all filter widgets
```

This already works via the existing `like` filtering since `extractCustomWidgetType()` provides the uppercased type name for catalog storage.

## Testing Strategy

1. Run `describe page` against pages in all 3 test projects containing each widget type
2. Verify output is valid MDL that captures the widget's essential configuration
3. Compare output against Studio Pro's property panel to ensure completeness
4. Ensure generic fallback still works for unknown/third-party widgets

## Summary

| Phase | Widgets | Instances Covered | Lines (est.) |
|-------|---------|-------------------|-------------|
| Phase 1 | Image | 784 | ~60 |
| Phase 2 | Tooltip, Accordion, HTMLElement | 196 | ~120 |
| Phase 3 | Badge, Switch, ProgressBar, ProgressCircle, Slider | 119 | ~80 |
| Phase 4 | 7 chart types | 73 | ~100 |
| Phase 5 | PopupMenu, Events, Timeline, Markdown, etc. | 99 | ~150 |
| **Total** | **~25 widget types** | **1,271 instances** | **~510 lines** |

After implementation, 32 of the ~35+ pluggable widget types found in real projects would have meaningful DESCRIBE output, up from the current 7.
