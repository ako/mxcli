// SPDX-License-Identifier: Apache-2.0

// cmd_pages_describe_datasource.go — reading a widget's datasource out of page
// BSON, and rendering it back as MDL.
//
// There used to be five copies of the read switch and six of the write switch,
// one per widget family, and they disagreed about which datasource `$Type`s
// exist. The pluggable copy — the one every Gallery, DataGrid2 and chart goes
// through — knew about microflows and nanoflows but not associations, selection
// targets or context sources, so a Gallery bound over an association described
// back with no datasource at all while a ListView bound the same way kept it
// (#941). The write side had the mirror-image defect: the object-list item
// emitter had no type switch, so a chart series bound to a microflow described
// as `database from Module.TheMicroflow` and re-executing it reported
// "entity not found".
//
// A datasource is the same thing whatever contains it, so it is read and
// rendered in one place. Adding a widget family does not mean copying a switch,
// and adding a `$Type` means one case rather than five.
package executor

import (
	"fmt"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"strings"
)

// Datasource storage names. The set is the metamodel's `DataSource` interface
// (generated/metamodel/types.go): AssociationSource, CustomWidgetXPathSource,
// DataViewSource, GridXPathSource, ImageViewerSource, ListViewXPathSource,
// ListenTargetSource, MicroflowSource, NanoflowSource, ReferenceSetSource —
// plus the CustomWidgets-namespaced nanoflow source seen in stored documents.
const (
	dsTypeMicroflow         = "Forms$MicroflowSource"
	dsTypeNanoflow          = "Forms$NanoflowSource"
	dsTypeCustomNanoflow    = "CustomWidgets$CustomWidgetNanoflowSource"
	dsTypeDatabase          = "Forms$DatabaseSource"
	dsTypeCustomWidgetXPath = "CustomWidgets$CustomWidgetXPathSource"
	dsTypeListViewXPath     = "Forms$ListViewXPathSource"
	dsTypeGridXPath         = "Forms$GridXPathSource"
	dsTypeReferenceSet      = "Forms$ReferenceSetSource"
	dsTypeImageViewer       = "Forms$ImageViewerSource"
	dsTypeAssociation       = "Forms$AssociationSource"
	dsTypeDataView          = "Forms$DataViewSource"
	dsTypeEntityPath        = "Forms$EntityPathSource"
	dsTypeListenTarget      = "Forms$ListenTargetSource"
)

// entityBackedSourceTypes are the datasources that name an entity plus an
// optional XPath constraint and sort. They differ only in which container
// declares them, which is why they share one reader.
var entityBackedSourceTypes = map[string]bool{
	dsTypeDatabase:          true,
	dsTypeCustomWidgetXPath: true,
	dsTypeListViewXPath:     true,
	dsTypeGridXPath:         true,
	dsTypeReferenceSet:      true,
	dsTypeImageViewer:       true,
}

