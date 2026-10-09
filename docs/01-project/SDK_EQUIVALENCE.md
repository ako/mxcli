# Mendix Model SDK Go - Equivalence Analysis

This document provides a comprehensive analysis of the original Mendix Model SDK (TypeScript) and the approach for creating a fully equivalent Go implementation.

## Table of Contents

1. [Overview](#overview)
2. [Original SDK Architecture](#original-sdk-architecture)
3. [Current Go Implementation](#current-go-implementation)
4. [Gap Analysis](#gap-analysis)
5. [Code Generation Strategy](#code-generation-strategy)
6. [Maintenance Process](#maintenance-process)
7. [Implementation Roadmap](#implementation-roadmap)

---

## Overview

The goal is to create a 100% functional equivalent of the Mendix Model SDK for local Mendix projects in Go.

| Component | Purpose | Go Equivalent |
|-----------|---------|---------------|
| **mendixmodelsdk** | High-level OO API for model manipulation | `modelsdk-go` packages |

### Key Differences

- **Original SDK**: Cloud-first, connects to Mendix Team Server, real-time collaboration
- **Go Implementation**: Local-first, works directly with `.mpr` files, no cloud dependency

---

## Original SDK Architecture

### mendixmodelsdk (v4.105.0)

The high-level SDK providing:

- **52 metamodel domains** with 820+ type definitions per version
- **Delta-based change system** for CRUD operations with undo/redo
- **Cloud connectivity** for Team Server (SVN/Git)
- **Real-time synchronization** via Server-Sent Events
- **Working copy management** with locking and collaboration

#### Key Domains

| Domain | Description | Types |
|--------|-------------|-------|
| `domainmodels` | Entities, attributes, associations | ~50 |
| `microflows` | Microflow activities and flows | 150+ |
| `pages` | UI widgets, layouts, snippets | 500+ |
| `workflows` | BPM workflow definitions | 200+ |
| `rest` | REST/OData services | ~40 |
| `webservices` | SOAP web services | ~30 |
| `security` | Access rules, user roles | ~25 |
| `navigation` | Menus and navigation | ~20 |
| `mappings` | Import/export mappings | ~30 |
| `expressions` | Expression language | ~100 |
| ... | 42 more domains | ... |

### Metamodel Reflection Data

`cmd/codegen` reads metamodel reflection data: per-version JSON type
definitions. This data is not publicly distributed, so it is not in the
repository and cannot be redistributed with it; see
[PROPOSAL_codegen_ownership.md](../11-proposals/PROPOSAL_codegen_ownership.md) §4.

#### Reflection Data Structure

```
reflection-data/
├── 6.0.0-structures.json      # Mendix 6.0.0 type definitions
├── 6.0.0-storageNames.json    # storage name mappings
├── ...
├── 11.6.0-structures.json     # Latest version
└── 11.6.0-storageNames.json
```

Each `{version}-structures.json` contains:

```json
{
  "DomainModels$entity": {
    "qualifiedName": "DomainModels$entity",
    "storageName": "DomainModels$entity",
    "superTypeName": "DomainModels$MaybeRemotableElement",
    "abstract": false,
    "type": "ELEMENT",
    "properties": {
      "name": {
        "name": "name",
        "storageName": "Name",
        "list": false,
        "typeInfo": {
          "type": "PRIMITIVE",
          "primitiveType": "string"
        }
      },
      "attributes": {
        "name": "attributes",
        "storageName": "attributes",
        "list": true,
        "typeInfo": {
          "type": "ELEMENT",
          "elementType": "DomainModels$attribute",
          "kind": "PART"
        }
      }
      // ... more properties
    },
    "defaultSettings": {
      "name": "",
      "documentation": ""
    }
  }
}
```

---

## Current Go Implementation

### Package Structure

```
modelsdk-go/
├── modelsdk.go           # Public api entry points
├── model/                # Core types (ID, module, project, etc.)
├── domainmodel/          # entity, attribute, association types
├── microflows/           # microflow, nanoflow types
├── pages/                # page, layout, snippet types
├── mpr/                  # MPR file reading/writing
│   ├── reader.go         # read-only access
│   ├── writer.go         # read-write access
│   ├── parser.go         # BSON parsing
│   └── utils.go          # UUID generation, etc.
└── examples/             # Usage examples
```

### Implemented Features

| Feature | Status | Notes |
|---------|--------|-------|
| MPR v1/v2 reading | ✅ Complete | SQLite + BSON parsing |
| MPR v1/v2 writing | ✅ Complete | Transaction support |
| Module CRUD | ✅ Complete | |
| Entity CRUD | ✅ Complete | |
| Attribute types | ✅ Complete | 9 types |
| Association CRUD | ✅ Complete | |
| Microflow basic | ⚠️ Partial | Basic structure only |
| Nanoflow CRUD | ⚠️ Partial | CREATE/DROP/DESCRIBE/SHOW/RENAME/MOVE, GRANT/REVOKE, diff, JavaScript action calls, association retrieve roundtrip |
| Page basic | ⚠️ Partial | Basic structure only |
| JSON export | ✅ Complete | |

### Type Coverage Comparison

| Domain | TypeScript Lines | Go Lines | Coverage |
|--------|-----------------|----------|----------|
| domainmodels | 3,597 | ~478 | ~13% |
| microflows | 6,628 | ~200 | ~3% |
| pages | 34,062 | ~100 | <1% |
| workflows | 3,471 | 0 | 0% |
| rest | 2,453 | 0 | 0% |
| expressions | 7,591 | 0 | 0% |

---

## Gap Analysis

### Missing Metamodel Domains

48 of 52 domains are not implemented:

**High Priority:**
- `workflows` - Workflow/BPM definitions
- `rest` - REST service consumption/publishing
- `webservices` - SOAP web services
- `security` - Access rules, module roles
- `navigation` - Navigation profiles, menus
- `expressions` - Expression language types

**Medium Priority:**
- `mappings`, `importmappings`, `exportmappings`
- `jsonstructures`, `xmlschemas`
- `javaactions`, `javascriptactions`
- `customwidgets`
- `reports`, `datasets`
- `documenttemplates`

**Lower Priority:**
- `kafka`, `queues`, `businessevents`
- `appservices`
- `mlmappings`
- `databaseconnector`
- `exceldataimporter`
- ... and more

### Missing Type Details

Even in implemented domains, many subtypes are missing:

**Microflows (150+ activities needed):**
- `ActionActivity`, `LoopedActivity`
- `CreateObjectAction`, `ChangeObjectAction`, `DeleteAction`
- `RetrieveAction`, `AggregateListAction`
- `MicroflowCallAction`, `JavaActionCallAction`
- `ShowMessageAction`, `ShowPageAction`
- `LogMessageAction`, `ValidationFeedbackAction`
- ... and 140+ more

**Pages (500+ widgets needed):**
- Container widgets: `dataview`, `listview`, `datagrid`, `TemplateGrid`
- Input widgets: `textbox`, `textarea`, `dropdown`, `datepicker`
- Button widgets: `actionbutton`, `linkbutton`
- Layout widgets: `container`, `groupbox`, `tabcontainer`
- ... and 480+ more

### Missing Infrastructure

| Component | Description | Status |
|-----------|-------------|--------|
| Metamodel reflection | Runtime type inspection | ❌ Not implemented |
| Version compatibility | Check type availability per version | ❌ Not implemented |
| Delta system | Change tracking with undo/redo | ❌ Not implemented |
| Property system | Type-safe property access | ❌ Not implemented |
| Qualified names | Path-based element resolution | ⚠️ Basic only |

---

## Code Generation Strategy

### Recommended Approach: Generate from Reflection Data

The reflection data contains complete metamodel definitions in JSON format. We can generate Go code automatically from these definitions.

### Benefits

1. **Accuracy**: Generated types match the official metamodel exactly
2. **Maintainability**: New Mendix versions only require running the generator
3. **Completeness**: All 820+ types per version can be generated
4. **Version support**: Can generate version-specific types or union types

### Generator Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Code Generator                            │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │   Parser     │───▶│  transformer │───▶│   Emitter    │  │
│  │              │    │              │    │              │  │
│  │ read json    │    │ build type   │    │ generate Go  │  │
│  │ metamodel    │    │ hierarchy    │    │ source code  │  │
│  └──────────────┘    └──────────────┘    └──────────────┘  │
│         │                   │                   │           │
│         ▼                   ▼                   ▼           │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │ structures   │    │ Go type      │    │ .go files    │  │
│  │ .json        │    │ definitions  │    │ per domain   │  │
│  └──────────────┘    └──────────────┘    └──────────────┘  │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Generator Implementation

```go
// cmd/generate/main.go
package main

// Generator reads reflection data and produces Go types
type Generator struct {
    version     string           // Target Mendix version
    structures  map[string]*structure
    OutputDir   string
}

// structure from reflection-data json
type structure struct {
    QualifiedName    string              `json:"qualifiedName"`
    StorageName      string              `json:"storageName"`
    SuperTypeName    string              `json:"superTypeName,omitempty"`
    Abstract         bool                `json:"abstract"`
    type             string              `json:"type"` // ELEMENT, MODEL_UNIT, STRUCTURAL_UNIT
    properties       map[string]*Property `json:"properties"`
    DefaultSettings  map[string]any      `json:"defaultSettings"`
}

// Property definition
type Property struct {
    Name        string    `json:"name"`
    StorageName string    `json:"storageName"`
    list        bool      `json:"list"`
    Public      bool      `json:"public"`
    TypeInfo    *TypeInfo `json:"typeInfo"`
}

// TypeInfo for property types
type TypeInfo struct {
    type          string `json:"type"` // PRIMITIVE, ELEMENT, enumeration, UNIT
    PrimitiveType string `json:"primitiveType,omitempty"`
    ElementType   string `json:"elementType,omitempty"`
    Kind          string `json:"kind,omitempty"` // PART, BY_ID_REFERENCE, BY_NAME_REFERENCE
}
```

### Generated Output Structure

```
generated/
├── domainmodels/
│   ├── entity.go
│   ├── attribute.go
│   ├── association.go
│   └── types.go
├── microflows/
│   ├── microflow.go
│   ├── activities.go      # 150+ activity types
│   ├── actions.go         # action implementations
│   └── types.go
├── pages/
│   ├── page.go
│   ├── widgets.go         # 500+ widget types
│   ├── layouts.go
│   └── types.go
├── workflows/
│   ├── workflow.go
│   ├── activities.go
│   └── types.go
└── ... (48 more domains)
```

### Type Mapping

| JSON Type | Go Type |
|-----------|---------|
| `PRIMITIVE/string` | `string` |
| `PRIMITIVE/integer` | `int64` |
| `PRIMITIVE/DOUBLE` | `float64` |
| `PRIMITIVE/boolean` | `bool` |
| `PRIMITIVE/DATE_TIME` | `time.Time` |
| `PRIMITIVE/GUID` | `model.ID` |
| `PRIMITIVE/POINT` | `model.Point` |
| `PRIMITIVE/SIZE` | `model.Size` |
| `PRIMITIVE/COLOR` | `model.Color` |
| `PRIMITIVE/BLOB` | `[]byte` |
| `ELEMENT` (single) | `*TypeName` |
| `ELEMENT` (list) | `[]*TypeName` |
| `BY_ID_REFERENCE` | `model.ID` |
| `BY_NAME_REFERENCE` | `model.QualifiedName` |
| `enumeration` | Custom enum type |

### Example Generated Code

Input (`11.6.0-structures.json`):
```json
{
  "DomainModels$entity": {
    "qualifiedName": "DomainModels$entity",
    "superTypeName": "DomainModels$MaybeRemotableElement",
    "properties": {
      "name": {
        "storageName": "Name",
        "typeInfo": { "type": "PRIMITIVE", "primitiveType": "string" }
      },
      "attributes": {
        "storageName": "attributes",
        "list": true,
        "typeInfo": { "type": "ELEMENT", "elementType": "DomainModels$attribute", "kind": "PART" }
      },
      "generalization": {
        "storageName": "generalization",
        "typeInfo": { "type": "ELEMENT", "elementType": "DomainModels$GeneralizationBase", "kind": "PART" }
      }
    }
  }
}
```

Output (`generated/domainmodels/entity.go`):
```go
// Code generated by modelsdk-generator. DO not EDIT.
// source: 11.6.0-structures.json

package domainmodels

import "github.com/mendixlabs/mxcli/model"

// entity represents a DomainModels$entity element.
type entity struct {
    model.BaseElement

    // Name is the entity name (storage: Name)
    Name string `json:"name" bson:"Name"`

    // attributes contains the entity's attributes (storage: Attributes)
    attributes []*attribute `json:"attributes,omitempty" bson:"attributes"`

    // generalization defines inheritance (storage: generalization)
    generalization GeneralizationBase `json:"generalization,omitempty" bson:"generalization"`
}

// GetName returns the entity's name.
func (e *entity) GetName() string {
    return e.Name
}

// Ensure entity implements the required interfaces.
var (
    _ model.Element      = (*entity)(nil)
    _ model.NamedElement = (*entity)(nil)
)
```

---

## Maintenance Process

### Adding Support for New Mendix Versions

When a new Mendix version is released:

```
┌─────────────────────────────────────────────────────────────┐
│                  version update Process                      │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  1. Obtain new reflection data                               │
│     └─▶ {version}-structures.json for the new version        │
│                                                              │
│  2. run generator                                            │
│     └─▶ go run cmd/generate/main.go -version=11.7.0         │
│                                                              │
│  3. Review changes                                           │
│     └─▶ git diff generated/                                 │
│                                                              │
│  4. update parser mappings                                   │
│     └─▶ add new type handlers if needed                     │
│                                                              │
│  5. run tests                                                │
│     └─▶ go test ./...                                       │
│                                                              │
│  6. update version constants                                 │
│     └─▶ add to supported versions list                      │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Step-by-Step Update Guide

#### 1. Obtain Reflection Data

The reflection data is not publicly distributed; see
[PROPOSAL_codegen_ownership.md](../11-proposals/PROPOSAL_codegen_ownership.md)
§4 and §4A for the licence-clean replacement.

#### 2. Run Generator

```bash
# generate for specific version
go run cmd/generate/main.go -version=11.7.0 -output=generated/

# generate for latest version
go run cmd/generate/main.go -latest -output=generated/

# generate for all versions (creates version-specific packages)
go run cmd/generate/main.go -all -output=generated/
```

#### 3. Review Generated Changes

```bash
# See what changed
git diff generated/

# check for breaking changes
go build ./...
go test ./...
```

#### 4. Update Version Support

```go
// version/versions.go
var SupportedVersions = []string{
    "10.0.0",
    "10.21.0",
    "11.0.0",
    "11.6.0",
    "11.7.0", // NEW
}

var LatestVersion = "11.7.0"
```

---

## Implementation Roadmap

### Phase 1: Generator Foundation (Week 1-2)

- [ ] Create `cmd/generate/` package
- [ ] Implement JSON parser for reflection data
- [ ] Implement basic Go code emitter
- [ ] Generate `domainmodels` package as proof of concept
- [ ] Validate generated code compiles

### Phase 2: Core Domains (Week 3-4)

- [ ] Generate `microflows` with all 150+ activity types
- [ ] Generate `pages` with all 500+ widget types
- [ ] Generate `workflows` domain
- [ ] Update parser to use generated types

### Phase 3: Complete Domains (Week 5-6)

- [ ] Generate remaining 48 domains
- [ ] Implement enum types
- [ ] Add interface generation for polymorphic types
- [ ] Add BSON tags for serialization

### Phase 4: Infrastructure (Week 7-8)

- [ ] Implement metamodel reflection system
- [ ] Add version compatibility checking
- [ ] Implement delta change tracking
- [ ] Add qualified name resolution

### Phase 5: Testing & Documentation (Week 9-10)

- [ ] Add comprehensive unit tests
- [ ] Test against real Mendix projects
- [ ] Document API usage
- [ ] Create migration guide from TypeScript SDK

---

## Appendix: Metamodel Statistics

### Type Counts by Domain (v11.6.0)

| Domain | Types | Properties |
|--------|-------|------------|
| pages | 312 | 2,847 |
| microflows | 156 | 1,203 |
| expressions | 89 | 412 |
| workflows | 78 | 534 |
| domainmodels | 52 | 389 |
| rest | 45 | 312 |
| customwidgets | 38 | 267 |
| reports | 36 | 289 |
| webservices | 34 | 245 |
| datatypes | 28 | 156 |
| ... | ... | ... |
| **Total** | **~820** | **~7,500** |

### Supported Mendix Versions

```
6.x:  6.0.0 → 6.10.4  (11 versions)
7.x:  7.0.0 → 7.23.0  (24 versions)
8.x:  8.0.0 → 8.18.0  (19 versions)
9.x:  9.0.0 → 9.24.0  (26 versions)
10.x: 10.0.0 → 10.21.0 (22 versions)
11.x: 11.0.0 → 11.6.0  (7 versions)
────────────────────────────────
Total: 88 versions supported
```

---

## References

- [Mendix Model SDK Documentation](https://docs.mendix.com/apidocs-mxsdk/mxsdk/)
- [mendixmodelsdk npm package](https://www.npmjs.com/package/mendixmodelsdk)
- [Mendix MPR File Format](https://docs.mendix.com/refguide/mpr-format/)
