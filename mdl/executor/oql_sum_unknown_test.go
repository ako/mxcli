// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// unresolvableCtx is the real shape of the bug: a project the checker can read,
// in which the view's SOURCE entity does not exist — because the script creates
// it in the same run, and `check --references` skips script-created objects.
func unresolvableCtx() *ExecContext {
	return &ExecContext{Backend: &mock.MockBackend{
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) {
			return nil, errors.New("no domain models")
		},
	}}
}

// SUM over an argument whose type could not be resolved must be Unknown, not
// Decimal — the same rule inferTypeStatic already states in its own comment and
// the project-aware path contradicted.
//
// # The failure this caused
//
// A view entity whose source entity is created BY THE SAME SCRIPT cannot have
// its attribute types resolved (check skips references to script-created
// objects), so `sum(s.Units)` resolved to Unknown and fell through to Decimal.
// mxcli then reported
//
//	attribute 'Units': declared as Integer but OQL expression 'sum(s.Units)'
//	returns Decimal. Fix: change to 'Units: Decimal'
//
// Measured against mxbuild 11.6.6, that hint INVERTS the truth. Two views over
// the same `sum(s.Units)` where Units is an Integer attribute:
//
//	declared Integer  -> 0 errors        (what mxcli flagged)
//	declared Decimal  -> CE6770          (what mxcli told the user to write)
//
// The control that the check is not simply inert: a view declaring a String
// column over `sum(s.Amount)` DOES fail CE6770 on the same mxbuild, so mxbuild
// really does validate these and the 0-errors result above means something.
//
// So the diagnostic did not merely cry wolf, it walked a working project into a
// broken one. An unresolvable type has to stay unresolved.
func TestInferAggregateType_SumOfUnknownIsUnknown(t *testing.T) {
	ctx := unresolvableCtx()
	aliasMap := map[string]string{"s": "ChartExamples.Sales"}

	got := inferAggregateType(ctx, "sum(s.Units)", &OQLColumnInfo{}, aliasMap)
	if got.Kind != ast.TypeUnknown {
		t.Errorf("sum() over an unresolvable argument inferred %s, want Unknown — "+
			"guessing Decimal makes the checker demand a declaration mxbuild rejects (CE6770)",
			formatDataTypeForError(got))
	}
}

// The control for the fix: SUM must still PROPAGATE a type it can resolve, or
// "return Unknown" degenerates into "never check sum() at all" and the rule
// stops detecting the real CE6770 it was written for.
func TestInferAggregateType_SumPropagatesAKnownType(t *testing.T) {
	ctx := unresolvableCtx()
	// A literal resolves without a project, so this exercises the propagation
	// branch rather than the entity lookup.
	if got := inferAggregateType(ctx, "sum(1.5)", &OQLColumnInfo{}, nil); got.Kind != ast.TypeDecimal {
		t.Errorf("sum(1.5) inferred %s, want Decimal — a resolvable argument type must "+
			"still propagate", formatDataTypeForError(got))
	}
	if got := inferAggregateType(ctx, "sum(2)", &OQLColumnInfo{}, nil); got.Kind != ast.TypeInteger {
		t.Errorf("sum(2) inferred %s, want Integer", formatDataTypeForError(got))
	}
}

// The second control: the neighbouring aggregates keep their own rules. COUNT is
// Integer whatever its argument, AVG is Decimal whatever its argument — a fix
// that made every aggregate Unknown would pass the first test and silently turn
// the whole rule off.
func TestInferAggregateType_NeighbouringAggregatesUnchanged(t *testing.T) {
	ctx := unresolvableCtx()
	aliasMap := map[string]string{"s": "ChartExamples.Sales"}

	if got := inferAggregateType(ctx, "count(s.ID)", &OQLColumnInfo{}, aliasMap); got.Kind != ast.TypeInteger {
		t.Errorf("count() inferred %s, want Integer", formatDataTypeForError(got))
	}
	if got := inferAggregateType(ctx, "avg(s.Units)", &OQLColumnInfo{}, aliasMap); got.Kind != ast.TypeDecimal {
		t.Errorf("avg() inferred %s, want Decimal", formatDataTypeForError(got))
	}
}
