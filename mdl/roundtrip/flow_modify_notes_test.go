// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/langver"
)

// ako/mxcli#859 (rehearsal M3): `create or modify` of a flow whose statement
// changes the notes on an activity — a note added, reworded or taken off — was
// refused under mdl 1 ("the annotations on the replaced … change"), because
// the splice kept a replaced activity's stored notes and could only refuse
// others. The rehearsal hit it on mxcli-ledger's BUILD_ScatterUrl (a second
// note added to a `set`), and on Studio Pro-authored PedApp nanoflows whose
// note text differs under mdl 1 (a mdl 0 description, `\r\n` escapes and all,
// run under the mdl 1 header). The refusal was on every run, so the script
// could never be re-run.
//
// A re-annotated activity is now replaced together with its notes: the stored
// notes go, the declared ones are drawn. The second run writes nothing.
const notesFlow = `create or modify microflow MyFirstModule.Rerun_Notes ($Filter: String)
returns String as $Url
begin
  declare $Url String = '';
  @annotation 'Built here so it can be escaped.'
  set $Url = '/odata/v1?$filter=' + $Filter;
  log info node 'Notes' 'built';
  return $Url;
end;
`

func TestSpliceRerun_ReannotatedActivity(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	if err := h.exec(notesFlow); err != nil {
		t.Fatalf("create: %v\n%s", err, h.out.String())
	}

	// Control: the statement as created is unchanged, under either header.
	for _, header := range []string{"", "mdl 1;\n"} {
		before := h.snapshot()
		if err := h.exec(header + notesFlow); err != nil {
			t.Fatalf("re-run under %q: %v", header, err)
		}
		if changed := before.diff(h.snapshot()); len(changed) != 0 {
			t.Fatalf("the unchanged statement under %q wrote %d unit(s)", header, len(changed))
		}
	}

	const note = "@annotation 'Built here so it can be escaped.'\n"
	cases := []struct {
		name, edit string
		want, gone []string
	}{
		{"a note reworded",
			strings.Replace(notesFlow, "so it can be escaped.", "so it can be escaped, as OData needs.", 1),
			[]string{"so it can be escaped, as OData needs."}, []string{"so it can be escaped.'"}},
		{"a second note added (ledger BUILD_ScatterUrl)",
			strings.Replace(notesFlow, note, note+"  @annotation 'A relative path, so it is same-origin.'\n", 1),
			[]string{"so it can be escaped.", "A relative path, so it is same-origin."}, nil},
		{"the note taken off",
			strings.Replace(notesFlow, "  "+note, "", 1),
			nil, []string{"so it can be escaped."}},
		{"a note put on another activity",
			strings.Replace(strings.Replace(notesFlow, "  "+note, "", 1), "  log info", "  @annotation 'Logged once.'\n  log info", 1),
			[]string{"Logged once."}, []string{"so it can be escaped."}},
		// The drop path: the annotated activity goes, and its note with it.
		// Left behind, the note became a free annotation the second run
		// refused ("the free annotations change").
		{"the annotated activity dropped",
			strings.Replace(notesFlow, "  "+note+"  set $Url = '/odata/v1?$filter=' + $Filter;\n", "", 1),
			nil, []string{"so it can be escaped."}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			script := "mdl 1;\n" + c.edit
			if c.edit == notesFlow {
				t.Fatal("the edit changed nothing")
			}
			before := h.snapshot()
			if err := h.exec(script); err != nil {
				t.Fatalf("exec 1: %v\n%s", err, h.out.String())
			}
			if !strings.Contains(h.out.String(), "Modified microflow: MyFirstModule.Rerun_Notes (spliced:") {
				t.Errorf("exec 1 must splice the change in:\n%s", h.out.String())
			}
			if len(before.diff(h.snapshot())) == 0 {
				t.Fatal("exec 1 wrote nothing")
			}
			got := h.describeUnder("mdl 1;", "microflow MyFirstModule.Rerun_Notes")
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("the description lacks %q:\n%s", w, got)
				}
			}
			for _, g := range c.gone {
				if strings.Contains(got, g) {
					t.Errorf("the description still has %q:\n%s", g, got)
				}
			}
			if n := strings.Count(got, "@annotation"); n != len(c.want) {
				t.Errorf("%d notes described, want %d:\n%s", n, len(c.want), got)
			}
			// The twice-exec rule.
			second := h.snapshot()
			if err := h.exec(script); err != nil {
				t.Fatalf("exec 2: %v\n%s", err, h.out.String())
			}
			if changed := second.diff(h.snapshot()); len(changed) != 0 {
				t.Errorf("exec 2 wrote %d unit(s):\n%s", len(changed), h.out.String())
			}
			// Back to the created statement, for the next case.
			if err := h.exec("mdl 1;\n" + notesFlow); err != nil {
				t.Fatalf("restore the statement: %v\n%s", err, h.out.String())
			}
		})
	}
}

