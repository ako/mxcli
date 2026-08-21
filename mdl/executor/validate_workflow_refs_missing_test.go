// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// wfRefCtx returns a context for a project holding exactly one module M with one
// entity, one microflow, one page and one workflow — so anything else a script
// names has to be a genuine missing reference.
func wfRefCtx(t *testing.T) *ExecContext {
	t.Helper()
	mod := &model.Module{Name: "M"}
	mod.ID = "mod1"
	ent := &domainmodel.Entity{Name: "Ctx"}
	ent.ContainerID = "dm1"
	dm := &domainmodel.DomainModel{Entities: []*domainmodel.Entity{ent}}
	dm.ID = "dm1"
	dm.ContainerID = "mod1"
	mf := &microflows.Microflow{Name: "ACT_Real"}
	mf.ContainerID = "mod1"
	pg := &pages.Page{Name: "RealPage"}
	pg.ContainerID = "mod1"
	wf := &workflows.Workflow{Name: "RealWF"}
	wf.ContainerID = "mod1"

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		ListMicroflowsFunc:   func() ([]*microflows.Microflow, error) { return []*microflows.Microflow{mf}, nil },
		ListPagesFunc:        func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		ListWorkflowsFunc:    func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

// checkScript runs the reference validation the way `check --references` does and
// returns the combined error text ("" when the script validates clean).
func checkScript(t *testing.T, ctx *ExecContext, src string) string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var out []string
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	for _, stmt := range prog.Statements {
		if err := validateWithContext(ctx, stmt, sc); err != nil {
			out = append(out, err.Error())
		}
	}
	return strings.Join(out, "\n")
}

// Issue #943. validateWorkflowParameterMappings explicitly defers a target that
// is not in the project to "the missing-reference check", which was never
// written — so every reference inside a workflow went unvalidated while the same
// mistake inside a plain microflow was caught.
func TestWorkflowRefs_MissingReferencesAreReported(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"call microflow",
			`create workflow M.W parameter $C: M.Ctx begin call microflow M.Nope; end workflow;`,
			"M.Nope",
		},
		{
			"call workflow",
			`create workflow M.W parameter $C: M.Ctx begin call workflow M.NopeWF; end workflow;`,
			"M.NopeWF",
		},
		{
			"user task page",
			`create workflow M.W parameter $C: M.Ctx begin user task T 'c' page M.NopePage outcomes 'A' { } 'B' { }; end workflow;`,
			"M.NopePage",
		},
		{
			"targeting users microflow",
			`create workflow M.W parameter $C: M.Ctx begin user task T 'c' page M.RealPage targeting users microflow M.NopeMF outcomes 'A' { } 'B' { }; end workflow;`,
			"M.NopeMF",
		},
		{
			"context entity",
			`create workflow M.W parameter $C: M.NopeEntity begin call microflow M.ACT_Real; end workflow;`,
			"M.NopeEntity",
		},
		{
			"nested in an outcome",
			`create workflow M.W parameter $C: M.Ctx begin user task T 'c' page M.RealPage outcomes 'A' { call microflow M.NestedNope; } 'B' { }; end workflow;`,
			"M.NestedNope",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := checkScript(t, wfRefCtx(t), c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("missing reference %s was not reported.\ngot: %s", c.want, got)
			}
		})
	}
}

// The workflow's own module is checked for a microflow but was not for a
// workflow, so a typo'd module name reached exec and silently created one.
func TestWorkflowRefs_MissingModuleIsReported(t *testing.T) {
	got := checkScript(t, wfRefCtx(t),
		`create workflow NoSuchMod.W parameter $C: M.Ctx begin call microflow M.ACT_Real; end workflow;`)
	if !strings.Contains(got, "NoSuchMod") {
		t.Errorf("missing module was not reported.\ngot: %s", got)
	}
}

// ALTER WORKFLOW had no case in the switch at all, so it fell through to
// "skip validation" and got nothing — not even a check that the workflow exists.
func TestWorkflowRefs_AlterIsValidated(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"inserted activity's target",
			`alter workflow M.RealWF insert after Step call microflow M.Nope;`,
			"M.Nope",
		},
		{
			"replaced activity's target",
			`alter workflow M.RealWF replace activity Step with call microflow M.Nope;`,
			"M.Nope",
		},
		{
			"the workflow itself",
			`alter workflow M.NoSuchWorkflow insert after Step call microflow M.ACT_Real;`,
			"M.NoSuchWorkflow",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := checkScript(t, wfRefCtx(t), c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("%s was not reported.\ngot: %s", c.want, got)
			}
		})
	}
}

// Controls. Every reference below resolves, so the script must validate clean —
// a check that reports everything is as useless as one that reports nothing.
func TestWorkflowRefs_ValidReferencesPass(t *testing.T) {
	cases := []struct{ name, src string }{
		{"all targets exist", `create workflow M.W parameter $C: M.Ctx begin user task T 'c' page M.RealPage targeting users microflow M.ACT_Real outcomes 'A' { call microflow M.ACT_Real; } 'B' { }; end workflow;`},
		{"call an existing workflow", `create workflow M.W parameter $C: M.Ctx begin call workflow M.RealWF; end workflow;`},
		{"alter an existing workflow", `alter workflow M.RealWF insert after Step call microflow M.ACT_Real;`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := checkScript(t, wfRefCtx(t), c.src); got != "" {
				t.Errorf("valid references were reported as errors: %s", got)
			}
		})
	}
}

// A workflow may target something the same script creates later. Those are not
// in the project yet, so the script context has to exempt them — including
// workflows, which it did not track at all.
func TestWorkflowRefs_CreatedInSameScriptIsExempt(t *testing.T) {
	src := `create microflow M.ACT_New () begin return; end;
create page M.NewPage (title: 'p', layout: Atlas_Core.Atlas_Default) { dynamictext dt (content: 'x') }
create workflow M.W1 parameter $C: M.Ctx begin call microflow M.ACT_New; end workflow;
create workflow M.W2 parameter $C: M.Ctx begin call workflow M.W1; end workflow;`
	if got := checkScript(t, wfRefCtx(t), src); got != "" {
		t.Errorf("references created in the same script were reported: %s", got)
	}
}
