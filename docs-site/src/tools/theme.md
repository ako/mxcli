# Default Styling (`mxcli theme`)

A blank Mendix app looks like a blank Mendix app. `mxcli new` applies a default
theme so a generated app looks like a product on first boot, and
`mxcli theme apply` adds the same theme to a project you already have.

```bash
mxcli theme list                        # built-in themes; the default is marked *
mxcli theme show signal                 # palette, colorway, tokens, files it writes
mxcli theme apply -p app.mpr            # apply to an existing project
mxcli theme apply ledger -p app.mpr     # switch theme (the previous one is removed)
mxcli theme apply -p app.mpr --dry-run  # report changes without writing
mxcli theme remove -p app.mpr           # take it back out

mxcli theme create acme -p app.mpr      # a theme this project owns
mxcli theme list -p app.mpr             # ...which then lists beside the built-ins

mxcli new MyApp --version 11.13.0                # applies `signal`
mxcli new MyApp --version 11.13.0 --theme none   # plain Atlas
```

## The themes

| Name | Default palette | Character |
|---|---|---|
| **signal** (default) | light | Cool slate, one teal signal colour, 4px radius, 32px rows, IBM Plex |
| **ledger** | light | Warm paper, hairline rules instead of card shadows, Source Serif headings over Source Sans, 2px radius, 30px rows |
| **console** | dark | Near-black ground, teal with a violet accent, Space Grotesk over JetBrains Mono, 6px radius, 28px rows, surfaces separated by lightness |

All three share the same density discipline: an 8px spacing unit, monospace
numerics, a visible focus ring on every focusable element, and every control
growing to a 44px touch target below 768px.

Only one theme applies at a time — `theme apply` removes the previous one,
because two themes mapping the same Atlas variables would fight in the cascade.

## What it writes

Six things, all under `theme/`:

| File | What |
|---|---|
| `theme/web/custom-variables.scss` | the theme's palette — this is the file to edit |
| `theme/web/_mxcli-atlas-map.scss` | the Atlas wiring: ~60 Atlas variables expressed in terms of the palette |
| `theme/web/_mxcli-<name>.scss` | the other palette, the variant blocks, `@font-face`, recipe classes |
| `theme/web/_mxcli-widgets.scss` | the widget-module layer: colours Sass bakes before any token exists |
| `theme/web/main.scss` | the variant switch plus the `@import` lines |
| `theme/web/mxcli-fonts/` | vendored fonts (SIL OFL 1.1) |

**The model is never touched.** No `.mpr` changes, so nothing here can affect a
build, and the theme hot-applies under `mxcli run --local --watch`.

**Atlas Core is never touched either.** Because Atlas components read the brand
tokens, retuning them cascades into buttons, backgrounds, form inputs, cards,
modals and the brand-aware pluggable widgets (Switch, Slider, ProgressBar,
BadgeButton) with no per-widget CSS. That is also why the project stays
upgradable across Mendix releases.

## Light and dark

`--variant auto` is the default and ships both palettes:

```bash
mxcli theme apply signal -p app.mpr                  # auto
mxcli theme apply signal -p app.mpr --variant dark   # bake one palette, no switching
```

Under `auto` the app follows the operating system's `prefers-color-scheme`
**before first paint** — no flash, no script — and honours a `theme-light` or
`theme-dark` class on the root element when something sets one.

Mendix ships that slot (`theme/web/_theme-dark.scss` declares `:root.theme-dark`)
but nothing that applies it, and its palette is stock Mendix blue. An mxcli theme
re-declares the same selector from a file that compiles later, so the theme's own
dark palette wins.

### A user-facing toggle

```bash
mxcli theme switcher install -p app.mpr --module MyFirstModule
```

**This is the one theme command that writes to the model.** It has to: the class
has to be set by something the browser can run, and there is no theme-level hook
to run script before first paint. It creates three JavaScript actions
(`ToggleAppTheme`, `SetAppTheme`, `ApplyStoredTheme`) and a nanoflow, then you
wire a button:

```sql
actionbutton btnTheme (caption: 'Theme', action: nanoflow MyFirstModule.ACT_ToggleTheme)
```

A click flips the palette and remembers the choice in `localStorage`. The class
goes on `<html>`, so popups and modals — which Mendix renders at `<body>`,
outside any page container — follow it too.

**Known limit:** after a reload the app goes back to following the OS. Mendix has
no page on-load event to re-apply the stored value, and the usual substitute (a
data view with a nanoflow data source) is not authorable by mxcli on either
engine yet. `ApplyStoredTheme` is installed and ready — wire it in Studio Pro if
you need the choice to persist across reloads.

## A theme this project owns

The three built-in themes are a house style, not a brand. `theme create`
scaffolds a fourth into `theme/mxcli-themes/<name>/`, and from then on it is a
theme like any other — `theme list -p` shows it, `theme apply <name>` installs
it, `theme remove` takes it out.

```bash
mxcli theme create acme -p app.mpr                        # scaffold from signal
mxcli theme create acme -p app.mpr --from console         # ...or from console
mxcli theme create acme -p app.mpr --from design.css      # ...and seed the palette
mxcli theme apply acme -p app.mpr
```

That folder is the right place because of two constraints. It is **committed** —
a theme derived from a design is source the team shares, which rules out
`.mxcli/`, that `mxcli init` puts in `.gitignore`. And it is **not compiled** —
mxbuild's entry point is `theme/web/main.scss` and it does not glob `theme/` for
other stylesheets, so the sources sit inert until `theme apply` copies them into
`theme/web/`. (Verified against a real 11.13 build: nothing under
`theme/mxcli-themes/` reaches `deployment/`.)

