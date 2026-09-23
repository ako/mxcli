// SPDX-License-Identifier: Apache-2.0

package executor

import "strings"

// classifyFlowArgValue decides how an action or data-source argument binds.
//
// Mendix stores a flow argument two ways and does not treat them as
// interchangeable: a reference to a page parameter, snippet parameter or page
// variable is a Forms$PageVariable under the mapping's Variable, while a literal
// or expression is text under Expression. Writing a $-reference as an Expression
// leaves the parameter unbound — Studio Pro reports CE1571 "No argument has been
// selected for parameter 'X' and no default is available" — and mxbuild builds
// that same document at 0 errors, so nothing catches it before the page is opened
// (mendixlabs/mxcli#1140).
//
// The four places that built a mapping each carried their own copy of the old
// `strings.HasPrefix(v, "$")` rule (show_page, microflow, nanoflow, data source).
// One classifier instead: the same argument must bind the same way wherever it is
// written, or the spelling decides the storage.
//
// kind is "" when the value is not a page-variable reference, which leaves the
// caller's existing behaviour untouched. Deliberately in that bucket:
//
//   - $currentObject — the enclosing context object. No Studio Pro reference for
//     the bare form was measured, and mxcli's show_page handling already depends
//     on the context object being inferred rather than named (MDL-PAGEARG01), so
//     changing it on a guess would risk the case that works.
//   - a path or dotted expression ($obj/Module.Assoc, $p.Attr) — an expression,
//     which is where Mendix stores it.
//   - a name that is not a declared object-typed parameter or variable of the
//     document being built — including a primitive parameter, which Mendix binds
//     as an expression (all six Expression-bound mappings in the Workflow Commons
//     reference are Boolean literals).
func (pb *pageBuilder) classifyFlowArgValue(value string) (variable, kind string) {
	name, ok := strings.CutPrefix(value, "$")
	if !ok || name == "" {
		return "", ""
	}
	if strings.ContainsAny(name, "/.") {
		return "", ""
	}
	if strings.EqualFold(name, "currentObject") {
		return "", ""
	}
	if pb.localVariables[name] {
		return value, "local"
	}
	// paramScope holds only the entity-typed parameters, which is the set Mendix
	// binds through a PageVariable.
	if _, isParam := pb.paramScope[name]; isParam {
		if pb.isSnippet {
			return value, "snippet"
		}
		return value, "parameter"
	}
	return "", ""
}

// pageVariableArgValue renders a mapping's Variable — a Forms$PageVariable — as
// the MDL `$name` that produced it, or "" when there is no variable binding.
//
// This is the read half of #1140, and it was wrong in a way that hid the write
// half. The three describe readers looked for a `Name` key on the sub-document;
// Forms$PageVariable has no such property. Studio Pro names the reference in
// whichever of PageParameter / SnippetParameter / LocalVariable applies, so every
// argument in Studio Pro-authored content described as absent: measured on
// Workflow Commons 4.11.0, `DESCRIBE PAGE` printed
//
//	Action: microflow WorkflowCommons.ACT_ConflictedWorkflowHelper_ApplyJumpTo
//
// for a button whose stored mapping binds $ConflictedWorkflowHelper — the same
// describe → exec loss as #835, one storage form over.
//
// Widget is deliberately not read. It names the grid whose SELECTION supplies a
// list argument, which MDL has no syntax for; rendering it as `$widgetName` would
// emit a statement that re-executes into a different binding.
func pageVariableArgValue(raw any) string {
	v, ok := raw.(map[string]any)
	if !ok || v == nil {
		return ""
	}
	for _, key := range []string{"PageParameter", "SnippetParameter", "LocalVariable"} {
		if name := extractString(v[key]); name != "" {
			return "$" + name
		}
	}
	return ""
}
