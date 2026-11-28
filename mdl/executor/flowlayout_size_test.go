// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// ako/mxcli#2589: Studio Pro stores a resized activity or decision as its
// Size. DESCRIBE printed nothing for it and MDL had no way to state it, so a
// rebuild (drop + create from the description) reset every box to the
// default. `@size(w, h)` — already a loop's box — now sets an activity's and a
// split's too, and DESCRIBE prints it for a box that is not the default.

const sizedFlowMDL = `
create microflow MyFirstModule.ACT_Sized ($N: Integer) returns Boolean
begin
  @size(260, 90)
  log info 'wide';
  @caption 'big?'
  @size(220, 120)
  if $N > 10 then
    log info 'big';
  else
    log info 'small';
  end if;
  log info 'default';
  return true;
end;
`

func TestSizeAnnotationOnActivitiesAndSplits_2589(t *testing.T) {
	exec, out := describeExecutor(t)
	run(t, exec, sizedFlowMDL)
	const name = "MyFirstModule.ACT_Sized"
	before := fullGeometry(t, exec, "microflow", name)
	joined := strings.Join(before, "\n")
	for _, want := range []string{"{260 90}", "{220 120}"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("@size %s was not written:\n%s", want, joined)
		}
	}

	mdl := describeText(t, exec, out, "describe microflow "+name)
	for _, want := range []string{"@size(260, 90)", "@size(220, 120)"} {
		if !strings.Contains(mdl, want) {
			t.Errorf("describe lost %s:\n%s", want, mdl)
		}
	}
	// A default box prints nothing: two @size lines, not one per object.
	if n := strings.Count(mdl, "@size("); n != 2 {
		t.Errorf("describe printed %d @size lines, want 2:\n%s", n, mdl)
	}

	// The rebuild of #2589: drop, then create from the description.
	run(t, exec, "drop microflow "+name+";")
	run(t, exec, withoutGrant(mdl))
	sameGeometry(t, "drop + create from the description", before, fullGeometry(t, exec, "microflow", name))
}

// The in-place path: `create or modify` with a changed @size resizes that box
// and nothing else. It reported "Unchanged" and kept the stored box.
func TestSizeAnnotationResizesInPlace_2589(t *testing.T) {
	exec, out := describeExecutor(t)
	const name = "MyFirstModule.ACT_Sized"
	script := strings.Replace(sizedFlowMDL, "create microflow", "create or modify microflow", 1)
	run(t, exec, script)
	before := fullGeometry(t, exec, "microflow", name)

	out.Reset()
	run(t, exec, strings.Replace(script, "@size(260, 90)", "@size(300, 100)", 1))
	if !strings.Contains(out.String(), "resized") {
		t.Errorf("the changed @size was not applied:\n%s", out.String())
	}
	after := fullGeometry(t, exec, "microflow", name)
	var changed []string
	for i := range before {
		if i < len(after) && before[i] != after[i] {
			changed = append(changed, before[i]+" -> "+after[i])
		}
	}
	if len(changed) != 1 || !strings.Contains(changed[0], "{300 100}") {
		t.Errorf("want only the activity resized to 300x100, got:\n  %s", strings.Join(changed, "\n  "))
	}

	// A script without @size keeps the stored box, as one without @position
	// keeps its place.
	out.Reset()
	run(t, exec, strings.Replace(strings.Replace(script, "@size(260, 90)\n", "", 1), "@size(220, 120)\n", "", 1))
	sameGeometry(t, "a script without @size", after, fullGeometry(t, exec, "microflow", name))
}
