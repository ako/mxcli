// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1769 (mendixlabs/mxcli#1673): a fragment spliced into a stored
// flow refuses a member it cannot qualify (qualifiedMembersOnly, #885) — but
// only on its own main line. Every body nested in it (a loop's, a while
// loop's, an error handler's) was built by a fresh flowBuilder that did not
// inherit the flag, so `find $L by Code = …` in a replaced loop over a list
// whose entity the fragment does not know was written as a find by the
// expression `Code = …`: check and exec silent, CE0117 from mx check.

// nestedShapes wraps one statement in each body a fragment can nest it in.
// %s is the statement; $Cs is a typed list to iterate, $C a typed object.
var nestedShapes = map[string]string{
	"loop body": `loop $It in $Cs
  begin
    %s
  end loop;`,
	"while body": `while $C != empty
  begin
    %s
  end while;`,
	"error handler": `$H = call microflow Mod.GetItem() on error without rollback begin
    %s
  end error;`,
	"loop in a loop": `loop $It in $Cs
  begin
    loop $Jt in $Cs
    begin
      %s
    end loop;
  end loop;`,
	"error handler in a loop": `loop $It in $Cs
  begin
    $H = call microflow Mod.GetItem() on error without rollback begin
      %s
    end error;
  end loop;`,
	"while in an error handler": `$H = call microflow Mod.GetItem() on error without rollback begin
    while $C != empty
    begin
      %s
    end while;
  end error;`,
	"if in a loop": `loop $It in $Cs
  begin
    if $It != empty then
      %s
    end if;
  end loop;`,
}

// memberStatements are the statements that name a member of their
// variable's entity. %[1]s is a list variable, %[2]s an object variable.
var memberStatements = map[string]string{
	"find":      `$F = find %[1]s by Name = 'x';`,
	"filter":    `$G = filter %[1]s by Name = 'x';`,
	"sort":      `$S = sort %[1]s by Name asc;`,
	"aggregate": `$M = maximum %[1]s by Name;`,
	"change":    `change %[2]s (Name = 'x');`,
}

func nestedFragmentBody(t *testing.T, shape, stmt string) []ast.MicroflowStatement {
	t.Helper()
	src := "mdl 1;\ncreate microflow Mod.Fragment ()\nbegin\n  " +
		strings.ReplaceAll(nestedShapes[shape], "%s", stmt) + "\nend;"
	return parsedFlowBody(t, src)
}

// TestSpliceFragment_NestedBodiesRefuseUnqualifiedMembers: every member a
// nested body names over a variable the fragment cannot type is refused, as
// it is on the fragment's main line. Control: over the typed $Cs / $C the
// same fragments build, and the find is a find by attribute.
func TestSpliceFragment_NestedBodiesRefuseUnqualifiedMembers(t *testing.T) {
	ctx, a := spliceFixture(t)
	for shape := range nestedShapes {
		for name, stmt := range memberStatements {
			untyped := strings.ReplaceAll(strings.ReplaceAll(stmt, "%[1]s", "$A"), "%[2]s", "$A")
			_, err := a.buildFragment(ctx, nestedFragmentBody(t, shape, untyped))
			if err == nil || !strings.Contains(err.Error(), `cannot qualify member "Name"`) {
				t.Errorf("%s in the %s over an untyped variable: want the refusal, got %v", name, shape, err)
			}

			typed := strings.ReplaceAll(strings.ReplaceAll(stmt, "%[1]s", "$Cs"), "%[2]s", "$C")
			frag, err := a.buildFragment(ctx, nestedFragmentBody(t, shape, typed))
			if err != nil {
				t.Errorf("control: %s in the %s over a typed variable: %v", name, shape, err)
				continue
			}
			if name != "find" {
				continue
			}
			ops := fragmentListOperations(frag.Objects)
			if len(ops) != 1 {
				t.Errorf("control: find in the %s: want one list operation, got %d", shape, len(ops))
				continue
			}
			if f, ok := ops[0].(*microflows.FindByAttributeOperation); !ok || f.Attribute != "Mod.Item.Name" {
				t.Errorf("control: find in the %s wrote %#v, want a find by attribute Mod.Item.Name", shape, ops[0])
			}
		}
	}
}

// fragmentListOperations returns the list operations among objs, loop bodies
// included.
func fragmentListOperations(objs []microflows.MicroflowObject) []microflows.ListOperation {
	var out []microflows.ListOperation
	for _, obj := range objs {
		switch o := obj.(type) {
		case *microflows.ActionActivity:
			if lo, ok := o.Action.(*microflows.ListOperationAction); ok {
				out = append(out, lo.Operation)
			}
		case *microflows.LoopedActivity:
			if o.ObjectCollection != nil {
				out = append(out, fragmentListOperations(o.ObjectCollection.Objects)...)
			}
		}
	}
	return out
}