// The Studio Pro-authored form of M3: a PedApp nanoflow whose note, described
// by mdl 0 (`\r\n` escapes), is run under the mdl 1 header, where a backslash
// is a character — so the note's text differs, and was refused on every run.
// It is a real change of the note (mdl 1 reads the text as written), written
// once; the second run writes nothing.
func TestSpliceRerun_ReannotatedStudioProNanoflow(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	const target = "nanoflow FeedbackModule.ACT_Feedback_TriggerScreenshotMode"
	mdl0 := h.mustDescribeMdl0(t, target)
	if !strings.Contains(mdl0, `\r\n`) {
		t.Fatalf("the fixture's note no longer holds a line break; the case is gone:\n%s", mdl0)
	}
	script := "mdl 1;\n" + mdl0
	before := h.snapshot()
	if err := h.exec(script); err != nil {
		t.Fatalf("exec 1: %v\n%s", err, h.out.String())
	}
	if changed := before.diff(h.snapshot()); len(changed) != 1 {
		t.Errorf("exec 1 wrote %d unit(s), want the nanoflow alone", len(changed))
	}
	got := h.describeUnder("mdl 1;", target)
	if !strings.Contains(got, `Feedback Widget. \r\nThe widget`) {
		t.Errorf("the note is not the text the mdl 1 script states:\n%s", got)
	}
	second := h.snapshot()
	if err := h.exec(script); err != nil {
		t.Fatalf("exec 2: %v\n%s", err, h.out.String())
	}
	if changed := second.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("exec 2 wrote %d unit(s):\n%s", len(changed), h.out.String())
	}
	// Control: the nanoflow's own mdl 1 description is no change at all.
	h.restore()
	own := "mdl 1;\n" + h.describeUnder("mdl 1;", target)
	fresh := h.snapshot()
	if err := h.exec(own); err != nil {
		t.Fatalf("exec of its own mdl 1 description: %v", err)
	}
	if changed := fresh.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("its own mdl 1 description wrote %d unit(s)", len(changed))
	}
}

// ako/mxcli#859: `on error rollback` is what an activity with no clause stores
// in a microflow, so describe never prints it (formatErrorHandlingSuffix) — and
// a statement that states it never matched its own stored activity. Whenever
// the flow changed anywhere, the activity was dropped and written again as
// part of the change: a new $ID, its curves redrawn.
func TestSpliceRerun_OnErrorRollbackMatchesItsActivity(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	if err := h.exec("create persistent entity MyFirstModule.RerunRbThing (Name: String(100));"); err != nil {
		t.Fatalf("create the entity: %v", err)
	}
	const flows = `create or modify microflow MyFirstModule.Rerun_Rollback ($E: MyFirstModule.RerunRbThing)
begin
  log info node 'Rb' 'one';
  commit $E on error rollback;
end;
create or modify nanoflow MyFirstModule.Rerun_RollbackNf ($E: MyFirstModule.RerunRbThing)
begin
  change $E (Name = 'one');
  delete $E on error rollback;
end;
`
	if err := h.exec(flows); err != nil {
		t.Fatalf("create: %v\n%s", err, h.out.String())
	}
	edited := "mdl 1;\n" + strings.Replace(strings.Replace(flows, "'Rb' 'one'", "'Rb' 'two'", 1), "Name = 'one'", "Name = 'two'", 1)
	before := h.snapshot()
	if err := h.exec(edited); err != nil {
		t.Fatalf("exec 1: %v\n%s", err, h.out.String())
	}
	for _, want := range []string{
		"Modified microflow: MyFirstModule.Rerun_Rollback (spliced: 1 edited in place)",
		"Modified nanoflow: MyFirstModule.Rerun_RollbackNf (spliced: 1 edited in place)",
	} {
		// The control is in the same line: the change itself is written.
		if !strings.Contains(h.out.String(), want) {
			t.Errorf("want %q — the commit must match its stored activity:\n%s", want, h.out.String())
		}
	}
	if len(before.diff(h.snapshot())) == 0 {
		t.Fatal("exec 1 wrote nothing")
	}
	second := h.snapshot()
	if err := h.exec(edited); err != nil {
		t.Fatalf("exec 2: %v\n%s", err, h.out.String())
	}
	if changed := second.diff(h.snapshot()); len(changed) != 0 {
		t.Errorf("exec 2 wrote %d unit(s):\n%s", len(changed), h.out.String())
	}
}

