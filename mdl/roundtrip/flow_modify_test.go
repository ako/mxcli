// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#747 (plan item 4.2g): `create or modify microflow` on an existing
// flow is diff-then-patch on top of the #739 splice. Every flow here is Studio
// Pro-authored (PedApp), because an mxcli-authored flow is laid out the way the
// rebuild lays it out and cannot show what the rebuild loses.

const (
	valFeedback  = "microflow FeedbackModule.VAL_Feedback"
	sendToServer = "microflow FeedbackModule.SUB_Feedback_SendToServer"
)

// An unchanged describe -> create or modify writes nothing: the unit's bytes
// are identical. The controls below prove the same pipeline does write when
// the definition changes.
func TestFlowModify_UnchangedIsByteIdentical(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	// VAL_Feedback stores a line break in a message, which mdl 1 has no
	// spelling for (its description's `\n` is a backslash and an n there; see
	// MDL1ReadsStoredStringsAsStored), so its mdl 1 leg uses a flow without
	// one: SUB_Feedback_SendToServer, with merges and an error handler.
	for _, c := range []struct{ header, target, name string }{
		{"", valFeedback, "VAL_Feedback"},
		{"", sendToServer, "SUB_Feedback_SendToServer"},
		{"mdl 1;\n", sendToServer, "SUB_Feedback_SendToServer"},
	} {
		described := h.mustDescribeMdl0(t, c.target)
		before := h.flowUnit(t, c.name)
		if err := h.exec(c.header + described); err != nil {
			t.Fatalf("%s, header %q: exec unchanged describe output: %v", c.name, c.header, err)
		}
		if got := h.flowUnit(t, c.name); !bytes.Equal(got, before) {
			t.Fatalf("%s, header %q: the unchanged definition rewrote the unit", c.name, c.header)
		}
		if changed := h.orig.diff(h.snapshot()); len(changed) != 0 {
			t.Fatalf("%s, header %q: the unchanged definition wrote: %s", c.name, c.header, strings.Join(changed, "; "))
		}
		if want := "Unchanged microflow: FeedbackModule." + c.name; !strings.Contains(h.out.String(), want) {
			t.Errorf("%s, header %q: want an Unchanged report, got:\n%s", c.name, c.header, h.out.String())
		}
	}
}

// Under mdl 1 a backslash in a string is an ordinary character, so the
// description's `'…characters\n'` (mdl 0 for a line break, which is what
// VAL_Feedback stores) states a backslash and an n. The stored side must be
// read as stored — not by re-parsing its mdl 0 description under the script's
// header — or the two misreadings agree, the statement reports Unchanged and
// the value the script states is silently not written.
func TestFlowModify_MDL1ReadsStoredStringsAsStored(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	described := h.mustDescribeMdl0(t, valFeedback)
	const mdl0 = "characters\\n';"
	if !strings.Contains(described, mdl0) {
		t.Fatalf("describe output has no %q — the fixture changed:\n%s", mdl0, described)
	}
	// Control: under mdl 0 the same text is the stored value.
	if err := h.exec(described); err != nil {
		t.Fatalf("mdl 0: %v", err)
	}
	if changed := h.orig.diff(h.snapshot()); len(changed) != 0 {
		t.Fatalf("mdl 0: the unchanged definition wrote: %s", strings.Join(changed, "; "))
	}

	before := h.flowUnit(t, "VAL_Feedback")
	if err := h.exec("mdl 1;\n" + described); err != nil {
		t.Fatalf("mdl 1: %v", err)
	}
	if bytes.Equal(h.flowUnit(t, "VAL_Feedback"), before) {
		t.Fatal("mdl 1: a string that states another value than the stored one wrote nothing")
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 edited in place)") {
		t.Errorf("mdl 1: want the one statement edited in place by a splice, got:\n%s", h.out.String())
	}
	if again := h.mustDescribeMdl0(t, valFeedback); !strings.Contains(again, "characters\\\\n';") {
		t.Errorf("mdl 1: want the backslash stored as written:\n%s", again)
	}
}

