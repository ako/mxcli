// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateMicroflow checks a microflow for common issues that don't require a project connection.
// Returns a list of structured violations with rule IDs.
func ValidateMicroflow(stmt *ast.CreateMicroflowStmt) []linter.Violation {
	v := &microflowValidator{
		mfName:     stmt.Name.String(),
		returnType: stmt.ReturnType,
		varKinds:   map[string]exprcheck.TypeKind{},
	}
	// Seed the variable→kind scope with the microflow's parameters so numeric
	// assignment checks can resolve operands like $count.
	for _, p := range stmt.Parameters {
		if k, ok := astKindToExprKind(p.Type.Kind); ok {
			v.varKinds[p.Name] = k
		}
	}
	// Validate parameter entity references — reject bare names without module prefix
	for _, p := range stmt.Parameters {
		if p.Type.EntityRef != nil && p.Type.EntityRef.Module == "" {
			v.addViolation("MDL008", linter.SeverityError,
				fmt.Sprintf("parameter '$%s': entity type '%s' is missing module prefix",
					p.Name, p.Type.EntityRef.Name),
				fmt.Sprintf("Use a qualified name like 'Module.%s' or 'System.%s'",
					p.Type.EntityRef.Name, p.Type.EntityRef.Name))
		}
	}
	v.validate(stmt.Body)
	return v.violations
}

// microflowValidator holds state for validating a single microflow.
type microflowValidator struct {
	mfName        string
	returnType    *ast.MicroflowReturnType // nil = void
	violations    []linter.Violation
	loopDepth     int             // Track nesting depth inside loops
	emptyListVars map[string]bool // List variables declared empty and never populated
	// varKinds maps in-scope variable names (params + declared) to their kind,
	// used to detect assigning a Decimal expression to an Integer/Long target.
	varKinds map[string]exprcheck.TypeKind
}

func (v *microflowValidator) addViolation(ruleID string, severity linter.Severity, message, suggestion string) {
	v.violations = append(v.violations, linter.Violation{
		RuleID:   ruleID,
		Severity: severity,
		Message:  message,
		Location: linter.Location{
			DocumentType: "microflow",
			DocumentName: v.mfName,
		},
		Suggestion: suggestion,
	})
}

// validate runs all checks on the microflow body.
func (v *microflowValidator) validate(body []ast.MicroflowStatement) {
	// Walk the body for per-statement checks (validation feedback, return value checks)
	v.emptyListVars = make(map[string]bool)
	v.walkBody(body)

	// Check 5: missing RETURN on non-void microflow paths
	if v.returnType != nil && v.returnType.Type.Kind != ast.TypeVoid {
		if !bodyReturns(body) {
			v.addViolation("MDL003", linter.SeverityError,
				fmt.Sprintf("microflow returns %s but not all code paths have a return statement",
					returnTypeString(v.returnType)),
				"Add return statements to all code paths")
		}
	}

	// Check 3: variable scope — detect variables declared inside branches but used after
	v.checkBranchScoping(body)

	// Duplicate loop iterator names — a Mendix loop variable is scoped to the whole
	// microflow, so reusing a name across loops is CE0111 at build time.
	v.checkDuplicateLoopVariables(body)

	// The other half of that rule: names are unique flow-wide, but a loop's
	// variables are only VISIBLE inside its body, so using one after the loop
	// is CE0108.
	v.checkLoopScoping(body)
}

// checkDuplicateLoopVariables flags a loop iterator name used by more than one
// loop in the same microflow. A Mendix loop variable is scoped to the WHOLE
// microflow (not to its loop), so two `loop $R in …` — even sequential ones, or a
// nested loop reusing an outer name — build as CE0111 "Duplicate variable name".
// (ledger finding #64). Fix for the user: give each loop a distinct iterator.
func (v *microflowValidator) checkDuplicateLoopVariables(body []ast.MicroflowStatement) {
	seen := map[string]bool{}
	var walk func([]ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			switch st := s.(type) {
			case *ast.LoopStmt:
				if name := st.LoopVariable; name != "" {
					if seen[name] {
						v.addViolation("MDL052", linter.SeverityError,
							fmt.Sprintf("loop iterator '$%s' is reused by another loop in this microflow; "+
								"a Mendix loop variable is scoped to the whole microflow, so this builds as "+
								"CE0111 \"Duplicate variable name\"", name),
							fmt.Sprintf("Give each loop a distinct iterator name (e.g. rename one '$%s' to '$%s2')", name, name))
					}
					seen[name] = true
				}
				walk(st.Body)
			case *ast.IfStmt:
				walk(st.ThenBody)
				walk(st.ElseBody)
			case *ast.WhileStmt:
				walk(st.Body)
			case *ast.EnumSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body)
				}
				walk(st.ElseBody)
			case *ast.InheritanceSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body)
				}
				walk(st.ElseBody)
			}
		}
	}
	walk(body)
}

