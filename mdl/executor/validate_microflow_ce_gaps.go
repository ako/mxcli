// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// Three constructs from upstream #893 that parsed, passed `mxcli check`, were
// written by `exec`, and were then rejected by the build. Each was reproduced on
// mxbuild 11.6.6 against a blank app with a 1-error baseline, and each fix
// suggested below was measured to take that app back to the baseline:
//
//	MDL061  declare with no value            CE0038  fix: `= ''`
//	MDL062  return inside a loop             CE0068  fix: `break`
//	MDL063  duplicate variable name          CE0111  fix: drop the declare
//
// Error severity is enough to close the gap the issue was filed about: exec runs
// a pre-flight check over the whole script and refuses with nothing written, so
// "green check, green exec, red Errors pane" no longer happens. Verified end to
// end on the issue's own reproduction — 3 errors reported, 0 microflows written.
//
// They are deliberately NOT added to execEnforcedMicroflowRules, which would
// make `--no-check` refuse them too. That list's bar is a claim verified against
// a real mxbuild, which these meet; the reason to stay off it is different.
// Unlike the XPath rules there, these three predict what the BUILDER emits, and
// building this change turned up four shapes where the AST says "broken" and the
// build says otherwise — two of them in mxcli's own shipped examples. Each is
// exempted and tested below, but a rule with that failure mode should leave the
// author an escape hatch rather than become an unconditional write barrier.

// skipCEGapRules reports whether the three rules stand down for this microflow.
//
// An @excluded document is not part of the build, and mxbuild does not check it:
// measured on 11.6.6, the very microflow that is CE0111 when included produces
// no error at all when excluded. #312 made `check --references` skip excluded
// microflows for that reason — agentic workflows deliberately stash broken
// intermediate state in excluded scaffolding — and error-severity rules block
// `exec`, so flagging them here would undo that fix by another route.
func (v *microflowValidator) skipCEGapRules() bool { return v.excluded }

// checkDeclareHasValue flags a `declare` with no initial value — MDL061.
//
// A declare maps to a Create Variable activity, whose Value property Mendix
// requires: CE0038 "The 'Value' property is required." There is no
// uninitialized variable in a microflow, so this is not a defaultable omission;
// mxcli cannot pick the value without inventing semantics (0? empty? for a
// DateTime, what?), and the author writing the value is a one-token fix.
//
// A list or object declare is left to MDL040/MDL043, which reject the TYPE. On
// those, "supply a value" points at the wrong fix — the activity cannot hold
// the type at all, so adding `= empty` does not help.
func (v *microflowValidator) checkDeclareHasValue(stmt *ast.DeclareStmt) {
	if v.skipCEGapRules() || stmt.InitialValue != nil {
		return
	}
	if stmt.Type.Kind == ast.TypeListOf {
		return // MDL040
	}
	if stmt.Type.Kind == ast.TypeEntity ||
		(stmt.Type.Kind == ast.TypeEnumeration && stmt.Type.EnumRef != nil && !stmt.Type.ExplicitEnum) {
		return // MDL043
	}

	v.addViolation("MDL061", linter.SeverityError,
		fmt.Sprintf("declare '$%s' has no initial value, but the Create Variable activity it builds "+
			"requires one — mxbuild rejects it with CE0038 \"The 'Value' property is required.\". "+
			"Mendix has no uninitialized variable: every variable is created with a value.",
			stmt.Variable),
		fmt.Sprintf("Give it a starting value, e.g. declare $%s %s = %s",
			stmt.Variable, declaredTypeLabel(stmt.Type), zeroValueHint(stmt.Type)))
}

// declaredTypeLabel renders a declare's type the way the statement spells it,
// for use in the suggested replacement line.
func declaredTypeLabel(t ast.DataType) string {
	if t.Kind == ast.TypeEnumeration && t.EnumRef != nil {
		return "Enumeration(" + t.EnumRef.String() + ")"
	}
	return t.Kind.String()
}

// zeroValueHint is the value the suggestion offers as a starting point. It is
// advice printed in a message, never something mxcli writes on the author's
// behalf — picking the value is the author's call, which is the whole reason
// MDL061 is a refusal rather than a silent default.
func zeroValueHint(t ast.DataType) string {
	switch t.Kind {
	case ast.TypeString:
		return "''"
	case ast.TypeInteger, ast.TypeLong:
		return "0"
	case ast.TypeDecimal:
		return "0.0"
	case ast.TypeBoolean:
		return "false"
	default:
		return "empty"
	}
}

