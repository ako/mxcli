// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// dropIfExistsTargets is every document-level DROP. Each one names a document
// that does not exist in a project that does have the module, so the handler
// reaches its own lookup rather than stopping at "module not found".
var dropIfExistsTargets = []string{
	"DROP ENTITY %s MyModule.Gone",
	"DROP ASSOCIATION %s MyModule.Gone",
	"DROP ENUMERATION %s MyModule.Gone",
	"DROP CONSTANT %s MyModule.Gone",
	"DROP MICROFLOW %s MyModule.Gone",
	"DROP NANOFLOW %s MyModule.Gone",
	"DROP RULE %s MyModule.Gone",
	"DROP PAGE %s MyModule.Gone",
	"DROP LAYOUT %s MyModule.Gone",
	"DROP SNIPPET %s MyModule.Gone",
	"DROP MENU %s MyModule.Gone",
	"DROP MODULE %s Gone",
	"DROP QUEUE %s MyModule.Gone",
	"DROP SCHEDULED EVENT %s MyModule.Gone",
	"DROP REGULAR EXPRESSION %s MyModule.Gone",
	"DROP JAVA ACTION %s MyModule.Gone",
	"DROP JAVASCRIPT ACTION %s MyModule.Gone",
	"DROP ODATA CLIENT %s MyModule.Gone",
	"DROP ODATA SERVICE %s MyModule.Gone",
	"DROP BUSINESS EVENT SERVICE %s MyModule.Gone",
	"DROP WORKFLOW %s MyModule.Gone",
	"DROP IMAGE COLLECTION %s MyModule.Gone",
	"DROP JSON STRUCTURE %s MyModule.Gone",
	"DROP MESSAGE DEFINITION COLLECTION %s MyModule.Gone",
	"DROP IMPORT MAPPING %s MyModule.Gone",
	"DROP EXPORT MAPPING %s MyModule.Gone",
	"DROP REST CLIENT %s MyModule.Gone",
	"DROP PUBLISHED REST SERVICE %s MyModule.Gone",
	"DROP DATA TRANSFORMER %s MyModule.Gone",
	"DROP MODEL %s MyModule.Gone",
	"DROP CONSUMED MCP SERVICE %s MyModule.Gone",
	"DROP KNOWLEDGE BASE %s MyModule.Gone",
	"DROP AGENT %s MyModule.Gone",
	"DROP CONFIGURATION %s 'Gone'",
	"DROP FOLDER %s 'Gone' IN MyModule",
}

func dropIfExistsCtx(t *testing.T) (*ExecContext, *strings.Builder) {
	mod := mkModule("MyModule")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) {
			return &domainmodel.DomainModel{ContainerID: mod.ID}, nil
		},
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return nil, nil },
		ListRulesFunc:     func() ([]*microflows.Rule, error) { return nil, nil },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			return &model.ProjectSettings{Configuration: &model.ConfigurationSettings{}}, nil
		},
		// The by-name lookups return an error for a missing document in the
		// real backend (mdl/backend/modelsdk); mirror it, since a nil, nil
		// from the mock is a shape production never produces.
		GetImportMappingByQualifiedNameFunc: func(m, n string) (*model.ImportMapping, error) {
			return nil, fmt.Errorf("import mapping not found: %s.%s", m, n)
		},
		GetExportMappingByQualifiedNameFunc: func(m, n string) (*model.ExportMapping, error) {
			return nil, fmt.Errorf("export mapping not found: %s.%s", m, n)
		},
		GetJsonStructureByQualifiedNameFunc: func(m, n string) (*types.JsonStructure, error) {
			return nil, fmt.Errorf("json structure not found: %s.%s", m, n)
		},
		GetMenuDocumentByQualifiedNameFunc: func(m, n string) (*types.MenuDocument, error) {
			return nil, fmt.Errorf("menu not found: %s.%s", m, n)
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	var out strings.Builder
	ctx.Output = &out
	return ctx, &out
}

