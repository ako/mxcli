// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"errors"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// FlowRebuildRule is the rule check reports a `create or modify` of a stored
// flow under when its change cannot be spliced in: an error under mdl 1, where
// exec refuses the statement, and a warning under mdl 0, where exec rebuilds.
const FlowRebuildRule = "MDL-V1-REBUILD"

// alterFlowRefusedRule is check's report of an `alter microflow|nanoflow` exec
// would refuse, under every language version: alter is a patch and never
// falls back to a rebuild.
const alterFlowRefusedRule = "MDL090"

// CheckFlowVerdicts reports, for every `create or modify` and `alter` of a
// microflow or nanoflow the connected project holds, what exec would refuse
// (ako/mxcli#876). It runs the verdict exec acts on (decideFlowModify, and
// alterFlowContext.plan for an alter), so check, diff and exec cannot disagree:
//
//   - a change the splice cannot make is an MDL-V1-REBUILD error with exec's
//     own message under mdl 1, and the MDL-V1-REBUILD warning exec prints under
//     mdl 0, where exec rebuilds the flow instead;
//   - an alter exec would refuse is an MDL090 error, under every version.
//
// The prediction is made against the project as stored, so a statement is only
// predicted where the stored project is what exec will see when it gets there:
//
//   - an earlier statement of the script that creates, alters, drops, renames
//     or moves the same flow leaves it unpredicted, and so does one that
//     renames or drops its module or moves a folder out of or into it;
//   - the declared flow (or each fragment of an alter) must build against the
//     stored project. One that does not usually names something an earlier
//     statement creates; exec builds it after that statement has run, so
//     nothing can be said here, and reference validation has already reported
//     whatever is really missing.
//
// It writes nothing: the patch is applied in memory and discarded.
func (e *Executor) CheckFlowVerdicts(prog *ast.Program) []linter.Violation {
	if e == nil || prog == nil {
		return nil
	}
	defer e.enterLanguage(prog.LanguageVersion)()
	defer e.CacheUnitReads()()
	return CheckFlowVerdicts(e.newExecContext(context.Background()), prog)
}

// CheckFlowVerdicts is the ExecContext-level entry point. A disconnected
// context returns nothing: the verdict is about what the project holds.
func CheckFlowVerdicts(ctx *ExecContext, prog *ast.Program) []linter.Violation {
	if ctx == nil || prog == nil || !ctx.Connected() {
		return nil
	}
	touched := map[string]bool{}        // "microflow:M.N" / "nanoflow:M.N", lower-cased
	touchedModules := map[string]bool{} // modules renamed or dropped, or a folder moved out of or into
	key := func(nanoflow bool, qn ast.QualifiedName) string {
		k := "microflow:"
		if nanoflow {
			k = "nanoflow:"
		}
		return strings.ToLower(k + qn.String())
	}
	fresh := func(nanoflow bool, qn ast.QualifiedName) bool {
		return !touched[key(nanoflow, qn)] && !touchedModules[strings.ToLower(qn.Module)]
	}

	var out []linter.Violation
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			if s.CreateOrModify && fresh(false, s.Name) && validateMicroflowRules(s) == nil {
				out = append(out, flowModifyViolations(ctx, microflowDecl(s))...)
			}
			touched[key(false, s.Name)] = true
		case *ast.CreateNanoflowStmt:
			if s.CreateOrModify && fresh(true, s.Name) {
				out = append(out, flowModifyViolations(ctx, nanoflowDecl(s))...)
			}
			touched[key(true, s.Name)] = true
		case *ast.AlterFlowStmt:
			if fresh(s.Nanoflow, s.Name) {
				out = append(out, alterFlowViolations(ctx, s)...)
			}
			touched[key(s.Nanoflow, s.Name)] = true
		case *ast.DropMicroflowStmt:
			touched[key(false, s.Name)] = true
		case *ast.DropNanoflowStmt:
			touched[key(true, s.Name)] = true
		case *ast.MoveStmt:
			touched[key(false, s.Name)] = true
			touched[key(true, s.Name)] = true
			if s.TargetModule != "" {
				moved := ast.QualifiedName{Module: s.TargetModule, Name: s.Name.Name}
				touched[key(false, moved)] = true
				touched[key(true, moved)] = true
			}
		case *ast.DropModuleStmt:
			// The module's flows go with it: exec creates the flow afterwards.
			touchedModules[strings.ToLower(s.Name)] = true
		case *ast.MoveFolderStmt:
			// The folder's flows go with it; which ones is the project's to
			// say, so neither module is predicted from here on.
			touchedModules[strings.ToLower(s.Name.Module)] = true
			if s.TargetModule != "" {
				touchedModules[strings.ToLower(s.TargetModule)] = true
			}
		case *ast.RenameStmt:
			switch strings.ToLower(s.ObjectType) {
			case "module":
				touchedModules[strings.ToLower(s.Name.Module)] = true
				touchedModules[strings.ToLower(s.NewName)] = true
			default:
				renamed := ast.QualifiedName{Module: s.Name.Module, Name: s.NewName}
				for _, nf := range []bool{false, true} {
					touched[key(nf, s.Name)] = true
					touched[key(nf, renamed)] = true
				}
			}
		}
	}
	return out
}

// flowModifyViolations is the check-time report of decideFlowModify.
func flowModifyViolations(ctx *ExecContext, d *flowDecl) []linter.Violation {
	if _, _, _, err := d.build(ctx); err != nil {
		return nil // not predictable against the stored project (see CheckFlowVerdicts)
	}
	v := decideFlowModify(ctx, d)
	if v.why == nil {
		return nil
	}
	loc := linter.Location{Module: d.name.Module, DocumentType: d.kind(), DocumentName: d.name.Name}
	if v.refused {
		return []linter.Violation{{
			RuleID:   FlowRebuildRule,
			Severity: linter.SeverityError,
			Message:  "exec would refuse this statement: " + flowRefusal(d, v.why).Error(),
			Location: loc,
		}}
	}
	return []linter.Violation{{
		RuleID:   FlowRebuildRule,
		Severity: linter.SeverityWarning,
		Message:  flowRebuildWarning(ctx, d, v.why),
		Location: loc,
	}}
}

// alterFlowViolations is the check-time report of an alter exec would refuse.
func alterFlowViolations(ctx *ExecContext, s *ast.AlterFlowStmt) []linter.Violation {
	a, err := loadAlterFlow(ctx, s)
	if err != nil {
		return nil // no such flow: reference validation reports it
	}
	// A fragment that does not build against the stored project is not
	// predictable (see CheckFlowVerdicts). Only the builder's own errors say
	// so; a fragment that builds and cannot be spliced is a refusal.
	for _, op := range s.Operations {
		if len(op.Body) == 0 {
			continue
		}
		fb := a.fragmentBuilder(ctx)
		fb.buildFlowGraph(op.Body, nil)
		if len(fb.GetErrors()) > 0 {
			return nil
		}
	}
	_, err = a.plan(ctx)
	if err == nil {
		return nil
	}
	var nf *mdlerrors.NotFoundError
	if errors.As(err, &nf) {
		return nil
	}
	return []linter.Violation{{
		RuleID:   alterFlowRefusedRule,
		Severity: linter.SeverityError,
		Message:  "exec would refuse this statement and write nothing: " + err.Error(),
		Location: linter.Location{Module: s.Name.Module, DocumentType: s.Kind(), DocumentName: s.Name.Name},
	}}
}
