// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#885: `create or modify` that splices a replacement activity built
// it with a builder that only knew the types of the stored flow's parameters,
// declared variables and created objects. A variable bound by a retrieve, a
// microflow call or a list operation was untyped, so the replacement's members
// were written without their entity: a change's member as the bare `Name`
// instead of `System.User.Name` — and Mendix cannot load a project holding one
// ("The text 'Name' is not a valid AttributeIdentifier") — a find's `Name = 'y'`
// as a find by that (invalid) expression, an aggregate's attribute as nothing.
// A parameter-bound variable was typed and is the control.
//
// The splice must write what a full build of the same statement writes, so each
// case compares the members the splice wrote with the members `create` writes
// for the edited flow under another name.
const memberQualifiedSetup = `mdl 1;
create or modify microflow MyFirstModule.T885_GetUser ()
returns System.User as $U
begin
  retrieve $U from System.User first;
  return $U;
end;
create or modify microflow MyFirstModule.T885_GetUsers ()
returns List of System.User as $Us
begin
  retrieve $Us from System.User;
  return $Us;
end;
`

const memberQualifiedFlow = `create or modify microflow MyFirstModule.T885_Flow ($P: System.User)
begin
  retrieve $R from System.User first;
  change $R (Name = 'r');
  $C = call microflow MyFirstModule.T885_GetUser();
  change $C (Name = 'c');
  $L = call microflow MyFirstModule.T885_GetUsers();
  $S = SORT $L BY Name ASC;
  $F = FIND $L BY Name = 'x';
  $Sum = SUM $L BY FailedLogins;
  $H = HEAD $L;
  change $H (Name = 'h');
  change $P (Name = 'p');
  loop $It in $L
  begin
    change $It (Name = 'i');
  end loop;
end;
`

func TestFlowModify_SplicedMembersAreQualified(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	cases := []struct {
		name, from, to string
	}{
		{name: "change on a retrieve-bound variable", from: "(Name = 'r')", to: "(Name = 'r2')"},
		{name: "change on a call-bound variable", from: "(Name = 'c')", to: "(Name = 'c2')"},
		{name: "change on a list-operation-bound variable", from: "(Name = 'h')", to: "(Name = 'h2')"},
		{name: "sort of a call-bound list", from: "BY Name ASC", to: "BY Name DESC"},
		{name: "find in a call-bound list", from: "BY Name = 'x'", to: "BY Name = 'y'"},
		{name: "aggregate of a call-bound list", from: "SUM $L", to: "MAXIMUM $L"},
		{name: "control: change on a parameter", from: "(Name = 'p')", to: "(Name = 'p2')"},
		// Spliced in place inside the loop body (ako/mxcli#1106): the
		// iterator's members must be qualified there too.
		{name: "change on a loop iterator", from: "(Name = 'i')", to: "(Name = 'i2')"},
	}
	for _, header := range []string{"mdl 1;\n", ""} {
		for _, c := range cases {
			label := fmt.Sprintf("%s (header %q)", c.name, strings.TrimSpace(header))
			h.restore()
			if err := h.exec(memberQualifiedSetup + memberQualifiedFlow); err != nil {
				t.Fatalf("%s: setup: %v", label, err)
			}
			edited := strings.Replace(memberQualifiedFlow, c.from, c.to, 1)
			if edited == memberQualifiedFlow {
				t.Fatalf("%s: the flow has no %q", label, c.from)
			}
			err := h.exec(header + edited)
			switch {
			case err != nil:
				t.Errorf("%s: exec: %v", label, err)
				continue
			case !strings.Contains(h.out.String(), "(spliced: 1 edited in place)"):
				t.Errorf("%s: want one statement edited in place by a splice, got:\n%s", label, h.out.String())
			}
			spliced := flowMembers(t, h.flowUnit(t, "T885_Flow"))
			for _, m := range spliced {
				if bare, ok := strings.CutPrefix(m, "bare "); ok {
					t.Errorf("%s: the splice wrote an unqualified member %s", label, bare)
				}
			}

			// Parity: what `create` writes for the edited flow.
			full := strings.Replace(edited, "MyFirstModule.T885_Flow", "MyFirstModule.T885_Full", 1)
			if err := h.exec(header + full); err != nil {
				t.Fatalf("%s: full build: %v", label, err)
			}
			if want := flowMembers(t, h.flowUnit(t, "T885_Full")); !slices.Equal(spliced, want) {
				t.Errorf("%s: the splice wrote other members than a full build\n only spliced: %v\n    only full: %v",
					label, without(spliced, want), without(want, spliced))
			}
		}
	}
}

// flowMembers lists, sorted, every entity member a flow unit names — a change
// or create item's attribute or association, a list operation's, an
// aggregate's, a sort item's — with the $Type that holds it. A list operation
// that names its member in an expression is listed by its $Type, so a find by
// member that turns into a find by expression is a difference. A non-empty
// attribute that is not Module.Entity.Attribute is listed as "bare …".
func flowMembers(t *testing.T, raw []byte) []string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode unit: %v", err)
	}
	var out []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			typ := ""
			fields := map[string]string{}
			for _, e := range x {
				if s, ok := e.Value.(string); ok {
					fields[e.Key] = s
				}
			}
			typ = fields["$Type"]
			switch typ {
			case "Microflows$ChangeActionItem", "DomainModels$AttributeRef", "Microflows$Find", "Microflows$Filter",
				"Microflows$AggregateAction":
				for _, k := range []string{"Attribute", "Association"} {
					s := fields[k]
					if k == "Attribute" && s != "" && strings.Count(s, ".") < 2 {
						out = append(out, fmt.Sprintf("bare %s.%s=%q", typ, k, s))
						continue
					}
					out = append(out, fmt.Sprintf("%s.%s=%q", typ, k, s))
				}
			case "Microflows$FindByExpression", "Microflows$FilterByExpression":
				out = append(out, typ)
			}
			for _, e := range x {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	walk(doc)
	slices.Sort(out)
	return out
}

// without returns a minus b, as multisets.
func without(a, b []string) []string {
	left := map[string]int{}
	for _, s := range b {
		left[s]++
	}
	var out []string
	for _, s := range a {
		if left[s] > 0 {
			left[s]--
			continue
		}
		out = append(out, s)
	}
	return out
}