// checkReturnInLoop flags a `return` anywhere inside a loop — MDL062.
//
// A return builds an End event, and Mendix forbids one inside a LoopedActivity:
// CE0068 "End events cannot be placed inside a loop." Nesting does not help — a
// return inside a branch inside the loop is still inside the loop — so the walk
// tracks loop depth rather than looking only at the loop's immediate body.
//
// `break` is the construct that replaces it: it leaves the loop, and execution
// continues after it. Returning a value from inside a loop needs the value
// stashed in a variable and a single return after the loop.
//
// The rule predicts what the BUILDER emits, not what the MDL looks like, and
// two forms measured clean on mxbuild 11.6.6 are therefore exempt. Both were
// found by running the rule over the shipped examples before wiring it up:
//
//   - `while true` is built as an ExclusiveMerge back-edge, not a
//     LoopedActivity (#350). With no loop object there is no "inside a loop",
//     and the return is an ordinary End event. A plain `while <cond>` IS a
//     LoopedActivity and is not exempt — measured separately.
//   - `returns T as $Var` makes buildFlowGraph synthesize the End event from
//     the variable, and no End event lands inside the loop. Firing here would
//     name the wrong defect: that shape builds CE0109 ("Undefined variable")
//     instead, which is a separate gap and not what this rule is about.
func (v *microflowValidator) checkReturnInLoop(body []ast.MicroflowStatement) {
	if v.skipCEGapRules() {
		return
	}
	// See the AS-clause note above: the builder routes the return elsewhere.
	if v.returnType != nil && v.returnType.Variable != "" {
		return
	}

	var walk func(stmts []ast.MicroflowStatement, depth int)
	walk = func(stmts []ast.MicroflowStatement, depth int) {
		for _, s := range stmts {
			switch st := s.(type) {
			case *ast.ReturnStmt:
				if depth > 0 {
					v.addViolation("MDL062", linter.SeverityError,
						"return inside a loop builds an End event inside the loop, which Mendix does "+
							"not allow — mxbuild rejects it with CE0068 \"End events cannot be placed "+
							"inside a loop.\"",
						"Use break to leave the loop; to return a value, assign it to a variable "+
							"inside the loop and put a single return after the loop")
				}
			case *ast.LoopStmt:
				walk(st.Body, depth+1)
			case *ast.WhileStmt:
				inner := depth + 1
				if isUnconditionalTrueWhile(st) {
					inner = depth // an ExclusiveMerge back-edge, not a loop object
				}
				walk(st.Body, inner)
			case *ast.IfStmt:
				walk(st.ThenBody, depth)
				walk(st.ElseBody, depth)
			case *ast.EnumSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body, depth)
				}
				walk(st.ElseBody, depth)
			case *ast.InheritanceSplitStmt:
				for _, c := range st.Cases {
					walk(c.Body, depth)
				}
				walk(st.ElseBody, depth)
			}
		}
	}
	walk(body, 0)
}

// buildsAsAProducer reports whether a statement that LOOKS like a producer in
// the AST actually builds as one.
//
// `set $Match = contains($Hay, $Needle)` on two Strings parses as a
// ListOperationStmt — the visitor cannot tell a string function from a list
// operation at parse time — but addListOperationAction rewrites it into a
// Change Variable when the input is a declared String, precisely because a list
// operation would create its output variable and collide with the declare
// (ledger findings #53/#63, whose examples exist to pin that rewrite).
//
// MDL063 must not report the CE0111 that rewrite exists to prevent: both
// examples build at 0 errors above baseline on mxbuild 11.6.6, and an earlier
// draft of this rule flagged them. The operation test is shared with the
// builder (stringOverloadedListOp) so the two cannot drift apart silently.
func (v *microflowValidator) buildsAsAProducer(s ast.MicroflowStatement) bool {
	lo, ok := s.(*ast.ListOperationStmt)
	if !ok {
		return true
	}
	return !(stringOverloadedListOp(lo.Operation) && v.varKinds[lo.InputVariable] == exprcheck.KindString)
}

// stringOverloadedListOp reports whether a list operation shares its name with a
// String function, so the same MDL spells both. Read by the builder, which
// rewrites these into a Change Variable, and by MDL063, which must stay silent
// on exactly the statements that rewrite covers.
func stringOverloadedListOp(op ast.ListOperationType) bool {
	return op == ast.ListOpContains || op == ast.ListOpFind
}

// producedVar is one definition of a variable name within a microflow.
type producedVar struct {
	name  string
	label string // how the definition is described to the author
	loop  bool   // the definition is a loop iterator (MDL052's territory)
}

