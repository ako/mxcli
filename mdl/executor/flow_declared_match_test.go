// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func parseFlowBody(t *testing.T, body string) *ast.CreateMicroflowStmt {
	t.Helper()
	prog, errs := visitor.Build("create or modify microflow M.F ($In: String)\nbegin\n" + body + "\nend;\n")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	s, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("got %T", prog.Statements[0])
	}
	return s
}

// The stored side is what describe prints: geometry on every statement.
const storedFlowBody = `  @position(200, 200)
  @curve(from: (30, 0), to: (-15, 0))
  @caption 'Say hello'
  log info node 'N' 'hello';
  @position(400, 200)
  if $In = 'x' then
    @position(400, 300)
    @anchor(from: bottom, to: top)
    log info node 'N' 'x';
  end if;`

func TestDeclaredMatches_OmittedGeometryIsNotADifference(t *testing.T) {
	stored := parseFlowBody(t, storedFlowBody)
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"identical", storedFlowBody, true},
		{"no geometry at all", `  @caption 'Say hello'
  log info node 'N' 'hello';
  if $In = 'x' then
    log info node 'N' 'x';
  end if;`, true},
		{"a node moved", `  @position(210, 200)
  @caption 'Say hello'
  log info node 'N' 'hello';
  if $In = 'x' then
    log info node 'N' 'x';
  end if;`, false},
		{"a caption dropped", `  log info node 'N' 'hello';
  if $In = 'x' then
    log info node 'N' 'x';
  end if;`, false},
		{"a nested statement changed", `  @caption 'Say hello'
  log info node 'N' 'hello';
  if $In = 'x' then
    log info node 'N' 'y';
  end if;`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			declared := parseFlowBody(t, tc.body)
			if got := declaredMatches(declared.Body, stored.Body); got != tc.want {
				t.Errorf("declaredMatches = %v, want %v", got, tc.want)
			}
		})
	}

	// The asymmetry: geometry the STORED side lacks is not a wildcard. A
	// declared position against a stored flow drawn elsewhere is a move.
	bare := parseFlowBody(t, `  @caption 'Say hello'
  log info node 'N' 'hello';`)
	placed := parseFlowBody(t, `  @position(200, 200)
  @caption 'Say hello'
  log info node 'N' 'hello';`)
	if declaredMatches(placed.Body, bare.Body) {
		t.Error("a declared @position matched a statement stored without one")
	}
}

func TestLCSStatements_PairsTheUnchangedRuns(t *testing.T) {
	stored := parseFlowBody(t, `  log info node 'N' 'a';
  log info node 'N' 'b';
  log info node 'N' 'c';
  log info node 'N' 'd';`)
	declared := parseFlowBody(t, `  log info node 'N' 'a';
  log info node 'N' 'new';
  log info node 'N' 'b';
  log info node 'N' 'C';
  log info node 'N' 'd';`)
	got := lcsStatements(declared.Body, stored.Body)
	want := [][2]int{{0, 0}, {2, 1}, {4, 3}}
	if len(got) != len(want) {
		t.Fatalf("pairs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pairs = %v, want %v", got, want)
		}
	}
}

// ako/mxcli#818: a statement drawn elsewhere is the stored node moved — it
// matches once positions are ignored — but a redrawn connector is not.
func TestSameExceptPositions_PositionsOnly(t *testing.T) {
	stored := parseFlowBody(t, storedFlowBody)
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"a node and a nested node moved", `  @position(210, 260)
  @curve(from: (30, 0), to: (-15, 0))
  @caption 'Say hello'
  log info node 'N' 'hello';
  @position(400, 200)
  if $In = 'x' then
    @position(420, 330)
    @anchor(from: bottom, to: top)
    log info node 'N' 'x';
  end if;`, true},
		{"a start stated", `  @start(10, 10)
  @caption 'Say hello'
  log info node 'N' 'hello';
  if $In = 'x' then
    log info node 'N' 'x';
  end if;`, true},
		{"a curve redrawn", `  @curve(from: (30, 0), to: (-15, 20))
  @caption 'Say hello'
  log info node 'N' 'hello';
  if $In = 'x' then
    log info node 'N' 'x';
  end if;`, false},
		{"an anchor redrawn", `  @caption 'Say hello'
  log info node 'N' 'hello';
  if $In = 'x' then
    @anchor(from: right, to: top)
    log info node 'N' 'x';
  end if;`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := parseFlowBody(t, c.body)
			if got := sameExceptPositions(d.Body, stored.Body); got != c.want {
				t.Errorf("sameExceptPositions = %v, want %v", got, c.want)
			}
		})
	}
}

