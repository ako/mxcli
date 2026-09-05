// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// execShowCallers handles SHOW CALLERS OF Module.Microflow [TRANSITIVE].
// callerRefKinds are the reference kinds that mean "this thing invokes the
// target". `show callers` filtered on 'call' alone, which is the kind a MICROFLOW
// activity produces — so a microflow called from a page action button (kind
// 'action') reported "(no callers found)" even though the reference was sitting
// in the refs table, and a page opened by a button or a menu was equally
// invisible (issue #773).
//
// A false negative here reads as "nothing uses this", which is the answer
// somebody acts on before deleting a document — so the set errs toward
// including a kind rather than omitting it. 'schedule' is the same shape: a
// microflow run only by a scheduled event reported "(no callers found)", was
// listed in GRAPH_DEAD_ASSETS, and drew QUAL004 "is not called from anywhere"
// with the suggestion "Remove if unused" — on a microflow that runs nightly.
//
// Deliberately excluded: 'datasource', 'parameter', 'return', 'retrieve',
// 'create', 'change', 'delete', 'associate', 'generalize' and 'layout'. Those
// are uses of a TYPE or a LAYOUT, not invocations, and folding them in would
// make `show callers of <entity>` a synonym for `show references to`.
var callerRefKinds = []string{
	RefKindCallerCall,      // microflow/nanoflow call activity
	RefKindCallerAction,    // widget action: button, on-change, on-click
	RefKindCallerShowPage,  // microflow show-page activity, or a widget action opening a page
	RefKindCallerCalculate, // calculated attribute
	RefKindCallerHomePage,  // navigation
	RefKindCallerLoginPage,
	RefKindCallerMenuItem,
	RefKindCallerSchedule, // scheduled event: the microflow it runs
}

// Kind literals, kept next to the set that uses them so the SQL below cannot
// drift from mdl/catalog's constants unnoticed.
const (
	RefKindCallerCall      = "call"
	RefKindCallerAction    = "action"
	RefKindCallerShowPage  = "show_page"
	RefKindCallerCalculate = "calculate"
	RefKindCallerHomePage  = "home_page"
	RefKindCallerLoginPage = "login_page"
	RefKindCallerMenuItem  = "menu_item"
	RefKindCallerSchedule  = "schedule"
)

// callerRefKindsSQL renders callerRefKinds as a SQL IN list.
func callerRefKindsSQL() string {
	quoted := make([]string, len(callerRefKinds))
	for i, k := range callerRefKinds {
		quoted[i] = "'" + k + "'"
	}
	return "(" + strings.Join(quoted, ", ") + ")"
}

func execShowCallers(ctx *ExecContext, s *ast.ShowStmt) error {
	if s.Name == nil {
		return mdlerrors.NewValidation("target name required for show callers")
	}

	// Ensure catalog is available with full mode for refs
	if err := ensureCatalog(ctx, true); err != nil {
		return err
	}

	targetName := s.Name.String()
	fmt.Fprintf(ctx.Output, "\nCallers of %s", targetName)
	if s.Transitive {
		fmt.Fprintln(ctx.Output, " (transitive)")
	} else {
		fmt.Fprintln(ctx.Output, "")
	}

	var query string
	if s.Transitive {
		// Recursive CTE for transitive callers
		query = `
			with RECURSIVE callers_cte as (
				select SourceName as Caller, 1 as Depth
				from refs
				where TargetName = ? and RefKind in ` + callerRefKindsSQL() + `
				union all
				select r.SourceName, c.Depth + 1
				from refs r
				join callers_cte c on r.TargetName = c.Caller
				where r.RefKind in ` + callerRefKindsSQL() + ` and c.Depth < 10
			)
			select distinct Caller, min(Depth) as Depth
			from callers_cte
			GROUP by Caller
			ORDER by Depth, Caller
		`
	} else {
		// Direct callers only
		query = `
			select distinct SourceName as Caller, 1 as Depth
			from refs
			where TargetName = ? and RefKind in ` + callerRefKindsSQL() + `
			ORDER by Caller
		`
	}

	result, err := ctx.Catalog.Query(strings.Replace(query, "?", "'"+targetName+"'", 1))
	if err != nil {
		return mdlerrors.NewBackend("query callers", err)
	}

	if result.Count == 0 {
		fmt.Fprintln(ctx.Output, "(no callers found)")
		return nil
	}

	fmt.Fprintf(ctx.Output, "Found %d caller(s)\n", result.Count)
	outputCatalogResults(ctx, result)
	return nil
}