// walkBody recursively walks microflow body statements looking for per-statement issues.
func (v *microflowValidator) walkBody(body []ast.MicroflowStatement) {
	for _, s := range body {
		switch stmt := s.(type) {
		case *ast.ValidationFeedbackStmt:
			if isEmptyMessage(stmt.Message) {
				v.addViolation("MDL007", linter.SeverityWarning,
					"validation feedback has empty message template. "+
						"Mendix requires a non-empty feedback message (CE0091).",
					"Add a message template to the validation feedback action")
			}
		case *ast.ReturnStmt:
			v.checkReturn(stmt)
			v.checkExprFunctions("return", stmt.Value)
			v.checkDivisionSlash("return", stmt.Value)
			v.checkDateTimeLiterals("return", stmt.Value)
		case *ast.IfStmt:
			v.checkExprFunctions("if condition", stmt.Condition)
			v.checkDivisionSlash("if condition", stmt.Condition)
			v.checkDateTimeLiterals("if condition", stmt.Condition)
			v.walkBody(stmt.ThenBody)
			v.walkBody(stmt.ElseBody)
		case *ast.EnumSplitStmt:
			// Mendix enumeration splits map to exclusive splits with one outgoing
			// flow per enum value. Multiple values per branch and a default (else)
			// flow are not supported — Studio Pro will reject both with CE errors.
			if len(stmt.ElseBody) > 0 {
				v.addViolation("MDL008", linter.SeverityError,
					fmt.Sprintf("case statement on '$%s' has an else branch; "+
						"Mendix enumeration splits do not support a default case. "+
						"Add an explicit when branch for every enum value instead.",
						stmt.Variable),
					"Add an explicit when branch for every enum value instead of using else")
			}
			for _, c := range stmt.Cases {
				if len(c.Values) > 1 {
					v.addViolation("MDL009", linter.SeverityError,
						fmt.Sprintf("case statement on '$%s': when branch lists %d values (%s); "+
							"Mendix enumeration splits require exactly one value per branch.",
							stmt.Variable, len(c.Values), strings.Join(c.Values, ", ")),
						"Split into separate when branches, one per enum value")
				}
				v.walkBody(c.Body)
			}
			v.walkBody(stmt.ElseBody)
		case *ast.InheritanceSplitStmt:
			for _, c := range stmt.Cases {
				v.walkBody(c.Body)
			}
			v.walkBody(stmt.ElseBody)
		case *ast.DeclareStmt:
			if stmt.Type.Kind == ast.TypeListOf {
				// A `declare` maps to a Create Variable activity, which cannot
				// produce a list — Studio Pro rejects it with CE0053 ("type not
				// allowed") and CE0038 ("value required"). Lists must come from a
				// microflow parameter, a `retrieve`, or a `create list`. (#607)
				v.addViolation("MDL040", linter.SeverityError,
					fmt.Sprintf("declare '$%s' creates a list variable, but Mendix does not allow the "+
						"Create Variable activity to produce a list (CE0053/CE0038). "+
						"Pass the list as a microflow parameter, populate it with retrieve, or use create list.",
						stmt.Variable),
					"Accept the list as a parameter, use retrieve, or use create list — do not declare a list variable")
				// Track list variables declared as empty (candidates for the empty-list-in-loop anti-pattern)
				if isEmptyInit(stmt.InitialValue) {
					v.emptyListVars[stmt.Variable] = true
				}
			}
			// A bare `Module.X` declare type parses as TypeEnumeration with EnumRef
			// (the documented entity/enum ambiguity); an explicit `Enumeration(...)`
			// sets ExplicitEnum. Treat the ambiguous form — and a resolved
			// TypeEntity — as an object declare.
			if stmt.Type.Kind == ast.TypeEntity ||
				(stmt.Type.Kind == ast.TypeEnumeration && stmt.Type.EnumRef != nil && !stmt.Type.ExplicitEnum) {
				// Same restriction as lists (MDL040): a `declare` maps to a Create
				// Variable activity, which only holds primitive types. An object type
				// is rejected by Studio Pro/mxbuild with CE0053 ("Selected type is
				// not allowed") — bare or initialized — plus CE0038 ("Value
				// required") and CE7247 on any following `set`. Object variables must
				// come from a microflow parameter, a retrieve, a create object, or a
				// loop iterator; there is no Create Variable form for them. mxcli
				// check previously passed this, so the invalid microflow surfaced
				// only later in mxbuild.
				typeName := stmt.Variable
				switch {
				case stmt.Type.EntityRef != nil:
					typeName = stmt.Type.EntityRef.String()
				case stmt.Type.EnumRef != nil:
					typeName = stmt.Type.EnumRef.String()
				}
				v.addViolation("MDL043", linter.SeverityError,
					fmt.Sprintf("declare '$%s' creates an object variable of type %s, but Mendix does not allow the "+
						"Create Variable activity to hold an object (CE0053). "+
						"Get the object from a microflow parameter, a retrieve, or a create object instead — do not declare an object variable. "+
						"(If %s is an enumeration, write it as Enumeration(%s).)",
						stmt.Variable, typeName, typeName, typeName),
					"Accept the object as a parameter, use retrieve, or use create object — do not declare an object variable")
			}
			// Register the declared variable's kind for later assignment checks,
			// and flag a Decimal initial value assigned to an Integer/Long declare.
			if k, ok := astKindToExprKind(stmt.Type.Kind); ok {
				v.varKinds[stmt.Variable] = k
				if stmt.InitialValue != nil {
					v.checkNumericAssignment("$"+stmt.Variable, k, stmt.InitialValue)
				}
			}
			v.checkExprFunctions(fmt.Sprintf("declare '$%s'", stmt.Variable), stmt.InitialValue)
			v.checkDivisionSlash(fmt.Sprintf("declare '$%s'", stmt.Variable), stmt.InitialValue)
			v.checkDateTimeLiterals(fmt.Sprintf("declare '$%s'", stmt.Variable), stmt.InitialValue)
		case *ast.MfSetStmt:
			// SET on a plain variable target (not $var/Member = …, which is a
			// member change). Flag a Decimal value assigned to an Integer/Long var.
			if !strings.Contains(stmt.Target, "/") {
				if k, ok := v.varKinds[stmt.Target]; ok {
					v.checkNumericAssignment("$"+stmt.Target, k, stmt.Value)
				}
			}
			v.checkExprFunctions(fmt.Sprintf("set '%s'", stmt.Target), stmt.Value)
			v.checkDivisionSlash(fmt.Sprintf("set '%s'", stmt.Target), stmt.Value)
			v.checkDateTimeLiterals(fmt.Sprintf("set '%s'", stmt.Target), stmt.Value)
		case *ast.RetrieveStmt:
			// RETRIEVE populates a list variable — remove from empty tracking
			delete(v.emptyListVars, stmt.Variable)
			if stmt.Where != nil {
				xp := expressionToXPath(stmt.Where)
				v.checkXPathAssociationEmpty(stmt.Variable, xp)
				v.checkXPathIdConstraint(stmt.Variable, xp)
				v.checkXPathVariableTraversal(stmt.Variable, xp)
			}
		case *ast.CallMicroflowStmt:
			v.checkAssociationObjectArgs("microflow "+stmt.MicroflowName.String(), stmt.Arguments)
		case *ast.CallNanoflowStmt:
			v.checkAssociationObjectArgs("nanoflow "+stmt.NanoflowName.String(), stmt.Arguments)
		case *ast.LoopStmt:
			// Check: @caption on a loop is silently dropped — Mendix for-loops
			// have no caption (Microflows$LoopedActivity has no Caption
			// property; Studio Pro auto-labels them from the iterator). The
			// supported way to label a loop is an annotation note.
			if stmt.Annotations != nil && stmt.Annotations.Caption != "" {
				v.addViolation("MDL042", linter.SeverityWarning,
					"@caption on a loop has no effect — Mendix loops have no caption "+
						"(the loop activity has no Caption property, so it is dropped). "+
						"Use @annotation to attach a note to the loop instead.",
					"Replace @caption with @annotation to label the loop")
			}
			// Check: nested loop anti-pattern. This is a heuristic — a nested loop is
			// only wasteful when the inner loop is a key LOOKUP (find one matching
			// item). Intentional aggregation that must visit every element (group ×
			// category × month totals) is O(N*M) by nature and correct as written, so
			// the message flags the lookup case without asserting the loop is wrong.
			if v.loopDepth > 0 {
				v.addViolation("MDL001", linter.SeverityWarning,
					"nested loop detected (loop inside a loop). If the inner loop is a "+
						"key LOOKUP (finding one matching item), replace it with "+
						"FIND($List, <condition>) for an in-memory match (O(N) vs O(N^2)). "+
						"If it is intentional aggregation that must visit every element "+
						"(e.g. totals over group x category x month), this is correct — ignore this hint.",
					"For a lookup: $Match = FIND($List, key = $item/key). For genuine aggregation over all elements, no change is needed (a plain retrieve ... where cannot filter a list variable).")
			}
			// Check: loop over empty declared list
			if v.emptyListVars[stmt.ListVariable] {
				v.addViolation("MDL002", linter.SeverityWarning,
					fmt.Sprintf("loop iterates over '$%s' which was declared as an empty list and never populated. "+
						"Pass the list as a microflow parameter instead of creating an empty variable.",
						stmt.ListVariable),
					"Pass the list as a microflow parameter instead of creating an empty variable")
			}
			v.loopDepth++
			v.walkBody(stmt.Body)
			v.loopDepth--
		case *ast.CreateObjectStmt:
			// Attribute values in a `create` are expressions too — an aggregate
			// (sum/count/…) or an unknown function here fails the build with CE0117,
			// but check previously only inspected return/if/declare/set (FINDINGS #17).
			for _, ch := range stmt.Changes {
				v.checkExprFunctions(fmt.Sprintf("create %s attribute '%s'", stmt.EntityType.String(), ch.Attribute), ch.Value)
			}
		case *ast.ChangeObjectStmt:
			for _, ch := range stmt.Changes {
				v.checkExprFunctions(fmt.Sprintf("change '%s' attribute '%s'", stmt.Variable, ch.Attribute), ch.Value)
			}
		}
		// Check error handling inside loops
		if eh := stmtErrorHandling(s); eh != nil {
			v.checkErrorHandlingInLoop(s, eh)
			// Also walk ON ERROR bodies
			if len(eh.Body) > 0 {
				v.walkBody(eh.Body)
			}
		}
	}
}

