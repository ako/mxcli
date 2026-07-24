---
title: Atlas Design System — a skill for visually appealing Mendix apps
status: proposed
date: 2026-07-24
related:
  - page-styling-support.md
  - show-describe-building-blocks.md
  - PROPOSAL_mxcli_dev_warm_loop.md
  - PR #13 (P0 warm-loop fixes)
---

# Atlas Design System — a skill for visually appealing Mendix apps

> Drafted from the Itinera travel-planner build (test app `ako/MxcliTest1`). This is a
> **design-system / skill layer**, not new infrastructure — it sits *on top of* four existing
> proposals and consumes what they build.

## Overview

mxcli can generate a fully-functional Mendix app, but the default output looks **bland** — it leans
on raw Atlas defaults plus ad-hoc SCSS. Reaching the polish of a hand-designed HTML artifact is
possible but currently costs a day of manual iteration, fighting Atlas specificity, and
re-discovering the same gotchas.

The Itinera build proved a **repeatable method** for closing that gap. This proposal captures it as
a skill so a generated app reaches "designed product" quality *by default* — the way
`artifact-design` does for standalone HTML and `dataviz` does for charts.

**Scope boundary:** `atlas-design` is the *taste + workflow* layer. It does **not** re-specify the
styling mechanics, composition primitives, or building-block/template introspection — those are the
four proposals below. It *uses* them and adds the design system, the Atlas-first method, the recipe
library, the gotchas catalog, and the verify loop.

---

## Relationship to existing proposals (reconciliation)

This proposal deliberately **defers** to and **depends on** work already proposed. It should not
duplicate any of it.

