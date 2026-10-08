// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// A page or snippet variable's default is a Mendix expression. It was written
// in a string whose CONTENT is the expression — `$show: boolean = 'true'`,
// `= 'if (3 < 4) then true else false'` — and is now written bare (R5). The
// string form keeps its meaning under every language version (mdl 1 is
// frozen) and is the deprecated alias MDL-DEPR086.

func pageVariables(t *testing.T, src string) ([]ast.PageVariable, *ast.Program) {
	t.Helper()
	prog := mustBuild(t, src)
	for _, s := range prog.Statements {
		switch st := s.(type) {
		case *ast.CreatePageStmtV3:
			return st.Variables, prog
		case *ast.CreateSnippetStmtV3:
			return st.Variables, prog
		case *ast.AlterPageStmt:
			for _, op := range st.Operations {
				if add, ok := op.(*ast.AddVariableOp); ok {
					return []ast.PageVariable{add.Variable}, prog
				}
			}
		}
	}
	t.Fatalf("no variables in %q", src)
	return nil, nil
}

func TestPageVariableDefault_BareExpression(t *testing.T) {
	const page = "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Variables: ( %s )) { };"
	for _, tc := range []struct{ decl, want string }{
		{"$show: boolean = true", "true"},
		{"$show: boolean = if 3 < 4 then true else false", "if 3 < 4 then true else false"},
		{"$n: integer = 20", "20"},
		{"$when: datetime = [%CurrentDateTime%]", "[%CurrentDateTime%]"},
		{"$name: string = 'a' + 'b'", "'a' + 'b'"},
	} {
		vars, prog := pageVariables(t, fmt.Sprintf(page, tc.decl))
		if len(vars) != 1 || vars[0].DefaultValue != tc.want {
			t.Errorf("%s: stored %+v, want default %q", tc.decl, vars, tc.want)
		}
		if got := deprecationCodes(prog); len(got) != 0 {
			t.Errorf("%s: recorded %v, want none", tc.decl, got)
		}
	}
	// Several, the list's commas not taken for the expression's.
	vars, _ := pageVariables(t, fmt.Sprintf(page, "$a: boolean = not(false), $b: integer = max(1, 2), $c: boolean = $a"))
	want := []string{"not(false)", "max(1, 2)", "$a"}
	var got []string
	for _, v := range vars {
		got = append(got, v.DefaultValue)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("defaults %v, want %v", got, want)
	}
}

func TestPageVariableDefault_SnippetAndAlter(t *testing.T) {
	vars, _ := pageVariables(t, "create snippet M.S (Variables: ( $show: boolean = false )) { };")
	if len(vars) != 1 || vars[0].DefaultValue != "false" {
		t.Errorf("snippet: %+v", vars)
	}
	vars, _ = pageVariables(t, "alter page M.P { add variables $show: boolean = 1 < 2 };")
	if len(vars) != 1 || vars[0].DefaultValue != "1 < 2" {
		t.Errorf("alter add variables: %+v", vars)
	}
}

// The string form keeps its meaning — the content is the expression — and is
// reported, except where it has no bare spelling: a default that is itself a
// string (`”'abc”'`, whose bare `'abc'` is this very alias) or empty.
func TestPageVariableDefault_StringFormIsTheDeprecatedAlias(t *testing.T) {
	const page = "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Variables: ( %s )) { };"
	for _, tc := range []struct {
		decl, want string
		reported   bool
	}{
		{"$show: boolean = 'true'", "true", true},
		{"$show: boolean = 'if (3 < 4) then true else false'", "if (3 < 4) then true else false", true},
		{"$n: integer = '20'", "20", true},
		{"$s: string = '''abc'''", "'abc'", false},
		{"$s: string = ''", "", false},
	} {
		for _, header := range []string{"", "mdl 1;\n"} {
			vars, prog := pageVariables(t, header+fmt.Sprintf(page, tc.decl))
			if len(vars) != 1 || vars[0].DefaultValue != tc.want {
				t.Errorf("%q%s: stored %+v, want default %q", header, tc.decl, vars, tc.want)
			}
			codes := deprecationCodes(prog)
			if tc.reported && !reflect.DeepEqual(codes, []string{deprecation.QuotedVariableDefault}) {
				t.Errorf("%q%s: recorded %v, want [%s]", header, tc.decl, codes, deprecation.QuotedVariableDefault)
			}
			if !tc.reported && len(codes) != 0 {
				t.Errorf("%q%s: recorded %v, want none", header, tc.decl, codes)
			}
		}
	}
}
