---
title: SHOW/DESCRIBE Image Collections
status: proposed
---

# Proposal: SHOW/DESCRIBE Image Collections

## Overview

**Document type:** `Images$ImageCollection`
**Prevalence:** 65 across test projects (20 Enquiries, 25 Evora, 20 Lato)
**Priority:** Medium — present in every project, contains image assets

Image Collections group image resources used in pages and navigation. Each collection contains named images with their format (PNG, SVG, JPEG, etc.).

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | Partial | `sdk/mpr/reader_types.go` — `ImageCollection{Name, Images[]{ID, Name}}` |
| **Parser** | Partial | `sdk/mpr/parser_misc.go` line 444 — Name + image names, no format/data |
| **Reader** | Yes | `ListImageCollections()` in `sdk/mpr/reader_types.go` |
| **Generated metamodel** | Yes | `generated/metamodel/types.go` line 2943 |

## BSON Structure (from test projects)

```
Images$ImageCollection:
  Name: string
  documentation: string
  Excluded: bool
  ExportLevel: string
  Images: []*Images$image
    - Name: string
    - image: binary (raw image data)
    - ImageFormat: string ("Png", "Svg", "Jpg", "Gif", "Bmp", "Webp", "Unknown")
```

## Proposed MDL Syntax

### SHOW IMAGE COLLECTIONS

```
show image COLLECTIONS [in module]
```

| Qualified Name | Module | Name | Images | Formats |
|----------------|--------|------|--------|---------|

Where "Images" is the count and "Formats" shows distinct formats used (e.g., "PNG, SVG").

### DESCRIBE IMAGE COLLECTION

```
describe image collection Module.Name
```

Output format:

```
image collection MyModule.Icons
{
  Arrow_Down: PNG
  Arrow_Up: PNG
  Logo: SVG
  Banner: JPG
  placeholder: PNG
};

-- (5 images)
/
```

Binary image data is not shown — only names and formats are listed.

## Implementation Steps

### 1. Enhance Parser (sdk/mpr/parser_misc.go)

Extend existing parser to capture `ImageFormat` for each image. Skip binary `image` data (not useful for MDL output).

Update `ImageCollection` struct to include `ImageFormat` per image.

### 2. Add AST Types

```go
ShowImageCollections    // in ShowObjectType enum
DescribeImageCollection // in DescribeObjectType enum
```

### 3. Add Grammar Rules

```antlr
image: 'IMAGE';
collection: 'COLLECTION';
COLLECTIONS: 'COLLECTIONS';

// show image COLLECTIONS [in module]
// describe image collection qualifiedName
```

### 4. Add Executor (mdl/executor/cmd_image_collections.go)

Standard show/describe pattern.

### 5. Add Autocomplete

```go
func (e *Executor) GetImageCollectionNames(moduleFilter string) []string
```

## Testing

- Verify against all 3 test projects