// Control: an inserted statement is written, as a splice. Every element the
// stored unit had keeps its $ID and stays in the unit — including the merges
// describe cannot show and the rebuild deleted (#721 A) — and only the new
// activity and one new flow are added.
func TestFlowModify_InsertIsSpliced(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	described := h.mustDescribeMdl0(t, valFeedback)
	const anchor = "  declare $ValidFeedback Boolean = true;\n"
	const inserted = "  log info node 'Feedback' 'validating feedback';\n"
	edited := strings.Replace(described, anchor, anchor+inserted, 1)
	if edited == described {
		t.Fatalf("describe output has no %q — the fixture changed:\n%s", anchor, described)
	}
	before := h.flowUnit(t, "VAL_Feedback")
	if err := h.exec(edited); err != nil {
		t.Fatalf("exec edited definition: %v", err)
	}
	if !strings.Contains(h.out.String(), "Modified microflow: FeedbackModule.VAL_Feedback (spliced: 1 inserted)") {
		t.Errorf("want a splice report, got:\n%s", h.out.String())
	}
	after := h.flowUnit(t, "VAL_Feedback")
	if bytes.Equal(after, before) {
		t.Fatal("an inserted statement wrote nothing")
	}
	requireKept(t, before, after, "")
	if added := len(elementIDs(t, after)) - len(elementIDs(t, before)); added <= 0 {
		t.Errorf("the insert added %d elements", added)
	}
	if got, want := countType(t, after, "Microflows$ExclusiveMerge"), countType(t, before, "Microflows$ExclusiveMerge"); got != want {
		t.Errorf("merges: %d after the insert, %d before — the rebuild ran", got, want)
	}
	again := h.mustDescribeMdl0(t, valFeedback)
	if !strings.Contains(again, "log node 'Feedback' 'validating feedback';") {
		t.Errorf("describe does not show the inserted statement:\n%s", again)
	}

	// Executing the new description writes nothing more: the splice's own
	// output is a fixed point too.
	settled := h.flowUnit(t, "VAL_Feedback")
	if err := h.exec(again); err != nil {
		t.Fatalf("exec the description after the insert: %v", err)
	}
	if !bytes.Equal(h.flowUnit(t, "VAL_Feedback"), settled) {
		t.Error("re-executing the description of the spliced flow rewrote it")
	}
}

// A replaced statement that carries a shared annotation keeps the one stored
// note: the splice re-attaches it, and the declared note is not drawn again.
func TestFlowModify_ReplaceKeepsSharedNote(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const target = "microflow FeedbackModule.PopulateUserAttributes"
	described := h.mustDescribeMdl0(t, target)
	const old = "SubmitterDisplayName = $CurrentUser/Name)"
	edited := strings.Replace(described, old, "SubmitterDisplayName = 'anonymous')", 1)
	if edited == described {
		t.Fatalf("describe output has no %q:\n%s", old, described)
	}
	before := h.flowUnit(t, "PopulateUserAttributes")
	if err := h.exec(edited); err != nil {
		t.Fatalf("exec edited definition: %v", err)
	}
	after := h.flowUnit(t, "PopulateUserAttributes")
	if bytes.Equal(after, before) {
		t.Fatal("a replaced statement wrote nothing")
	}
	if got := countType(t, after, "Microflows$Annotation"); got != 1 {
		t.Errorf("%d annotation notes after the replace, want the 1 stored", got)
	}
	again := h.mustDescribeMdl0(t, target)
	if !strings.Contains(again, "SubmitterDisplayName = 'anonymous'") || strings.Count(again, "@annotation(id: n1") != 2 {
		t.Errorf("want the new statement with the shared note on both activities:\n%s", again)
	}
	// Only the replaced activity (and what it contains) is gone.
	requireKept(t, before, after, "Microflows$ChangeAction") // ChangeObjectAction's storage name
}

// Dropping a statement whose output only a statement changed in the same
// definition read: the scope check sees the flow as the replace left it, so
// the drop is not refused for a use that is about to go.
func TestFlowModify_DropAndReplace(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const target = "microflow FeedbackModule.SUB_Feedback_Sanitize"
	described := h.mustDescribeMdl0(t, target)
	edited := described
	for _, cut := range []string{
		"  @position(368, 200)\n  @curve(from: (30, 0), to: (-30, 0))\n  $SanitizedPageName = call java action FeedbackModule.XSS_Sanitizer(stringToSanitize = $Feedback/PageName);\n",
		" PageName = $SanitizedPageName,",
	} {
		next := strings.Replace(edited, cut, "", 1)
		if next == edited {
			t.Fatalf("describe output has no %q:\n%s", cut, described)
		}
		edited = next
	}
	before := h.flowUnit(t, "SUB_Feedback_Sanitize")
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("exec edited definition: %v", err)
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 edited in place, 1 dropped)") {
		t.Errorf("want a splice report, got:\n%s", h.out.String())
	}
	after := h.flowUnit(t, "SUB_Feedback_Sanitize")
	if got, want := countType(t, after, "Microflows$JavaActionCallAction"), countType(t, before, "Microflows$JavaActionCallAction")-1; got != want {
		t.Errorf("%d java action calls after, want %d", got, want)
	}
	again := h.mustDescribeMdl0(t, target)
	if strings.Contains(again, "SanitizedPageName") {
		t.Errorf("the dropped statement or its use is still described:\n%s", again)
	}
}

