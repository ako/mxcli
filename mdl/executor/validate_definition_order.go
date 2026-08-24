// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation of definition order within one script.
//
// The executor resolves most cross-document references at the moment it writes
// the referring document, so a statement naming something a LATER statement
// creates fails partway through `exec` — after earlier statements have already
// been written, because `exec` is not transactional. `ValidateProgram` (the
// --references tier) cannot see this: it collects every name in the script up
// front, so a forward reference is indistinguishable from a backward one and
// the script is reported clean.
//
// `exec` already computes the diagnosis — annotateForwardRef appends
// "… is defined later in this script — move its create statement before this
// one" — but only once a statement has failed, which is one write too late.
// This pass says the same thing before anything is written. (issue #955)
//
// ValidateScriptPageOrder is the same idea for a widget's page reference and
// came first; this covers the other reference kinds. The two are kept separate
// because a page reference is found by walking a widget tree rather than a
// statement's own fields, and MDL-PAGE01 is already an established code.
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// Document kinds a forward reference can denote. These are namespaces for the
// purposes of this rule, not Mendix metamodel types.
const (
	defEntity      = "entity"
	defEnumeration = "enumeration"
	defMicroflow   = "microflow"
	defNanoflow    = "nanoflow"
	defPage        = "page"
)

// defKey identifies a document by kind and qualified name. Kind is part of the
// key because Mendix namespaces documents by type — an entity and a microflow
// may share a qualified name without colliding.
type defKey struct{ kind, name string }

// defRef is one reference the executor resolves EAGERLY, i.e. at the moment it
// writes the referring document.
//
// kinds lists the document kinds the name could denote. It is usually one, but
// a bare `Module.Name` written as a type is genuinely ambiguous: the MDL visitor
// records it as TypeEnumeration with EnumRef set whether the author meant an
// entity or an enumeration (see "TypeEnumeration vs TypeEntity Ambiguity" in
// CLAUDE.md), so both are accepted and whichever the script defines is the one
// that matters.
type defRef struct {
	name  string
	kinds []string
	site  string // where the reference sits, for the message
}

// ValidateScriptDefinitionOrder flags (MDL-ORDER01) a statement that references
// a document created LATER in the same script by a plain CREATE.
//
// The "plain CREATE" condition is what makes this sound without a project, and
// is the same argument ValidateScriptPageOrder rests on: a plain CREATE fails if
// the document already exists, so a script containing one asserts the document
// does not exist yet — the earlier reference therefore cannot resolve against
// the project either. `CREATE OR MODIFY` / `CREATE OR REPLACE` carry no such
// assertion (the document may well already be in the project, in which case the
// forward reference resolves fine), so those are left alone here.
//
// Only reference kinds MEASURED to fail at exec are included. Several plausible
// ones do NOT fail and are deliberately absent, because flagging them would
// reject scripts that work today:
//
//   - an entity's EXTENDS generalization
//   - CALL JAVA ACTION / CALL JAVASCRIPT ACTION
//   - RETRIEVE … FROM an entity
//   - CREATE <entity> inside a microflow body
//
// The matrix behind that split is in mdl-examples/bug-tests/955-*.mdl.
func ValidateScriptDefinitionOrder(prog *ast.Program) []linter.Violation {
	if prog == nil {
		return nil
	}
	plain := plainCreateIndex(prog)
	if len(plain) == 0 {
		return nil
	}

	created := map[defKey]bool{}
	var out []linter.Violation
	for i, stmt := range prog.Statements {
		for _, ref := range eagerDefRefs(stmt) {
			kind, at, ok := laterPlainCreate(plain, created, ref)
			// at == i is a document referring to itself (a recursive microflow
			// call, an entity whose attribute names its own enumeration); the
			// executor has the document in hand by then, so it is not an
			// ordering problem.
			if !ok || at <= i {
				continue
			}
			out = append(out, linter.Violation{
				RuleID:   "MDL-ORDER01",
				Severity: linter.SeverityError,
				Message: fmt.Sprintf(
					"%s references %s %s before it is created — the executor resolves this in statement order, so it fails partway through `exec` with earlier statements already written",
					ref.site, kind, ref.name),
				Suggestion: fmt.Sprintf(
					"Move the CREATE statement for %s above this one.", ref.name),
			})
		}
		markCreated(created, stmt)
	}
	return out
}