// execShowCallees handles SHOW CALLEES OF Module.Microflow [TRANSITIVE].
func execShowCallees(ctx *ExecContext, s *ast.ShowStmt) error {
	if s.Name == nil {
		return mdlerrors.NewValidation("target name required for show callees")
	}

	// Ensure catalog is available with full mode for refs
	if err := ensureCatalog(ctx, true); err != nil {
		return err
	}

	sourceName := s.Name.String()
	fmt.Fprintf(ctx.Output, "\nCallees of %s", sourceName)
	if s.Transitive {
		fmt.Fprintln(ctx.Output, " (transitive)")
	} else {
		fmt.Fprintln(ctx.Output, "")
	}

	var query string
	if s.Transitive {
		// Recursive CTE for transitive callees
		query = `
			with RECURSIVE callees_cte as (
				select TargetName as Callee, 1 as Depth
				from refs
				where SourceName = ? and RefKind = 'call'
				union all
				select r.TargetName, c.Depth + 1
				from refs r
				join callees_cte c on r.SourceName = c.Callee
				where r.RefKind = 'call' and c.Depth < 10
			)
			select distinct Callee, min(Depth) as Depth
			from callees_cte
			GROUP by Callee
			ORDER by Depth, Callee
		`
	} else {
		// Direct callees only
		query = `
			select distinct TargetName as Callee, 1 as Depth
			from refs
			where SourceName = ? and RefKind = 'call'
			ORDER by Callee
		`
	}

	result, err := ctx.Catalog.Query(strings.Replace(query, "?", "'"+sourceName+"'", 1))
	if err != nil {
		return mdlerrors.NewBackend("query callees", err)
	}

	if result.Count == 0 {
		fmt.Fprintln(ctx.Output, "(no callees found)")
		return nil
	}

	fmt.Fprintf(ctx.Output, "Found %d callee(s)\n", result.Count)
	outputCatalogResults(ctx, result)
	return nil
}

// execShowReferences handles SHOW REFERENCES TO Module.Entity.
func execShowReferences(ctx *ExecContext, s *ast.ShowStmt) error {
	if s.Name == nil {
		return mdlerrors.NewValidation("target name required for show references")
	}

	// Ensure catalog is available with full mode for refs
	if err := ensureCatalog(ctx, true); err != nil {
		return err
	}

	typed := s.Name.String()
	fmt.Fprintf(ctx.Output, "\nReferences to %s\n", typed)

	// A widget's TargetName is stored SHOUTED (COMBOBOX) while MDL keywords are
	// written in lower case, so an exact-only match answers the natural spelling
	// with "(no references found)" — wrong, not missing. See resolveReferenceTarget.
	targetName, loose := resolveReferenceTarget(ctx, typed)
	reportResolvedTarget(ctx, typed, targetName, loose)

	// Find all references to this target
	query := `
		select SourceType, SourceName, RefKind
		from refs
		where TargetName = ?
		ORDER by RefKind, SourceType, SourceName
	`

	result, err := ctx.Catalog.Query(strings.Replace(query, "?", "'"+escapeSQLString(targetName)+"'", 1))
	if err != nil {
		return mdlerrors.NewBackend("query references", err)
	}

	if result.Count == 0 {
		fmt.Fprintln(ctx.Output, "(no references found)")
		return nil
	}

	fmt.Fprintf(ctx.Output, "Found %d reference(s)\n", result.Count)
	outputCatalogResults(ctx, result)
	return nil
}

// execShowImpact handles SHOW IMPACT OF Module.Entity.
// This shows all elements that would be affected by changing the target.
func execShowImpact(ctx *ExecContext, s *ast.ShowStmt) error {
	if s.Name == nil {
		return mdlerrors.NewValidation("target name required for show impact")
	}

	// Ensure catalog is available with full mode for refs
	if err := ensureCatalog(ctx, true); err != nil {
		return err
	}

	typed := s.Name.String()
	fmt.Fprintf(ctx.Output, "\nImpact analysis for %s\n", typed)

	targetName, loose := resolveReferenceTarget(ctx, typed)
	reportResolvedTarget(ctx, typed, targetName, loose)

	// Find all direct references to this target
	directQuery := `
		select SourceType, SourceName, RefKind
		from refs
		where TargetName = ?
		ORDER by SourceType, SourceName
	`

	result, err := ctx.Catalog.Query(strings.Replace(directQuery, "?", "'"+escapeSQLString(targetName)+"'", 1))
	if err != nil {
		return mdlerrors.NewBackend("query impact", err)
	}

	if result.Count == 0 {
		fmt.Fprintln(ctx.Output, "(no impact - element is not referenced)")
		return nil
	}

	// Group by type for summary
	typeCounts := make(map[string]int)
	for _, row := range result.Rows {
		if len(row) > 0 {
			if t, ok := row[0].(string); ok {
				typeCounts[t]++
			}
		}
	}

	fmt.Fprintf(ctx.Output, "\nSummary:\n")
	for t, count := range typeCounts {
		fmt.Fprintf(ctx.Output, "  %s: %d\n", t, count)
	}
	fmt.Fprintln(ctx.Output)

	fmt.Fprintf(ctx.Output, "Found %d affected element(s)\n", result.Count)
	outputCatalogResults(ctx, result)

	return nil
}

// --- Executor method wrappers for backward compatibility ---