// Of two alike stored statements, the one the script leaves in place keeps
// its node; the moved one pairs with the other.
func TestLCSStatements_ExactMatchWinsOverAMove(t *testing.T) {
	stored := parseFlowBody(t, `  @position(200, 200)
  log info node 'N' 'same';
  @position(400, 200)
  log info node 'N' 'same';`).Body
	declared := parseFlowBody(t, `  @position(400, 200)
  log info node 'N' 'same';`).Body
	pairs := lcsStatements(declared, stored)
	if len(pairs) != 1 || pairs[0] != [2]int{0, 1} {
		t.Fatalf("pairs %v, want the declared statement paired with the stored one it matches exactly", pairs)
	}
}

// mendixlabs/mxcli#1598, case 6: a new `if` inserted before a stored one that
// is restated with a change in its branch. The new if is the stored one but
// for its condition, and the restated if is the stored one but for its
// branch; both counted the same, and the tie went to the earlier pair, so the
// new if took over the stored decision (its condition set in place) and the
// restated if was inserted as a new node — on top of it, at the position the
// script states. The same condition is the closer match.
func TestLCSStatements_TheSameConditionWinsOverAChangedOne(t *testing.T) {
	stored := parseFlowBody(t, `  @position(-500, 200)
  log info node 'N' 'first';
  @position(-299, 200)
  if $In = 'x' then
    @position(-124, 100)
    log info node 'N' 'a';
  end if;`).Body
	declared := parseFlowBody(t, `  @position(-500, 200)
  log info node 'N' 'first';
  if $In = 'y' then
    log info node 'N' 'new';
  end if;
  @position(-299, 200)
  if $In = 'x' then
    @position(-124, 100)
    log info node 'N' 'b';
  end if;`).Body
	pairs := lcsStatements(declared, stored)
	if len(pairs) != 2 || pairs[0] != [2]int{0, 0} || pairs[1] != [2]int{2, 1} {
		t.Fatalf("pairs %v, want [[0 0] [2 1]]: the restated if paired with the stored one, the new if inserted", pairs)
	}
}

// mendixlabs/mxcli#1505: Studio Pro stores a multi-line note with CRLF line
// breaks and describe prints those bytes as they are. An editor that turns
// them into LF changed nothing anyone can see on the canvas, yet the free
// annotations no longer matched and `create or modify` under mdl 1 refused
// the splice ("the free annotations change; the splice edits activities
// only"). A note on an activity re-spliced that activity for the same reason.
func TestDeclaredMatches_NoteLineEndingsAreNotADifference(t *testing.T) {
	body := func(eol string) string {
		return "  @annotation 'First line." + eol + "Second line.'\n" +
			"  @position(200, 200)\n" +
			"  @annotation 'On the log." + eol + "Two lines.'\n" +
			"  log info node 'N' 'hello';"
	}
	stored := parseFlowBody(t, body("\r\n"))
	storedStmts, storedFree := withoutFreeNotes(stored.Body)
	if len(storedFree) != 1 || storedFree[0].Text != "First line.\r\nSecond line." {
		t.Fatalf("the stored free note is not the CRLF one: %#v", storedFree)
	}
	cases := []struct {
		name, eol string
		want      bool
	}{
		{"CRLF as described", "\r\n", true},
		{"LF from an editor", "\n", true},
		{"a bare CR", "\r", true},
		// Control: a real change to the text is still a change.
		{"the break taken out", " ", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			declared := parseFlowBody(t, body(c.eol))
			declaredStmts, declaredFree := withoutFreeNotes(declared.Body)
			if got := declaredMatches(declaredFree, storedFree); got != c.want {
				t.Errorf("free notes: declaredMatches = %v, want %v", got, c.want)
			}
			if got := declaredMatches(declaredStmts[0], storedStmts[0]); got != c.want {
				t.Errorf("activity note: declaredMatches = %v, want %v", got, c.want)
			}
		})
	}
}

// The refusal names the note, so a real change is found without diffing the
// whole flow by hand (mendixlabs/mxcli#1505).
func TestFreeNoteChange_NamesTheNote(t *testing.T) {
	note := func(text string) ast.MicroflowAnnotation { return ast.MicroflowAnnotation{Text: text} }
	cases := []struct {
		name             string
		declared, stored []ast.MicroflowAnnotation
		want             string
	}{
		{"reworded", []ast.MicroflowAnnotation{note("a"), note("b2")}, []ast.MicroflowAnnotation{note("a"), note("b")},
			`note "b" is now "b2"`},
		{"added", []ast.MicroflowAnnotation{note("a")}, nil, `note "a" is not in the stored flow`},
		{"removed", nil, []ast.MicroflowAnnotation{note("a")}, `note "a" is not in the script`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := freeNoteChange(c.declared, c.stored); got != c.want {
				t.Errorf("got %s, want %s", got, c.want)
			}
		})
	}
}