// The rehearsal's form of M3, over every PedApp nanoflow: its mdl 0
// description put under the `mdl 1;` header — the upgrade a user makes by
// adding the header alone. The parity property (TestPedAppFlowSpliceParity)
// runs each flow's OWN mdl 1 description, which never differs from the stored
// flow; this one can (a note's `\r\n` is two characters under mdl 1), and two
// of the thirteen were refused on every run. Each must now be written at most
// once: executed a second time, it writes nothing.
func TestSpliceRerun_PedAppNanoflowsUnderTheHeader(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	n := 0
	for _, d := range h.documents() {
		if d.keyword != "nanoflow" {
			continue
		}
		n++
		t.Run(d.key(), func(t *testing.T) {
			defer h.restore()
			mdl0, err := h.describeAs(langver.V0, d.target())
			if err != nil {
				t.Fatalf("describe: %v", err)
			}
			script := "mdl 1;\n" + mdl0
			if err := h.exec(script); err != nil {
				t.Fatalf("exec 1: %v", err)
			}
			second := h.snapshot()
			if err := h.exec(script); err != nil {
				t.Fatalf("exec 2: %v", err)
			}
			if changed := second.diff(h.snapshot()); len(changed) != 0 {
				t.Errorf("exec 2 wrote %d unit(s):\n%s", len(changed), h.out.String())
			}
		})
	}
	if n == 0 {
		t.Fatal("PedApp has no nanoflow — the enumeration is broken")
	}
}

// mendixlabs/mxcli#1505: a multi-line free annotation holds CRLF, as Studio
// Pro stores it and describe prints it. The same flow with LF line breaks —
// what an editor saves — was refused under mdl 1 ("the free annotations
// change; the splice edits activities only"), on every run. Line breaks are
// not a change: the run writes nothing, and the note keeps its CRLF bytes.
func TestSpliceRerun_FreeNoteLineEndings(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	flow := func(eol string) string {
		return "mdl 1;\ncreate or modify microflow MyFirstModule.Rerun_FreeNoteEol ()\nbegin\n" +
			"  @annotation 'First line." + eol + "Second line.'\n" +
			"  @position(200, 200)\n" +
			"  @annotation 'On the log." + eol + "Two lines.'\n" +
			"  log info node 'Notes' 'hello';\nend;\n"
	}
	if err := h.exec(flow("\r\n")); err != nil {
		t.Fatalf("create: %v\n%s", err, h.out.String())
	}
	got := h.describeUnder("mdl 1;", "microflow MyFirstModule.Rerun_FreeNoteEol")
	if !strings.Contains(got, "First line.\r\nSecond line.") {
		t.Fatalf("the free note was not stored with CRLF; the case is gone:\n%q", got)
	}
	for _, eol := range []string{"\r\n", "\n"} {
		before := h.snapshot()
		if err := h.exec(flow(eol)); err != nil {
			t.Fatalf("re-run with %q line breaks: %v\n%s", eol, err, h.out.String())
		}
		if changed := before.diff(h.snapshot()); len(changed) != 0 {
			t.Errorf("re-run with %q line breaks wrote %d unit(s)", eol, len(changed))
		}
	}
	// Control: a real change to the free note is a change — written in place
	// (mendixlabs/mxcli#1517), not taken for a line-ending difference.
	h.out.Reset()
	before := h.snapshot()
	if err := h.exec(strings.Replace(flow("\n"), "Second line.", "Second line, reworded.", 1)); err != nil {
		t.Fatalf("a reworded free note: %v\n%s", err, h.out.String())
	}
	if !strings.Contains(h.out.String(), "(set: free notes)") || len(before.diff(h.snapshot())) == 0 {
		t.Errorf("a reworded free note was not written:\n%s", h.out.String())
	}
	if got := h.describeUnder("mdl 1;", "microflow MyFirstModule.Rerun_FreeNoteEol"); !strings.Contains(got, "Second line, reworded.") {
		t.Errorf("describe after the reword:\n%q", got)
	}
}