// plainCreateIndex maps each document a plain CREATE defines to the index of
// that statement. Only the FIRST plain CREATE of a name is recorded: a second
// one is a duplicate definition, which CheckScriptDuplicates reports.
func plainCreateIndex(prog *ast.Program) map[defKey]int {
	out := map[defKey]int{}
	for i, stmt := range prog.Statements {
		key, plain := definedBy(stmt)
		if key.name == "" || !plain {
			continue
		}
		if _, seen := out[key]; !seen {
			out[key] = i
		}
	}
	return out
}

// markCreated records a document as existing from this statement onward. Unlike
// plainCreateIndex this counts EVERY create form: a `CREATE OR MODIFY` earlier
// in the script has still put the document in the project by the time a later
// statement refers to it.
func markCreated(created map[defKey]bool, stmt ast.Statement) {
	if key, _ := definedBy(stmt); key.name != "" {
		created[key] = true
	}
}

// definedBy reports the document a statement creates and whether it does so
// with a plain CREATE (as opposed to CREATE OR MODIFY / OR REPLACE).
func definedBy(stmt ast.Statement) (key defKey, plain bool) {
	switch s := stmt.(type) {
	case *ast.CreateEntityStmt:
		return qualifiedKey(defEntity, s.Name), !s.CreateOrModify
	case *ast.CreateEnumerationStmt:
		return qualifiedKey(defEnumeration, s.Name), !s.CreateOrModify
	case *ast.CreateMicroflowStmt:
		return qualifiedKey(defMicroflow, s.Name), !s.CreateOrModify
	case *ast.CreateNanoflowStmt:
		return qualifiedKey(defNanoflow, s.Name), !s.CreateOrModify
	case *ast.CreatePageStmtV3:
		return qualifiedKey(defPage, s.Name), !s.IsModify && !s.IsReplace
	}
	return defKey{}, false
}

// qualifiedKey builds a key, or the zero key for an unqualified name (which
// this rule cannot resolve and so ignores).
func qualifiedKey(kind string, name ast.QualifiedName) defKey {
	if name.Module == "" {
		return defKey{}
	}
	return defKey{kind: kind, name: name.String()}
}

// laterPlainCreate reports the kind and statement index of a plain CREATE that
// defines ref, when the name has not already been created earlier in the script.
func laterPlainCreate(plain map[defKey]int, created map[defKey]bool, ref defRef) (kind string, at int, ok bool) {
	for _, k := range ref.kinds {
		key := defKey{kind: k, name: ref.name}
		if created[key] {
			return "", 0, false // exists by now; nothing to report for any kind
		}
	}
	for _, k := range ref.kinds {
		if idx, found := plain[defKey{kind: k, name: ref.name}]; found {
			return k, idx, true
		}
	}
	return "", 0, false
}

// eagerDefRefs returns the references a statement resolves at write time.
func eagerDefRefs(stmt ast.Statement) []defRef {
	switch s := stmt.(type) {
	case *ast.CreateMicroflowStmt:
		return flowDefRefs("microflow "+s.Name.String(), s.Parameters, s.ReturnType, s.Body)
	case *ast.CreateNanoflowStmt:
		return flowDefRefs("nanoflow "+s.Name.String(), s.Parameters, s.ReturnType, s.Body)
	case *ast.CreateRuleStmt:
		return flowDefRefs("rule "+s.Name.String(), s.Parameters, s.ReturnType, s.Body)

	case *ast.CreateEntityStmt:
		// Attribute types only. An entity's EXTENDS generalization is resolved
		// lazily and a forward one executes correctly (measured), so it is not
		// an ordering error.
		site := "entity " + s.Name.String()
		var out []defRef
		for _, attr := range s.Attributes {
			out = append(out, typeDefRefs(attr.Type, fmt.Sprintf("%s: attribute %q", site, attr.Name))...)
		}
		return out

	case *ast.CreateAssociationStmt:
		site := "association " + s.Name.String()
		var out []defRef
		for _, end := range []ast.QualifiedName{s.Parent, s.Child} {
			if end.Module != "" {
				out = append(out, defRef{name: end.String(), kinds: []string{defEntity}, site: site})
			}
		}
		return out

	case *ast.GrantEntityAccessStmt:
		return oneDefRef(s.Entity, defEntity, "grant on entity "+s.Entity.String())
	case *ast.GrantMicroflowAccessStmt:
		return oneDefRef(s.Microflow, defMicroflow, "grant execute on microflow "+s.Microflow.String())
	case *ast.GrantNanoflowAccessStmt:
		return oneDefRef(s.Nanoflow, defNanoflow, "grant execute on nanoflow "+s.Nanoflow.String())
	case *ast.GrantPageAccessStmt:
		return oneDefRef(s.Page, defPage, "grant view on page "+s.Page.String())
	}
	return nil
}

