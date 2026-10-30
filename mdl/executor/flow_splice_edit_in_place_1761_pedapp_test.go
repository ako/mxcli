// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ako/mxcli#1761 (mendixlabs/mxcli#1667, reported on Mendix 10.24.28): re-run
// `create or modify microflow` with only a filter's expression changed, and
// mxcli did not edit the activity, it replaced it:
//
//	$BigList: the Microflows$ActionActivity, Microflows$ListOperationsAction
//	and Microflows$FilterByExpression all got new $IDs. [...] The decision's
//	ExclusiveSplit and SplitCondition kept theirs.
//
// and exec reported both as `spliced: 1 replaced`. Studio Pro compares and
// merges by $ID, so it showed the filter as one activity removed and another
// added. An activity replaced by one of the same kind is now an edit of the
// stored one, which keeps its $ID and those of its action and the elements
// under it; exec says which it was.

const edit1761Setup = `mdl 1;
create persistent entity MyFirstModule.Sale1761 ( Amount: Integer, Note: String(200) );
create persistent entity MyFirstModule.Customer1761 ( Name: String(200) );
create association MyFirstModule.Sale1761_Customer1761 from MyFirstModule.Sale1761 to MyFirstModule.Customer1761;
create microflow MyFirstModule.SUB1761 ( $Sale: MyFirstModule.Sale1761, $Factor: Integer ) returns Boolean as $Ok
begin
  return true;
end;
create nanoflow MyFirstModule.NSUB1761 ( $Sale: MyFirstModule.Sale1761, $Factor: Integer ) returns Boolean as $Ok
begin
  return true;
end;`

// edit1761Flow is the flow under test, a microflow or a nanoflow (kind), which
// calls callee with call ("call microflow", "call nanoflow").
func edit1761Flow(kind, call, callee string) string {
	return fmt.Sprintf(`mdl 1;
create or modify %s MyFirstModule.Edit1761 ( $SaleList: List of MyFirstModule.Sale1761, $Cust: MyFirstModule.Customer1761 )
returns Boolean as $Ok
begin
  $Small = filter $SaleList where $currentObject/Amount < 5;
  $Found = find $SaleList where $currentObject/Amount = 3;
  retrieve $Big from MyFirstModule.Sale1761 where Amount > 100;
  change $Found (Amount = 7, Note = 'x', MyFirstModule.Sale1761_Customer1761 = $Cust);
  $New = create MyFirstModule.Sale1761 (Amount = 1, Note = 'n');
  $Ok = %s MyFirstModule.%s (Sale = $New, Factor = 2);
  return $Ok;
end;`, kind, call, callee)
}

// storedFlowUnit returns the stored bytes of the microflow or nanoflow
// module.name.
func storedFlowUnit(t *testing.T, exec *Executor, module, name string) []byte {
	t.Helper()
	ctx := exec.newExecContext(t.Context())
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mfs, err := ctx.Backend.ListMicroflows()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range mfs {
		if m.Name == name && h.GetModuleName(h.FindModuleID(m.ContainerID)) == module {
			raw, err := ctx.Backend.GetRawUnitBytes(m.ID)
			if err != nil {
				t.Fatal(err)
			}
			return append([]byte(nil), raw...)
		}
	}
	nfs, err := ctx.Backend.ListNanoflows()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range nfs {
		if n.Name == name && h.GetModuleName(h.FindModuleID(n.ContainerID)) == module {
			raw, err := ctx.Backend.GetRawUnitBytes(n.ID)
			if err != nil {
				t.Fatal(err)
			}
			return append([]byte(nil), raw...)
		}
	}
	t.Fatalf("%s.%s not found", module, name)
	return nil
}

// storedActivity is one activity of a stored flow: its $ID, the $ID of every
// element under it by path, and how many sequence flows enter and leave it.
type storedActivity struct {
	id       string
	ids      map[string]string
	in, out  int
	contents []byte
}