// checkNumericAssignment flags assigning a Decimal-typed expression to an
// Integer or Long target. Mendix integer division (`div`) yields a Decimal, so
// `set $IntVar = $a * 100 div $b;` fails mx check with CE0117 even though the
// syntax is valid. Only Integer/Long targets with a provably-Decimal value are
// flagged (unknown inference never fires), keeping false positives out.
func (v *microflowValidator) checkNumericAssignment(targetLabel string, targetKind exprcheck.TypeKind, value ast.Expression) {
	if targetKind != exprcheck.KindInteger && targetKind != exprcheck.KindLong {
		return
	}
	src := microflowExprSource(value)
	if src == "" {
		return
	}
	// Flag a Decimal-typed value assigned to an Integer/Long target: a raw
	// arithmetic Decimal (e.g. `$a div $b`) or a Decimal-returning built-in such
	// as random() / secondsBetween(...). Rounding functions (round/floor/ceil/
	// trunc) are excluded — Mendix accepts their whole-number result. (findings #2)
	if !exprcheck.SourceRejectedForIntegerTarget(src, v.varKinds) {
		return
	}
	target := "Integer"
	if targetKind == exprcheck.KindLong {
		target = "Long"
	}
	v.addViolation("MDL041", linter.SeverityError,
		fmt.Sprintf("assigning a Decimal expression to %s variable '%s' — Mendix rejects this with CE0117. "+
			"Integer division ('div') and functions like random()/secondsBetween() yield a Decimal.", target, targetLabel),
		fmt.Sprintf("Declare '%s' as Decimal, or round the value (e.g. round(%s) or floor(%s)).", targetLabel, src, src))
}

// mendixAggregateFuncs are the list aggregates that Mendix exposes as
// activities, not expression functions. Used inside an expression they fail
// CE0117; the fix is to assign the aggregate to a variable first.
var mendixAggregateFuncs = map[string]bool{
	"count": true, "sum": true, "average": true, "minimum": true, "maximum": true,
}

// checkExprFunctions flags calls to names that are not Mendix expression
// functions (e.g. a hallucinated randomInt()) — these parse and pass a naive
// check but fail the build with CE0117. label describes where the expression
// appears (e.g. "declare '$r'"). (findings #1)
func (v *microflowValidator) checkExprFunctions(label string, expr ast.Expression) {
	src := microflowExprSource(expr)
	if src == "" {
		return
	}
	for _, u := range exprcheck.UnknownFunctionCalls(src) {
		var suggestion string
		if mendixAggregateFuncs[strings.ToLower(u.Name)] {
			// count/sum/average/minimum/maximum are aggregate ACTIVITIES, not
			// expression functions — a did-you-mean against an unrelated math
			// function (e.g. count→round) sends the author the wrong way. Tell them
			// to assign the aggregate to a variable first, then use the variable.
			suggestion = fmt.Sprintf(
				"'%s' is an aggregate activity, not an expression function. Assign it to a variable first: $n = %s($List); then use $n in the expression.",
				u.Name, u.Name)
		} else {
			suggestion = "Use a built-in Mendix expression function (see 'mxcli syntax microflow')."
			if u.Suggestion != "" {
				suggestion = fmt.Sprintf("Did you mean '%s()'? ", u.Suggestion) + suggestion
			}
		}
		v.addViolation("MDL044", linter.SeverityError,
			fmt.Sprintf("%s calls '%s()', which is not a Mendix expression function — "+
				"the build fails CE0117 \"Error(s) in expression\"", label, u.Name),
			suggestion)
	}
}

// checkDivisionSlash flags `/` used as an arithmetic division operator, which
// Mendix rejects with CE0117 — in a Mendix expression `/` is only the
// member/association separator (`$obj/Attr`); integer/decimal division is `div`.
// `$Dec / 2` parses to a BinaryExpr whose operator is literally `/`; the walk
// finds it wherever it appears (nested in functions, if-then-else, etc.).
// (The `$a / $b` form degrades to a member-access path and is caught by
// `check --references` as an unresolvable attribute, so it is not re-flagged here.)
// (ledger finding #17)
func (v *microflowValidator) checkDivisionSlash(label string, expr ast.Expression) {
	// Two forms of `/`-as-division: a `BinaryExpr` with operator `/` (right operand
	// is a literal or parenthesized expr, e.g. `$Dec / 2`), and the variable/
	// variable form (`$Dec / $Dec2`) which parses as a member-access path — the
	// visitor preserves its raw source precisely because the `$` on the right
	// marks it as division, not navigation. The `$a / $b` form is detected from
	// the preserved source, so it is caught even when EMBEDDED in a larger
	// expression (`$a / $b + 1`, `round($a / $b)`) where the division degrades to
	// a member-path AttributePathExpr nested under a BinaryExpr/FunctionCallExpr.
	if exprHasSlashDivision(expr) || exprHasSlashDollarDivision(expr) {
		v.addViolation("MDL045", linter.SeverityError,
			fmt.Sprintf("%s uses '/' as a division operator, which Mendix rejects "+
				"(CE0117 \"Error(s) in expression\") — '/' navigates associations, it does not divide", label),
			"Use 'div' for division: `$a div $b` (integer/decimal division is always Decimal — wrap in round()/trunc() for an integer result).")
	}
}

