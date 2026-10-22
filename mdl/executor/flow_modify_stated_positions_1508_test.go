// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// mendixlabs/mxcli#1598 (ako/mxcli#1508), case 6, on a real project write.
//
// The flow is drawn with a @position on every statement. Restated with a new
// if/else after the declare and no @position on it, the splice made room by
// moving everything after the declare along by 339px — while the branch's two
// calls, replaced at the positions the script states, stayed where they were,
// left of their own decision:
//
//	Modified nanoflow: MyFirstModule.NF_Do (spliced: 1 inserted, 1 replaced, 1 dropped)
//	@position(40, 200)     if $Flag        was -299
//	@merge(525, 200)       its merge       was 186
//	@position(-124, 100)   call javascript action …
//	@position(869, 200)    return          was 530
//
// and re-running the same script was refused ("the connectors of the If at
// (40, 200) are redrawn"). The same script with a plain `if` and no else put
// the new if on top of the stored decision. Either way the stored flow said
// something other than the script, so under mdl 1 the splice now refuses to
// move a node the script states, and the script that leaves those positions
// unstated is the control: it is applied, and its re-run is Unchanged.

const nf1508Setup = `mdl 1;
create javascript action MyFirstModule.JS_Do(Msg: String) returns Boolean platform web as $$
return Promise.resolve(true);
$$;
create microflow MyFirstModule.SUB_Check() returns Boolean as $R
begin
  declare $R Boolean = true;
  return $R;
end;
create nanoflow MyFirstModule.NF_Do ($Flag: Boolean) returns Boolean as $Out
begin
  @position(-500, 200)
  declare $Out Boolean = false;
  @position(-299, 200)
  @merge(186, 200)
  if $Flag then
    @position(-124, 100)
    call javascript action MyFirstModule.JS_Do(Msg = 'a');
    @position(60, 100)
    call microflow MyFirstModule.SUB_Check();
  end if;
  @position(530, 200)
  return $Out;
end;
`

// nf1508Modify is the reported restatement: every @position kept, an if/else
// added after the declare without one, the JS argument changed and the call's
// result bound.
const nf1508Modify = `mdl 1;
create or modify nanoflow MyFirstModule.NF_Do ($Flag: Boolean) returns Boolean as $Out
begin
  @position(-500, 200)
  declare $Out Boolean = false;
  if $Out then
    log info node 'Probe' 'x';
  else
    log info node 'Probe' 'y';
  end if;
  @position(-299, 200)
  @merge(186, 200)
  if $Flag then
    @position(-124, 100)
    call javascript action MyFirstModule.JS_Do(Msg = 'b');
    @position(60, 100)
    $Ok = call microflow MyFirstModule.SUB_Check();
  end if;
  @position(530, 200)
  return $Out;
end;
`

