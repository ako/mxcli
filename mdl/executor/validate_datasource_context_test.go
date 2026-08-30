// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ledger #147: the CE1571 data source check fired as an ERROR on a page that
// builds. The rule's own header recorded the measurement it rested on —
//
//	page WITH a $Task parameter of the parameter's exact
//	  entity type, datasource without arguments             → CE1571
//
// — which is true, and incomplete. A page PARAMETER in scope is not the same
// thing as an enclosing DATA CONTEXT, and the two behave differently.
//
// Re-measured on mxbuild 11.13.0, five widgets on one project, one `mx check`:
//
//	dgMatrix     nested in a dataview of the parameter's type    → no error
//	dgDeep       matching context two levels up, mismatched      → no error
//	             dataview in between
//	dgWrong      nested in a dataview of a DIFFERENT type        → CE1571
//	dgLoose      no enclosing data context at all                → CE1571
//	dgParamScope page parameter of the exact type, no dataview   → CE1571
//
// So Mendix DOES fill the argument in from an enclosing data context of a
// compatible type, at any depth — and does not from a page parameter. The old
// message asserted the opposite as fact ("does not fill it in, even when an
// object of the right type is in scope"), and the rule refused a working page.
//
// The three controls below are the three shapes that still must be reported:
// without them this fix would simply delete the rule.

// sig builds a one-parameter object signature, the shape the whole rule is about.
func objSig(param, entity string) *flowSignature {
	return &flowSignature{Params: []flowParam{{Name: param, Entity: entity, Object: true}}}
}

// dvWidget is a data container establishing a context of the given kind.
func dvWidget(name string, ds *ast.DataSourceV3, children ...*ast.WidgetV3) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Name:       name,
		Type:       "dataview",
		Properties: map[string]any{"DataSource": ds},
		Children:   children,
	}
}

func mfDS(ref string, args ...string) *ast.DataSourceV3 {
	ds := &ast.DataSourceV3{Type: "microflow", Reference: ref}
	for _, a := range args {
		ds.Args = append(ds.Args, ast.FlowArgV3{Name: a, Value: "$x"})
	}
	return ds
}

// sameName is the compatibility test with no project behind it: exact match
// only. The generalization walk is exercised separately.
func sameName(a, b string) bool { return strings.EqualFold(a, b) }

func f147Sigs() map[string]*flowSignature {
	return map[string]*flowSignature{
		"ds.ds_rows":          objSig("Context", "DS147.ReportContext"),
		"ds.ds_reportcontext": {Returns: "DS147.ReportContext"},
		"ds.ds_onerow":        {Returns: "DS147.Row"},
	}
}

// The reported case: nested in a data context of the parameter's type.
func TestDataSourceArg_MatchingEnclosingContextSuppliesTheArgument(t *testing.T) {
	page := []*ast.WidgetV3{
		dvWidget("dvContext", mfDS("DS.DS_ReportContext"),
			dsWidget("dgMatrix", "DS.DS_Rows"),
		),
	}
	if errs := validateDataSourceArgumentsIn(nil, page, f147Sigs(), sameName); len(errs) != 0 {
		t.Errorf("mxbuild accepts this page; the check must not reject it:\n  %s",
			strings.Join(errs, "\n  "))
	}
}

// …at any depth, with a mismatched context in between (measured: dgDeep).
func TestDataSourceArg_MatchingContextTwoLevelsUp(t *testing.T) {
	page := []*ast.WidgetV3{
		dvWidget("dvOuter", mfDS("DS.DS_ReportContext"),
			dvWidget("dvInner", mfDS("DS.DS_OneRow"),
				dsWidget("dgDeep", "DS.DS_Rows"),
			),
		),
	}
	if errs := validateDataSourceArgumentsIn(nil, page, f147Sigs(), sameName); len(errs) != 0 {
		t.Errorf("a matching context two levels up still supplies the argument:\n  %s",
			strings.Join(errs, "\n  "))
	}
}

// CONTROL 1 (dgLoose): no enclosing data context at all — still CE1571.
func TestDataSourceArg_NoContextIsStillReported(t *testing.T) {
	page := []*ast.WidgetV3{dsWidget("dgLoose", "DS.DS_Rows")}
	errs := validateDataSourceArgumentsIn(nil, page, f147Sigs(), sameName)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "dgLoose") || !strings.Contains(errs[0], "CE1571") {
		t.Errorf("message should name the widget and CE1571: %s", errs[0])
	}
}