// exprHasSlashDollarDivision reports the `$a / $b` division-misuse form: a
// source-preserved expression whose raw source uses `/` (optionally spaced)
// immediately before a `$` variable, OUTSIDE any string literal. A real
// member/association path never writes `/$` — a path segment is a bare or
// qualified name — so this is an unambiguous division misuse. Scanning the
// preserved source (rather than requiring the SourceExpr to wrap an
// AttributePathExpr directly) catches the division even when it is nested in a
// larger expression: `$a / $b + 1`, `round($a / $b)`, `$a / $b * 100`. The
// string-literal-aware scan avoids a false positive on a `/$` inside a quoted
// literal (e.g. `'path/$x'`).
func exprHasSlashDollarDivision(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.SourceExpr:
		if sourceHasSlashDollarDivision(e.Source) {
			return true
		}
		return exprHasSlashDollarDivision(e.Expression)
	case *ast.BinaryExpr:
		return exprHasSlashDollarDivision(e.Left) || exprHasSlashDollarDivision(e.Right)
	case *ast.UnaryExpr:
		return exprHasSlashDollarDivision(e.Operand)
	case *ast.ParenExpr:
		return exprHasSlashDollarDivision(e.Inner)
	case *ast.FunctionCallExpr:
		for _, arg := range e.Arguments {
			if exprHasSlashDollarDivision(arg) {
				return true
			}
		}
	case *ast.IfThenElseExpr:
		return exprHasSlashDollarDivision(e.Condition) ||
			exprHasSlashDollarDivision(e.ThenExpr) ||
			exprHasSlashDollarDivision(e.ElseExpr)
	}
	return false
}

// sourceHasSlashDollarDivision scans a raw expression source for a `/` used as
// division with a variable divisor (`… / $x …`) outside any single-quoted
// string literal. Doubled ” inside a string is a Mendix-escaped quote.
func sourceHasSlashDollarDivision(source string) bool {
	inStr := false
	for i := 0; i < len(source); i++ {
		c := source[i]
		if c == '\'' {
			if inStr && i+1 < len(source) && source[i+1] == '\'' {
				i++ // skip the escaped quote pair
				continue
			}
			inStr = !inStr
			continue
		}
		if inStr || c != '/' {
			continue
		}
		j := i + 1
		for j < len(source) && (source[j] == ' ' || source[j] == '\t') {
			j++
		}
		if j < len(source) && source[j] == '$' {
			return true
		}
	}
	return false
}

// exprHasSlashDivision reports whether the expression tree contains a BinaryExpr
// whose operator is a literal `/` (an arithmetic-division misuse).
func exprHasSlashDivision(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		if strings.TrimSpace(e.Operator) == "/" {
			// A `/` whose RIGHT operand is a bare member name is association/member
			// navigation, not division. The MDL grammar parses `div`/`*`/`/` at one
			// precedence level, so `$a div $obj/Attr` mis-nests as `($a div $obj) / Attr`
			// with `Attr` a bare IdentifierExpr. Mendix has no `/` division operator, so
			// it re-parses the raw `$obj/Attr` as a path and the expression builds clean
			// (verified with mxbuild). Only a numeric/parenthesized/variable divisor is a
			// real division misuse — those are caught here (right operand is not an
			// IdentifierExpr) or by the source `/ $var` scan. (FINDINGS #52)
			if _, isMemberName := e.Right.(*ast.IdentifierExpr); !isMemberName {
				return true
			}
		}
		return exprHasSlashDivision(e.Left) || exprHasSlashDivision(e.Right)
	case *ast.UnaryExpr:
		return exprHasSlashDivision(e.Operand)
	case *ast.ParenExpr:
		return exprHasSlashDivision(e.Inner)
	case *ast.FunctionCallExpr:
		for _, arg := range e.Arguments {
			if exprHasSlashDivision(arg) {
				return true
			}
		}
	case *ast.IfThenElseExpr:
		return exprHasSlashDivision(e.Condition) || exprHasSlashDivision(e.ThenExpr) || exprHasSlashDivision(e.ElseExpr)
	case *ast.SourceExpr:
		return exprHasSlashDivision(e.Expression)
	}
	return false
}

// xpathAssocEmptyRe matches a module-qualified association compared directly to
// `empty` in an XPath constraint (`Ledger.Transaction_Category = empty`). The
// leading boundary class excludes a `/` (so an attribute-over-association path
// like `Assoc/Ledger.Category = empty` is NOT matched — that is a valid
// attribute nullability test) and a `.`/word char (so it captures the whole
// qualified name, not the tail of a 3-part enum literal).
var xpathAssocEmptyRe = regexp.MustCompile(`(^|[^\w./])([A-Za-z_]\w*\.[A-Za-z_]\w*)\s*=\s*empty\b`)

// xpathAssociationEmptyMatches returns the module-qualified association names an
// XPath constraint compares directly to `empty` (`Ledger.Transaction_Category =
// empty`). Shared by the microflow-retrieve check (MDL047) and the page/widget
// datasource check. Empty result → nothing to flag.
func xpathAssociationEmptyMatches(xpath string) []string {
	var out []string
	for _, m := range xpathAssocEmptyRe.FindAllStringSubmatch(xpath, -1) {
		out = append(out, m[2])
	}
	return out
}

// checkXPathAssociationEmpty flags `[Module.Association = empty]` in a retrieve
// constraint. Mendix XPath has no `= empty` test for an association — it fails
// the build with CE0161; the nullability test is `not(Module.Association/Module.Target)`.
// A bare attribute (`Name = empty`) is valid and is not module-qualified, so it
// never matches. (ledger finding #25)
func (v *microflowValidator) checkXPathAssociationEmpty(variable, xpath string) {
	for _, assoc := range xpathAssociationEmptyMatches(xpath) {
		v.addViolation("MDL047", linter.SeverityError,
			fmt.Sprintf("retrieve '$%s' constraint tests association `%s = empty`, which Mendix XPath does not support "+
				"(CE0161 \"Error(s) in XPath constraint\") — `= empty` works on attributes, not associations", variable, assoc),
			fmt.Sprintf("Test for the absence of the associated object with negation: `[not(%s/<Module.TargetEntity>)]`.", assoc))
	}
}