func oneDefRef(name ast.QualifiedName, kind, site string) []defRef {
	if name.Module == "" {
		return nil
	}
	return []defRef{{name: name.String(), kinds: []string{kind}, site: site}}
}

// flowDefRefs collects the eager references of a microflow, nanoflow or rule:
// its parameter types, its return type, and the flows its body calls.
func flowDefRefs(site string, params []ast.MicroflowParam, ret *ast.MicroflowReturnType, body []ast.MicroflowStatement) []defRef {
	var out []defRef
	for _, p := range params {
		out = append(out, typeDefRefs(p.Type, fmt.Sprintf("%s: parameter %q", site, p.Name))...)
	}
	if ret != nil {
		out = append(out, typeDefRefs(ret.Type, site+": return type")...)
	}
	for _, stmt := range flattenFlowBody(body) {
		switch c := stmt.(type) {
		case *ast.CallMicroflowStmt:
			out = append(out, oneDefRef(c.MicroflowName, defMicroflow, site)...)
		case *ast.CallNanoflowStmt:
			out = append(out, oneDefRef(c.NanoflowName, defNanoflow, site)...)
		}
	}
	return out
}

// typeDefRefs turns a data type into the document it names, if any.
//
// Both EntityRef and EnumRef are read regardless of Kind: a bare qualified name
// lands in EnumRef even when it denotes an entity, and `List of Module.Entity`
// lands in EntityRef, so keying off Kind alone would miss one or the other.
func typeDefRefs(t ast.DataType, site string) []defRef {
	if t.EntityRef != nil && t.EntityRef.Module != "" {
		return []defRef{{name: t.EntityRef.String(), kinds: []string{defEntity}, site: site}}
	}
	if t.EnumRef != nil && t.EnumRef.Module != "" {
		// `Enumeration(Module.Name)` / `ENUM Module.Name` says which it is;
		// a bare `Module.Name` does not, so accept either.
		kinds := []string{defEnumeration, defEntity}
		if t.ExplicitEnum {
			kinds = []string{defEnumeration}
		}
		return []defRef{{name: t.EnumRef.String(), kinds: kinds, site: site}}
	}
	return nil
}

// flattenFlowBody returns every statement in a flow body, descending into the
// containers that hold nested bodies. A call inside a loop or a split is
// resolved exactly like a top-level one, so it has to be reached.
func flattenFlowBody(body []ast.MicroflowStatement) []ast.MicroflowStatement {
	var out []ast.MicroflowStatement
	for _, stmt := range body {
		out = append(out, stmt)
		switch s := stmt.(type) {
		case *ast.LoopStmt:
			out = append(out, flattenFlowBody(s.Body)...)
		case *ast.WhileStmt:
			out = append(out, flattenFlowBody(s.Body)...)
		case *ast.IfStmt:
			out = append(out, flattenFlowBody(s.ThenBody)...)
			out = append(out, flattenFlowBody(s.ElseBody)...)
		case *ast.EnumSplitStmt:
			out = append(out, flattenFlowBody(appendEnumBodies(s))...)
		case *ast.InheritanceSplitStmt:
			for _, branch := range inheritanceBranchBodies(s) {
				out = append(out, flattenFlowBody(branch)...)
			}
		}
		if h := errorHandlerBody(stmt); len(h) > 0 {
			out = append(out, flattenFlowBody(h)...)
		}
	}
	return out
}

// errorHandlerBody returns the custom ON ERROR body of a call activity, which
// is a nested body like any other.
func errorHandlerBody(stmt ast.MicroflowStatement) []ast.MicroflowStatement {
	switch s := stmt.(type) {
	case *ast.CallMicroflowStmt:
		if s.ErrorHandling != nil {
			return s.ErrorHandling.Body
		}
	case *ast.CallNanoflowStmt:
		if s.ErrorHandling != nil {
			return s.ErrorHandling.Body
		}
	case *ast.CallJavaActionStmt:
		if s.ErrorHandling != nil {
			return s.ErrorHandling.Body
		}
	}
	return nil
}
