// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// upstream #979: a multi-clause XPath constraint is stored as one long line.
//
// The request was filed as "preserve the whitespace I typed", but preserving is
// the weaker half: the formatting the reporter cares about comes out of Studio
// Pro's constraint editor, and it dies on the next regeneration no matter how
// carefully the MDL was written. What they need is a constraint that is
// READABLE after mxcli writes it, whatever it looked like going in.
//
// So the rule here is canonical, not preserving: a constraint that fits stays on
// one line, and one that does not is broken at its boolean joints. That is also
// what keeps the change from churning every project — see
// TestFormatXPathConstraint_ShortConstraintIsUntouched, which is the control for
// every other case in this file.

// A constraint that fits must come back BYTE-IDENTICAL. Without this, every
// page and access rule mxcli rewrites would get a reformatted constraint and
// ADR-0008's write elision would stop skipping anything.
func TestFormatXPathConstraint_ShortConstraintIsUntouched(t *testing.T) {
	for _, in := range []string{
		"[RequestTitle = 'abc']",
		"[Status = 'Open' and Priority = 'High']",
		"[Owner/Module.Owner_User/System.User = '[%CurrentUser%]']",
		"[Status = 'Open'][Priority = 'High']",
		"[not(Completed)]",
	} {
		if got := FormatXPathConstraint(in); got != in {
			t.Errorf("FormatXPathConstraint(%q)\n got %q\nwant it unchanged", in, got)
		}
	}
}

// The reported case: a filter with several clauses, well past 80 columns.
func TestFormatXPathConstraint_BreaksAtBooleanJoints(t *testing.T) {
	in := "[Status = 'Open' and Priority = 'High' and Assignee/MaintenanceManagement.Request_Employee/MaintenanceManagement.Employee/Name = 'Ada' and ReportedOn > '[%BeginOfCurrentDay%]']"

	want := strings.Join([]string{
		"[",
		"  Status = 'Open'",
		"  and Priority = 'High'",
		"  and Assignee/MaintenanceManagement.Request_Employee/MaintenanceManagement.Employee/Name = 'Ada'",
		"  and ReportedOn > '[%BeginOfCurrentDay%]'",
		"]",
	}, "\n")

	if got := FormatXPathConstraint(in); got != want {
		t.Errorf("FormatXPathConstraint\n got:\n%s\nwant:\n%s", got, want)
	}
}

// A clause that is itself too long and has boolean structure of its own gets the
// same treatment one level down. Where `and` and `or` meet, the tighter-binding
// `and` runs get explicit parentheses: Mendix's precedence is not what most
// readers see at a glance, and the filter is being broken up precisely because it
// had stopped being obvious.
func TestFormatXPathConstraint_ExpandsNestedGroups(t *testing.T) {
	in := "[Archived = false and (Status = 'Open' and Priority = 'High' and Category = 'Electrical' or Escalated = true and Severity > 3)]"

	want := strings.Join([]string{
		"[",
		"  Archived = false",
		"  and (",
		"    (Status = 'Open' and Priority = 'High' and Category = 'Electrical')",
		"    or (Escalated = true and Severity > 3)",
		"  )",
		"]",
	}, "\n")

	if got := FormatXPathConstraint(in); got != want {
		t.Errorf("FormatXPathConstraint\n got:\n%s\nwant:\n%s", got, want)
	}
}

// The same mixed chain, wide enough that the parenthesised runs must open up too.
func TestFormatXPathConstraint_ExpandsMixedChainAllTheWayDown(t *testing.T) {
	in := "[Archived = false and (Status = 'Open' and Priority = 'High' and Category = 'Electrical' or Escalated = true and Severity > 3)]"

	want := strings.Join([]string{
		"[",
		"  Archived = false",
		"  and (",
		"    (",
		"      Status = 'Open'",
		"      and Priority = 'High'",
		"      and Category = 'Electrical'",
		"    )",
		"    or (",
		"      Escalated = true",
		"      and Severity > 3",
		"    )",
		"  )",
		"]",
	}, "\n")

	if got := FormatXPathConstraintWidth(in, 40); got != want {
		t.Errorf("FormatXPathConstraintWidth(…, 40)\n got:\n%s\nwant:\n%s", got, want)
	}
}