// findActivity returns the top-level activity whose action is an actType that
// outputs v.
func findActivity(t *testing.T, raw []byte, actType, v string) storedActivity {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	oc, _ := afGet(d, "ObjectCollection").(bson.D)
	var found bson.D
	for _, el := range afList(afGet(oc, "Objects")) {
		o, _ := el.(bson.D)
		a, _ := afGet(o, "Action").(bson.D)
		if a == nil || afGet(a, "$Type") != actType {
			continue
		}
		for _, k := range []string{"ResultVariableName", "VariableName", "ChangeVariableName", "OutputVariableName"} {
			if s, _ := afGet(a, k).(string); s == v {
				found = o
			}
		}
	}
	if found == nil {
		t.Fatalf("no %s outputting $%s in the stored flow", actType, v)
	}
	sa := storedActivity{id: afIDOf(afGet(found, "$ID")), ids: map[string]string{}}
	sa.contents, _ = bson.Marshal(found)
	var walk func(v any, path string)
	walk = func(v any, path string) {
		switch n := v.(type) {
		case bson.D:
			if id, ok := afGet(n, "$ID").(bson.Binary); ok {
				sa.ids[path] = types.BlobToUUID(id.Data)
			}
			for _, e := range n {
				walk(e.Value, path+"/"+e.Key)
			}
		case bson.A:
			for i, el := range n {
				name := fmt.Sprint(i)
				if ed, ok := el.(bson.D); ok {
					for _, k := range []string{"Attribute", "Association", "Parameter"} {
						if s, _ := afGet(ed, k).(string); s != "" {
							name = s
						}
					}
				}
				walk(el, path+"["+name+"]")
			}
		}
	}
	walk(found, "")
	for _, el := range afList(afGet(d, "Flows")) {
		f, _ := el.(bson.D)
		if afGet(f, "$Type") != "Microflows$SequenceFlow" {
			continue
		}
		if afIDOf(afGet(f, "OriginPointer")) == sa.id {
			sa.out++
		}
		if afIDOf(afGet(f, "DestinationPointer")) == sa.id {
			sa.in++
		}
	}
	return sa
}

// assertSameIdentity fails unless after is before with every element $ID
// kept and its flows still running through it.
func assertSameIdentity(t *testing.T, before, after storedActivity) {
	t.Helper()
	if after.id != before.id {
		t.Errorf("the activity got a new $ID: %s -> %s", before.id, after.id)
	}
	for path, id := range before.ids {
		if got := after.ids[path]; got != id {
			t.Errorf("%s: $ID %s -> %s", path, id, got)
		}
	}
	for path := range after.ids {
		if _, ok := before.ids[path]; !ok {
			t.Errorf("%s: an element that was not there", path)
		}
	}
	if after.in != before.in || after.out != before.out {
		t.Errorf("flows through the activity: %d in, %d out, want %d in, %d out", after.in, after.out, before.in, before.out)
	}
}

func TestCreateOrModify_SameKindActivityIsEditedInPlace(t *testing.T) {
	for _, kind := range []struct{ flow, call, callee string }{
		{"microflow", "call microflow", "SUB1761"},
		{"nanoflow", "call nanoflow", "NSUB1761"},
	} {
		base := edit1761Flow(kind.flow, kind.call, kind.callee)
		for _, tc := range []struct {
			name, from, to string
			actType, v     string
			want           string // in the stored activity after the edit
		}{
			{"filter expression", "$currentObject/Amount < 5", "$currentObject/Amount <= 5",
				"Microflows$ListOperationsAction", "Small", "$currentObject/Amount <= 5"},
			{"find expression", "$currentObject/Amount = 3", "$currentObject/Amount = 4",
				"Microflows$ListOperationsAction", "Found", "$currentObject/Amount = 4"},
			{"retrieve where", "where Amount > 100", "where Amount > 200",
				"Microflows$RetrieveAction", "Big", "[Amount > 200]"},
			{"change member value", "Amount = 7", "Amount = 8",
				"Microflows$ChangeAction", "Found", "8"},
			{"create member value", "Amount = 1", "Amount = 2",
				"Microflows$CreateChangeAction", "New", "2"},
			{"call argument", "Factor = 2", "Factor = 3",
				"", "Ok", "3"},
		} {
			t.Run(kind.flow+"/"+tc.name, func(t *testing.T) {
				actType := tc.actType
				if actType == "" {
					actType = "Microflows$MicroflowCallAction"
					if kind.flow == "nanoflow" {
						actType = "Microflows$NanoflowCallAction"
					}
				}
				exec, out, _ := openPedAppCopy(t)
				if err := agreeExec(t, exec, edit1761Setup); err != nil {
					t.Fatalf("setup: %v\n%s", err, out.String())
				}
				if err := agreeExec(t, exec, base); err != nil {
					t.Fatalf("create: %v\n%s", err, out.String())
				}
				before := findActivity(t, storedFlowUnit(t, exec, "MyFirstModule", "Edit1761"), actType, tc.v)

				edited := strings.Replace(base, tc.from, tc.to, 1)
				if edited == base {
					t.Fatalf("%q is not in the script", tc.from)
				}
				out.Reset()
				if err := agreeExec(t, exec, edited); err != nil {
					t.Fatalf("the edit is refused: %v\n%s", err, out.String())
				}
				if want := "(spliced: 1 edited in place)"; !strings.Contains(out.String(), want) {
					t.Errorf("want %q, got:\n%s", want, out.String())
				}
				after := findActivity(t, storedFlowUnit(t, exec, "MyFirstModule", "Edit1761"), actType, tc.v)
				assertSameIdentity(t, before, after)
				if !bytes.Contains(after.contents, []byte(tc.want)) {
					t.Errorf("the new value %q was not written", tc.want)
				}
				// The edit is the whole of the change: running it again
				// writes nothing.
				out.Reset()
				if err := agreeExec(t, exec, edited); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(out.String(), "Unchanged") {
					t.Errorf("re-running the edited script wrote again:\n%s", out.String())
				}
			})
		}
	}
}

