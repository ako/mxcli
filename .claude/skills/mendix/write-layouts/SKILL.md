---
name: write-layouts
description: "CREATE LAYOUT syntax — the frame a page is built on: scroll-container regions, the navigation tree, and the placeholders pages bind to. Use when a topbar, sidebar or page frame has to change, or when copying an Atlas layout into a module you own."
---

# CREATE LAYOUT — the frame a page is built on

A **layout** is the frame every page renders inside: the topbar, the navigation
sidebar, and the hole the page's own content drops into. Until now it was the one
document mxcli could not author, which put the topbar out of reach of MDL.

## Do not edit Atlas_Core

Mendix's own documentation says it plainly: *"Do not change the supplied layouts.
Either create a separate module with the custom layouts, page templates, and
building blocks or create your own."* An Atlas layout lives in a Marketplace
module, and a Marketplace update replaces the module wholesale — every local edit
is gone, silently.

`CREATE LAYOUT` **refuses** a Marketplace module for that reason. Put your layout
in a module you own.

The usual starting point is a copy of an Atlas layout, and `describe` already
gives you one:

```bash
mxcli -p app.mpr -c "describe layout Atlas_Core.Atlas_Default" > mine.mdl
# edit the qualified name to your own module, then:
mxcli exec mine.mdl -p app.mpr
```

`DESCRIBE LAYOUT` emits re-executable MDL, so describe → rename → exec *is* the
copy operation. There is no `COPY DOCUMENT` verb and none is needed.

## Syntax

```sql
create [or replace] layout MyModule.App_Default (
  layouttype: 'Responsive'
) {
  scrollcontainer layoutContainer {
    region top (size: 60, sizemode: 'Fixed', class: 'region-topbar') {
      snippetcall topbar (snippet: MyModule.SNIPPET_TopBar)
    }
    region left (size: 232, sizemode: 'Pixels', class: 'region-sidebar') {
      navigationtree navMenu (profile: 'Responsive')
    }
    region center (class: 'region-content') {
      placeholder Main
    }
  }
}
```

A page then names the layout, and its widgets land in the `Main` placeholder:

```sql
create page MyModule.Home (title: 'Home', layout: MyModule.App_Default) {
  dynamictext welcome (content: 'Hello')
}
```

## The four things only a layout has

| Element | MDL | Notes |
|---------|-----|-------|
| Scroll container | `scrollcontainer name { … }` | The layout's root. Its children are **regions**, never widgets |
| Region | `region top \| right \| bottom \| left \| center` | Five **named slots**, not a list. One region per slot; a repeat is refused |
| Placeholder | `placeholder Main` | The hole a page's content goes into. No properties, no body |
| Navigation tree | `navigationtree name (profile: 'Responsive')` | The sidebar menu. The profile is a navigation profile name |

Region properties: `size` (integer), `sizemode` (`Fixed` / `Pixels` / `Auto`),
`class`. Unset is Studio Pro's `200` / `Auto`.

## Layout type, and why there is no `native:` flag

| Platform | Values |
|----------|--------|
| Web | `Responsive`, `Phone`, `Tablet`, `ModalPopup` |
| Native | `Default`, `Popup` |

Measured across all 22 layouts Atlas ships. The two sets are **disjoint**, so the
platform is inferred from the type — a `native:` property could only ever
contradict it. A cross-platform value (`Default` on a web layout) is refused, not
silently accepted.

`layouttype` is the **only** header property. Anything else is an error rather
than an ignored key, which is the point of the next section.

## Gotchas

- **A placeholder's name is API.** A page binds to it as
  `Module.Layout.<Name>`, stored as a qualified name. Renaming one unbinds every
  page that used it — the page still builds, and its content vanishes.
- **Name one placeholder `Main`.** `Forms$Layout` has no property saying which
  placeholder is the main one; the convention is the mechanism, and 22 of 22
  Atlas layouts follow it.
- **There is no `mainplaceholder:` property, on purpose.** `modelsdk/gen`
  declares `MainPlaceholderName` on `Layout` so the setter compiles, and mxbuild
  accepts the result — measured 0 errors. But `generated/metamodel` does not
  declare it and no Studio Pro layout carries it, and Studio Pro resolves every
  stored property against the type's list. Writing it gives you a layout that
  builds and cannot be opened.
- **A layout must declare at least one placeholder.** Otherwise no page can use
  it. Refused at write time.
- **Authoring is modelsdk-only.** The legacy writer cannot produce the `Content`
  wrapper the widget tree hangs off; it refuses rather than writing a layout with
  no tree.

## Validate

```bash
mxcli check layout.mdl                       # syntax
mxcli check layout.mdl -p app.mpr --references
mxcli exec layout.mdl -p app.mpr
mxcli -p app.mpr -c "describe layout MyModule.App_Default"   # round-trip
```

A layout is a rendering artifact, so a clean `mx check` proves little on its own.
To see it, boot the app: `mxcli run --local -p app.mpr --screenshot` and look at
the regions.