// xpathVarTraversalRe matches a path rooted at a $variable with TWO OR MORE
// segments (`$P/Mod.Assoc/Name`). One segment is deliberately not matched: both
// `$P/Code` (the parameter's own attribute) and `$P/Mod.Assoc` (one hop to the
// associated object) are valid XPath. The boundary is the hop count, not whether
// a segment is module-qualified — see checkXPathVariableTraversal.
var xpathVarTraversalRe = regexp.MustCompile(`\$(\w+)((?:/[A-Za-z_][\w.]*){2,})`)

// checkXPathVariableTraversal flags a retrieve constraint that traverses an
// association FROM a variable (`[Name = $RefProduct/Mod.Product_Category/Name]`).
// Mendix XPath reaches at most one hop off a variable, so this fails the build
// with CE0161 while mxcli accepted it silently (issue #831).
//
// Verified against mxbuild 11.6.6 — the boundary is narrower than it looks:
//
//	$Var/Attr             VALID   a parameter's own attribute
//	$Var/Mod.Assoc        VALID   one hop, the associated object
//	$Var/Mod.Assoc/Attr   CE0161  two or more hops
//
// There is no valid serialization of the two-hop form, which is why this is a
// rejection rather than a writer fix: the constraint has to be restructured, and
// only the author knows which of the two shapes they meant.
func (v *microflowValidator) checkXPathVariableTraversal(variable, xpath string) {
	for _, m := range xpathVarTraversalRe.FindAllStringSubmatch(xpath, -1) {
		root, path := m[1], "$"+m[1]+m[2]
		segs := strings.Split(strings.TrimPrefix(m[2], "/"), "/")
		firstHop, leaf := segs[0], segs[len(segs)-1]
		v.addViolation("MDL055", linter.SeverityError,
			fmt.Sprintf("retrieve '$%s' constraint traverses an association from a variable (`%s`), which Mendix XPath "+
				"does not support (CE0161 \"Error(s) in XPath constraint\") — a constraint reaches at most one hop off a variable",
				variable, path),
			fmt.Sprintf("Retrieve the associated object first, then constrain on its own attribute: "+
				"`retrieve $Related from $%s/%s;` and use `[%s = $Related/%s]`. Or invert the constraint so the "+
				"traversal starts at the entity being retrieved: `[%s/<Module.Entity> = $%s]`. Both forms build clean.",
				root, firstHop, leaf, leaf, firstHop, root))
	}
}

// xpathIdConstraintRe matches a constraint comparing the object id against a VALUE
// (`id = $strVar`, `id = '123'`, `id != 5`). It captures the right-hand operand.
// Comparing `id` against an OBJECT variable (`[id != $ExistingOrder]` — the valid
// "exclude self" pattern) is intentionally NOT matched here: the operand filter in
// checkXPathIdConstraint requires a primitive value, and an object variable is not
// one. `id` is a Mendix reserved member, so a bare `id` in identifier position is
// always the system id.
var xpathIdConstraintRe = regexp.MustCompile(`(?:^|[^\w./])(?i:id)\s*(?:=|!=|<|>)\s*(\$\w+|'[^']*'|-?\d+)`)

// checkXPathIdConstraint flags a retrieve that constrains the object id against a
// stored id VALUE (`where [id = $Id]` with `$Id` a String/Long, or a literal).
// Mendix XPath has no id operator reachable from a microflow expression, so this
// fails the build with CE0161. Comparing `id` to an OBJECT variable is a valid
// identity test and is left alone (its operand is not a primitive). (ledger #42)
func (v *microflowValidator) checkXPathIdConstraint(variable, xpath string) {
	for _, m := range xpathIdConstraintRe.FindAllStringSubmatch(xpath, -1) {
		operand := m[1]
		// `[id = '[%CurrentUser%]']` (and other `'[%…%]'` server tokens) is the
		// standard, build-clean Mendix idiom for the signed-in user's id — Mendix's
		// XPath engine resolves the token to a GUID, so it IS a valid id operand.
		// Only a STORED id value (String/Long variable or a plain literal) is the
		// unsupported case this rule targets. (FINDINGS #53)
		if strings.HasPrefix(operand, "'[%") && strings.HasSuffix(operand, "%]'") {
			continue
		}
		if strings.HasPrefix(operand, "$") {
			// A $-variable operand: flag only when it is a primitive VALUE (a stored
			// id — String/Long/Integer). An object variable is not in varKinds, so an
			// identity comparison (`id != $obj`) is correctly not flagged.
			if _, isPrimitive := v.varKinds[operand[1:]]; !isPrimitive {
				continue
			}
		}
		v.addViolation("MDL048", linter.SeverityError,
			fmt.Sprintf("retrieve '$%s' constrains the object id against a value (`[id = %s]`), which Mendix XPath does not support "+
				"(CE0161 \"Error(s) in XPath constraint\") — there is no id operator reachable from a microflow expression", variable, operand),
			"Retrieve by GUID with a marketplace action (NanoflowCommons GetObjectByGuid / CommunityCommons), "+
				"or expose the id as a String on a view entity (`cast(id as string) as ObjectId`) and constrain on that String column. "+
				"(Comparing id against an OBJECT variable — `[id != $obj]` — IS valid and is not flagged.)")
		return
	}
}

// checkAssociationObjectArgs flags a call argument bound to an association-object
// path (`$obj/Module.Assoc`, which yields the associated OBJECT). Mendix rejects
// an association path used as a value with CE0117 — it must be materialized first
// (`retrieve $x from $obj/Module.Assoc;`). An attribute value over an association
// (`$obj/Module.Assoc/Attr`) is a legal value and is NOT flagged. (ledger #43/#44)
func (v *microflowValidator) checkAssociationObjectArgs(callee string, args []ast.CallArgument) {
	for _, a := range args {
		if exprIsAssociationObjectPath(a.Value) {
			v.addViolation("MDL049", linter.SeverityError,
				fmt.Sprintf("call %s: argument '%s' passes an association path (an object reached over an association), "+
					"which Mendix rejects as a value (CE0117 \"Error(s) in expression\")", callee, a.Name),
				fmt.Sprintf("Materialize the object first, then pass the variable: `retrieve $%s from %s;` then `%s = $%s`.",
					a.Name, microflowExprSource(a.Value), a.Name, a.Name))
		}
	}
}

// exprIsAssociationObjectPath reports whether an expression is an attribute path
// whose FINAL segment is a module-qualified association (`$obj/Module.Assoc`) —
// i.e. it resolves to an associated OBJECT, not an attribute value. A final bare
// segment (`$obj/Module.Assoc/Attr` → `Attr`) is an attribute and returns false.
func exprIsAssociationObjectPath(expr ast.Expression) bool {
	if se, ok := expr.(*ast.SourceExpr); ok {
		expr = se.Expression
	}
	ap, ok := expr.(*ast.AttributePathExpr)
	if !ok || len(ap.Path) == 0 {
		return false
	}
	return strings.Contains(ap.Path[len(ap.Path)-1], ".")
}