// CONTROL 2 (dgWrong): an enclosing context of a DIFFERENT type does not supply it.
func TestDataSourceArg_MismatchedContextIsStillReported(t *testing.T) {
	page := []*ast.WidgetV3{
		dvWidget("dvRow", &ast.DataSourceV3{Type: "parameter", Reference: "$Row"},
			dsWidget("dgWrong", "DS.DS_Rows"),
		),
	}
	params := []ast.PageParameter{{Name: "Row", EntityType: ast.QualifiedName{Module: "DS147", Name: "Row"}}}
	errs := validateDataSourceArgumentsIn(params, page, f147Sigs(), sameName)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0], "dgWrong") {
		t.Errorf("message should name the widget: %s", errs[0])
	}
	// And it should say what the context actually is, or the reader has to guess
	// why a page that looks nested is being rejected.
	if !strings.Contains(errs[0], "DS147.Row") {
		t.Errorf("message should name the enclosing context type: %s", errs[0])
	}
}

// CONTROL 3 (dgParamScope): a page PARAMETER of the exact type is not a data
// context. This is the measurement the rule was originally built on and it still
// holds — it is the reason the fix is "enclosing data context", not "in scope".
func TestDataSourceArg_PageParameterAloneDoesNotSupplyIt(t *testing.T) {
	page := []*ast.WidgetV3{dsWidget("dgParamScope", "DS.DS_Rows")}
	params := []ast.PageParameter{
		{Name: "Ctx", EntityType: ast.QualifiedName{Module: "DS147", Name: "ReportContext"}},
	}
	errs := validateDataSourceArgumentsIn(params, page, f147Sigs(), sameName)
	if len(errs) != 1 {
		t.Fatalf("a page parameter of the right type is NOT a data context (measured: CE1571); got %d errors: %v",
			len(errs), errs)
	}
}

// A data source whose context type cannot be resolved suppresses the report. The
// alternative is to guess, and guessing wrong here rejects a working page — the
// exact failure this fix is about.
func TestDataSourceArg_UnresolvableContextSuppresses(t *testing.T) {
	for _, ds := range []*ast.DataSourceV3{
		{Type: "association", Reference: "DS147.Row_Ctx"},
		{Type: "selection", Reference: "someGrid"},
		{Type: "parameter", Reference: "$Undeclared"},
	} {
		t.Run(ds.Type+" "+ds.Reference, func(t *testing.T) {
			page := []*ast.WidgetV3{dvWidget("dv", ds, dsWidget("dgUnknown", "DS.DS_Rows"))}
			if errs := validateDataSourceArgumentsIn(nil, page, f147Sigs(), sameName); len(errs) != 0 {
				t.Errorf("an unresolvable context must not be reported as a mismatch: %v", errs)
			}
		})
	}
}

// A PRIMITIVE parameter can never come from a data context, so it is reported
// however deeply the widget is nested. Without this the fix would blunt the rule
// far past the false positive it is removing.
func TestDataSourceArg_PrimitiveParameterIsAlwaysReported(t *testing.T) {
	sigs := map[string]*flowSignature{
		"ds.ds_search":        {Params: []flowParam{{Name: "Term"}}},
		"ds.ds_reportcontext": {Returns: "DS147.ReportContext"},
	}
	page := []*ast.WidgetV3{
		dvWidget("dvContext", mfDS("DS.DS_ReportContext"),
			dsWidget("dgSearch", "DS.DS_Search"),
		),
	}
	if errs := validateDataSourceArgumentsIn(nil, page, sigs, sameName); len(errs) != 1 {
		t.Fatalf("a String parameter is never filled in from a data context; got %d errors: %v",
			len(errs), errs)
	}
}

// An argument naming no parameter is a typo whatever the context is — the second
// half of the rule, which the context has nothing to do with.
func TestDataSourceArg_UnknownArgumentIsReportedInsideAContext(t *testing.T) {
	page := []*ast.WidgetV3{
		dvWidget("dvContext", mfDS("DS.DS_ReportContext"),
			dsWidget("dgTypo", "DS.DS_Rows", "Contxet"),
		),
	}
	errs := validateDataSourceArgumentsIn(nil, page, f147Sigs(), sameName)
	if len(errs) != 1 || !strings.Contains(errs[0], "'Contxet'") {
		t.Fatalf("the unknown-argument half must survive; got %v", errs)
	}
}

