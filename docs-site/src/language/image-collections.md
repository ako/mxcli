# Image Collections

Image collections are Mendix's way of bundling images (icons, logos, graphics) within a module. Each collection can contain multiple images in various formats (PNG, SVG, GIF, JPEG, BMP, WebP).

## Inspecting Image Collections

```sql
-- List all image collections across all modules
SHOW IMAGE COLLECTION;

-- Filter by module
SHOW IMAGE COLLECTION IN MyModule;

-- View full definition including embedded images
DESCRIBE IMAGE COLLECTION MyModule.AppIcons;
```

The `DESCRIBE` output includes the full `CREATE` statement. If the collection contains images, they are shown with `IMAGE 'name' FROM FILE 'path'` syntax that can be copied and re-executed. In the TUI, images are rendered inline when the terminal supports it (Kitty, iTerm2, Sixel).

## CREATE IMAGE COLLECTION

```sql
CREATE IMAGE COLLECTION <Module>.<Name>
  [EXPORT LEVEL 'Hidden'|'Public']
  [COMMENT '<description>']
  [(
    IMAGE <Name> FROM FILE '<path>',
    ...
  )];
```

| Option | Description | Default |
|--------|-------------|---------|
| `EXPORT LEVEL` | `'Hidden'` (internal to module) or `'Public'` (accessible from other modules) | `'Hidden'` |
| `COMMENT` | Documentation for the collection | (none) |
| `IMAGE Name FROM FILE` | Load an image from the filesystem into the collection | (none) |

The image format is detected automatically from the file extension. Relative paths are resolved from the current working directory. Supported formats: PNG, SVG, GIF, JPEG, BMP, WebP.

### Examples

```sql
-- Minimal: empty collection
CREATE IMAGE COLLECTION MyModule.AppIcons;

-- With export level
CREATE IMAGE COLLECTION MyModule.SharedIcons EXPORT LEVEL 'Public';

-- With comment
CREATE IMAGE COLLECTION MyModule.StatusIcons
  COMMENT 'Icons for order and task status indicators';

-- With images from files
CREATE IMAGE COLLECTION MyModule.NavigationIcons (
  IMAGE home FROM FILE 'assets/home.png',
  IMAGE settings FROM FILE 'assets/settings.svg'
);

-- All options combined
CREATE IMAGE COLLECTION MyModule.BrandAssets
  EXPORT LEVEL 'Public'
  COMMENT 'Company branding assets' (
  IMAGE logo_dark FROM FILE 'assets/logo-dark.png',
  IMAGE logo_light FROM FILE 'assets/logo-light.png'
);
```

## DROP IMAGE COLLECTION

Remove a collection and all its embedded images:

```sql
DROP IMAGE COLLECTION MyModule.StatusIcons;
```

## Icon collections (read-only)

Distinct from image collections, **icon collections**
(`CustomIcons$CustomIconCollection`, e.g. `Atlas_Core.Atlas_Filled`) ship with the
theme/Atlas. Their icons are referenced from a widget as
`Module.Collection.IconName` — most commonly a button's `Icon:` property. They're
read-only in mxcli; use `SHOW` / `DESCRIBE` to discover valid icon names (icons
have non-obvious names — it's `add`, not `plus`):

```sql
SHOW ICON COLLECTIONS;                              -- name, prefix, export level, icon count
DESCRIBE ICON COLLECTION Atlas_Core.Atlas_Filled;   -- every icon + its reference form
```

Then use one on a button:

```sql
ACTIONBUTTON btnEdit (Caption: 'Edit', Action: PAGE App.Edit,
  Icon: 'Atlas_Core.Atlas_Filled.pencil')
```

## See Also

- [MDL Quick Reference](../appendixes/quick-reference.md) -- syntax summary table