// checkDuplicateVariableNames flags a name defined more than once — MDL063.
//
// A microflow's variable namespace is FLAT. Measured on mxbuild 11.6.6, each of
// these is CE0111 "Duplicate variable name", isolated one microflow at a time:
// two declares in one body; a declare outside a loop and another inside it; a
// declare in each of two SIBLING if/else branches; two retrieves; a parameter
// and a declare; a loop iterator and a declare. Neither branches nor loop bodies
// open a scope, so the walk deliberately does not track one.
//
// The distinction that matters is create versus assign: `$X = 'b'` after
// `declare $X` is a Change Variable, which defines nothing and measured clean.
// A rule that keyed on "the name appears on the left of `=`" would flag the
// normal way to update a variable.
//
// Iterator-versus-iterator is left to MDL052, whose message explains loop
// scoping; two rules on one line is noise.
func (v *microflowValidator) checkDuplicateVariableNames(params []ast.MicroflowParam, body []ast.MicroflowStatement) {
	if v.skipCEGapRules() {
		return
	}
	first := map[string]producedVar{}

	report := func(p producedVar) {
		prev, seen := first[p.name]
		if !seen {
			first[p.name] = p
			return
		}
		if prev.loop && p.loop {
			return // MDL052
		}
		v.addViolation("MDL063", linter.SeverityError,
			fmt.Sprintf("'$%s' is created twice in this microflow — first by %s, then by %s. "+
				"A microflow's variable names are unique flow-wide (branches and loop bodies do "+
				"not open a scope), so mxbuild rejects this with CE0111 \"Duplicate variable name\".",
				p.name, prev.label, p.label),
			fmt.Sprintf("Rename one of them, or — if you meant to reuse the first — drop the "+
				"redundant definition: %s already creates '$%s', and an activity always creates "+
				"its own output variable rather than writing into an existing one",
				prev.label, p.name))
	}

	for _, p := range params {
		if p.Name != "" {
			report(producedVar{name: p.Name, label: "the microflow parameter"})
		}
	}

	var walk func(stmts []ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			if v.buildsAsAProducer(s) {
				for _, p := range statementProducedVars(s) {
					report(p)
				}
			}
			switch st := s.(type) {
			case *ast.LoopStmt:
				walk(st.Body)
			case *ast.WhileStmt:
				walk(st.Body)
			case *ast.IfStmt:
				walk(st.ThenBody)
				walk(st.ElseBody)
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

// statementProducedVars returns the variables a statement CREATES.
//
// The `OutputVariable` field is read by reflection rather than from a switch:
// fifteen statement types carry it, the name is unambiguous — a field called
// OutputVariable is always a producer — and a statement type added later is
// covered without anyone remembering to extend a list. The alternative, a
// hand-maintained type switch, is exactly the shape of the blind spot that let
// #892's DROP FOLDER guard miss five document kinds.
//
// Reflection is NOT used for the `Variable` field, which is ambiguous: it names
// the produced variable on declare/create/retrieve/create list, and a CONSUMED
// one on change/commit/delete/rollback and on the split statements. Those five
// producers are therefore listed explicitly.
func statementProducedVars(s ast.MicroflowStatement) []producedVar {
	var out []producedVar

	switch st := s.(type) {
	case *ast.DeclareStmt:
		out = append(out, producedVar{name: st.Variable, label: "declare"})
	case *ast.CreateObjectStmt:
		out = append(out, producedVar{name: st.Variable, label: "create"})
	case *ast.CreateListStmt:
		out = append(out, producedVar{name: st.Variable, label: "create list"})
	case *ast.RetrieveStmt:
		out = append(out, producedVar{name: st.Variable, label: "retrieve"})
	case *ast.LoopStmt:
		out = append(out, producedVar{name: st.LoopVariable, label: "the loop iterator", loop: true})
	}

	if name := outputVariableField(s); name != "" {
		out = append(out, producedVar{name: name, label: statementProducerLabel(s)})
	}

	// A producer with an empty name is an optional output the author left off.
	kept := out[:0]
	for _, p := range out {
		if p.name != "" {
			kept = append(kept, p)
		}
	}
	return kept
}

// outputVariableField reads a statement's OutputVariable field, if it has one.
func outputVariableField(s ast.MicroflowStatement) string {
	rv := reflect.ValueOf(s)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return ""
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return ""
	}
	f := rv.FieldByName("OutputVariable")
	if !f.IsValid() || f.Kind() != reflect.String {
		return ""
	}
	return f.String()
}

// statementProducerLabel describes an activity in the author's vocabulary.
// stmtActivityName covers the common ones; anything it does not know falls back
// to the AST type name rather than the generic "Activity", so a statement type
// added later still names itself in the message.
func statementProducerLabel(s ast.MicroflowStatement) string {
	if name := stmtActivityName(s); name != "Activity" {
		return name
	}
	t := reflect.TypeOf(s)
	if t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t == nil {
		return "an activity"
	}
	return "the " + strings.TrimSuffix(t.Name(), "Stmt") + " activity"
}