// dateTimeLiteralFuncs are the date-construction functions whose arguments
// Mendix requires to be literal numeric constants — a variable or computed
// argument fails the build with CE0117.
var dateTimeLiteralFuncs = map[string]bool{"datetime": true, "datetimeutc": true}

// checkDateTimeLiterals flags a dateTime()/dateTimeUTC() call with a non-literal
// argument. Mendix builds these from hardcoded numeric constants only; a
// variable or expression (`dateTime(2026, $Month, $Day)`) is CE0117. (ledger #21)
func (v *microflowValidator) checkDateTimeLiterals(label string, expr ast.Expression) {
	if exprHasNonLiteralDateTime(expr) {
		v.addViolation("MDL046", linter.SeverityError,
			fmt.Sprintf("%s calls dateTime()/dateTimeUTC() with a non-literal argument, which Mendix rejects "+
				"(CE0117 \"Error(s) in expression\") — these functions accept only hardcoded numeric constants", label),
			"Step off a literal anchor date instead: `addDays(addMonths(dateTime(2026,1,1), $Month - 1), $Day - 1)` (addDays/addMonths take variables).")
	}
}

// exprHasNonLiteralDateTime reports whether the tree contains a dateTime/
// dateTimeUTC call any of whose arguments is not a plain numeric literal.
func exprHasNonLiteralDateTime(expr ast.Expression) bool {
	switch e := expr.(type) {
	case *ast.FunctionCallExpr:
		if dateTimeLiteralFuncs[strings.ToLower(e.Name)] {
			for _, arg := range e.Arguments {
				if _, ok := arg.(*ast.LiteralExpr); !ok {
					return true
				}
			}
		}
		for _, arg := range e.Arguments {
			if exprHasNonLiteralDateTime(arg) {
				return true
			}
		}
	case *ast.BinaryExpr:
		return exprHasNonLiteralDateTime(e.Left) || exprHasNonLiteralDateTime(e.Right)
	case *ast.UnaryExpr:
		return exprHasNonLiteralDateTime(e.Operand)
	case *ast.ParenExpr:
		return exprHasNonLiteralDateTime(e.Inner)
	case *ast.IfThenElseExpr:
		return exprHasNonLiteralDateTime(e.Condition) || exprHasNonLiteralDateTime(e.ThenExpr) || exprHasNonLiteralDateTime(e.ElseExpr)
	case *ast.SourceExpr:
		return exprHasNonLiteralDateTime(e.Expression)
	}
	return false
}

// microflowExprSource returns the Mendix source text of a microflow value
// expression: the preserved raw source when available, otherwise the structured
// expression rendered back to a string. Returns "" when nothing is available.
func microflowExprSource(expr ast.Expression) string {
	if expr == nil {
		return ""
	}
	if se, ok := expr.(*ast.SourceExpr); ok && se.Source != "" {
		return se.Source
	}
	return expressionToString(expr)
}

// astKindToExprKind maps an MDL primitive data-type kind to an exprcheck kind.
// Returns false for non-primitive / unmappable kinds (entities, lists, void).
func astKindToExprKind(k ast.DataTypeKind) (exprcheck.TypeKind, bool) {
	switch k {
	case ast.TypeString, ast.TypeStringTemplate:
		return exprcheck.KindString, true
	case ast.TypeInteger, ast.TypeAutoNumber:
		return exprcheck.KindInteger, true
	case ast.TypeLong:
		return exprcheck.KindLong, true
	case ast.TypeDecimal:
		return exprcheck.KindDecimal, true
	case ast.TypeBoolean:
		return exprcheck.KindBoolean, true
	case ast.TypeDateTime, ast.TypeDate:
		return exprcheck.KindDateTime, true
	case ast.TypeBinary:
		return exprcheck.KindBinary, true
	case ast.TypeEnumeration:
		return exprcheck.KindEnumeration, true
	default:
		return exprcheck.KindUnknown, false
	}
}

// checkErrorHandlingInLoop warns if custom error handling is used inside a loop.
// Mendix requires error handling to be 'Rollback' inside looped activities (CE0644, CE6035).
func (v *microflowValidator) checkErrorHandlingInLoop(stmt ast.MicroflowStatement, eh *ast.ErrorHandlingClause) {
	if v.loopDepth == 0 {
		return // Not inside a loop
	}

	// Only Rollback is allowed inside loops
	if eh.Type != ast.ErrorHandlingRollback && eh.Type != "" {
		activityName := stmtActivityName(stmt)
		v.addViolation("MDL006", linter.SeverityWarning,
			fmt.Sprintf("%s has error handling type '%s' inside a loop. "+
				"Mendix requires error handling to be 'Rollback' inside looped activities (CE0644).",
				activityName, eh.Type),
			"Extract the activity with custom error handling into a submicroflow")
	}
}

// stmtActivityName returns a human-readable name for a statement type.
func stmtActivityName(stmt ast.MicroflowStatement) string {
	switch stmt.(type) {
	case *ast.CreateObjectStmt:
		return "create"
	case *ast.DeleteObjectStmt:
		return "delete"
	case *ast.MfCommitStmt:
		return "commit"
	case *ast.RetrieveStmt:
		return "retrieve"
	case *ast.CallMicroflowStmt:
		return "call microflow"
	case *ast.CallNanoflowStmt:
		return "call nanoflow"
	case *ast.CallJavaActionStmt:
		return "call java action"
	case *ast.CallJavaScriptActionStmt:
		return "call javascript action"
	case *ast.CallWebServiceStmt:
		return "call web service"
	case *ast.ExecuteDatabaseQueryStmt:
		return "execute database query"
	default:
		return "Activity"
	}
}

// checkReturn validates a RETURN statement against the microflow's return type.
func (v *microflowValidator) checkReturn(stmt *ast.ReturnStmt) {
	isVoid := v.returnType == nil || v.returnType.Type.Kind == ast.TypeVoid
	hasValue := stmt.Value != nil

	// Check 1: RETURN with no value when microflow has a return type
	if !isVoid && !hasValue {
		v.addViolation("MDL004", linter.SeverityError,
			fmt.Sprintf("return requires a value because microflow returns %s",
				returnTypeString(v.returnType)),
			fmt.Sprintf("Add a return value of type %s", returnTypeString(v.returnType)))
		return
	}

	// Check 2: RETURN with value when microflow returns Void
	if isVoid && hasValue {
		// Allow RETURN empty; on void microflows (it's a no-op)
		if lit, ok := stmt.Value.(*ast.LiteralExpr); ok {
			if lit.Kind == ast.LiteralEmpty || lit.Kind == ast.LiteralNull {
				return
			}
		}
		v.addViolation("MDL004", linter.SeverityError,
			"return has a value but microflow does not declare a return type",
			"Remove the return value or add a return type to the microflow")
		return
	}

	// Check 4: literal RETURN from entity-typed microflow
	if !isVoid && hasValue {
		retKind := v.returnType.Type.Kind
		if retKind == ast.TypeEntity || retKind == ast.TypeListOf {
			if isScalarLiteral(stmt.Value) {
				v.addViolation("MDL004", linter.SeverityError,
					fmt.Sprintf("return has a %s literal but microflow returns %s",
						literalKindName(stmt.Value), returnTypeString(v.returnType)),
					fmt.Sprintf("Return an object of type %s instead of a scalar literal", returnTypeString(v.returnType)))
			}
		}
	}
}

