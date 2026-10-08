// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"
)

// mendixlabs/mxcli#1325: a retrieve constraint comparing an attribute with a
// variable of another type passed `check --references` and exec, then mxbuild
// reported CE0161 "Error(s) in XPath constraint." Each row was measured on
// mxbuild 11.14.0, `=` and `>=` alike (see validate_retrieve_operand_types.go).
const operandTypesHead = `mdl 1;
create module G53;
create enumeration G53.Kind (A 'A', B 'B');
create enumeration G53.Other (A 'A', C 'C');
create persistent entity G53.Item (Kind: Enumeration(G53.Kind), Email: String(200), Visits: Integer, Seen: DateTime, Done: Boolean, Amount: Decimal);
`

const operandTypesFlow = `create microflow G53.SUB_Find ($P: %s) returns Integer as $N
begin
  retrieve $L from G53.Item where %s;
  declare $N Integer = length($L);
  return $N;
end;
`

var operandTypeCases = []struct {
	varType, where, want string // want "" = builds clean, must pass check
}{
	// The three shapes of the issue.
	{"String", "Kind = $P", "compares Kind (Enumeration G53.Kind) with $P (String)"},
	{"DateTime", "Email >= $P", "compares Email (String) with $P (DateTime)"},
	{"String", "Visits = $P", "compares Visits (Integer) with $P (String)"},
	// Further measured mismatches.
	{"Enumeration(G53.Other)", "Kind = $P", "compares Kind (Enumeration G53.Kind) with $P (Enumeration G53.Other)"},
	{"Integer", "Done = $P", "compares Done (Boolean) with $P (Integer)"},
	{"Integer", "Seen = $P", "compares Seen (DateTime) with $P (Integer)"},
	{"String", "$P = Visits", "compares Visits (Integer) with $P (String)"},
	{"String", "Email = 'x' and (Visits = $P)", "compares Visits (Integer) with $P (String)"},
	// Controls, each measured clean.
	{"Enumeration(G53.Kind)", "Kind = $P", ""},
	{"String", "Email = $P", ""},
	{"String", "Seen >= $P", ""}, // a String against a DateTime attribute builds
	{"Long", "Visits = $P", ""},  // the numeric types compare with one another
	{"Integer", "Amount >= $P", ""},
	{"Boolean", "Done = $P", ""},
	{"String", "Kind = 'A'", ""}, // #176: an enumeration against a string literal
}

func TestRetrieveOperandTypes_ScriptEntity(t *testing.T) {
	for _, c := range operandTypeCases {
		exec, _, dir := openPedAppCopy(t)
		src := operandTypesHead + fmt.Sprintf(operandTypesFlow, c.varType, c.where)
		got := strings.Join(agreeCheck(t, exec, dir, src), "\n")
		if c.want == "" {
			if got != "" {
				t.Errorf("%s with $P: %s — control must pass check, reported:\n%s", c.where, c.varType, got)
			}
			continue
		}
		if !strings.Contains(got, c.want) || !strings.Contains(got, "CE0161") {
			t.Errorf("%s with $P: %s — check -p must report %q, reported:\n%s", c.where, c.varType, c.want, got)
		}
	}
}

// The entity already in the project, the microflow in a later script: the
// attribute's type comes from the stored domain model, not a declaration.
func TestRetrieveOperandTypes_ProjectEntity(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, operandTypesHead); err != nil {
		t.Fatalf("setup: %v", err)
	}
	bad := "mdl 1;\n" + fmt.Sprintf(operandTypesFlow, "String", "Kind = $P")
	got := strings.Join(agreeCheck(t, exec, dir, bad), "\n")
	if !strings.Contains(got, "compares Kind (Enumeration G53.Kind) with $P (String)") {
		t.Errorf("check -p must report the mismatch on a stored entity, reported:\n%s", got)
	}
	if !strings.Contains(got, "string literal of the value key") {
		t.Errorf("the enumeration case must carry the hint, reported:\n%s", got)
	}
	good := "mdl 1;\n" + fmt.Sprintf(operandTypesFlow, "Enumeration(G53.Kind)", "Kind = $P")
	assertAgree(t, exec, dir, good, "")
}

// A declared variable is typed the same as a parameter.
func TestRetrieveOperandTypes_DeclaredVariable(t *testing.T) {
	exec, _, dir := openPedAppCopy(t)
	src := operandTypesHead + `create microflow G53.SUB_Declared () returns Integer as $N
begin
  declare $T String = 'x';
  retrieve $L from G53.Item where Visits = $T;
  declare $N Integer = length($L);
  return $N;
end;
`
	got := strings.Join(agreeCheck(t, exec, dir, src), "\n")
	if !strings.Contains(got, "compares Visits (Integer) with $T (String)") {
		t.Errorf("check -p must report a declared variable's mismatch, reported:\n%s", got)
	}
}
