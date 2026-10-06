// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// A Boolean constant's DefaultValue is stored as "True" / "False" — that is what
// Studio Pro writes, and its constant dialog reads a stored "true" as False while
// the runtime reads it as true (mendixlabs/mxcli#1321). The literal `true` reached
// the backend as Go's fmt rendering "true"; only the quoted 'True' was right.

var booleanConstantDefaults = []struct {
	name, mdl, want string
}{
	{"lower true", "true", "True"},
	{"title True", "True", "True"},
	{"lower false", "false", "False"},
	{"upper FALSE", "FALSE", "False"},
	{"quoted lower", "'true'", "True"},
	// Already in Studio Pro's form: the control.
	{"quoted True", "'True'", "True"},
}

func TestCreateConstant_BooleanDefaultStoredAsStudioProCase(t *testing.T) {
	for _, tc := range booleanConstantDefaults {
		t.Run(tc.name, func(t *testing.T) {
			mod := mkModule("MyModule")
			var captured *model.Constant
			mb := &mock.MockBackend{
				IsConnectedFunc:   func() bool { return true },
				ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
				ListConstantsFunc: func() ([]*model.Constant, error) { return nil, nil },
				CreateConstantFunc: func(c *model.Constant) error {
					captured = c
					return nil
				},
			}
			ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

			prog := parseMDL(t, "create constant MyModule.Flag (Type: Boolean, DefaultValue: "+tc.mdl+");")
			if err := createConstant(ctx, prog.Statements[0].(*ast.CreateConstantStmt)); err != nil {
				t.Fatalf("createConstant: %v", err)
			}
			if captured == nil {
				t.Fatal("CreateConstant was not called")
			}
			if captured.DefaultValue != tc.want {
				t.Errorf("DefaultValue: %s stored as %q, want %q", tc.mdl, captured.DefaultValue, tc.want)
			}
		})
	}
}

// `create or modify` rewrites an existing constant through UpdateConstant — the
// second write path the same value takes.
func TestCreateOrModifyConstant_BooleanDefaultStoredAsStudioProCase(t *testing.T) {
	mod := mkModule("MyModule")
	existing := &model.Constant{
		ContainerID:  mod.ID,
		Name:         "Flag",
		Type:         model.ConstantDataType{Kind: "Boolean"},
		DefaultValue: "False",
	}
	var captured *model.Constant
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListConstantsFunc: func() ([]*model.Constant, error) { return []*model.Constant{existing}, nil },
		UpdateConstantFunc: func(c *model.Constant) error {
			captured = c
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	prog := parseMDL(t, "create or modify constant MyModule.Flag (Type: Boolean, DefaultValue: true);")
	_ = createConstant(ctx, prog.Statements[0].(*ast.CreateConstantStmt)) // folder handling may warn; the write is under test
	if captured == nil {
		t.Fatal("UpdateConstant was not called")
	}
	if captured.DefaultValue != "True" {
		t.Errorf("DefaultValue: true stored as %q, want %q", captured.DefaultValue, "True")
	}
}