| Existing proposal | Status | What it owns | What `atlas-design` adds on top |
|---|---|---|---|
| `page-styling-support.md` | partial (Phase 1 done) | the 4 styling channels: `class`, inline `style`, `DynamicClasses`, typed `designproperties` | *which* classes/tokens to use and *when* (the Atlas vocabulary + palette method) |
| `proposal_page_composition.md` | proposed (fragments impl'd) | `define/use fragment`, `alter page`, partial updates, **parameterized fragments (future)** | ships the recipe library *as* fragments; designs recipes to migrate to parameterized fragments |
| `show-describe-building-blocks.md` | proposed (name-list only today — content unreadable) | `Forms$BuildingBlock` **read** (widget tree) → **instantiate** (`use`) → **author** (`create`) | uses Building Blocks as the eventual native home for recipes; maps Atlas blocks → our recipes; see the "what's needed" spec below |
| `show-describe-page-templates.md` | proposed (list-only today) | `Forms$PageTemplate` introspection | the page-template → screen map (Detail_Cards, Grid_Card, Dashboard_*) |

If any of these advance, `atlas-design` should shrink accordingly (e.g., once `use building block`
lands, the `.mdl`/fragment recipe library becomes a thin adapter over native Building Blocks).

---

## The core insight: be **Atlas-first**

Our first pass hand-rolled `.panel` / `.trip-card` / `.stat` that **partly reinvented what Atlas
already ships**. Atlas exposes a rich appearance system reachable from MDL today via `class:` (Phase
1 of `page-styling-support` is done), with more idiomatic typed `designproperties` coming.

### Atlas appearance vocabulary (from `atlas_core/web/design-properties.json`, verified in-project)

| Concern | Atlas classes (apply via `class:`) |
|---|---|
| **Cards** | `card`, `cards` (+ Card-style variants) — real CSS (`.card` = 19 rules) |
| **Backgrounds** | `background-{default,main,primary,secondary,success,warning,danger}` |
| **Buttons** | `btn-{primary,secondary,success,warning,danger}`, `btn-{lg,sm,bordered,block,icon-right,icon-top}` |
| **Flex / align** | `flex-{row,column,nowrap,items-grow,items-shrink}`, `align-x-{left,center,right,between,around,evenly}`, `align-y-*` |
| **Spacing utils** | `spacing-{outer,inner}-{top,right,bottom,left}[-medium|-large|-none]` |
| **Borders / overflow** | `div-border-toggle-{all,top,…,none}`, `div-overflow-{auto,hidden,visible}`, Border radius/color/style/width |
| **Elevation** | `Shadow` toggle |
| **Data grids** | `datagrid-{bordered,hover,striped,lined,lg,sm}` |
| **Group boxes** | `groupbox-{primary,danger,secondary,callout}` |

### Atlas page templates (`Forms$PageTemplate`, surfaced from `Atlas_Web_Content`, 46 pages)
Several map 1:1 onto what we hand-built: **`Detail_Cards` + `Detail_Timeline`** ≈ our detail page;
**`Dashboard_Status` + `Grid_Card`** ≈ our overview. `Grid_Card`, `List_Status`, `List_MasterDetail`,
`Dashboard_*`, `Wizard_Form`, `Tabs_Card` are canonical compositions to mirror.

### Atlas Building Blocks (`Forms$BuildingBlock`, 233 across the team's test projects)
The Mendix-native "card/title recipe library": reusable widget compositions **copied** onto pages
(templates, not runtime components — dragging one in *deep-copies* its widget tree; there is no live
link afterwards), with preview thumbnails and categories. This is the eventual native home for our
recipes.

#### Precise current state (verified in mxcli source)
"Can I create pages *with* Building Blocks today?" — **No.** Building Blocks are readable at the
name level only and cannot be inspected, instantiated, or authored via MDL:

| Layer | State | Evidence |
|---|---|---|
| Go type + reader (`ListBuildingBlocks()`) | ✅ present — used by the project-tree TUI, `examples/read_project`, and exposed (unused) on the backend interface | `sdk/pages/pages.go`, `sdk/mpr/reader_types.go`, `mdl/backend/page.go` |
| Parser | ⚠️ reads **Name + Documentation only** — *not* the widget tree | `sdk/mpr/parser_misc.go` (`parseBuildingBlock`) |
| `SHOW` / `DESCRIBE BUILDING BLOCK` | ❌ `cmd_describe.go` **explicitly excludes** them | `cmd/mxcli/cmd_describe.go` (`unitTypeToDescribe`) |
| Grammar / AST / visitor / executor | ❌ none | grep `mdl/grammar`, `mdl/ast`, `mdl/visitor`, `mdl/executor` |
| Instantiate onto a page / author a new one | ❌ no syntax | — |

So mxcli can list building-block names but can't read their content, describe them, copy one onto a
page, or create one. (`show-describe-building-blocks.md` is `status: proposed`.)

#### What "create pages with Building Blocks" actually requires (three capabilities, in order)
1. **Read their content** — extend the widget-tree reader (a Building Block carries the *same*
   `widgets: []` tree as `Forms$Page` / `Forms$Snippet`) to `Forms$BuildingBlock`, and surface it via
   `SHOW BUILDING BLOCKS` (name, module, category, platform, preview) + `DESCRIBE BUILDING BLOCK
   Mod.Name` (round-trippable MDL). Note: the reusable widget-tree logic already lives in the
   executor (`parseRawWidget` / `getSnippetWidgetsFromRaw` over `Backend.GetRawUnit`, in
   `mdl/executor/cmd_pages_describe.go`), not in `sdk/mpr` — that is what to reuse. Without this you
   can't discover-then-reuse what a project already ships. **This is the prerequisite for everything
   else.**
2. **Instantiate onto a page** — `USE BUILDING BLOCK Mod.Name [prefix 'p_']` inside a page/container
   that **deep-copies** the block's widget tree in. Mechanically this is *fragment expansion sourced
   from a persisted document* instead of a script-scoped `define fragment` — the copy semantics,
   prefix/name-collision handling, and "no live link" behaviour are identical, so it can reuse the
   fragment-expansion machinery. This is the capability that makes "compose a page from pre-built
   sections" real.
3. **Author new ones (optional)** — `CREATE BUILDING BLOCK Mod.Name { widgets }` so generated apps
   *contribute* reusable blocks back into the Studio-Pro toolbox (category, platform, preview
   thumbnail). Lower priority than read + instantiate.

**Why this matters for `atlas-design`:** because a Building Block is a *copy template*, our recipe
library maps onto it 1:1 (a recipe is also a copied widget shape). Once (1)+(2) land, the
`.mdl`/fragment recipes become a thin adapter over native Building Blocks *and* become visible in
Studio Pro — the ideal end-state. Until then, recipes ship as fragments + `.mdl` fill-in (below).

**Rule of thumb — reach *down* the stack first:** need a card? `class:'card'` before a `.panel`
rule. Brand blue on buttons? `--brand-primary` before overriding `.btn-primary`. Custom CSS is the
**last** resort, for identity only.

---

## Findings from live testing (this app, mxcli `b990548`)

Ran a controlled experiment on the running app: a page of **pure Atlas classes, zero custom CSS**,
authored purely with mxcli's `class:` property. Result — **the Atlas-first thesis holds**:

| Authored via `class:` | Rendered |
|---|---|
| `card` + `spacing-inner-large` | real Atlas card (surface bg, border, radius, padding) ✅ |
| `background-primary` | **the app's azure** — the Atlas utility inherited our retuned `--brand-primary` ✅ |
| `flex-row` + `align-x-between` | children spread left/right ✅ |
| `buttonstyle: primary`, `btn-lg` (class), `btn-bordered` (class) | solid / large / outlined Atlas buttons ✅ |

**Confirmed for the proposal:**
1. **Raw `class:` strings are sufficient today** — `page-styling-support` Phase 1 (`class`) already renders
   the full Atlas appearance vocabulary. The typed `designproperties` channel is **not required for the
   visual result** (it matters only for round-tripping into Studio Pro's Appearance tab). → recipes can
   ship on `class:` now; `designproperties` support is a nicety, not a blocker.
2. **Brand tokens propagate *down* into Atlas utilities** — `background-primary`/`btn-primary` resolve to
   our `--brand-primary`. This validates the layered model: retune Layer 1 tokens and Layer 0 Atlas
   classes follow for free. Strong argument to **delete** the hand-rolled `.panel`/`.trip-card`/`.stat`/
   `.insight-card` and use `class:'card …'`, keeping custom SCSS only for identity (mono type, pills,
   timeline spine, elevation curve).

**Tooling finding for the mxcli session (a real bug — now fixed):**
- Under `run --local --watch`, a **model change that added a page and repointed the home** hot-applied
  "via restart, client re-bundled (gen 2)" but then **`/dist/index.js` 404'd** → the app served only the
  `<noscript>` shell and was unbootable. A **clean full restart fixed it** (the same page renders fine).
  The watch client-re-bundle path did not reliably regenerate/serve `/dist/index.js` on *structural*
  model changes. **Fixed in PR #13**: the watch-apply loop now gates the "applied" report on
  `/dist/index.js` being present on disk *and* served (200), recovering via a synchronous one-shot
  bundle when it is not. (Theme-only SCSS edits always hot-applied correctly — that path was unaffected.)

## Styling the standard widgets (from the widget-lab exercise)

Building a page that exercises every marketplace widget surfaced a second, distinct lesson: the
**pluggable widgets Mendix ships look unfinished by default**, and closing that gap is a *separate*
recipe from Atlas-class composition. Two families dominate: **charts** and **dark-mode surfaces**.

### Charts — a `dataviz`-grade recipe for the Mendix chart widgets
Out of the box the chart widgets (Column/Bar/Area/Pie/Line) render **raw Plotly defaults**: one flat
primary colour, a floating mode-bar on hover, wide margins, heavy gridlines. That is the single
biggest "not a real product" tell. The widgets expose three Plotly hooks — barely used by generated
apps — that turn them into designed charts. All three are **plain JSON strings** (no Mendix
expression quoting), so they are trivial to template:

| Property | Plotly layer | Use it for |
|---|---|---|
| `customLayout` | `layout` | transparent `paper_bgcolor`+`plot_bgcolor`, system font, `#8a94a6` ticks, tight `margin`, faint `gridcolor`, `zeroline:false`/`showline:false`, dark `hoverlabel` |
| `customConfigurations` | `config` | `{"displayModeBar":false,"responsive":true}` — removes the floating toolbar |
| `customSeriesOptions` (per series; chart-level on Pie) | trace | brand colour, `marker.cornerradius` (rounded bars), `line.shape:"spline"` + translucent `fillcolor` (area), Pie colour array + white inside labels |

**Key trick — transparent background = theme-agnostic charts.** Setting `paper_bgcolor`/`plot_bgcolor`
to `rgba(0,0,0,0)` makes the plot inherit whatever panel it sits on, so **one config is correct in
both light and dark** with zero per-theme CSS and no fighting Plotly's SVG. Pair it with a neutral
tick colour (`#8a94a6`) that reads on either background. This is the chart analogue of the
`dataviz` skill and should ship as a ready-made `chart-theme` asset (a shared `customLayout`
+ `customConfigurations`, plus per-type series snippets).

**Chart gotchas (each cost real time):**
- **BarChart "0" prefix.** A *horizontal* bar needs `staticXAttribute = value`, `staticYAttribute =
  category`; but with `aggregationType: sum` Mendix prepends a `0` group-key to every category tick
  (`"0Tokyo Spring"`). Use `aggregationType: none` when the datasource is already one row per
  category (e.g. an aggregating view entity). Column charts (category on X) are unaffected.
- **The mode-bar and the white paper are the two ugliest defaults** — always kill both
  (`displayModeBar:false` + transparent bg).
- **All chart types are MDL-authorable today** (verified against `ako/mxcli`). Column/Bar/Area/Pie
  *and* Line/Bubble/Heatmap/TimeSeries — the `line`/`scalecolor` object-lists have grammar keywords
  and working, check-suite examples (`mdl-examples/doctype-tests/34-chart-widget-examples.mdl`) on the
  modelsdk engine. (An older `create-page.md` note claimed only the first four were authorable — that
  note is stale and should be corrected.)

### Dark mode — Atlas widgets need an explicit override recipe
This resolves the proposal's open dark-mode question with a concrete finding. A token-driven
`@media (prefers-color-scheme: dark)` flip repaints *your* custom chrome, but **Atlas's own widgets
and Plotly do not follow** — they ship light-only surfaces, so on a dark page they render as white
boxes with (often) near-invisible text. Observed clashes and the fix:

| Widget | Light-only surface that clashed | Override (scoped to `.travel-app`, dark media query) |
|---|---|---|
| Text input / textarea / combobox field | white `.form-control` | surface token bg + ink text |
| Datagrid | white rows, near-white text; white `.filter-selector-button` chips | dark rows/headers/chips + ink text |
| Datagrid dropdown filter | `.widget-dropdown-filter-menu` paints a **hardcoded white scroll-fade `linear-gradient`** over its (themed) bg | `background-image:none` + brighten item text |
| Accordion / Fieldset | white group/legend surfaces | surface token bg + border/legend ink |
| **TreeNode** | expanded child rows carry a **white card bg**; dark ink text on it is invisible | drop the white so the dark panel shows through |
| Charts | white Plotly paper | transparent `customLayout` (above) — adapts automatically |
| Combobox/tooltip/dropdown-filter popovers | render at `<body>`, outside `.travel-app` | theme **globally**, not `.travel-app`-scoped |
| **Edit popups** (`.mx-window` / `.modal-content`) | white window container shows through the transparent header as a **white title bar**; white default buttons | theme `.mx-window`/`.modal` chrome + its form controls/buttons **globally** |

**Takeaways for the skill:** (1) ship a **dark-mode widget-override recipe** (the selector list above
is the starting inventory) as an optional asset — dark mode is *not* free once you leave your own
classes; (2) prefer the transparent-chart trick over any chart CSS; (3) if the app can't fund the
override recipe, **ship light-only** — a half-dark result (custom chrome dark, Atlas widgets light)
is worse than consistent light. Also a **runtime crash** to encode: the Slider/RangeSlider *tooltip*
calls React's removed `findDOMNode` on Mendix 11's React and throws "Could not render widget" on
drag — set `showTooltip:false`.

### Re-skinning an existing app end-to-end (the "Atlas MES" exercise)
We took the finished light-SaaS app and re-skinned it into a completely different identity — a dark,
industrial "manufacturing execution system" (near-black ground, Space Grotesk / IBM Plex, sharp
corners, green accent, gradient cards, glowing status dots) — **changing only the two theme files
(`custom-variables.scss` + `main.scss`). Zero page/microflow/MDL edits.** This is the strongest
validation of the architecture: the token + recipe-class layer is a genuine *skin*, swappable without
touching structure, and (being SCSS) it hot-applies via `--watch`. Concrete lessons:

- **A Layer-1 token retune cascades into pluggable widgets for free.** Setting `--brand-primary` to
  the MES green flowed straight into the **Switch, Slider, RangeSlider, ProgressBar, ProgressCircle
  and BadgeButton** (they read Atlas brand vars) — no per-widget CSS. This is the layered model's
  headline payoff, now shown on a full palette pivot, not just a tweak.
- **Use *both* token channels for a global shift.** `custom-variables.scss` retunes Atlas **leaves**
  (`--bg-color`, `--font-color-default`, `--form-input-*`, `--border-color-default`,
  `--card-border-radius`, radius→0) so Atlas surfaces *outside* your scoped classes (form inputs,
  popups) inherit the new palette; `main.scss` retunes the `--tv-*` recipe tokens + classes. Neither
  alone is enough.
- **Committing to a single theme is *less* work than dual-theme.** Because MES is dark-only, we
  dropped the `@media (prefers-color-scheme: dark)` gate entirely and made the widget overrides
  **unconditional and global** (not `.travel-app`-scoped). That simultaneously fixed the "half-dark
  clash" *and* covered the portal-rendered popups/modals. Decide theme-count up front: one committed
  theme is simpler and more robust than trying to support both.
- **Charts are the one thing that does NOT follow the CSS token cascade.** Every visual on the page
  is CSS and re-skins for free — except chart **series colours**, which live in the *model* (each
  chart's `customSeriesOptions` / `customLayout` JSON), not CSS. Matching charts to the new brand
  therefore needed an **MDL edit + a gen-2 restart**, unlike everything else (pure SCSS, hot-applied).
  → Flag for the roadmap: let charts read a theme colourway / CSS var so a palette pivot doesn't
  require a model change. Until then, treat chart colour as model config, not theme.
- **Web-font `@import` ordering gotcha.** `@import url('…fonts.googleapis…')` must be the **first
  line** of `main.scss` — before the `custom-variables` partial import and before any CSS rule — or
  the browser drops it (CSS ignores `@import` after rules). Always ship a **system fallback stack**
  (`"Space Grotesk", "IBM Plex Sans", system-ui, …`) so the layout survives a font-load failure.
- **Design-handoff ingestion.** The mockup arrived as a Claude Design bundle (`*.dc.html`): a
  templating shell (`sc-for`/`sc-if`/`{{ }}` + a `DCLogic` state class). The **CSS values in the
  inline styles are the spec** — extract palette / type / spacing / borders and ignore the templating
  and JS. (Private `claude.ai/design/…` share links are not fetchable by the agent — ask for the
  exported bundle or pasted HTML.)

### Building a bespoke dashboard, not just a re-skin (the MES Line-Overview exercise)
Everything above re-styled *existing* structure. To test whether mxcli can **build a bespoke
operational screen from scratch**, we authored a real Manufacturing-Execution-System "Line Overview"
dashboard — new `MES` module (ProductionLine / PlantEvent entities, an aggregating view, a KPI
carrier + `DS_MESStats` microflow, a committed demo seed), and a page with a custom **top-bar + nav-tabs
shell**, a 6-tile KPI grid, status-coloured line cards, a throughput chart, and a live event feed.
It rendered faithfully to the mockup. Verdict: **yes — mxcli can generate a designed dashboard, not
only re-theme one.** Reusable techniques it proved:

- **Custom shell without a custom Layout document.** We wanted a full-screen control-room (no Atlas
  sidebar), but a blank *layout* is hard to select (`Blank` is a page *template*, and the catalog
  doesn't expose layout refs). Solution: keep a normal Atlas layout and **hide the sidebar per-page
  with `.mx-page:has(.mes-app) .region-sidebar { display:none }`**, building the top-bar + tabs as page
  content. Page-scoped `:has()` is the clean way to get a bespoke shell without new layout docs.
- **Status-driven colour is one `dynamicclasses` expression + one CSS var.** A single
  `dynamicclasses` maps the status enum to `status-running|idle|down`; the card sets `--st` per state
  and that one variable cascades the colour into the pill, the OEE number, the pulsing dot, and the
  card border. This is *the* "colour-by-state" recipe — it should ship as a first-class recipe.
- **CSS animations port; client-side JS state does not.** Pulse/blink "alive" cues work via
  `@keyframes` + classes. The mockup's ticking clock / incrementing counters / sweep are client JS
  with no Mendix equivalent — you get static values unless you add a nanoflow-refresh timer. Document
  this ceiling so "real-time dashboard" expectations are set.
- **Aggregate KPIs (count/sum) compute fine** — the tiles showed real sums/counts — but see the
  microflow gotchas below for what the aggregate grammar does *not* allow.

This exercise is also the strongest case for the P1 Building-Block work: the line-card, KPI-tile, and
event-row shapes are exactly the reusable compositions a Building-Block library would hold.

## The standard — a 4-layer architecture

```
Layer 3  VERIFY      run --local --watch  +  Playwright screenshot   (mx check is NOT enough)
Layer 2  IDENTITY    themesource/<mod>/web/main.scss — --tv-* tokens + recipes (palette, mono type,
                     elevation, status pills, timeline) — ONLY what Atlas can't provide
Layer 1  BRAND       theme/web/custom-variables.scss — retune Atlas tokens (--brand-primary, bg,
                     semantic colors, radius) so Atlas components inherit the palette
Layer 0  ATLAS       Atlas classes / designproperties / templates / building blocks — structure & base
```

Grounded assets from the Itinera build: `mdlsource/06-redesign.mdl`, `07-shell.mdl`,
`themesource/travel/web/main.scss`, `theme/web/custom-variables.scss`, and the approved Itinera HTML
mockup (design-first spec).

---

## Reuse mechanism for the recipe library (settled)

There are **five** reuse mechanisms; the recipe library should use a blend, chosen per recipe and
designed to migrate to the roadmap.

| Mechanism | Persisted | Reuse | Params / slots | Support | Use for |
|---|---|---|---|---|---|
| `.mdl` recipe (text) | — | copy | full fill-in | today | content-varying blocks now |
| **Fragment** (`define/use`) | No (script-scoped) | copy, DRY-in-script | `prefix_` only (params future) | **impl'd** | fixed repeated groups (header, footer, pill) |
| Mendix **Snippet** | Yes | reference (live) | context entity only, no content slot | create/describe | genuinely-shared runtime components |
| **Building Block** | Yes (template) | copy (deep-copied in, no live link) | template + preview | name-list only (read/`use`/`create` all proposed) | **eventual native home** for recipes |
| **Page Template** | Yes | copy (new-page) | layout + tree | list-only | screen scaffolds |

**Decision:**
1. **Now** — ship recipes as **fragments** (a `define fragment` prelude scripts include) for fixed
   groups, plus **`.mdl` fill-in recipes** for content-varying blocks (a card wrapping arbitrary
   content — fragments can't parameterize content yet). Reserve **Mendix Snippets** for truly-shared
   runtime components.
2. **Design for the roadmap** — author recipes so they migrate cleanly to **parameterized fragments**
   (`proposal_page_composition.md`, future) and ultimately **`use building block`**
   (`show-describe-building-blocks.md`), which would also surface them in Studio Pro's toolbox.

Rationale: a design-recipe library is mostly *shapes whose content varies per use* (a status pill
binds `Trip.Status` in one place and `Activity.Category` in another). Mendix Snippets can't do that
(context-entity only, no content slot); fragments can't yet (no params); so today it's fragments +
fill-in. Parameterized fragments / Building Blocks close the gap.

---

## The skill: `atlas-design`

### Trigger
Load before styling a Mendix web app / page group, when the user asks to make an app "look good /
professional / branded / less bland", or to match a design mock. Companion to `create-page` (widget
*syntax*), `page-styling-support` (styling *mechanics*), and `fragments` (composition).

### Structure
```
.claude/skills/atlas-design.md            # the method (§ Atlas-first, 4-layer, verify)
.claude/skills/atlas-design/
  ├─ references/atlas-classes.md          # the Atlas appearance vocabulary + when to use each
  ├─ references/page-templates.md         # Atlas template → screen map (defers to show-describe-page-templates)
  ├─ references/gotchas.md                # the catalog below
  ├─ references/verify.md                 # run --watch + screenshot loop; "mx check misses client crashes"
  ├─ assets/custom-variables.scss         # Layer-1 brand-token scaffold (palette-swappable)
  ├─ assets/main.scss                     # Layer-2 token + recipe starter
  ├─ assets/dark-mode-overrides.scss      # optional — repaint Atlas widgets for dark (form/datagrid/
  │                                       #            accordion/fieldset/treenode/popovers)
  ├─ references/charts.md                 # the chart-styling recipe + gotchas (dataviz-for-Mendix)
  └─ recipes/                             # fragment prelude + .mdl fill-in recipes
       card / stat-tile / status-pill / page-header / hero-overlay / timeline-row / budget-row /
       chart-theme (customLayout + customConfigurations + per-type customSeriesOptions)
```

### Contents
1. Design-first workflow (mock → approve → port).
2. Atlas-first + the 4-layer architecture + "reach down the stack first".
3. The Atlas appearance cheat-sheet (use `class:`/`designproperties` before hand CSS).
4. Token architecture (the two-file split, both themes, style-through-tokens).
5. The recipe library (fragments + fill-in, migrating to parameterized fragments / Building Blocks).
6. **Standard-widget styling** — the chart theme (transparent Plotly `customLayout` + `displayModeBar:false`
   + per-type `customSeriesOptions`) and the optional dark-mode Atlas-widget override recipe.
7. The gotchas catalog.
8. The verify loop (**runtime verification is mandatory** — see below).

---

## Gotchas catalog (encode these — each cost real time this session)

| Gotcha | Fix |
|---|---|
| `$` in `dynamictext content:` breaks the parser (starts a variable token) | put `$` in CSS `::before`; bind only the number |
| Enum `dynamictext` renders the **key**, not the caption | accept, or map via `dynamicclasses` |
| `sort by` not allowed on **association-sourced** listviews | sort the parent, or use a DB source |
| Aggregates can't be inlined in a create-object assignment (CE0117) | compute into vars first |
| Reserved widget identifiers exist (`v3`) | prefix names (`sv3`); avoid bare `v<n>` |
| Pluggable widgets impose their own DOM (charts/timeline/treenode) | for pixel-fidelity use native `listview`/`gallery` you fully style |
| Chart widgets render **raw Plotly defaults** (flat colour, floating mode-bar, white paper, heavy grid) | `customLayout` (transparent bg + font + faint grid) + `customConfigurations` `displayModeBar:false` + per-series `customSeriesOptions` (colour, `cornerradius`, spline) |
| Horizontal **BarChart** with `aggregationType: sum` prepends a `0` group-key to category ticks (`"0Tokyo Spring"`) | use `aggregationType: none` when the datasource is already one row per category |
| **Slider/RangeSlider** throw "Could not render widget" on drag (tooltip calls React `findDOMNode`, removed in MX 11) | set `showTooltip: false` |
| Atlas widgets + Plotly **aren't dark-aware** — a `prefers-color-scheme` flip leaves them light on a dark page | ship the dark-mode widget-override recipe (form controls, datagrid + filters/popovers, accordion, fieldset, **treenode white rows**, transparent charts), or ship light-only |
| `.widget-dropdown-filter-menu` paints a **hardcoded white scroll-fade gradient** even after bg is themed | override `background-image:none` and brighten menu-item text |
| **Edit popup has a white title bar** — `.mx-window`/`.modal-content` renders at `<body>`, outside `.travel-app` | theme `.mx-window-content`/`.modal-content` + header + form controls/buttons **globally**, not scoped |
| **Chart colours don't re-skin** — series colour lives in the model (`customSeriesOptions`), not CSS | accept it's model config; a palette pivot needs an MDL edit + gen-2 restart, not a theme edit |
| Google-fonts `@import url()` silently dropped | make it the **first line** of `main.scss` (before the partial import + any rule); keep a system fallback stack |
| Full re-skin desired (new identity) | it's **theme-only** — retune `custom-variables.scss` (Atlas leaves) + `main.scss` (`--tv-*` + classes); no page/MDL edits, hot-applies |
| Full-screen page (no Atlas sidebar) but no blank layout resolves | keep a normal layout; hide the shell per-page with `.mx-page:has(.my-app) .region-sidebar { display:none }` |
| "Colour by state" (status pills/cards) | one `dynamicclasses` enum→class expression + one `--st` CSS var cascaded into pill/number/dot/border |
| **`grant view on page` to a *cross-module* role → CE0148 "reselect roles" that BLOCKS the build** | grant the page's **own-module** role (add that role to the user role for access); cost real time to diagnose |
| Seed microflow data doesn't appear (queries empty) | **`create` doesn't persist — add `commit $obj;`**; the miss is silent (no error) |
| Bare `$x = avg(...)` or `$x = 2` fails to parse | bare `$x = …` accepts only `count`/`sum` aggregates; use `declare $x T = expr` for other expressions, `set $x = expr` to reassign a declared var |
| Integer/integer division `$a / $b` → CE0117 | Mendix `/` needs a decimal operand; averaging integers is awkward — compute upstream or store decimals |
| View entity flagged CE6770 "out of sync" | the view's declared attribute types must match its OQL source columns (Decimal vs Integer mismatch trips it) |
| **`mx check` passes but the browser client crashes** (e.g. old ListView `SearchRefs`; the slider `findDOMNode` throw only fires on interaction) | **always Playwright-verify a running build; never ship on `mx check` alone** |
| "SCSS cache" — edits don't show | never a cache: `--watch` (now watches theme source) or clean restart; kill stale process first |
| Stale process serves old output, looks like a cache | `run --local` refuses occupied ports; free them (pgrep/kill/curl 000). **PR #13** also makes teardown reap child process groups, so Ctrl-C no longer orphans a serve/runtime that holds the port |
| `ALTER PAGE SET layout … map(…)` swaps a page onto a sidebar shell | without rebuilding the widget tree |

---

## mxcli work required (prioritized)

`atlas-design` is a skill layer, but it depends on and surfaces concrete mxcli work. Consolidated
here so the tooling asks are visible in one place, ranked by how much they unblock the design
workflow. **P0 = fix before the loop is reliable; P1 = enables the workflow; P2 = polish / round-trip.**
"New" = surfaced by this session; otherwise it lives in the named existing proposal.

| Pri | Item | Why it matters | Home |
|---|---|---|---|
| ~~**P0**~~ ✅ | **Watch-mode re-serves `/dist/*` on *structural* model changes** — readiness probe verifies `/dist/index.js` is `200` before reporting "build applied", recovering via a one-shot re-bundle | Every structural change this session left the app blank/unbootable (gen-2 404) until a manual clean restart | **Fixed — PR #13** |
| ~~**P0**~~ ✅ | **`run` teardown kills its child `mxbuild --serve` / runtime** on stop | A stray serve held the port (`port 6543 in use`), so the next run refused to start | **Fixed — PR #13** (process-group reap) |
| **P1** | **`grant view on page` to a cross-module role must not emit a build-blocking CE0148** | Granting a page a role from another module set the allowed-roles list correctly yet still raised CE0148 "reselect roles", which **fails the deploy** — confusing and costly; the own-module role works, so the serialization is wrong for cross-module grants | New (bug) |
| **P1** | **Building Blocks — READ**: extend the page/snippet widget-tree reader to `Forms$BuildingBlock`; expose `SHOW BUILDING BLOCKS` + `DESCRIBE BUILDING BLOCK` | Prerequisite for *any* Building-Block reuse — today content is unreadable (name-only), so you can't discover-then-reuse what a project ships | `show-describe-building-blocks.md` |
| **P1** | **Building Blocks — INSTANTIATE**: `USE BUILDING BLOCK Mod.Name [prefix]` (deep-copy; reuse fragment-expansion) | The "compose a page from pre-built sections" capability — the native home for the recipe library | `show-describe-building-blocks.md` |
| **P1** | **Parameterized fragments** | Lets content-varying recipes (a card wrapping arbitrary content) ship without `.mdl` fill-in; materially simplifies the recipe library | `proposal_page_composition.md` |
| **P2** | **Chart theme colourway** — let a chart read a CSS var / named theme colourway | Charts are the *only* thing that doesn't re-skin via CSS (colour lives in the model's `customSeriesOptions`); today a re-brand needs an MDL edit + restart | New |
| **P2** | **Building Blocks — AUTHOR**: `CREATE BUILDING BLOCK` | Generated apps contribute reusable blocks back to the Studio-Pro toolbox | `show-describe-building-blocks.md` |
| **P2** | **Typed `designproperties`** (later phase) | Studio-Pro Appearance-tab round-trip; recipes could use Atlas tokens idiomatically vs. raw `class:` strings (not a blocker — `class:` renders everything today) | `page-styling-support.md` |
| **P2** | **Lint rules**: hardcoded hex over token; data widget shipped without a recorded runtime verification | Enforces the token discipline and the "verify at runtime" rule the standard depends on | New (skill Phase 5) |
| **P2** | **Widget-skill note**: Slider/RangeSlider `showTooltip:false` default (React `findDOMNode` removed in MX 11) | Prevents a "Could not render widget" crash that `mx check` can't catch | New (docs) |
| **P2** | **Correct the stale `create-page.md` chart note** — all chart types (incl. Line/Bubble/Heatmap/TimeSeries) are MDL-authorable today | The note says only Column/Bar/Area/Pie are authorable; the grammar + check-suite examples prove otherwise | New (docs) |

The P1 Building-Block trio (read → instantiate → author) is the single biggest lever: it turns the
recipe library from a `.mdl`/fragment workaround into native, Studio-Pro-visible components. The two
P0 items (now fixed in PR #13) were small but blocked the tight verify loop the standard is built on.

## Rollout

- **Phase 1 — skill core**: `atlas-design.md` + `gotchas.md` + `verify.md` (captures the method while fresh).
- **Phase 2 — Atlas vocabulary**: `atlas-classes.md` + page-template map (pairs with `show-describe-*`).
- **Phase 3 — token scaffold**: `custom-variables.scss` + `main.scss` starter.
- **Phase 4 — recipe library**: fragment prelude + `.mdl` fill-in recipes extracted from Itinera,
  plus the **chart-theme** recipe and the optional **dark-mode widget-override** SCSS (both extracted
  from the widget-lab exercise, `mdlsource/08-widgetlab.mdl` + the dark block in `main.scss`).
- **Phase 5 (optional) — lint rules**: flag hardcoded hex over tokens; flag data widgets shipped
  without a recorded runtime verification.

## Dependencies
> For the ranked, consolidated view of everything mxcli needs to build, see **"mxcli work required
> (prioritized)"** above. This section lists only the hard prerequisites for the skill itself.

- `page-styling-support.md` Phase 1 (`class`/`style`) — **done**, required. Typed `designproperties`
  (later phase) would let recipes use Atlas tokens idiomatically instead of raw class strings.
- `proposal_page_composition.md` fragments — **implemented**, required for the fragment recipes.
  **Parameterized fragments (future)** would materially simplify the library.
- `show-describe-building-blocks.md` `use` — **proposed**; the ideal long-term recipe home.

## Open questions
- **`designproperties` vs `class` strings**: **resolved by live testing** — raw `class:` renders the full
  Atlas vocabulary today, so recipes ship on `class:` now. Typed `designproperties` becomes a *later*
  nicety (Studio-Pro Appearance-tab round-trip), not a prerequisite.
- **Dark mode**: **resolved by this session.** A `prefers-color-scheme` flip repaints your own
  classes but **not** Atlas widgets / Plotly (they ship light-only), producing a half-dark clash. The
  robust answer is to **commit to one theme**: for a dark app, drop the media gate and make the
  widget overrides **unconditional + global** (validated by the MES re-skin — this also covers
  portal popups/modals). Ship **light-only** if you can't fund the override recipe; a half-dark
  result is worse than either. Still open only: a user-facing runtime toggle (Atlas has none built in).
- **Charts don't follow the token cascade**: series colour lives in the model
  (`customSeriesOptions`), so a re-brand needs an MDL edit, not a theme edit. Should mxcli let a chart
  read a theme colourway / CSS var (so charts re-skin with everything else), or is chart colour
  legitimately model config? Leaning "let it read a colourway" for the design workflow.
- **Recipe home end-state**: parameterized fragments vs native Building Blocks — likely both, with
  Building Blocks winning once `use` lands (Studio-Pro-visible).
- **Verification is non-skippable**: the standard's correctness depends on runtime verification
  because `mx check` demonstrably misses client crashes; the skill must enforce it.

---

### Appendix — reference artifacts
Itinera build: `mdlsource/06-redesign.mdl`, `07-shell.mdl`, `themesource/travel/web/main.scss`
(the `@media (prefers-color-scheme: dark)` block is the dark-mode widget-override starting inventory),
`theme/web/custom-variables.scss` — **these two files now carry the dark "Atlas MES" skin**, a full
second identity proving the token+recipe layer is a swappable skin (the earlier light-Itinera version
is in git history); the styled charts + every-widget validation page in
`mdlsource/08-widgetlab.mdl` (chart `customLayout`/`customConfigurations`/`customSeriesOptions`,
`showTooltip:false` slider fix); the bespoke dashboard build in `mdlsource/09-mes-dashboard.mdl`
(custom top-bar+tabs shell, status `dynamicclasses`, committed seed, aggregate KPIs) with its MES
styling block in `main.scss`; widget-authoring findings in `WIDGET-FINDINGS.md`; approved visual
direction in the Itinera HTML mockup + the Atlas MES handoff bundle.