// A statement changed inside an `if` branch is spliced too: the branch's
// activities are nodes of the stored graph like any other.
func TestFlowModify_BranchEditIsSpliced(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	described := h.mustDescribeMdl0(t, valFeedback)
	const old = "message 'Subject is required';"
	edited := strings.Replace(described, old, "message 'A subject is required';", 1)
	if edited == described {
		t.Fatalf("describe output has no %q:\n%s", old, described)
	}
	// mdl 0 only: under mdl 1 this flow's description also restates its
	// stored line break as a backslash and an n (MDL1ReadsStoredStringsAsStored
	// covers a branch splice under mdl 1).
	before := h.flowUnit(t, "VAL_Feedback")
	if err := h.exec(edited); err != nil {
		t.Fatalf("exec edited definition: %v", err)
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 edited in place)") {
		t.Errorf("want a splice report, got:\n%s", h.out.String())
	}
	after := h.flowUnit(t, "VAL_Feedback")
	if bytes.Equal(after, before) {
		t.Fatal("a statement changed in a branch wrote nothing")
	}
	// The one replaced activity may go; every other element stays.
	if got, want := countType(t, after, "Microflows$ValidationFeedbackAction"), countType(t, before, "Microflows$ValidationFeedbackAction"); got != want {
		t.Errorf("%d validation feedback actions after the replace, %d before", got, want)
	}
	requireKeptBut(t, before, after, "Microflows$ValidationFeedbackAction", "Subject is required")
	if got, want := countType(t, after, "Microflows$ExclusiveMerge"), countType(t, before, "Microflows$ExclusiveMerge"); got != want {
		t.Errorf("merges: %d after, %d before — the rebuild ran", got, want)
	}
	if !strings.Contains(h.mustDescribeMdl0(t, valFeedback), "'A subject is required'") {
		t.Error("describe does not show the change")
	}
}

// A change the splice cannot make — here a connector redrawn — is refused
// under mdl 1 with nothing written, and under mdl 0 still rebuilds, with the
// MDL-V1-REBUILD warning. (A moved node used to be the example; it is a patch
// now, ako/mxcli#818.)
func TestFlowModify_UnspliceableChange(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	// mdl 1: refused, nothing written. (On SUB_Feedback_SendToServer, whose
	// description means under mdl 1 what it means under mdl 0; see
	// UnchangedIsByteIdentical.)
	described := h.mustDescribeMdl0(t, sendToServer)
	const oldV1 = "@position(-730, -50)\n  @curve(from: (30, 0), to: (-15, 0))"
	edited := strings.Replace(described, oldV1, "@position(-730, -50)\n  @curve(from: (30, 0), to: (-15, 10))", 1)
	if edited == described {
		t.Fatalf("describe output has no %q:\n%s", oldV1, described)
	}
	err := h.exec("mdl 1;\n" + edited)
	if err == nil || !strings.Contains(err.Error(), "cannot be spliced") || !strings.Contains(err.Error(), "redrawn") {
		t.Fatalf("under mdl 1 want a refusal naming the redrawn connector, got %v", err)
	}
	if changed := h.orig.diff(h.snapshot()); len(changed) != 0 {
		t.Fatalf("the refused statement wrote: %s", strings.Join(changed, "; "))
	}

	// mdl 0: rebuilt, with the warning.
	described = h.mustDescribeMdl0(t, valFeedback)
	const old = "@position(-390, 200)\n  @curve(from: (30, 0), to: (-15, 0))"
	edited = strings.Replace(described, old, "@position(-390, 200)\n  @curve(from: (30, 0), to: (-15, 10))", 1)
	if edited == described {
		t.Fatalf("describe output has no %q:\n%s", old, described)
	}
	if err := h.exec(edited); err != nil {
		t.Fatalf("under mdl 0: %v", err)
	}
	if !strings.Contains(h.out.String(), "Warning [MDL-V1-REBUILD]") {
		t.Errorf("under mdl 0 want the MDL-V1-REBUILD warning, got:\n%s", h.out.String())
	}
	if !strings.Contains(h.mustDescribeMdl0(t, valFeedback), "to: (-15, 10)") {
		t.Error("under mdl 0 the rebuild did not write the redrawn connector")
	}
}

