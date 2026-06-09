---
title: Terminology Bridge
category: glossary
last-synced: 4e185f73
sources:
  - CLAUDE.md
  - sdk/mpr/parser_microflow.go
  - README.md
---

> **Do not duplicate**: Mendix-official terminology (link to [Mendix docs](https://docs.mendix.com/refguide/)), the full storage-name mapping table (CLAUDE.md is canonical), or MDL syntax tables (`docs/01-project/MDL_QUICK_REFERENCE.md`). This page only bridges names — it does not define syntax.

## What this is

A contributor moves through three overlapping vocabularies daily: **Mendix UI terms** (the labels in Studio Pro), **mxcli / SDK terms** (qualified names and MDL keywords), and **BSON storage names** (the raw `$Type` strings persisted inside the `.mpr`). The same concept often has a different name in each. This page maps them and points to the canonical home for each.

## Terms

- **Association** — a relationship between two entities. In MDL: `create association Mod.Child_Parent from Mod.Child to Mod.Parent`. Its BSON pointers are counter-intuitively named — see ParentPointer/ChildPointer below.
- **Built-in Widget** — a widget native to the page metamodel (dataview, textbox, datagrid), serialized as a typed BSON element. Contrast with Pluggable Widget. See [[architecture/widget-engine]].
- **BSON** — the binary document format Mendix uses to store each model element inside the `.mpr`. mxcli parses and writes it directly; the `$Type` field carries the storage name. See [[architecture/mpr-read-write]].
- **Entity** — a persistable or non-persistable data type in the domain model; stored as `DomainModels$Entity`.
- **MDL (Mendix Definition Language)** — mxcli's SQL-shaped DSL for querying and modifying the model (`show`, `describe`, `create`, `alter`, `drop`). See [[rationale/mdl-as-sql]]; full syntax in `docs/01-project/MDL_QUICK_REFERENCE.md`.
- **Marketplace vs API Catalog** — two different Mendix products mxcli surfaces via separate commands; easy to conflate. The **Marketplace** (`marketplace-api.mendix.com`, via `mxcli marketplace`) hosts installable modules, widgets, and themes — what you download and import into a project. The **API Catalog** (`catalog.mendix.com`, via `mxcli catalog`) is a registry of published OData/REST *services* and data-sharing assets. They are not interchangeable: marketplace content is absent from the Catalog index, and the two authenticate against different hosts. Usage lives in `docs-site/`.
- **Microflow vs Nanoflow** — both are visual logic flows; a microflow runs server-side (`Microflows$Microflow`), a nanoflow runs client-side in the browser/device. They share most activities but nanoflows disallow server-only actions.
- **Module** — the top-level container grouping entities, microflows, pages, and security; the unit of qualified-name prefixing (`MyModule.Customer`).
- **MPR (`.mpr`)** — the Mendix project file. v1 is a single SQLite database; v2 is metadata plus an `mprcontents/` folder of per-document files. See [[architecture/mpr-read-write]].
- **Page / Form** — Studio Pro calls it a Page; BSON stores it under the `Forms$` namespace because "Form" was the original term. `ShowPageAction`/`ClosePageAction` persist as `ShowFormAction`/`CloseFormAction`. See [[models/storage-vs-qualified-names]].
- **ParentPointer / ChildPointer** — inverted association pointers: `ParentPointer` points to the **FROM** entity (the FK owner), `ChildPointer` points to the **TO** entity (the referenced one). Entity access rules attach only to the FROM entity. See [[models/association-pointers]].
- **Pluggable Widget** — a marketplace/custom widget (DataGrid2, ComboBox, Gallery) whose BSON requires matching `type` (PropertyTypes schema) and `object` (default values) blocks. See [[architecture/widget-engine]] and [[bug-patterns/widget-type-object-drift]].
- **Qualified Name vs Storage Name** — the qualified name is what the SDK and docs show (e.g. `CreateObjectAction`); the storage name is what lands in the BSON `$Type` (e.g. `CreateChangeAction`). Using the wrong one triggers `TypeCacheUnknownTypeException` in Studio Pro. The full mapping table is in CLAUDE.md; the concept is explained in [[models/storage-vs-qualified-names]].

## See also

- [../CLAUDE.md](../CLAUDE.md) — canonical storage-name and association-pointer tables
- [[models/storage-vs-qualified-names]] — why names differ and how to find the right one
- [[models/association-pointers]] — the inverted ParentPointer/ChildPointer convention
- [[architecture/widget-engine]] — built-in vs pluggable widget serialization
- [[rationale/mdl-as-sql]] — what MDL is and why it reads like SQL
- [[positioning/vs-typescript-sdk]] — how mxcli relates to the official SDK
