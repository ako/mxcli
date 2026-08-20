// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// The two spellings of a type split must produce the same AST apart from the
// flags that exist only to drive the MDL065 deprecation warning. If they ever
// diverge, the "both build the identical flow" promise in the warning's own
// text stops being true. (#913)
func TestInheritanceSplit_BothSpellingsProduceTheSameAST(t *testing.T) {
	const legacy = `CREATE MICROFLOW Sample.Route ($Input: Sample.Animal)
RETURNS String
BEGIN
  split type $Input
    case Sample.Dog
      return 'woof';
    case Sample.Cat
      return 'meow';
    else
      return 'none';
  end split;
END;`

	const modern = `CREATE MICROFLOW Sample.Route ($Input: Sample.Animal)
RETURNS String
BEGIN
  split type $Input
    when Sample.Dog then
      return 'woof';
    when Sample.Cat then
      return 'meow';
    when (empty) then
      return 'none';
  end split;
END;`

	oldSplit := parseInheritanceSplit(t, legacy)
	newSplit := parseInheritanceSplit(t, modern)

	if oldSplit.Variable != newSplit.Variable {
		t.Errorf("variable: legacy %q, modern %q", oldSplit.Variable, newSplit.Variable)
	}
	if len(oldSplit.Cases) != len(newSplit.Cases) {
		t.Fatalf("case count: legacy %d, modern %d", len(oldSplit.Cases), len(newSplit.Cases))
	}
	for i := range oldSplit.Cases {
		if got, want := newSplit.Cases[i].Entity.String(), oldSplit.Cases[i].Entity.String(); got != want {
			t.Errorf("case %d entity: legacy %q, modern %q", i, want, got)
		}
		if got, want := len(newSplit.Cases[i].Body), len(oldSplit.Cases[i].Body); got != want {
			t.Errorf("case %d body length: legacy %d, modern %d", i, want, got)
		}
	}
	if got, want := len(newSplit.ElseBody), len(oldSplit.ElseBody); got != want {
		t.Errorf("empty-branch body length: legacy %d, modern %d", want, got)
	}
	if len(newSplit.ElseBody) == 0 {
		t.Error("`when (empty) then` did not populate ElseBody — the empty branch was dropped")
	}
}

// The spelling flags are what MDL065 keys off. A parser that stopped setting
// them would silently retire the warning, and nothing else would fail.
func TestInheritanceSplit_SpellingFlagsRecordTheSource(t *testing.T) {
	tests := []struct {
		name           string
		branch, empty  string
		wantCaseLegacy bool
		wantElseLegacy bool
	}{
		{"both legacy", "case Sample.Dog", "else", true, true},
		{"both modern", "when Sample.Dog then", "when (empty) then", false, false},
		{"legacy branch, modern empty", "case Sample.Dog", "when (empty) then", true, false},
		{"modern branch, legacy empty", "when Sample.Dog then", "else", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := `CREATE MICROFLOW Sample.Route ($Input: Sample.Animal)
RETURNS String
BEGIN
  split type $Input
    ` + tc.branch + `
      return 'woof';
    ` + tc.empty + `
      return 'none';
  end split;
END;`
			split := parseInheritanceSplit(t, src)
			if split.LegacyCaseKeyword != tc.wantCaseLegacy {
				t.Errorf("LegacyCaseKeyword = %v, want %v", split.LegacyCaseKeyword, tc.wantCaseLegacy)
			}
			if split.LegacyElseKeyword != tc.wantElseLegacy {
				t.Errorf("LegacyElseKeyword = %v, want %v", split.LegacyElseKeyword, tc.wantElseLegacy)
			}
		})
	}
}

// Mixing the spellings within one statement parses. Nothing depends on it, but
// a grammar change that made the two alternatives mutually exclusive would
// reject scripts mid-migration, so the tolerance is pinned deliberately.
func TestInheritanceSplit_MixedSpellingsParse(t *testing.T) {
	split := parseInheritanceSplit(t, `CREATE MICROFLOW Sample.Route ($Input: Sample.Animal)
RETURNS String
BEGIN
  split type $Input
    case Sample.Dog
      return 'woof';
    when Sample.Cat then
      return 'meow';
    when (empty) then
      return 'none';
  end split;
END;`)
	if len(split.Cases) != 2 {
		t.Fatalf("case count = %d, want 2", len(split.Cases))
	}
	if !split.LegacyCaseKeyword {
		t.Error("LegacyCaseKeyword = false, want true — one branch used `case`")
	}
	if split.LegacyElseKeyword {
		t.Error("LegacyElseKeyword = true, want false — the empty branch used the modern spelling")
	}
}

func parseInheritanceSplit(t *testing.T, src string) *ast.InheritanceSplitStmt {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("statement 0: got %T, want *ast.CreateMicroflowStmt", prog.Statements[0])
	}
	split, ok := mf.Body[0].(*ast.InheritanceSplitStmt)
	if !ok {
		t.Fatalf("body statement 0: got %T, want *ast.InheritanceSplitStmt", mf.Body[0])
	}
	return split
}