// A decision added inside a loop body is not a splice: the splice puts plain
// activities in a body (ako/mxcli#1693), and replacing the whole loop would
// renumber and redraw every node it holds — the rebuild's loss, confined to the
// loop but just as silent. So under mdl 1 it is refused (naming `replace body
// of` the loop), and under mdl 0 it takes the warned rebuild. A value edit of a
// body activity is spliced in place (ako/mxcli#1106), here beside an `if` in
// the body. Control: a change after the loop in the same flow is spliced.
func TestFlowModify_LoopBodyChangeIsNotSpliced(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	// PedApp has no loop, so this flow is mxcli-authored; the test is about
	// what is refused, not about identity.
	const create = `create microflow MyFirstModule.LoopFlow (
  $Items: List of FeedbackModule.Feedback
)
returns Integer
begin
  declare $N Integer = 0;
  loop $It in $Items begin
    if $It/Subject != empty then
      set $N = $N + 1;
    end if;
    log info node 'X' 'in loop';
  end loop;
  log info node 'X' 'after loop';
  return $N;
end;`
	if err := h.exec(create); err != nil {
		t.Fatalf("create: %v", err)
	}
	const target = "microflow MyFirstModule.LoopFlow"
	described := h.mustDescribeMdl0(t, target)
	edited := strings.Replace(described, "'in loop'", "'in the loop'", 1)
	if edited == described {
		t.Fatalf("describe output has no 'in loop':\n%s", described)
	}
	if err := h.exec("mdl 1;\n" + edited); err != nil {
		t.Fatalf("a value edit in the loop body under mdl 1: %v", err)
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 edited in place)") {
		t.Errorf("a value edit in the loop body: want a splice, got:\n%s", h.out.String())
	}
	described = h.mustDescribeMdl0(t, target)
	// A decision added in the body: a change of its shape the splice does not
	// make (a plain activity it does, ako/mxcli#1693).
	inLoop := strings.Replace(described, "log node 'X' 'in the loop';",
		"log node 'X' 'in the loop';\n    if $It/Subject = empty then\n      log node 'X' 'grown';\n    end if;", 1)
	if inLoop == described {
		t.Fatalf("describe output has no 'in the loop':\n%s", described)
	}
	before := h.flowUnit(t, "LoopFlow")

	err := h.exec("mdl 1;\n" + inLoop)
	if err == nil || !strings.Contains(err.Error(), "cannot be spliced") || !strings.Contains(err.Error(), "loop") {
		t.Fatalf("under mdl 1 want a refusal naming the loop, got %v", err)
	}
	if !bytes.Equal(h.flowUnit(t, "LoopFlow"), before) {
		t.Fatal("the refused statement wrote")
	}

	if err := h.exec(inLoop); err != nil {
		t.Fatalf("under mdl 0: %v", err)
	}
	if !strings.Contains(h.out.String(), "Warning [MDL-V1-REBUILD]") {
		t.Errorf("under mdl 0 want the MDL-V1-REBUILD warning, got:\n%s", h.out.String())
	}
	if !strings.Contains(h.mustDescribeMdl0(t, target), "'grown'") {
		t.Error("under mdl 0 the rebuild did not write the change")
	}

	// Control: after the loop, the same kind of change is spliced.
	described = h.mustDescribeMdl0(t, target)
	after := strings.Replace(described, "'after loop'", "'after the loop'", 1)
	if err := h.exec("mdl 1;\n" + after); err != nil {
		t.Fatalf("control under mdl 1: %v", err)
	}
	if !strings.Contains(h.out.String(), "(spliced: 1 edited in place)") {
		t.Errorf("control: want a splice, got:\n%s", h.out.String())
	}
}

// mustDescribeMdl0 is the mdl 0 description (`describe --mdl 0`) the focused
// flow tests below are written against: each runs it headerless (its mdl 0
// leg) and under an explicit `mdl 1;` (the upgrade a user makes by adding the
// header alone). The default-language round trip — mdl 1, headed by `mdl 1;`
// since the freeze — is runRoundTrip's.
func (h *harness) mustDescribeMdl0(t *testing.T, target string) string {
	t.Helper()
	out, err := h.describeAs(langver.V0, target)
	if err != nil || strings.TrimSpace(out) == "" {
		t.Fatalf("describe %s: %v (output %q)", target, err, out)
	}
	return out
}

