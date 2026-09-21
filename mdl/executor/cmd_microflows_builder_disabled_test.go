// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1139: "Studio Pro allows marking any activity as disabled
// (right-click → Disable) — the activity remains in the flow but is skipped at
// runtime. There is currently no MDL equivalent for this flag."
//
// The flag DID have a spelling — `@excluded` — which the visitor parsed, the
// builder's applyAnnotations knew how to apply, the writer knew how to store
// and DESCRIBE knew how to emit. It never reached the model, because the one
// hop between them (mergeStatementAnnotations, which fills pendingAnnotations
// — the ONLY thing applyAnnotations is ever called with) copied Position,
// Caption, Color, Notes and every anchor and dropped this one field. So the
// annotation parsed, checked clean, executed clean, and the activity came out
// enabled.
//
// Control for this test: revert the `if ann.Disabled` copy in
// mergeStatementAnnotations and both subtests fail with
// "activity ... Disabled = false, want true".
func TestDisabledAnnotationReachesTheActivity(t *testing.T) {
	for _, tc := range []struct {
		name string
		ann  string
	}{
		{"@disabled", "@disabled"},
		// The spelling DESCRIBE has always emitted. It has to keep working, or
		// describe → exec silently drops the flag on every existing project.
		{"@excluded", "@excluded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `create microflow M.Flow () returns Boolean
begin
  ` + tc.ann + `
  log info 'skipped';
  log info 'runs';
  return true;
end;`
			coll := buildMicroflowFromMDL(t, src)

			var acts []*microflows.ActionActivity
			for _, obj := range coll.Objects {
				if a, ok := obj.(*microflows.ActionActivity); ok {
					acts = append(acts, a)
				}
			}
			if len(acts) != 2 {
				t.Fatalf("got %d action activities, want 2 (the two LOG steps) — "+
					"the assertion below would not be testing what it claims", len(acts))
			}
			if !acts[0].Disabled {
				t.Errorf("activity %q: Disabled = false, want true — %s was dropped "+
					"between the AST and the model", acts[0].Caption, tc.ann)
			}
			// Control: the flag must attach to ONE activity, not the whole flow.
			if acts[1].Disabled {
				t.Errorf("activity %q: Disabled = true, want false — %s leaked onto "+
					"the following statement", acts[1].Caption, tc.ann)
			}
		})
	}
}

// MDL087's list of statements that cannot carry `@disabled` is hand-written.
// This pins it against what the flow builder actually does: for each listed
// statement, `@disabled` on it must leave EVERY action activity in the graph
// enabled — which is the condition that makes refusing the script right. If one
// of them ever starts producing a disabled ActionActivity, the refusal has
// become wrong and this fails instead of quietly rejecting a legal script.
//
// The control is TestDisabledAnnotationReachesTheActivity above: same builder,
// same annotation, on a LOG — one activity comes out disabled. Without it, a
// builder that had simply stopped reading the flag would pass this test.
func TestDisabledAnnotationTargetsAreNotActionActivities(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"if", "@disabled\n  if 1 = 1 then\n    log info 'a';\n  else\n    log info 'b';\n  end if;"},
		{"loop", "@disabled\n  loop $x in $L\n  begin\n    log info 'a';\n  end loop;"},
		{"while", "@disabled\n  while 1 = 1\n    break;\n  end while;"},
		{"return", "@disabled\n  return true;"},
		{"break", "loop $x in $L\n  begin\n    @disabled\n    break;\n  end loop;"},
		{"continue", "loop $x in $L\n  begin\n    @disabled\n    continue;\n  end loop;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The leading LOG is what keeps this from being vacuous: a `return`
			// or a `break` contributes no action activity of its own, so
			// without it "no disabled activity" would hold for an empty set.
			// It doubles as the leak check — the flag must not slide onto a
			// neighbouring statement either.
			src := "create microflow M.Flow ($L : list of M.E) returns Boolean\nbegin\n" +
				"  log info 'neighbour';\n  " + tc.body + "\n  return true;\nend;"
			coll := buildMicroflowFromMDL(t, src)

			saw := 0
			for _, obj := range coll.Objects {
				a, ok := obj.(*microflows.ActionActivity)
				if !ok {
					continue
				}
				saw++
				if a.Disabled {
					t.Errorf("a %s carrying @disabled produced a DISABLED action "+
						"activity — MDL087 refuses this statement and should not", tc.name)
				}
			}
			if saw == 0 {
				t.Fatal("no action activity in the graph; the assertion above is vacuous")
			}
		})
	}
}

// Every statement MDL087 names must be one the validator can still recognise
// after the builder has had it — the two halves are written apart, and a
// renamed AST type would leave the message naming a statement nothing matches.
func TestNonDisablableStatementCoversItsMessage(t *testing.T) {
	for _, s := range []ast.MicroflowStatement{
		&ast.IfStmt{}, &ast.EnumSplitStmt{}, &ast.InheritanceSplitStmt{},
		&ast.LoopStmt{}, &ast.WhileStmt{}, &ast.MergeStmt{}, &ast.JoinStmt{},
		&ast.ReturnStmt{}, &ast.RaiseErrorStmt{}, &ast.BreakStmt{}, &ast.ContinueStmt{},
	} {
		kind, becomes, ok := nonDisablableStatement(s)
		if !ok || kind == "" || becomes == "" {
			t.Errorf("%T is not listed by nonDisablableStatement", s)
		}
	}
	// Control: an action activity must NOT be listed, or MDL087 would refuse
	// the only statement the annotation exists for.
	if _, _, ok := nonDisablableStatement(&ast.LogStmt{}); ok {
		t.Error("LogStmt is listed by nonDisablableStatement; MDL087 would refuse a legal script")
	}
}