// isScalarLiteral returns true if the expression is a string, integer, boolean, or decimal literal.
func isScalarLiteral(expr ast.Expression) bool {
	lit, ok := expr.(*ast.LiteralExpr)
	if !ok {
		return false
	}
	switch lit.Kind {
	case ast.LiteralString, ast.LiteralInteger, ast.LiteralDecimal, ast.LiteralBoolean:
		return true
	}
	return false
}

// literalKindName returns a human-readable name for a literal expression's kind.
func literalKindName(expr ast.Expression) string {
	lit, ok := expr.(*ast.LiteralExpr)
	if !ok {
		return "unknown"
	}
	switch lit.Kind {
	case ast.LiteralString:
		return "String"
	case ast.LiteralInteger:
		return "Integer"
	case ast.LiteralDecimal:
		return "Decimal"
	case ast.LiteralBoolean:
		return "Boolean"
	default:
		return "unknown"
	}
}

// returnTypeString formats a MicroflowReturnType for display in messages.
func returnTypeString(rt *ast.MicroflowReturnType) string {
	if rt == nil {
		return "Void"
	}
	switch rt.Type.Kind {
	case ast.TypeEntity:
		if rt.Type.EntityRef != nil {
			return rt.Type.EntityRef.String()
		}
		return "Entity"
	case ast.TypeListOf:
		if rt.Type.EntityRef != nil {
			return "List of " + rt.Type.EntityRef.String()
		}
		return "List"
	default:
		return rt.Type.Kind.String()
	}
}

// bodyReturns returns true if all execution paths in the body end with a RETURN.
func bodyReturns(stmts []ast.MicroflowStatement) bool {
	if len(stmts) == 0 {
		return false
	}
	// Check from the last statement backwards for a RETURN or exhaustive IF/ELSE
	last := stmts[len(stmts)-1]
	switch s := last.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.IfStmt:
		// Both branches must return, and ELSE must be present
		return len(s.ElseBody) > 0 && bodyReturns(s.ThenBody) && bodyReturns(s.ElseBody)
	case *ast.WhileStmt:
		return isUnconditionalTrueWhile(s) && !containsBreakForCurrentLoop(s.Body)
	case *ast.EnumSplitStmt:
		// else is not supported by Mendix; treat the split as exhaustive if
		// every explicit case ends with a return. Unhandled enum values fall
		// through to the next statement, so callers should add a return after
		// end case when the split may not cover all values.
		if len(s.Cases) == 0 {
			return false
		}
		for _, c := range s.Cases {
			if !bodyReturns(c.Body) {
				return false
			}
		}
		return true
	case *ast.InheritanceSplitStmt:
		if len(s.Cases) == 0 || len(s.ElseBody) == 0 || !bodyReturns(s.ElseBody) {
			return false
		}
		for _, c := range s.Cases {
			if !bodyReturns(c.Body) {
				return false
			}
		}
		return true
	}
	return false
}

func isUnconditionalTrueWhile(s *ast.WhileStmt) bool {
	if s == nil {
		return false
	}
	lit, ok := s.Condition.(*ast.LiteralExpr)
	if !ok || lit.Kind != ast.LiteralBoolean {
		return false
	}
	value, ok := lit.Value.(bool)
	return ok && value
}

// checkBranchScoping detects variables declared inside IF/ELSE branches that are
// referenced in subsequent statements at the same level.
func (v *microflowValidator) checkBranchScoping(body []ast.MicroflowStatement) {
	// Collect variables that are only declared inside branches
	branchVars := make(map[string]string) // varName -> "IF branch" / "ELSE branch" / "ON ERROR body"

	for i, s := range body {
		switch stmt := s.(type) {
		case *ast.IfStmt:
			// Collect vars declared in THEN branch
			for varName := range collectDeclaredVars(stmt.ThenBody) {
				branchVars[varName] = "if branch"
			}
			// Collect vars declared in ELSE branch
			for varName := range collectDeclaredVars(stmt.ElseBody) {
				branchVars[varName] = "else branch"
			}
			// Recurse into branches for nested scoping checks
			v.checkBranchScoping(stmt.ThenBody)
			v.checkBranchScoping(stmt.ElseBody)
		case *ast.EnumSplitStmt:
			for _, c := range stmt.Cases {
				for varName := range collectDeclaredVars(c.Body) {
					branchVars[varName] = "enum split branch"
				}
				v.checkBranchScoping(c.Body)
			}
			for varName := range collectDeclaredVars(stmt.ElseBody) {
				branchVars[varName] = "enum split else branch"
			}
			v.checkBranchScoping(stmt.ElseBody)
		case *ast.InheritanceSplitStmt:
			for _, c := range stmt.Cases {
				for varName := range collectDeclaredVars(c.Body) {
					branchVars[varName] = "split type branch"
				}
				v.checkBranchScoping(c.Body)
			}
			for varName := range collectDeclaredVars(stmt.ElseBody) {
				branchVars[varName] = "split type else branch"
			}
			v.checkBranchScoping(stmt.ElseBody)
		case *ast.LoopStmt:
			v.checkBranchScoping(stmt.Body)
		}

		// Check ON ERROR bodies
		if eh := stmtErrorHandling(s); eh != nil && len(eh.Body) > 0 {
			for varName := range collectDeclaredVars(eh.Body) {
				branchVars[varName] = "on error body"
			}
			v.checkBranchScoping(eh.Body)
		}

		// After processing this statement, check if subsequent statements reference branch vars
		if len(branchVars) > 0 {
			for _, subsequent := range body[i+1:] {
				for _, refVar := range referencedVars(subsequent) {
					if scope, ok := branchVars[refVar]; ok {
						v.addViolation("MDL005", linter.SeverityWarning,
							fmt.Sprintf("variable '$%s' is declared inside %s but used outside",
								refVar, scope),
							fmt.Sprintf("Declare '$%s' before the if/else block", refVar))
						// Remove to avoid duplicate warnings
						delete(branchVars, refVar)
					}
				}
			}
		}
	}
}

