// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// An entity's EXTENDS target was never validated. Found alongside #972, where
// System.Thumbnail turned out not to exist in the modeler: `extends
// System.Thumbnail` passed `mxcli check --references` with "All references
// valid", and so did `extends SameModule.CompletelyMadeUp`.
//
// The two failures are not the same size, both measured on mxbuild 11.13.0:
//
//   - A QUALIFIED name that resolves to nothing is stored verbatim and reported
//     at build time — recoverable:
//
//     [CE1613] "The selected entity 'Gen.AlsoMadeUp' no longer exists."
//     at Entity 'Gen.BadLocal'
//
//   - An UNQUALIFIED name is stored as a bare word, and Mendix cannot construct
//     an entity identifier from it. The project does not load at all:
//
//     ERROR: System.AggregateException: … (An error occurred when trying to set
//     the 'Generalization' property of a Generalization in a Domain model …)
//
//     mx check dies before validating anything, which is the same shape as the
//     bare attribute name in #973.
//
// A forward reference is legitimate — the generalization is resolved lazily and
// `create entity A extends B; create entity B;` executes correctly (see
// eagerDefRefs, which deliberately does not treat it as an ordering error) — so
// the check resolves against the whole script, not the statements before it.

// generalizationFixture is a project with one module holding one entity, plus a
// System module holding the two entities real projects actually extend.
func generalizationFixture(t *testing.T) *ExecContext {
	t.Helper()
	proj := mkModule("Proj")
	sys := &model.Module{Name: "System"}
	sys.ID = model.ID("m-system")

	projDM := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm-proj"},
		ContainerID: proj.ID,
		Entities:    []*domainmodel.Entity{{Name: "Existing"}},
	}
	sysDM := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm-system"},
		ContainerID: sys.ID,
		Entities:    []*domainmodel.Entity{{Name: "Image"}, {Name: "FileDocument"}},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{proj, sys}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			switch name {
			case "Proj":
				return proj, nil
			case "System":
				return sys, nil
			}
			return nil, fmt.Errorf("module not found: %s", name)
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return []*domainmodel.DomainModel{projDM, sysDM}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(proj, sys)))
	return ctx
}

// entityExtending builds `create persistent entity Proj.Sub extends <gen>`.
func entityExtending(gen *ast.QualifiedName) *ast.CreateEntityStmt {
	return &ast.CreateEntityStmt{
		Name:           ast.QualifiedName{Module: "Proj", Name: "Sub"},
		Generalization: gen,
	}
}

func TestValidateEntityGeneralization_RefusesWhatDoesNotResolve(t *testing.T) {
	ctx := generalizationFixture(t)
	sc := &scriptContext{
		modules:      map[string]bool{},
		entities:     map[string]bool{},
		enumerations: map[string]bool{},
	}

	cases := []struct {
		name   string
		gen    ast.QualifiedName
		expect string
	}{
		{
			"System entity the modeler does not have",
			ast.QualifiedName{Module: "System", Name: "CompletelyMadeUpName"},
			"System.CompletelyMadeUpName",
		},
		{
			"entity in a real module that does not exist",
			ast.QualifiedName{Module: "Proj", Name: "AlsoMadeUp"},
			"Proj.AlsoMadeUp",
		},
		{
			"module that does not exist",
			ast.QualifiedName{Module: "NoSuchModule", Name: "Thing"},
			"NoSuchModule",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gen := tc.gen
			err := validateWithContext(ctx, entityExtending(&gen), sc)
			if err == nil {
				t.Fatalf("extends %s should be reported by --references; mxbuild rejects it with CE1613", gen.String())
			}
			if !strings.Contains(err.Error(), tc.expect) {
				t.Errorf("error should name %q so it can be found, got: %v", tc.expect, err)
			}
		})
	}
}

// The unqualified form is the severe one: it makes the .mpr unloadable, so the
// message has to say so rather than reading like a lookup miss.
func TestValidateEntityGeneralization_RefusesAnUnqualifiedName(t *testing.T) {
	ctx := generalizationFixture(t)
	sc := &scriptContext{
		modules:      map[string]bool{},
		entities:     map[string]bool{"Proj.Existing": true},
		enumerations: map[string]bool{},
	}

	// Bare "Existing" names an entity that really is in the project — so this
	// cannot be passing merely because the name is unknown.
	err := validateWithContext(ctx, entityExtending(&ast.QualifiedName{Name: "Existing"}), sc)
	if err == nil {
		t.Fatal("an unqualified generalization must be refused: it is stored as a bare word and Mendix cannot load the project")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Existing") {
		t.Errorf("error should quote the name given, got: %v", err)
	}
	if !strings.Contains(strings.ToLower(msg), "qualif") {
		t.Errorf("error should say the name must be qualified, got: %v", err)
	}
}

// The controls. Each of these must keep passing, or the check is not "validate
// the target" but "reject inheritance".
func TestValidateEntityGeneralization_AcceptsWhatResolves(t *testing.T) {
	ctx := generalizationFixture(t)

	cases := []struct {
		name string
		gen  *ast.QualifiedName
		sc   *scriptContext
	}{
		{
			"no generalization at all",
			nil,
			&scriptContext{modules: map[string]bool{}, entities: map[string]bool{}, enumerations: map[string]bool{}},
		},
		{
			"an entity already in the project",
			&ast.QualifiedName{Module: "Proj", Name: "Existing"},
			&scriptContext{modules: map[string]bool{}, entities: map[string]bool{}, enumerations: map[string]bool{}},
		},
		{
			// #972's own case: System entities must still resolve, or the fix for
			// that issue would have broken every image entity in a different way.
			"a System entity",
			&ast.QualifiedName{Module: "System", Name: "Image"},
			&scriptContext{modules: map[string]bool{}, entities: map[string]bool{}, enumerations: map[string]bool{}},
		},
		{
			// The generalization is resolved lazily, so a parent defined LATER in
			// the same script is correct. Resolving against statement order would
			// break it.
			"an entity created later in the same script",
			&ast.QualifiedName{Module: "Proj", Name: "Base"},
			&scriptContext{
				modules:      map[string]bool{},
				entities:     map[string]bool{"Proj.Base": true},
				enumerations: map[string]bool{},
			},
		},
		{
			"an entity in a module the script creates",
			&ast.QualifiedName{Module: "New", Name: "Base"},
			&scriptContext{
				modules:      map[string]bool{"New": true},
				entities:     map[string]bool{"New.Base": true},
				enumerations: map[string]bool{},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateWithContext(ctx, entityExtending(tc.gen), tc.sc); err != nil {
				t.Errorf("this must stay valid, got: %v", err)
			}
		})
	}
}

// The check is a pre-flight and `exec --no-check` skips it, so the case that
// produces an unopenable .mpr is refused by the executor too. The qualified-but-
// missing case deliberately is NOT: a parent created later in the same script is
// legitimate, and the executor sees one statement at a time.
func TestExecCreateEntity_RefusesAnUnqualifiedGeneralization(t *testing.T) {
	ctx := generalizationFixture(t)

	err := execCreateEntity(ctx, entityExtending(&ast.QualifiedName{Name: "Existing"}))
	if err == nil {
		t.Fatal("exec must refuse a bare generalization — writing it produces a project Mendix cannot load")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "qualif") {
		t.Errorf("error should say the name must be qualified, got: %v", err)
	}
}
