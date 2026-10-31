// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1640 (ako/mxcli#1649): since Mendix 11.5 a page parameter
// can be optional (Studio Pro: untick Required; Forms$PageParameter.IsRequired
// = false), and a menu item may then open the page with no argument — mx check
// 0 errors. mxcli could neither say nor see it: describe page dropped the flag,
// describe navigation invented `(Sale = $currentObject)` for that menu item, and
// executing describe's output was refused with MDL-PAGEARG01, so the navigation
// profile could no longer be changed from MDL at all.

const issue1649Setup = `create persistent entity MyFirstModule.Sale (Name: String(100));`

const issue1649Page = `create page MyFirstModule.Sale_View (
  Title: 'Sale',
  Layout: Atlas_Core.Atlas_Default,
  Params: ( $Sale: MyFirstModule.Sale optional )
) {
  dataview dvSale (DataSource: $Sale) { textbox tbName (Attribute: Name) }
};`

// storedParameterRequired reads the stored IsRequired of the page's parameter,
// and whether the key is stored at all.
func storedParameterRequired(t *testing.T, exec *Executor, id string) (required, present bool) {
	t.Helper()
	params := bsonDocs(bsonValue(storedUnitDoc(t, exec, id), "Parameters"))
	if len(params) != 1 {
		t.Fatalf("page stores %d parameters", len(params))
	}
	v, ok := bsonnav.DGet(params[0], "IsRequired").(bool)
	return v, ok
}

func TestIssue1649_OptionalPageParameterIsWrittenAndDescribed(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue1649Setup)
	run(t, exec, issue1649Page)
	id := unitIDByName(t, exec, "page", "Sale_View")
	if required, present := storedParameterRequired(t, exec, id); required || !present {
		t.Errorf("stored IsRequired = %v (present %v), want false", required, present)
	}

	out.Reset()
	run(t, exec, "describe page MyFirstModule.Sale_View")
	described := out.String()
	if !strings.Contains(described, "Params: ( $Sale: MyFirstModule.Sale optional )") {
		t.Fatalf("describe page does not say the parameter is optional:\n%s", described)
	}
	out.Reset()
	run(t, exec, described)
	if got := out.String(); !strings.Contains(got, "Unchanged page") {
		t.Errorf("re-running describe wrote the page:\n%s", got)
	}

	// Without the word the parameter is required, as before.
	run(t, exec, strings.Replace(described, " optional", "", 1))
	if required, _ := storedParameterRequired(t, exec, id); !required {
		t.Error("dropping `optional` left the parameter optional")
	}
}

// The report's scenario: a Studio Pro page whose parameter is optional, opened
// by a menu item with no argument.
func TestIssue1649_DescribeNavigationOpensAnOptionalParameterPageWithNoArgument(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue1649Setup)
	run(t, exec, strings.Replace(issue1649Page, " optional", "", 1))
	id := unitIDByName(t, exec, "page", "Sale_View")

	// Untick Required, as Studio Pro stores it.
	doc := storedUnitDoc(t, exec, id)
	param := bsonDocs(bsonValue(doc, "Parameters"))[0]
	if !bsonnav.DSet(param, "IsRequired", false) {
		t.Fatal("the stored parameter has no IsRequired to clear")
	}
	writeUnitDoc(t, exec, id, doc)

	out.Reset()
	run(t, exec, "describe page MyFirstModule.Sale_View")
	if !strings.Contains(out.String(), "$Sale: MyFirstModule.Sale optional") {
		t.Errorf("describe page hides the Studio Pro optional flag:\n%s", out.String())
	}

	run(t, exec, `create or modify navigation Responsive
  home page MyFirstModule.Home_Web
{
  menu item 'Home' ( OnClick: show page MyFirstModule.Home_Web )
  menu item 'Sale' ( OnClick: show page MyFirstModule.Sale_View )
};`)
	out.Reset()
	run(t, exec, "describe navigation Responsive")
	described := out.String()
	if strings.Contains(described, "Sale = $currentObject") {
		t.Fatalf("describe navigation invents an argument for the optional parameter:\n%s", described)
	}
	if !strings.Contains(described, "show page MyFirstModule.Sale_View") {
		t.Fatalf("describe navigation lost the menu item:\n%s", described)
	}
	out.Reset()
	if err := runErr(t, exec, described); err != nil {
		t.Fatalf("re-running describe navigation is refused: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Unchanged") {
		t.Errorf("re-running describe navigation wrote it:\n%s", got)
	}
}