// collectDeclaredVars returns the set of variable names declared in a body.
func collectDeclaredVars(body []ast.MicroflowStatement) map[string]bool {
	vars := make(map[string]bool)
	for _, s := range body {
		switch stmt := s.(type) {
		case *ast.DeclareStmt:
			vars[stmt.Variable] = true
		case *ast.CreateObjectStmt:
			if stmt.Variable != "" {
				vars[stmt.Variable] = true
			}
		case *ast.RetrieveStmt:
			if stmt.Variable != "" {
				vars[stmt.Variable] = true
			}
		case *ast.CallMicroflowStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.CallNanoflowStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.CallJavaActionStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.CallJavaScriptActionStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.ExecuteDatabaseQueryStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.ListOperationStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.AggregateListStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.CreateListStmt:
			if stmt.Variable != "" {
				vars[stmt.Variable] = true
			}
		case *ast.EnumSplitStmt:
		case *ast.CastObjectStmt:
			if stmt.OutputVariable != "" {
				vars[stmt.OutputVariable] = true
			}
		case *ast.InheritanceSplitStmt:
			for _, c := range stmt.Cases {
				for varName := range collectDeclaredVars(c.Body) {
					vars[varName] = true
				}
			}
			for varName := range collectDeclaredVars(stmt.ElseBody) {
				vars[varName] = true
			}
		}
	}
	return vars
}

// referencedVars returns the variable names referenced in a statement (SET targets, RETURN values, etc.).
func referencedVars(stmt ast.MicroflowStatement) []string {
	var refs []string
	switch s := stmt.(type) {
	case *ast.MfSetStmt:
		// SET $Var = expr — the target variable is a reference
		refs = append(refs, extractVarName(s.Target))
		refs = append(refs, exprVarRefs(s.Value)...)
	case *ast.ReturnStmt:
		if s.Value != nil {
			refs = append(refs, exprVarRefs(s.Value)...)
		}
	case *ast.ChangeObjectStmt:
		refs = append(refs, s.Variable)
	case *ast.MfCommitStmt:
		refs = append(refs, s.Variable)
	case *ast.DeleteObjectStmt:
		refs = append(refs, s.Variable)
	case *ast.AddToListStmt:
		if s.Value != nil {
			refs = append(refs, exprVarRefs(s.Value)...)
		} else {
			refs = append(refs, s.Item)
		}
		refs = append(refs, s.List)
	case *ast.RemoveFromListStmt:
		refs = append(refs, s.Item, s.List)
	case *ast.LogStmt:
		refs = append(refs, exprVarRefs(s.Node)...)
		refs = append(refs, exprVarRefs(s.Message)...)
	case *ast.EnumSplitStmt:
		refs = append(refs, extractVarName(s.Variable))
	case *ast.CastObjectStmt:
		if s.ObjectVariable != "" {
			refs = append(refs, s.ObjectVariable)
		}
	case *ast.InheritanceSplitStmt:
		refs = append(refs, s.Variable)
		for _, c := range s.Cases {
			for _, nested := range c.Body {
				refs = append(refs, referencedVars(nested)...)
			}
		}
		for _, nested := range s.ElseBody {
			refs = append(refs, referencedVars(nested)...)
		}
	}
	return refs
}

// extractVarName extracts the base variable name from a target that may include
// a $ prefix or attribute path (e.g., "$Var/Attr" → "Var").
func extractVarName(target string) string {
	name := strings.TrimPrefix(target, "$")
	if before, _, ok := strings.Cut(name, "/"); ok {
		return before
	}
	return name
}

// exprVarRefs extracts variable names referenced in an expression.
func exprVarRefs(expr ast.Expression) []string {
	if expr == nil {
		return nil
	}
	var refs []string
	switch e := expr.(type) {
	case *ast.VariableExpr:
		refs = append(refs, e.Name)
	case *ast.AttributePathExpr:
		refs = append(refs, e.Variable)
	case *ast.BinaryExpr:
		refs = append(refs, exprVarRefs(e.Left)...)
		refs = append(refs, exprVarRefs(e.Right)...)
	case *ast.UnaryExpr:
		refs = append(refs, exprVarRefs(e.Operand)...)
	case *ast.FunctionCallExpr:
		for _, arg := range e.Arguments {
			refs = append(refs, exprVarRefs(arg)...)
		}
	case *ast.ParenExpr:
		refs = append(refs, exprVarRefs(e.Inner)...)
	case *ast.IfThenElseExpr:
		refs = append(refs, exprVarRefs(e.Condition)...)
		refs = append(refs, exprVarRefs(e.ThenExpr)...)
		refs = append(refs, exprVarRefs(e.ElseExpr)...)
	case *ast.SourceExpr:
		refs = append(refs, exprVarRefs(e.Expression)...)
	}
	return refs
}

// stmtErrorHandling returns the ErrorHandlingClause for statements that support it.
func stmtErrorHandling(stmt ast.MicroflowStatement) *ast.ErrorHandlingClause {
	switch s := stmt.(type) {
	case *ast.CreateObjectStmt:
		return s.ErrorHandling
	case *ast.DeleteObjectStmt:
		return s.ErrorHandling
	case *ast.MfCommitStmt:
		return s.ErrorHandling
	case *ast.RetrieveStmt:
		return s.ErrorHandling
	case *ast.CallMicroflowStmt:
		return s.ErrorHandling
	case *ast.CallNanoflowStmt:
		return s.ErrorHandling
	case *ast.CallJavaActionStmt:
		return s.ErrorHandling
	case *ast.DownloadFileStmt:
		return s.ErrorHandling
	case *ast.CallJavaScriptActionStmt:
		return s.ErrorHandling
	case *ast.CallWebServiceStmt:
		return s.ErrorHandling
	case *ast.ExecuteDatabaseQueryStmt:
		return s.ErrorHandling
	}
	return nil
}

// isEmptyInit checks if a variable initializer is empty/nil (used to detect "DECLARE $List List of ... = empty").
func isEmptyInit(expr ast.Expression) bool {
	if expr == nil {
		return true
	}
	if lit, ok := expr.(*ast.LiteralExpr); ok {
		return lit.Kind == ast.LiteralEmpty || lit.Kind == ast.LiteralNull
	}
	return false
}

// isEmptyMessage checks if a message expression is empty or nil.
func isEmptyMessage(expr ast.Expression) bool {
	if expr == nil {
		return true
	}
	if lit, ok := expr.(*ast.LiteralExpr); ok {
		if lit.Kind == ast.LiteralString {
			if s, ok := lit.Value.(string); ok && s == "" {
				return true
			}
		}
		if lit.Kind == ast.LiteralEmpty || lit.Kind == ast.LiteralNull {
			return true
		}
	}
	return false
}