// Mendix stores sibling predicate groups concatenated. Each is its own line —
// they are already the author's top-level conjunction.
func TestFormatXPathConstraint_SplitsPredicateGroups(t *testing.T) {
	in := "[Status = 'Open' and Priority = 'High' and Category = 'Electrical' and Severity > 3][Archived = false]"

	want := strings.Join([]string{
		"[",
		"  Status = 'Open'",
		"  and Priority = 'High'",
		"  and Category = 'Electrical'",
		"  and Severity > 3",
		"]",
		"[Archived = false]",
	}, "\n")

	if got := FormatXPathConstraint(in); got != want {
		t.Errorf("FormatXPathConstraint\n got:\n%s\nwant:\n%s", got, want)
	}
}

// There is nothing to break in a single long comparison, and cutting one
// anywhere else would produce a constraint Mendix rejects. Leave it whole and
// over width — an honest long line beats a broken short one.
func TestFormatXPathConstraint_LeavesAnUnbreakableClauseWhole(t *testing.T) {
	in := "[Assignee/MaintenanceManagement.Request_Employee/MaintenanceManagement.Employee/Department/MaintenanceManagement.Department_Site/MaintenanceManagement.Site/Name = 'Rotterdam']"

	if got := FormatXPathConstraint(in); got != in {
		t.Errorf("FormatXPathConstraint(%q)\n got %q\nwant it unchanged — there is no boolean joint to break", in, got)
	}
}

// A constraint the grammar cannot read is handed back untouched. Rewriting one
// we cannot parse is how a formatter turns into a data-loss bug.
func TestFormatXPathConstraint_PassesThroughWhatItCannotParse(t *testing.T) {
	for _, in := range []string{
		"",
		"   ",
		"not a constraint at all",
		"[unbalanced = 'x'",
		"[Status = 'Open' and and]",
	} {
		if got := FormatXPathConstraint(in); got != in {
			t.Errorf("FormatXPathConstraint(%q)\n got %q\nwant it unchanged", in, got)
		}
	}
}

// Re-running the formatter over its own output must be a no-op, or the stored
// constraint changes on every write and nothing is ever elided.
func TestFormatXPathConstraint_IsIdempotent(t *testing.T) {
	for _, in := range []string{
		"[RequestTitle = 'abc']",
		"[Status = 'Open' and Priority = 'High' and Category = 'Electrical' and Severity > 3 and Archived = false]",
		"[Archived = false and (Status = 'Open' and Priority = 'High' and Category = 'Electrical' or Escalated = true and Severity > 3)]",
		"[Status = 'Open' and Priority = 'High' and Category = 'Electrical' and Severity > 3][Archived = false]",
	} {
		once := FormatXPathConstraint(in)
		if twice := FormatXPathConstraint(once); twice != once {
			t.Errorf("FormatXPathConstraint is not idempotent for %q\n once:\n%s\ntwice:\n%s", in, once, twice)
		}
	}
}

// fullyParenthesised renders an expression with every binary node bracketed, so
// two trees that differ only in redundant parentheses render identically and two
// that group differently do not. Comparing the plain rendering would fail the
// moment the formatter makes an implied grouping explicit — which is a thing it
// deliberately does — while comparing this catches a formatter that actually
// re-associated the expression.
func fullyParenthesised(expr ast.Expression) string {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		return "(" + fullyParenthesised(e.Left) + " " + strings.ToLower(e.Operator) + " " + fullyParenthesised(e.Right) + ")"
	case *ast.ParenExpr:
		// The tree already carries the grouping; the node itself adds nothing.
		return fullyParenthesised(e.Inner)
	case *ast.UnaryExpr:
		if strings.EqualFold(e.Operator, "not") {
			return "not(" + fullyParenthesised(e.Operand) + ")"
		}
	}
	return xpathExprToString(expr)
}