// parseDataSource reads any widget datasource. Returns nil when ds is not a
// datasource at all; a non-nil result with Unsupported set means the datasource
// is real but has no MDL spelling, which the emitter reports as a comment
// rather than dropping (silent loss) or rendering half of it (a parse error).
func parseDataSource(ds map[string]any) *rawDataSource {
	if ds == nil {
		return nil
	}
	dsType := extractString(ds["$Type"])
	if dsType == "" {
		return nil
	}

	// Known-but-empty and unknown are different facts and get different
	// answers. A known type whose payload is empty (a listen target with no
	// target) describes a datasource the model itself has not finished — there
	// is nothing to reproduce and nothing useful to say, so it yields nil, as it
	// always has. A type this build has never heard of is mxcli's gap, not the
	// model's, and is reported rather than guessed at or silently dropped.
	known := true
	switch {
	case dsType == dsTypeMicroflow:
		if mf := microflowSourceRef(ds); mf != "" {
			return &rawDataSource{Type: "microflow", Reference: mf, Args: flowSourceArgs(ds, "MicroflowSettings", mf)}
		}
	case dsType == dsTypeNanoflow:
		if nf := nanoflowSourceRef(ds); nf != "" {
			return &rawDataSource{Type: "nanoflow", Reference: nf, Args: flowSourceArgs(ds, "NanoflowSettings", nf)}
		}
	case dsType == dsTypeCustomNanoflow:
		if nf := extractString(ds["Nanoflow"]); nf != "" {
			return &rawDataSource{Type: "nanoflow", Reference: nf}
		}
	case dsType == dsTypeListenTarget:
		if target := extractString(ds["ListenTarget"]); target != "" {
			return &rawDataSource{Type: "selection", Reference: target}
		}
	case dsType == dsTypeAssociation:
		if path, ctxVar := associationSourcePath(ds); path != "" {
			return &rawDataSource{Type: "association", Reference: path, ContextVariable: ctxVar}
		}
	case dsType == dsTypeDataView || dsType == dsTypeEntityPath:
		if got := parseContextSource(ds); got != nil {
			return got
		}
	case entityBackedSourceTypes[dsType]:
		if got := parseEntitySource(ds); got != nil {
			return got
		}
		// An entity-backed source with no entity is really a context source:
		// Studio Pro stores a pluggable list bound over an association or to a
		// page parameter as an XPath source whose EntityRef is empty and whose
		// navigation lives in Steps / SourceVariable / EntityPath. Reading only
		// EntityRef produced `database from ` with an empty slot (#941).
		if got := parseContextSource(ds); got != nil {
			return got
		}
	default:
		known = false
	}

	if known {
		return nil
	}
	// A datasource type this build does not know. Reported, not guessed at: an
	// invented `database from` is a statement that cannot be re-executed, and
	// dropping it loses the binding without saying so.
	return &rawDataSource{Unsupported: dsType}
}

// parseEntitySource reads the entity, XPath and sort of an entity-backed
// datasource, or nil when it names no entity.
func parseEntitySource(ds map[string]any) *rawDataSource {
	entityRef, ok := ds["EntityRef"].(map[string]any)
	if !ok || entityRef == nil {
		return nil
	}
	entity := extractString(entityRef["Entity"])
	if entity == "" {
		return nil
	}
	result := &rawDataSource{
		Type:             "database",
		Reference:        entity,
		XPathConstraint:  extractString(ds["XPathConstraint"]),
		SortColumns:      parseSortColumns(ds),
		SearchAttributes: parseSearchAttributes(ds),
	}
	return result
}

// parseContextSource reads the "data from context" forms: over an association,
// or straight from a page parameter.
func parseContextSource(ds map[string]any) *rawDataSource {
	if path, ctxVar := associationSourcePath(ds); path != "" {
		return &rawDataSource{Type: "association", Reference: path, ContextVariable: ctxVar}
	}
	if sv, ok := ds["SourceVariable"].(map[string]any); ok && sv != nil {
		if param := extractString(sv["PageParameter"]); param != "" {
			return &rawDataSource{Type: "parameter", Reference: param}
		}
	}
	if entityPath := extractString(ds["EntityPath"]); entityPath != "" {
		return &rawDataSource{Type: "parameter", Reference: entityPath}
	}
	return nil
}

