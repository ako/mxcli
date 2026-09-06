// SPDX-License-Identifier: Apache-2.0

package catalog

// insertWidgetRefs emits the `widget` edge: one row per (page or snippet) x
// widget definition actually used on it.
//
// Slice 5 of PROPOSAL_def_driven_widget_bodies.md. A widget was the one MDL
// extension point with no edge in CATALOG.REFS, so "which pages use this
// widget?" was unanswerable while the same question about a Java action was one
// query. It is also the question an upgrade asks: a .mpk shipped in widgets/
// that no page uses is dead weight, and one used on forty pages is not
// something to swap lightly.
//
// Both halves are already in the catalog, so this is a projection and costs no
// extra parse: widgets_data.WidgetType carries the widget ID for a pluggable or
// custom widget (buildPages resolves Type.WidgetId out of the
// CustomWidgets$CustomWidget wrapper), and widget_definitions_data is keyed by
// that same ID.
//
// # Only widgets that resolve to a definition
//
// The join is the filter. A built-in Mendix widget stores its BSON $Type in the
// same column (Forms$DynamicText, Forms$ActionButton, ...) and has no
// definition, so it gets no edge — deliberately. An edge is a pointer to
// something describable, and `Forms$TextBox` is a language primitive, not a
// document: emitting one would put a target in the graph that nothing can
// resolve. "Which pages have a text box?" is already answerable directly from
// CATALOG.WIDGETS.
//
// # TargetName is the MDL name, not the widget ID
//
// Measured on testdata/expr-checker (15 widget edges either way), with the
// widget ID as TargetName:
//
//	graph_module_coupling   gains a module "com" with 14 edges, from three
//	                        different source modules
//	graph_god_nodes         reports com.mendix.widget.web.image.Image with
//	                        ModuleName "com"
//
// Those views derive a module by taking everything before the FIRST dot, which
// is sound for a qualified name and nonsense for a dotted widget ID. Using the
// MDL name (IMAGE, COMBOBOX) leaves graph_module_coupling identical to the
// baseline, because its existing instr(TargetName, '.') > 0 guard skips a
// non-dotted target for free. It is also the spelling a user has in hand:
// `show references to combobox` is what you type after `describe widget
// combobox`, and it is the keyword the page body uses.
//
// The widget ID is not lost — it goes in TargetId, which is what that column is
// for. Two packages shipping the same MDL name would share a TargetName and be
// told apart by TargetId; SHOW REFERENCES would list both, which is a better
// failure than being unable to name the widget at all.
//
// # One edge per page, not per instance
//
// DISTINCT collapses the seven comboboxes on a page into one edge, matching the
// four sibling projections in buildReferences. Per-instance rows would say
// nothing SHOW REFERENCES or SHOW IMPACT could use, and CATALOG.WIDGETS already
// holds the instances.
func insertWidgetRefs(tx CatalogTx, projectID, snapshotID string) (int, error) {
	res, err := tx.Exec(
		`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
		 SELECT DISTINCT w.ContainerType, '', w.ContainerQualifiedName,
		        'WIDGET', d.WidgetId, d.MdlName, ?, w.ModuleName, ?, ?
		 FROM widgets_data w
		 JOIN widget_definitions_data d ON d.WidgetId = w.WidgetType
		 WHERE w.ContainerQualifiedName != '' AND d.MdlName != ''`,
		RefKindWidget, projectID, snapshotID)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}