// flowUnit returns the raw bytes of the microflow or nanoflow named name.
func (h *harness) flowUnit(t *testing.T, name string) []byte {
	t.Helper()
	for _, b := range h.snapshot().units {
		typ, n := typeAndName(b)
		if n == name && (typ == "Microflows$Microflow" || typ == "Microflows$Nanoflow") {
			return b
		}
	}
	t.Fatalf("flow %s not found", name)
	return nil
}

// requireKept fails when an element of before is missing from after, or is
// no longer the same element type. except names the action $Type of the one
// activity a replace is expected to take out: that activity and everything
// in it may go, nothing else.
func requireKept(t *testing.T, before, after []byte, except string) {
	t.Helper()
	a := elementIDs(t, after)
	b := elementIDsExcept(t, before, except)
	if except != "" && len(b) == len(elementIDs(t, before)) {
		t.Fatalf("no activity holds a %s — the fixture changed", except)
	}
	for id, typ := range b {
		got, ok := a[id]
		switch {
		case ok && got == typ:
		case ok:
			t.Errorf("$ID %s was a %s and is now a %s", uuidOf([]byte(id)), typ, got)
		default:
			t.Errorf("the %s with $ID %s is gone", typ, uuidOf([]byte(id)))
		}
	}
}

// elementIDs maps every element $ID in a unit to its $Type.
func elementIDs(t *testing.T, raw []byte) map[string]string {
	return elementIDsExcept(t, raw, "")
}

// elementIDsExcept is elementIDs without the activities whose action is a
// skipAction, and without anything they contain.
func elementIDsExcept(t *testing.T, raw []byte, skipAction string) map[string]string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode unit: %v", err)
	}
	out := map[string]string{}
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			if skipAction != "" {
				for _, e := range x {
					if act, ok := e.Value.(bson.D); ok && e.Key == "Action" {
						for _, ae := range act {
							if ae.Key == "$Type" && ae.Value == skipAction {
								return
							}
						}
					}
				}
			}
			var id, typ string
			for _, e := range x {
				switch e.Key {
				case "$ID":
					if b, ok := e.Value.(bson.Binary); ok {
						id = string(b.Data)
					}
				case "$Type":
					typ, _ = e.Value.(string)
				default:
					walk(e.Value)
				}
			}
			if id != "" {
				out[id] = typ
			}
		case bson.A:
			for _, el := range x {
				walk(el)
			}
		}
	}
	walk(doc)
	return out
}

// requireKeptBut is requireKept for a replace of the one activity whose
// action is a skipAction and whose subtree contains text.
func requireKeptBut(t *testing.T, before, after []byte, skipAction, text string) {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(before, &doc); err != nil {
		t.Fatalf("decode unit: %v", err)
	}
	gone := map[string]bool{}
	var walk func(v any, inside bool)
	walk = func(v any, inside bool) {
		switch x := v.(type) {
		case bson.D:
			if !inside {
				for _, e := range x {
					if act, ok := e.Value.(bson.D); ok && e.Key == "Action" {
						raw, _ := bson.Marshal(act)
						for _, ae := range act {
							if ae.Key == "$Type" && ae.Value == skipAction && bytes.Contains(raw, []byte(text)) {
								inside = true
							}
						}
					}
				}
			}
			for _, e := range x {
				if b, ok := e.Value.(bson.Binary); ok && e.Key == "$ID" && inside {
					gone[string(b.Data)] = true
				}
				walk(e.Value, inside)
			}
		case bson.A:
			for _, el := range x {
				walk(el, inside)
			}
		}
	}
	walk(doc, false)
	if len(gone) == 0 {
		t.Fatalf("no %s holds %q — the fixture changed", skipAction, text)
	}
	a := elementIDs(t, after)
	for id, typ := range elementIDs(t, before) {
		if gone[id] {
			continue
		}
		if got, ok := a[id]; !ok || got != typ {
			t.Errorf("the %s with $ID %s is gone or changed type (now %q)", typ, uuidOf([]byte(id)), got)
		}
	}
}

func countType(t *testing.T, raw []byte, typ string) int {
	t.Helper()
	n := 0
	for _, got := range elementIDs(t, raw) {
		if got == typ {
			n++
		}
	}
	return n
}