func TestCreateOrModify_InsertDoesNotMoveStatedNodes1508(t *testing.T) {
	// Each case starts from the flow as the setup draws it.
	setup := func(t *testing.T) (*Executor, *bytes.Buffer, func() string) {
		t.Helper()
		exec, out := openPedAppFixture(t)
		if err := agreeExec(t, exec, nf1508Setup); err != nil {
			t.Fatalf("setup: %v\n%s", err, out.String())
		}
		describe := func() string {
			out.Reset()
			if err := afRun(t, exec, "describe nanoflow MyFirstModule.NF_Do;"); err != nil {
				return err.Error()
			}
			return out.String()
		}
		return exec, out, describe
	}
	refused := func(t *testing.T, src string) {
		exec, out, describe := setup(t)
		before := nf1508Unit(t, exec)
		out.Reset()
		err := agreeExec(t, exec, src)
		said := out.String()
		if err != nil {
			said += err.Error()
		}
		if err == nil || !strings.Contains(said, "which the script draws where it is") ||
			!strings.Contains(said, "the ExclusiveSplit '$Flag' at (-299, 200)") {
			t.Errorf("want a refusal naming the decision the script draws at (-299, 200), got %v\n%s", err, said)
		}
		if !bytes.Equal(nf1508Unit(t, exec), before) {
			t.Errorf("the refused statement wrote the nanoflow:\n%s", describe())
		}
	}
	t.Run("the reported if/else", func(t *testing.T) { refused(t, nf1508Modify) })
	// A refusal is new meaning, so it applies under mdl 1 only (ADR-0011).
	// Under mdl 0 the refused splice would fall back to rebuilding the whole
	// flow — losing the merges, curves and $IDs the shift keeps — so a
	// headerless script is spliced as it was.
	t.Run("mdl 0 splices as before", func(t *testing.T) {
		exec, out, _ := setup(t)
		out.Reset()
		if err := agreeExec(t, exec, strings.TrimPrefix(nf1508Modify, "mdl 1;\n")); err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		if said := out.String(); strings.Contains(said, "MDL-V1-REBUILD") ||
			!strings.Contains(said, "Modified nanoflow: MyFirstModule.NF_Do (spliced:") {
			t.Errorf("want the headerless script spliced, not rebuilt, got:\n%s", said)
		}
	})
	// With no else the new if took over the stored decision (the same if but
	// for its condition) and the restated `if $Flag` was inserted on top of it.
	t.Run("a plain if", func(t *testing.T) {
		refused(t, strings.Replace(nf1508Modify, "  else\n    log info node 'Probe' 'y';\n", "", 1))
	})

	// The way out the refusal names: leave the positions after the insert
	// unstated, so they can move along. Everything moves by the same amount —
	// the replaced calls with their decision — and re-running reports Unchanged.
	t.Run("control: no position stated after the insert", func(t *testing.T) {
		exec, out, describe := setup(t)
		free := nf1508Modify
		for _, line := range []string{"  @position(-299, 200)\n", "  @merge(186, 200)\n", "    @position(-124, 100)\n",
			"    @position(60, 100)\n", "  @position(530, 200)\n"} {
			free = strings.Replace(free, line, "", 1)
		}
		out.Reset()
		if err := agreeExec(t, exec, free); err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		if !strings.Contains(out.String(), "Modified nanoflow: MyFirstModule.NF_Do") {
			t.Errorf("want the nanoflow modified, got:\n%s", out.String())
		}
		v := parseFlowView(t, nf1508Unit(t, exec))
		if v.objectAt(t, -500, 200, "Microflows$ActionActivity") == "" {
			t.Errorf("the declare the script draws at (-500, 200) moved:\n%s", describe())
		}
		// The parameter is not on the line the room was made on: it stays on
		// its derived row (ako/mxcli#1547), where it moved to (539, 53).
		for _, id := range v.order {
			if o := v.objs[id]; afGet(o, "$Type") == "Microflows$MicroflowParameter" {
				if x, y := afPoint(o); x != 200 || y != 53 {
					t.Errorf("$Flag moved to (%d, %d), want it left at (200, 53):\n%s", x, y, describe())
				}
			}
		}
		split, merge, branch := nf1508Branch(t, v, "$Flag")
		if len(branch) != 2 {
			t.Fatalf("`if $Flag` has %d activities in its branch, want 2:\n%s", len(branch), describe())
		}
		for _, x := range branch {
			if x <= split || x >= merge {
				t.Errorf("a branch activity of `if $Flag` is at x=%d, outside its decision (%d) and merge (%d):\n%s",
					x, split, merge, describe())
			}
		}
		out.Reset()
		if err := agreeExec(t, exec, free); err != nil {
			t.Fatalf("re-run: %v\n%s", err, out.String())
		}
		if !strings.Contains(out.String(), "Unchanged nanoflow: MyFirstModule.NF_Do") {
			t.Errorf("re-run: want Unchanged, got:\n%s", out.String())
		}
	})
}

// nf1508Unit returns MyFirstModule.NF_Do's stored bytes.
func nf1508Unit(t *testing.T, exec *Executor) []byte {
	t.Helper()
	ctx := exec.newExecContext(t.Context())
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	all, err := ctx.Backend.ListNanoflows()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range all {
		if n.Name == "NF_Do" && h.GetModuleName(h.FindModuleID(n.ContainerID)) == "MyFirstModule" {
			raw, err := ctx.Backend.GetRawUnitBytes(n.ID)
			if err != nil {
				t.Fatal(err)
			}
			return append([]byte(nil), raw...)
		}
	}
	t.Fatal("MyFirstModule.NF_Do not found")
	return nil
}

// nf1508Branch returns the x of the decision on cond, of the merge its paths
// join at, and of every activity on those paths.
func nf1508Branch(t *testing.T, v flowView, cond string) (split, merge int, branch []int) {
	t.Helper()
	var splitID string
	for _, id := range v.order {
		o := v.objs[id]
		if afGet(o, "$Type") != "Microflows$ExclusiveSplit" {
			continue
		}
		if sc, _ := afGet(o, "SplitCondition").(bson.D); sc != nil && afGet(sc, "Expression") == cond {
			splitID = id
			split, _ = afPoint(o)
		}
	}
	if splitID == "" {
		t.Fatalf("no decision on %s", cond)
	}
	seen := map[string]bool{splitID: true}
	for frontier := []string{splitID}; len(frontier) > 0; frontier = frontier[1:] {
		for _, f := range v.flows {
			dest := afIDOf(afGet(f, "DestinationPointer"))
			if afIDOf(afGet(f, "OriginPointer")) != frontier[0] || seen[dest] {
				continue
			}
			seen[dest] = true
			o := v.objs[dest]
			x, _ := afPoint(o)
			switch afGet(o, "$Type") {
			case "Microflows$ExclusiveMerge":
				merge = x
			case "Microflows$ActionActivity":
				branch = append(branch, x)
				frontier = append(frontier, dest)
			}
		}
	}
	if merge == 0 {
		t.Fatalf("the decision on %s reaches no merge", cond)
	}
	return split, merge, branch
}