// A control for fullyParenthesised itself: it must tell apart the two groupings
// that Mendix's precedence makes easy to confuse. Without this the round-trip
// test below could pass against a canonicaliser that flattens everything.
func TestFullyParenthesised_DistinguishesGrouping(t *testing.T) {
	loose, ok := ParseXPathConstraint("[a = 1 or b = 2 and c = 3]")
	if !ok {
		t.Fatal("fixture does not parse")
	}
	tight, ok := ParseXPathConstraint("[(a = 1 or b = 2) and c = 3]")
	if !ok {
		t.Fatal("fixture does not parse")
	}
	if fullyParenthesised(loose) == fullyParenthesised(tight) {
		t.Errorf("fullyParenthesised cannot tell the two groupings apart: %s", fullyParenthesised(loose))
	}
	redundant, ok := ParseXPathConstraint("[a = 1 or (b = 2 and c = 3)]")
	if !ok {
		t.Fatal("fixture does not parse")
	}
	if got, want := fullyParenthesised(redundant), fullyParenthesised(loose); got != want {
		t.Errorf("redundant parentheses changed the canonical form\n got %s\nwant %s", got, want)
	}
}

// The wrapped form has to mean the same thing as the flat one.
func TestFormatXPathConstraint_WrappedFormReparsesToTheSameExpression(t *testing.T) {
	for _, in := range []string{
		"[Status = 'Open' and Priority = 'High' and Category = 'Electrical' and Severity > 3 and Archived = false]",
		"[Archived = false and (Status = 'Open' and Priority = 'High' and Category = 'Electrical' or Escalated = true and Severity > 3)]",
		"[not(Status = 'Closed' and Priority = 'Low' and Category = 'Electrical' and Archived = true)]",
	} {
		flat, ok := ParseXPathConstraint(in)
		if !ok {
			t.Fatalf("fixture does not parse: %q", in)
		}
		wrapped := FormatXPathConstraint(in)
		back, ok := ParseXPathConstraint(wrapped)
		if !ok {
			t.Fatalf("wrapped form does not parse back:\n%s", wrapped)
		}
		if got, want := fullyParenthesised(back), fullyParenthesised(flat); got != want {
			t.Errorf("wrapping changed the expression\n got %s\nwant %s\nwrapped:\n%s", got, want, wrapped)
		}
	}
}

// A string literal is a leaf. " and " inside one is data, not a joint.
func TestFormatXPathConstraint_DoesNotBreakInsideAStringLiteral(t *testing.T) {
	in := "[Description = 'electrical and plumbing and heating and ventilation work order' and Status = 'Open']"

	got := FormatXPathConstraint(in)
	if !strings.Contains(got, "'electrical and plumbing and heating and ventilation work order'") {
		t.Errorf("the literal was broken up:\n%s", got)
	}
	if n := strings.Count(got, "\n"); n != 3 {
		t.Errorf("got %d newlines, want 3 (open bracket, two clauses, close):\n%s", n, got)
	}
}

// A `not(...)` wrapping a long chain opens up like any other group rather than
// running off the line.
func TestFormatXPathConstraint_ExpandsNot(t *testing.T) {
	in := "[not(Status = 'Closed' and Priority = 'Low' and Category = 'Electrical' and Archived = true)]"

	want := strings.Join([]string{
		"[",
		"  not(",
		"    Status = 'Closed'",
		"    and Priority = 'Low'",
		"    and Category = 'Electrical'",
		"    and Archived = true",
		"  )",
		"]",
	}, "\n")

	if got := FormatXPathConstraint(in); got != want {
		t.Errorf("FormatXPathConstraint\n got:\n%s\nwant:\n%s", got, want)
	}
}

// The width is a parameter so the caller can pin it; 80 is the default the
// reporter asked for.
func TestFormatXPathConstraintWidth_HonoursTheWidth(t *testing.T) {
	in := "[Status = 'Open' and Priority = 'High']"

	if got := FormatXPathConstraintWidth(in, 200); got != in {
		t.Errorf("at width 200 the constraint should stay flat, got:\n%s", got)
	}
	wide := FormatXPathConstraintWidth(in, 20)
	if !strings.Contains(wide, "\n") {
		t.Errorf("at width 20 the constraint should wrap, got %q", wide)
	}
	if FormatXPathConstraint(in) != in {
		t.Error("the default width must leave this one flat")
	}
}