// The explicit argument still wins: writing it inside a matching context is not
// an error either, and the widget's own data source does NOT supply its own
// parameter (the context is what ENCLOSES it).
func TestDataSourceArg_ExplicitArgumentInsideAContextIsClean(t *testing.T) {
	page := []*ast.WidgetV3{
		dvWidget("dvContext", mfDS("DS.DS_ReportContext"),
			dsWidget("dgOK", "DS.DS_Rows", "Context"),
		),
	}
	if errs := validateDataSourceArgumentsIn(nil, page, f147Sigs(), sameName); len(errs) != 0 {
		t.Errorf("explicit arguments reported: %v", errs)
	}
}

// A widget's own data source establishes context for its CHILDREN, not for
// itself: a self-supplying loop would silence every top-level grid.
func TestDataSourceArg_AWidgetDoesNotSupplyItsOwnArgument(t *testing.T) {
	sigs := map[string]*flowSignature{
		// A microflow returning the same entity its own parameter takes.
		"ds.ds_self": {
			Params:  []flowParam{{Name: "Context", Entity: "DS147.ReportContext", Object: true}},
			Returns: "DS147.ReportContext",
		},
	}
	page := []*ast.WidgetV3{dsWidget("dgSelf", "DS.DS_Self")}
	if errs := validateDataSourceArgumentsIn(nil, page, sigs, sameName); len(errs) != 1 {
		t.Fatalf("a widget must not supply its own argument; got %d errors: %v", len(errs), errs)
	}
}

// The message must no longer assert what measurement falsified.
func TestDataSourceArg_MessageDoesNotClaimMendixNeverFillsItIn(t *testing.T) {
	page := []*ast.WidgetV3{dsWidget("dgLoose", "DS.DS_Rows")}
	errs := validateDataSourceArgumentsIn(nil, page, f147Sigs(), sameName)
	if len(errs) != 1 {
		t.Fatalf("got %v", errs)
	}
	if strings.Contains(errs[0], "even when an object of the right type is in scope") {
		t.Errorf("the message still asserts the falsified claim:\n%s", errs[0])
	}
	// …and should point at the remedy measurement actually found.
	if !strings.Contains(errs[0], "DS147.ReportContext") {
		t.Errorf("the message should name the type a data container would supply:\n%s", errs[0])
	}
}

// A DATABASE data source names its entity directly, so it resolves without a
// flow signature — the commonest enclosing context on a real page.
func TestDataSourceArg_DatabaseContextResolves(t *testing.T) {
	sigs := map[string]*flowSignature{
		"ds.ds_lines": objSig("Order", "Sales.Order"),
	}
	page := []*ast.WidgetV3{
		dvWidget("dvOrder", &ast.DataSourceV3{Type: "database", Reference: "Sales.Order"},
			dsWidget("dgLines", "DS.DS_Lines"),
		),
	}
	if errs := validateDataSourceArgumentsIn(nil, page, sigs, sameName); len(errs) != 0 {
		t.Errorf("a database data source of the parameter's entity supplies it: %v", errs)
	}
}

// Compatibility is delegated, so a specialization in the context satisfies a
// parameter typed as its generalization.
func TestDataSourceArg_CompatibilityIsDelegated(t *testing.T) {
	sigs := map[string]*flowSignature{
		"ds.ds_lines": objSig("Person", "HR.Person"),
	}
	page := []*ast.WidgetV3{
		dvWidget("dvEmp", &ast.DataSourceV3{Type: "database", Reference: "HR.Employee"},
			dsWidget("dgLines", "DS.DS_Lines"),
		),
	}
	compat := func(ctxQN, paramQN string) bool {
		return sameName(ctxQN, paramQN) ||
			(strings.EqualFold(ctxQN, "HR.Employee") && strings.EqualFold(paramQN, "HR.Person"))
	}
	if errs := validateDataSourceArgumentsIn(nil, page, sigs, compat); len(errs) != 0 {
		t.Errorf("a specialization in context satisfies a generalization parameter: %v", errs)
	}
	// Control: without the generalization it is a mismatch.
	if errs := validateDataSourceArgumentsIn(nil, page, sigs, sameName); len(errs) != 1 {
		t.Errorf("the control did not report the mismatch: %v", errs)
	}
}
