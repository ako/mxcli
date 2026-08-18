// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// upstream #922. A REST call storing its response in a file document had no MDL
// form at all: the grammar offered only String / response / mapping / none, the
// model had no FileDocument variant, and BOTH readers fell back to "String".
//
// Measured on mxbuild 11.6.6 before the fix, on a project whose baseline is one
// error: a describe → exec round trip rewrote the stored ResultHandlingType from
// FileDocument to String and the VariableType from ObjectType(MyModule.MyFile)
// to StringType — and mx check still reported only the baseline, so nothing
// downstream could notice the activity had been retyped.

func parseRestResult(t *testing.T, returns string) ast.RestResult {
	t.Helper()
	src := `create microflow Synthetic.MF_Rest (Location: String)
begin
  $out = rest call get '{1}' with ({1} = $Location)
    timeout 300
    returns ` + returns + `;
end;`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse error for `returns %s`: %v", returns, errs[0])
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	for _, s := range mf.Body {
		if rc, ok := s.(*ast.RestCallStmt); ok {
			return rc.Result
		}
	}
	t.Fatalf("no rest call statement parsed from `returns %s`", returns)
	return ast.RestResult{}
}

func TestRestReturnsFileDocumentParses(t *testing.T) {
	got := parseRestResult(t, "MyModule.MyFile")
	if got.Type != ast.RestResultFileDocument {
		t.Fatalf("Type = %v, want RestResultFileDocument", got.Type)
	}
	if got.ResultEntity.String() != "MyModule.MyFile" {
		t.Errorf("ResultEntity = %q, want MyModule.MyFile", got.ResultEntity.String())
	}
}

// The new alternative sits last in restCallReturnsClause, so it must not shadow
// any keyword form. Each of these is its own token, but a grammar change that
// reordered the alternatives would silently reclassify them — and `String`
// becoming a "file document named String" is exactly the class of bug #922 was.
func TestRestReturnsKeywordFormsStillWin(t *testing.T) {
	cases := map[string]ast.RestResultType{
		"String":   ast.RestResultString,
		"response": ast.RestResultResponse,
		"none":     ast.RestResultNone,
		"nothing":  ast.RestResultNone,
	}
	for returns, want := range cases {
		t.Run(returns, func(t *testing.T) {
			if got := parseRestResult(t, returns).Type; got != want {
				t.Errorf("`returns %s` parsed as %v, want %v", returns, got, want)
			}
		})
	}
}

// The mapping form takes qualified names too, so it is the alternative most at
// risk from the new one.
func TestRestReturnsMappingUnaffected(t *testing.T) {
	got := parseRestResult(t, "mapping MyModule.IMM_X as list of MyModule.Item")
	if got.Type != ast.RestResultMapping {
		t.Fatalf("Type = %v, want RestResultMapping", got.Type)
	}
	if !got.IsList {
		t.Error("IsList = false, want true for `as list of`")
	}
	if got.MappingName.String() != "MyModule.IMM_X" || got.ResultEntity.String() != "MyModule.Item" {
		t.Errorf("mapping=%q entity=%q, want MyModule.IMM_X / MyModule.Item",
			got.MappingName.String(), got.ResultEntity.String())
	}
}

// The describer is the half the issue was filed about: it must render the entity
// rather than the word String.
func TestFormatRestCallRendersFileDocumentEntity(t *testing.T) {
	a := &microflows.RestCallAction{
		OutputVariable: "fileResponseGet",
		HttpConfiguration: &microflows.HttpConfiguration{
			HttpMethod:       microflows.HttpMethodGet,
			LocationTemplate: "{1}",
			LocationParams:   []string{"$Location"},
		},
		ResultHandling: &microflows.ResultHandlingFileDocument{
			VariableName: "fileResponseGet",
			EntityRef:    "MyModule.MyFile",
		},
	}
	out := formatRestCallAction(nil, a)
	if !strings.Contains(out, "returns MyModule.MyFile") {
		t.Errorf("describe output does not name the file document type:\n%s", out)
	}
	if strings.Contains(out, "returns String") {
		t.Errorf("describe output still claims String — this is the #922 symptom:\n%s", out)
	}
	if !strings.Contains(out, "$fileResponseGet = ") {
		t.Errorf("describe output lost the output variable, which the legacy reader also used to drop:\n%s", out)
	}
}

// A result handling the reader could not reconstruct must produce something the
// parser REJECTS, not a plausible-looking `returns String`. Rendering an unknown
// handling as String is what let a FileDocument result round-trip into a String
// one with a green build; a describe that fails is recoverable, a describe that
// lies is not.
func TestFormatRestCallRefusesUnknownResultHandling(t *testing.T) {
	for name, a := range map[string]*microflows.RestCallAction{
		"nil handling": {
			OutputVariable:    "x",
			HttpConfiguration: &microflows.HttpConfiguration{HttpMethod: microflows.HttpMethodGet},
		},
	} {
		t.Run(name, func(t *testing.T) {
			out := formatRestCallAction(nil, a)
			if strings.Contains(out, "returns String;") {
				t.Fatalf("an unreadable result handling still renders as a valid `returns String`:\n%s", out)
			}
			if !strings.Contains(out, "unsupported result handling") {
				t.Errorf("output does not say what went wrong:\n%s", out)
			}
			// It must not parse: that is the whole point.
			if _, errs := visitor.Build("create microflow M.F () begin " + out + " end;"); len(errs) == 0 {
				t.Errorf("the refusal text parses as valid MDL, so it can still round-trip:\n%s", out)
			}
		})
	}
}

// MDL064: the base type is rejected by Mendix itself (CE0362), and an
// unqualified name is a typo waiting to happen.
func TestMDL064_FileDocumentResultType(t *testing.T) {
	cases := []struct {
		name    string
		returns string
		wantMsg string
	}{
		{"base System.FileDocument is refused", "System.FileDocument", "CE0362"},
		{"unqualified name is refused", "MyFile", "module prefix"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := `create microflow Synthetic.MF_Rest (Location: String)
begin
  $out = rest call get '{1}' with ({1} = $Location)
    returns ` + tc.returns + `;
end;`
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse error: %v", errs[0])
			}
			vs := ValidateMicroflow(prog.Statements[0].(*ast.CreateMicroflowStmt))
			var found bool
			for _, v := range vs {
				if v.RuleID == "MDL064" {
					found = true
					if !strings.Contains(v.Message, tc.wantMsg) {
						t.Errorf("MDL064 message = %q, want it to mention %q", v.Message, tc.wantMsg)
					}
				}
			}
			if !found {
				t.Fatalf("expected MDL064 for `returns %s`, got %#v", tc.returns, vs)
			}
		})
	}
}

// The control: a real specialization must pass, or the rule makes the feature
// unusable. Verified against mxbuild — this exact shape builds at baseline.
func TestMDL064_SpecializationIsClean(t *testing.T) {
	src := `create microflow Synthetic.MF_Rest (Location: String)
begin
  $out = rest call get '{1}' with ({1} = $Location)
    returns MyModule.MyFile;
end;`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	for _, v := range ValidateMicroflow(prog.Statements[0].(*ast.CreateMicroflowStmt)) {
		if v.RuleID == "MDL064" {
			t.Errorf("MDL064 fired on a valid FileDocument specialization: %+v", v)
		}
	}
}
