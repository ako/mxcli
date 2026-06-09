# Wiki Sync Log

Append-only audit trail for every `/mxcli-dev:wiki-sync` run. Records *what
triggered the resynth* and *what sources were read* — information git does
not capture, because sources are upstream of the commit.

**Rules** (enforced by [`maintain-wiki` skill](../.claude/skills/maintain-wiki.md)):

- Append only. Never edit historical rows.
- The Sources column lists what was **actually read**, not what was relevant.
- Write the row as the final step of every sync, even if the synthesis
  produced no diff.

| Date | Page | Sources read | Note |
|------|------|--------------|------|
| 2026-05-24 | architecture/mdl-execution.md | mdl/grammar/MDLParser.g4, mdl/visitor/visitor.go, mdl/executor/executor.go, mdl/backend/{backend,doc,domainmodel}.go, docs/03-development/MDL_PARSER_ARCHITECTURE.md | Initial synthesis: five-layer grammar→visitor→executor→backend pipeline and the seam-level failure modes |
| 2026-05-24 | architecture/mpr-read-write.md | sdk/mpr/reader.go, sdk/mpr/writer_core.go, sdk/mpr/parser.go, sdk/mpr/writer_widgets.go, modelsdk.go | Initial synthesis: v1/v2 format detection, reader/writer nesting, staged WriteTransaction, why the writer is BSON-aware. Dropped non-existent sdk/mpr/writer.go |
| 2026-05-24 | architecture/widget-engine.md | sdk/widgets/definitions/loader.go, sdk/widgets/definitions/combobox.def.json, sdk/widgets/templates/README.md, sdk/mpr/writer_widgets.go, mdl/executor/cmd_pages_builder_v3_widgets.go, docs/03-development/{PAGE_BSON_SERIALIZATION,WIDGET_BSON_VERSION_COMPATIBILITY}.md | Initial synthesis: dual type/object template, .def.json mapping + 3-tier registry, two-layer version resilience |
| 2026-05-24 | models/association-pointers.md | CLAUDE.md, sdk/mpr/writer_domainmodel.go, sdk/domainmodel/domainmodel.go | Initial synthesis: ParentPointer=FROM / ChildPointer=TO inversion and the CE0066 member-access consequence. Narrowed dir source to domainmodel.go |
| 2026-05-24 | models/storage-vs-qualified-names.md | CLAUDE.md, sdk/mpr/parser_microflow.go | Initial synthesis: two naming systems, Form=Page history, strict-write/lenient-read asymmetry. Dropped non-existent reference/mendixmodellib/reflection-data/ |
| 2026-05-24 | models/version-gating.md | sdk/versions/registry.go, sdk/versions/mendix-11.yaml, mdl/executor/cmd_features.go, .claude/skills/version-awareness.md | Initial synthesis: registry as source of truth, checkFeature() pre-check contract and its escape hatches. Narrowed dir sources to specific files |
| 2026-05-24 | rationale/mdl-as-sql.md | docs/13-decisions/0003-mdl-is-sql-shaped.md, .claude/skills/design-mdl-syntax.md, docs/11-proposals/PROPOSAL_mdl_syntax_design_guidelines.md, docs/01-project/MDL_QUICK_REFERENCE.md | Initial synthesis: citizen-developer design tradition, PL/SQL pattern for imperative constructs, verbosity trade-off. Cites ADR-0003 |
| 2026-05-24 | rationale/backend-abstraction.md | docs/13-decisions/0002-backend-abstraction.md, mdl/backend/{doc,backend,domainmodel}.go, CLAUDE.md | Initial synthesis: executor-is-wrong-layer-for-BSON seam, composed-domain interfaces, four-touch overhead. Cites ADR-0002 |
| 2026-05-24 | positioning/vs-typescript-sdk.md | docs/01-project/SDK_EQUIVALENCE.md, README.md | Initial synthesis: local-vs-cloud, SQL-shaped DSL, pure-Go; "which tool for which job". Dropped non-existent reference/mendixmodelsdk/ |
| 2026-05-24 | glossary.md | CLAUDE.md, sdk/mpr/parser_microflow.go, README.md | Initial synthesis: three-vocabulary bridge (Mendix UI / mxcli-SDK / BSON storage names). Dropped non-existent reference/mendixmodellib/reflection-data/ |
| 2026-05-24 | bug-patterns/bson-numeric-width.md | .claude/skills/fix-issue.md, sdk/mpr/parser.go | Initial synthesis: silent zero-defaulting from narrow int32 assertions; extractInt as canonical fix |
| 2026-05-24 | bug-patterns/visitor-wiring-gaps.md | .claude/skills/fix-issue.md, mdl/visitor/visitor_enumeration.go | Initial synthesis: parsed-but-not-stored pattern; the one hand-written bridge with no compiler check. Narrowed dir source to visitor_enumeration.go |
| 2026-05-24 | bug-patterns/widget-type-object-drift.md | .claude/skills/fix-issue.md, .claude/skills/debug-bson.md, sdk/widgets/templates/README.md, sdk/mpr/writer_widgets.go | Initial synthesis: Type↔Object coupling, CE0463 cascade, mx-check-tolerance trap |
| 2026-06-09 | glossary.md | cmd/mxcli/cmd_marketplace.go, cmd/mxcli/cmd_catalog.go, internal/auth/scheme.go | Added "Marketplace vs API Catalog" term: two distinct Mendix products (marketplace-api vs catalog.mendix.com) on separate commands, not interchangeable. Triggered by a tester conflating them |