Scaffolding copies an existing theme rather than starting from nothing, so the
Atlas wiring and the widget layer — identical in every theme, and where most of
the hard-won detail lives — come across byte for byte. What you edit is the
palette. A local theme named after a built-in shadows it, so "signal, but with
our brand" is a valid thing to create.

### Seeding a palette from a design

A theme's palette is nothing but `--mxt-*` custom properties, so a design tool
seeds one by declaring them. That is the whole contract — anywhere in a `.css`,
`.scss` or `.html` file:

```css
:root { --mxt-brand: #7f5af0; --mxt-ground: #fffffe; }
@media (prefers-color-scheme: dark) { :root { --mxt-ground: #16161a; } }
```

Declarations inside a dark block (`prefers-color-scheme: dark`, `.theme-dark`,
`[data-theme="dark"]`) seed the dark palette; everything else seeds the light
one. Tokens the design does not name keep the base theme's value, so a
three-colour design still yields a complete, working palette.

`mxcli theme show signal` prints the vocabulary. A `--mxt-*` name the base theme
does not declare is **an error**, not an extra: nothing would read it, so the
theme would apply cleanly and render unchanged — the one failure mode this path
exists to avoid.

Inferring a palette from a mockup's inline styles was the alternative, and it is
a guess: two greys in a design do not say which is the app ground and which is a
hovered row. Ask the design step to emit a token block instead.

## Re-branding

Change one line in `theme/web/custom-variables.scss`:

```scss
:root {
  --mxt-brand: #0f6e6b;   /* <- the one signal colour */
```

`_mxcli-atlas-map.scss` maps that onto `--brand-primary`, and Atlas builds the
whole derived ramp (`--brand-primary-50` … `-900`) from it with CSS
`color-mix()` — so buttons, links, active navigation and the brand-aware
pluggable widgets follow immediately, in both palettes.

The palette file declares only `--mxt-*` tokens, never Atlas variables directly.
That is what makes a variant cheap: the dark block restates about thirty values
instead of rewiring sixty. Pinning an Atlas variable to a literal colour is the
one thing that breaks switching — a hardcoded `--font-color-default` is
invisible the moment the ground goes dark.

### Import order in `main.scss`

`apply` appends its block to the **end** of `theme/web/main.scss`, so it lands
after any `@import` the project already had there. If your app carries a
stylesheet imported last specifically to win the cascade over Atlas, the theme's
partial now comes after it. Higher-specificity app classes are unaffected; a
rule that relied purely on being last is not. Move your `@import` below the
mxcli block — anything outside the fence is never touched.

### Two Atlas constraints worth knowing

- **The navigation rail stays dark in both palettes.** Atlas topbar widgets paint
  their own text assuming a dark rail — the language selector uses
  `--bg-color-secondary` with a `#fff` fallback, at a specificity
  (`.navbar-brand .widget-language-selector .current-language-text`) that a
  simple override cannot beat. Every mxcli theme keeps the rail dark and
  re-declares those selectors at matching specificity, resolving the colour
  through the rail token.
- **`themesource/<name>/` is only compiled when `<name>` matches a real module**,
  so a theme never writes there. `theme/web/main.scss` compiles last and is the
  correct home for app-level styling.
- **The widget modules bake some colours as Sass literals**, before any custom
  property exists, so no token can move them — the Data Grid 2 pager caption is
  the worst case, at 1.02:1 on a dark ground. `_mxcli-widgets.scss` corrects
  those with ordinary rules; it is regenerated with the theme, so leave it alone
  and put your own overrides outside the fence.

## Recipe classes

Apply with `class:` on any widget.

| Class | Use |
|---|---|
| `num` | monospace with tabular figures — ids, amounts, dates, so columns align |
| `num-right` | right-align a numeric column |
| `pill` + `pill-ok` / `pill-warn` / `pill-risk` / `pill-info` | status pills; pair with a `dynamicclasses` expression mapping an enum |
| `stat` + `stat-label` / `stat-value` / `stat-delta` / `stat-delta-up` / `stat-delta-down` | KPI tile |
| `density-compact` | 28px rows and inputs inside this container |

## Your edits are safe

Every generated region is fenced, and the closing marker records a digest of what
mxcli wrote:

```scss
// mxcli:theme:begin signal v1 — generated by `mxcli theme apply`; edit outside this block
…
// mxcli:theme:end signal b538a13336af87f6
```

- Edit **outside** the markers and mxcli never touches your lines.
- Edit **inside** them and the next `apply` refuses rather than discarding your
  work, naming the file and offering `--force`.
- `apply` is idempotent — re-running an unchanged theme reports `unchanged` for
  every file.
- `remove` cuts the blocks back out and restores the file byte for byte.

## Verifying

A theme that compiles cleanly can still render wrong, so check a running app
rather than the build log:

```bash
mxcli run --local --watch --screenshot -p app.mpr
```

SCSS edits hot-apply, so `--watch` gives you a tight loop. If a rule seems not to
apply, first confirm it is compiled at all — grep for the selector in
`theme-cache/web/theme.compiled.css`. Absent and overridden look identical in the
browser, and only one of them is a specificity problem.

See also the `atlas-design` skill for the method behind the theme, and
`theme-styling` for the SCSS compilation chain — both are installed into your
project by `mxcli init`.