// Control: an activity replaced by another kind of activity is new — new
// $IDs, and the report says so.
func TestCreateOrModify_AnotherKindOfActivityIsReplacedWithNewIDs(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, edit1761Setup); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	base := edit1761Flow("microflow", "call microflow", "SUB1761")
	if err := agreeExec(t, exec, base); err != nil {
		t.Fatalf("create: %v\n%s", err, out.String())
	}
	before := findActivity(t, storedFlowUnit(t, exec, "MyFirstModule", "Edit1761"), "Microflows$ListOperationsAction", "Small")

	// A find in place of the filter: the same action, another operation —
	// the activity and its action are the stored ones, the operation is new.
	out.Reset()
	if err := agreeExec(t, exec, strings.Replace(base, "$Small = filter", "$Small = find", 1)); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	after := findActivity(t, storedFlowUnit(t, exec, "MyFirstModule", "Edit1761"), "Microflows$ListOperationsAction", "Small")
	if after.id != before.id || after.ids["/Action"] != before.ids["/Action"] {
		t.Errorf("the activity or its action got a new $ID")
	}
	if after.ids["/Action/NewOperation"] == before.ids["/Action/NewOperation"] {
		t.Errorf("the find kept the filter's $ID")
	}

	// A log in place of the filter is another activity.
	out.Reset()
	if err := agreeExec(t, exec, strings.Replace(base, "$Small = filter $SaleList where $currentObject/Amount < 5;",
		"log info node 'T' 'no filter';", 1)); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if want := "(spliced: 1 replaced with new element IDs)"; !strings.Contains(out.String(), want) {
		t.Errorf("want %q, got:\n%s", want, out.String())
	}
	if raw := storedFlowUnit(t, exec, "MyFirstModule", "Edit1761"); bytes.Contains(raw, types.UUIDToBlob(before.id)) {
		t.Error("the filter's $ID is still in the flow")
	}
}

// The subject that can fail: a flow Studio Pro drew (PedApp's
// FeedbackModule.PopulateUserAttributes), whose retrieve and change share one
// note. Its describe output, with the retrieve's where and one change member
// edited, keeps both activities, their actions and every element under them,
// and the note stays on both.
func TestCreateOrModify_StudioProActivitiesAreEditedInPlace(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	var buf bytes.Buffer
	saved := exec.output
	exec.output = &buf
	if err := afRun(t, exec, "describe microflow FeedbackModule.PopulateUserAttributes;"); err != nil {
		t.Fatal(err)
	}
	exec.output = saved
	desc := buf.String()
	desc = desc[:strings.Index(desc, "\nend;")+len("\nend;")]

	raw := storedFlowUnit(t, exec, "FeedbackModule", "PopulateUserAttributes")
	retrieve := findActivity(t, raw, "Microflows$RetrieveAction", "CurrentUser")
	change := findActivity(t, raw, "Microflows$ChangeAction", "Feedback")
	notes := func(raw []byte) map[string]int {
		var d bson.D
		_ = bson.Unmarshal(raw, &d)
		n := map[string]int{}
		for _, el := range afList(afGet(d, "Flows")) {
			f, _ := el.(bson.D)
			if afGet(f, "$Type") == "Microflows$AnnotationFlow" {
				n[afIDOf(afGet(f, "OriginPointer"))+">"+afIDOf(afGet(f, "DestinationPointer"))]++
			}
		}
		return n
	}
	notesBefore := notes(raw)

	edited := strings.Replace(desc, "where id = $currentUser", "where Name = $currentUser/Name", 1)
	edited = strings.Replace(edited, "SubmitterDisplayName = $CurrentUser/Name", "SubmitterDisplayName = trim($CurrentUser/Name)", 1)
	if edited == desc || strings.Count(edited, "trim(") != 1 || !strings.Contains(edited, "where Name = $currentUser/Name") {
		t.Fatalf("the describe output does not hold the statements under test:\n%s", desc)
	}
	out.Reset()
	if err := agreeExec(t, exec, edited); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if want := "(spliced: 2 edited in place)"; !strings.Contains(out.String(), want) {
		t.Errorf("want %q, got:\n%s", want, out.String())
	}
	raw = storedFlowUnit(t, exec, "FeedbackModule", "PopulateUserAttributes")
	assertSameIdentity(t, retrieve, findActivity(t, raw, "Microflows$RetrieveAction", "CurrentUser"))
	assertSameIdentity(t, change, findActivity(t, raw, "Microflows$ChangeAction", "Feedback"))
	if got := notes(raw); fmt.Sprint(got) != fmt.Sprint(notesBefore) {
		t.Errorf("the note's lines changed: %v -> %v", notesBefore, got)
	}
	if !bytes.Contains(raw, []byte("trim($CurrentUser/Name)")) || !bytes.Contains(raw, []byte("[Name = $currentUser/Name]")) {
		t.Error("the edits were not written")
	}
}