func runDropScript(t *testing.T, ctx *ExecContext, src string) error {
	t.Helper()
	prog, errs := visitor.Build(src + ";")
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("parse %q: got %d statements, want 1", src, len(prog.Statements))
	}
	stmt := prog.Statements[0]
	if _, ok := stmt.(ast.IfExistsDrop); !ok {
		t.Fatalf("%q built %T, which does not carry the IF EXISTS guard", src, stmt)
	}
	return NewRegistry().Dispatch(ctx, stmt)
}

// Issue #531: "DROP PAGE IF EXISTS FieldService.Stub; — line 1:13 extraneous
// input 'EXISTS' expecting the start of a statement". No document-level DROP
// accepted IF EXISTS, so a script that dropped anything could not be re-run.
func TestDropIfExists_MissingTargetIsSkipped(t *testing.T) {
	for _, form := range dropIfExistsTargets {
		src := strings.Replace(form, "%s", "IF EXISTS", 1)
		t.Run(src, func(t *testing.T) {
			ctx, out := dropIfExistsCtx(t)
			if err := runDropScript(t, ctx, src); err != nil {
				t.Fatalf("IF EXISTS on a missing target errored: %v", err)
			}
			if !strings.Contains(out.String(), "skipped") {
				t.Errorf("a skipped drop should say so; output: %q", out.String())
			}
		})
	}
}

// CONTROL: the bare form must still fail on a missing target, and with a
// NotFoundError — that type is what the guard keys on, so a handler that
// reports "not found" any other way would pass the test above only if the
// guard swallowed every error.
func TestDropIfExists_BareFormStillReportsNotFound(t *testing.T) {
	for _, form := range dropIfExistsTargets {
		src := strings.Replace(form, "%s ", "", 1)
		t.Run(src, func(t *testing.T) {
			ctx, _ := dropIfExistsCtx(t)
			err := runDropScript(t, ctx, src)
			if err == nil {
				t.Fatal("dropping a missing target succeeded without IF EXISTS")
			}
			var nf *mdlerrors.NotFoundError
			if !errors.As(err, &nf) {
				t.Errorf("missing target reported as %T (%v), want *NotFoundError", err, err)
			}
		})
	}
}

// CONTROL: IF EXISTS must not swallow errors other than "not found". A
// disconnected project is the simplest one every handler checks first.
func TestDropIfExists_DoesNotSwallowOtherErrors(t *testing.T) {
	ctx, _ := dropIfExistsCtx(t)
	ctx.Backend = &mock.MockBackend{IsConnectedFunc: func() bool { return false }}
	err := runDropScript(t, ctx, "DROP PAGE IF EXISTS MyModule.Gone")
	if err == nil {
		t.Fatal("IF EXISTS hid a not-connected error")
	}
}

// `check --references` resolves a drop's module and refused one that is absent.
// Under IF EXISTS an absent module means an absent target — the skip exec
// takes — so check must not be the stricter gate.
func TestDropIfExists_ReferenceCheckAcceptsMissingModule(t *testing.T) {
	for _, src := range []string{
		"DROP ENTITY IF EXISTS NoSuchModule.Gone",
		"DROP MODULE IF EXISTS NoSuchModule",
		"DROP IMAGE COLLECTION IF EXISTS NoSuchModule.Gone",
	} {
		t.Run(src, func(t *testing.T) {
			ctx, _ := dropIfExistsCtx(t)
			prog, errs := visitor.Build(src + ";")
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}
			stmt := prog.Statements[0]
			if err := validateWithContext(ctx, stmt, newScriptContext()); err != nil {
				t.Errorf("check refused a guarded drop: %v", err)
			}
			// CONTROL: the unguarded form is still refused.
			stmt.(ast.IfExistsDrop).SetDropIfExists(false)
			if err := validateWithContext(ctx, stmt, newScriptContext()); err == nil {
				t.Error("check accepted an unguarded drop of a missing module")
			}
		})
	}
}