// parseSearchAttributes reads a List View search bar's attributes
// (Forms$ListViewSearch.SearchRefs). Each entry is a DomainModels$AttributeRef,
// the same element a sort item carries — so this reads the same field a sort
// column does, one level less deep.
//
// Its absence is not an error: every ListViewXPathSource carries a Search
// element and all 18 in a blank 11.12.2 project are empty, so "no search
// attributes" is the overwhelmingly common case (ako/mxcli#512).
func parseSearchAttributes(ds map[string]any) []string {
	search, ok := ds["Search"].(map[string]any)
	if !ok || search == nil {
		return nil
	}
	var out []string
	for _, item := range getBsonArrayElements(search["SearchRefs"]) {
		ref, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if name := shortAttributeName(extractString(ref["Attribute"])); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// parseSortColumns reads a datasource's sort, accepting both stored shapes:
// a GridSortBar (`SortBar.SortItems`, used by grid-like widgets) and a
// ListViewSort (`Sort.Paths`). Which one a datasource carries depends on its
// type, so reading whichever is present keeps this reader container-agnostic.
func parseSortColumns(ds map[string]any) []rawSortColumn {
	items := []any(nil)
	if sortBar, ok := ds["SortBar"].(map[string]any); ok && sortBar != nil {
		items = getBsonArrayElements(sortBar["SortItems"])
	}
	if len(items) == 0 {
		if sortObj, ok := ds["Sort"].(map[string]any); ok && sortObj != nil {
			items = getBsonArrayElements(sortObj["Paths"])
		}
	}
	var cols []rawSortColumn
	for _, item := range items {
		sortItem, ok := item.(map[string]any)
		if !ok {
			continue
		}
		col := rawSortColumn{Order: "asc"}
		if attrRef, ok := sortItem["AttributeRef"].(map[string]any); ok {
			col.Attribute = shortAttributeName(extractString(attrRef["Attribute"]))
			col.Associations = sortAttributeHops(attrRef)
		}
		if gridSortDirection(sortItem) == "Descending" {
			col.Order = "desc"
		}
		if col.Attribute != "" {
			cols = append(cols, col)
		}
	}
	return cols
}

// sortAttributeHops reads the association hops of a stored AttributeRef — its
// EntityRef.Steps, one DomainModels$EntityRefStep per hop. An own-entity
// attribute carries a DirectEntityRef or no EntityRef and yields none.
//
// Without this a sort over an association described as a bare attribute name,
// and the replay had to guess the hop back (mendixlabs/mxcli#1152).
func sortAttributeHops(attrRef map[string]any) []string {
	entityRef, ok := attrRef["EntityRef"].(map[string]any)
	if !ok || entityRef == nil {
		return nil
	}
	var hops []string
	for _, raw := range getBsonArrayElements(entityRef["Steps"]) {
		step, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if assoc := extractString(step["Association"]); assoc != "" {
			hops = append(hops, assoc)
		}
	}
	return hops
}

// sortColumnPath renders a sort column as `Assoc/.../Attribute`.
func sortColumnPath(col rawSortColumn) string {
	if len(col.Associations) == 0 {
		return col.Attribute
	}
	return strings.Join(append(append([]string{}, col.Associations...), col.Attribute), "/")
}

// dataSourceExpr renders a datasource as the MDL that reproduces it — the part
// after `DataSource: `. Returns "" when there is nothing to emit.
//
// One renderer for every widget, because a datasource means the same thing
// wherever it appears. The six hand-written switches this replaces disagreed:
// the object-list one had no switch at all and labelled everything `database`,
// and two others omitted the WHERE and SORT they had read (#941).
func dataSourceExpr(ds *rawDataSource) string {
	if ds == nil || ds.Unsupported != "" {
		return ""
	}
	switch ds.Type {
	case "database":
		// Never render a database source with no entity: `database from ` parses
		// `from` as the entity name and reports "entity not found: from".
		if ds.Reference == "" {
			return ""
		}
		expr := "database from " + ds.Reference
		if clause := xpathConstraintClause(ds.XPathConstraint); clause != "" {
			expr += " where " + clause
		}
		if len(ds.SortColumns) > 0 {
			parts := make([]string, 0, len(ds.SortColumns))
			for _, col := range ds.SortColumns {
				parts = append(parts, sortColumnPath(col)+" "+col.Order)
			}
			expr += " sort by " + strings.Join(parts, ", ")
		}
		// After `sort by`, matching the grammar's clause order so the emitted
		// text re-parses (ako/mxcli#512).
		if len(ds.SearchAttributes) > 0 {
			expr += " search by " + strings.Join(ds.SearchAttributes, ", ")
		}
		return expr
	case "microflow", "nanoflow":
		if ds.Reference == "" {
			return ""
		}
		expr := ds.Type + " " + ds.Reference
		if len(ds.Args) > 0 {
			parts := make([]string, 0, len(ds.Args))
			for _, arg := range ds.Args {
				parts = append(parts, arg.Name+": "+arg.Value)
			}
			expr += "(" + strings.Join(parts, ", ") + ")"
		}
		return expr
	case "parameter":
		if ds.Reference == "" {
			return ""
		}
		// A page parameter is stored bare ("Account") while an entity path
		// arrives already written as an expression ("$Account/Mod.Assoc"), and
		// both reach here as Type "parameter". Prefixing only the bare form
		// keeps each spelling as MDL writes it.
		if strings.HasPrefix(ds.Reference, "$") {
			return ds.Reference
		}
		return "$" + ds.Reference
	case "selection":
		if ds.Reference == "" {
			return ""
		}
		return "selection " + mdlIdent(ds.Reference)
	case "association":
		if ds.Reference == "" {
			return ""
		}
		return associationDataSourceExpr(ds)
	}
	return ""
}

// dataSourceProp renders the whole `DataSource: …` property, or "" when the
// datasource cannot be expressed.
func dataSourceProp(ds *rawDataSource) string {
	expr := dataSourceExpr(ds)
	if expr == "" {
		return ""
	}
	return "DataSource: " + expr
}

// dataSourceComment describes a datasource MDL cannot express, so a reader of
// the output learns the binding exists rather than silently losing it. Returns
// "" for a datasource that renders normally.
func dataSourceComment(ds *rawDataSource) string {
	if ds == nil {
		return ""
	}
	if ds.Unsupported != "" {
		return fmt.Sprintf("-- DataSource (%s) has no MDL spelling and is not reproduced here", ds.Unsupported)
	}
	if dataSourceExpr(ds) == "" && ds.Type != "" {
		return fmt.Sprintf("-- DataSource (%s) is incomplete in the model and is not reproduced here", ds.Type)
	}
	return ""
}

// appendDataSourceProp adds a widget's `DataSource: …` property, or — when the
// datasource cannot be spelled in MDL — a comment saying so.
//
// Every widget goes through this, so a datasource cannot be rendered one way in
// a DataView and another in a Gallery, which is the drift #941 was.
func appendDataSourceProp(props []string, ds *rawDataSource) []string {
	if prop := dataSourceProp(ds); prop != "" {
		return append(props, prop)
	}
	if comment := dataSourceComment(ds); comment != "" {
		return append(props, comment)
	}
	return props
}

// appendWidgetDataSources emits a widget's datasource properties: the named
// spelling when it has several, the generic `DataSource:` clause otherwise.
//
// Every branch of the emitter goes through this for the same reason they all go
// through appendDataSourceProp — a datasource must not be rendered one way in a
// DataView and another in a Gallery, which is the drift #941 was. A branch that
// read w.DataSource directly would silently keep describing a multi-source
// widget as single-source.
func appendWidgetDataSources(props []string, w rawWidget) []string {
	if len(w.NamedDataSources) > 0 {
		return appendNamedDataSourceProps(props, w.NamedDataSources)
	}
	return appendDataSourceProp(props, w.DataSource)
}

// appendNamedDataSourceProps adds one `<schemaKey>: <datasource>` property per
// datasource on a widget that has several, so a describe → exec round trip keeps
// each binding on the mapping it came from.
//
// A source whose schema key did not resolve falls back to the unnamed
// `DataSource:` spelling — the output it would have had before there was a key
// to print. Losing it instead would be #956 with extra steps.
func appendNamedDataSourceProps(props []string, sources []rawNamedDataSource) []string {
	for _, src := range sources {
		if src.Key == "" {
			props = appendDataSourceProp(props, src.DataSource)
			continue
		}
		if expr := dataSourceExpr(src.DataSource); expr != "" {
			props = append(props, fmt.Sprintf("%s: %s", src.Key, expr))
			continue
		}
		// Not spellable in MDL — say so under this key, so a reader learns which
		// of the widget's bindings is the one that did not come through.
		if comment := dataSourceComment(src.DataSource); comment != "" {
			props = append(props, strings.Replace(comment, "-- DataSource ", "-- "+src.Key+" ", 1))
		}
	}
	return props
}

// xpathConstraintClause renders a stored XPath constraint as the MDL the page
// grammar accepts after WHERE, or "" when there is no constraint.
//
// Always the bracketed `xpathConstraint` production, never a bare expression.
// The grammar takes either, and the bare form reads more nicely — but it only
// parses for simple comparisons, and whether a given XPath happens to also be a
// valid MDL expression is not a question this emitter can answer. Emitting it
// raw is what turned stock Administration.Account_New from describing cleanly
// into describing to MDL that does not parse: its constraint carries an inner
// predicate (`System.grantableRoles[reversed()]/…`), and the `[` ends the
// expression parse (mxcli-formula1 §57.1). A bracketed group is accepted for
// every constraint, so the emitter does not have to be right about which is
// which.
//
// Splitting goes through the same quote- and nesting-aware helper the microflow
// emitter uses (#772) rather than a second copy: the previous code here took
// the outer brackets off by testing the first and last byte, which turns
// `[a][b]` into the mangled `a][b`.
func xpathConstraintClause(constraint string) string {
	xpath := strings.TrimSpace(constraint)
	if xpath == "" {
		return ""
	}
	// A stored constraint may be broken across lines — mxcli formats a long one
	// that way so it can be read in Studio Pro's editor, and a person may have
	// done the same by hand (upstream #979). MDL keeps it on one line: the
	// datasource is one property among several on a widget, and the executor
	// re-derives the stored layout from the expression anyway.
	xpath = visitor.FlattenXPathConstraint(xpath)
	if groups := visitor.SplitXPathPredicateGroups(xpath); len(groups) > 0 {
		return strings.Join(groups, " ")
	}
	return "[" + xpath + "]"
}

// flowSourceArgs reads the argument bindings of a microflow or nanoflow
// datasource from its settings sub-document.
//
// Storage qualifies each parameter with the flow it belongs to
// ("Mod.DS_Filtered.Term"), while MDL names the parameter alone — the flow is
// already on the left of the parentheses. The prefix is stripped by matching
// the flow's own qualified name rather than by cutting at the last dot, so a
// document that stores the name bare is left alone instead of losing its first
// segment.
//
// The bound value lives in Expression for a literal or an expression, and in
// Variable — a Forms$PageVariable naming a page parameter, snippet parameter or
// page variable — for a $-reference. Reading only Expression, or reading Variable
// as a flat string, drops every argument Studio Pro wrote (#1140).
// A parameterless flow yields nil, which the renderer emits without parentheses
// — the grammar makes the list optional, and adding empty parens would churn
// every existing description.
func flowSourceArgs(ds map[string]any, settingsKey, flowName string) []rawDataSourceArg {
	settings, ok := ds[settingsKey].(map[string]any)
	if !ok || settings == nil {
		return nil
	}
	var out []rawDataSourceArg
	for _, item := range getBsonArrayElements(settings["ParameterMappings"]) {
		mapping, ok := item.(map[string]any)
		if !ok || mapping == nil {
			continue
		}
		name := strings.TrimPrefix(extractString(mapping["Parameter"]), flowName+".")
		if name == "" {
			continue
		}
		value := extractString(mapping["Expression"])
		if value == "" {
			value = pageVariableArgValue(mapping["Variable"])
		}
		if value == "" {
			// A pre-#1140 document, or another writer, may have put the bare
			// reference text in Variable.
			value = extractString(mapping["Variable"])
		}
		if value == "" {
			// A parameter with no bound value is what CE1571 is about. There is
			// nothing to reproduce, and inventing one would be worse than
			// leaving the gap the model already has.
			continue
		}
		out = append(out, rawDataSourceArg{Name: name, Value: value})
	}
	return out
}