// A required parameter is still mapped implicitly, and one stored before 11.5
// — no IsRequired key at all — reads as required, not as optional.
func TestIssue1649_AParameterWithNoStoredFlagIsRequired(t *testing.T) {
	exec, out := pageFixtureExecutor(t)
	run(t, exec, issue1649Setup)
	run(t, exec, strings.Replace(issue1649Page, " optional", "", 1))
	id := unitIDByName(t, exec, "page", "Sale_View")

	doc := storedUnitDoc(t, exec, id)
	params, _ := bsonValue(doc, "Parameters").(bson.A)
	p := params[1].(bson.D)
	var kept bson.D
	for _, e := range p {
		if e.Key != "IsRequired" && e.Key != "DefaultValue" {
			kept = append(kept, e)
		}
	}
	params[1] = kept
	writeUnitDoc(t, exec, id, doc)
	if _, present := storedParameterRequired(t, exec, id); present {
		t.Fatal("seed did not take")
	}

	out.Reset()
	run(t, exec, "describe page MyFirstModule.Sale_View")
	if strings.Contains(out.String(), "optional") {
		t.Errorf("a parameter with no stored IsRequired is described as optional:\n%s", out.String())
	}
}

func TestIssue1649_OptionalIsRefusedWhereItCannotBeStored(t *testing.T) {
	// A snippet parameter has no IsRequired at all.
	prog, errs := visitor.Build(`create snippet MyFirstModule.SaleCard (Params: ( $Sale: MyFirstModule.Sale optional )) {
  dynamictext txt (Content: 'x')
};`)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	var got []string
	for _, v := range validateSnippetParameters(prog.Statements[0]) {
		got = append(got, v.RuleID+" "+v.Message)
	}
	if !strings.Contains(strings.Join(got, "\n"), "optional") {
		t.Errorf("an optional snippet parameter is not refused: %v", got)
	}

	// A page parameter's IsRequired is 11.5+.
	page := &ast.CreatePageStmtV3{Name: ast.QualifiedName{Module: "M", Name: "P"},
		Parameters: []ast.PageParameter{{Name: "Sale", EntityType: ast.QualifiedName{Module: "M", Name: "Sale"}, Optional: true}}}
	for _, c := range []struct {
		v    [3]int
		want bool
	}{{[3]int{11, 4, 0}, true}, {[3]int{11, 5, 0}, false}} {
		prog := &ast.Program{Statements: []ast.Statement{page}}
		var hit bool
		for _, vi := range versionGateViolations(versionCtx(t, c.v[0], c.v[1], c.v[2]), prog) {
			if vi.RuleID == "MDL-VERSION01" && strings.Contains(vi.Message, "optional page parameter") {
				hit = true
			}
		}
		if hit != c.want {
			t.Errorf("Mendix %v: optional page parameter refused = %v, want %v", c.v, hit, c.want)
		}
	}
}

// `alter page … { add parameters $P: Type optional }` writes the flag too, and
// a snippet refuses it.
func TestIssue1649_AlterAddsAnOptionalParameter(t *testing.T) {
	exec, _ := pageFixtureExecutor(t)
	run(t, exec, issue1649Setup)
	run(t, exec, `create page MyFirstModule.Sale_List (Title: 'Sales', Layout: Atlas_Core.Atlas_Default) {
  dynamictext txt (Content: 'Sales')
};`)
	run(t, exec, `alter page MyFirstModule.Sale_List { add parameters $Sale: MyFirstModule.Sale optional; };`)
	id := unitIDByName(t, exec, "page", "Sale_List")
	if required, present := storedParameterRequired(t, exec, id); required || !present {
		t.Errorf("alter wrote IsRequired = %v (present %v), want false", required, present)
	}

	run(t, exec, `create snippet MyFirstModule.SaleCard { dynamictext txt (Content: 'x') };`)
	err := runErr(t, exec, `alter snippet MyFirstModule.SaleCard { add parameters $Sale: MyFirstModule.Sale optional; };`)
	if err == nil || !strings.Contains(err.Error(), "only a page parameter can be optional") {
		t.Errorf("an optional snippet parameter was added: %v", err)
	}
}
