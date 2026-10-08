// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// authService parses a service with the given authentication clause.
func authService(t *testing.T, clause string) *ast.CreateODataServiceStmt {
	t.Helper()
	script := `create odata service T.Api (
  path: 'odata/t/', version: '1.0.0', ODataVersion: OData4,
  namespace: 'T.Api', ServiceName: 'Api'
)
` + clause + `
{
  publish entity T.Row as 'Rows' (
    ReadMode: source, InsertMode: not_supported,
    UpdateMode: not_supported, DeleteMode: not_supported
  )
  expose ( K as 'k' (KEY) );
};`
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	svc, ok := prog.Statements[0].(*ast.CreateODataServiceStmt)
	if !ok {
		t.Fatalf("expected CreateODataServiceStmt, got %T", prog.Statements[0])
	}
	return svc
}

// mxcli-formula1 §40: the grammar always accepted the qualified name; the
// visitor discarded it. So `authentication microflow M.Auth` parsed, checked and
// executed while the model got a Microflow method with no microflow — and
// Mendix refused to build it (CE0333). Custom authentication is the fix for the
// per-request BCrypt cost, so this gap kept a real optimisation out of reach of
// a project whose model is written as MDL.
func TestODataAuth_CapturesTheMicroflowName(t *testing.T) {
	svc := authService(t, "authentication microflow T.Authenticate")
	if svc.AuthMicroflow != "T.Authenticate" {
		t.Errorf("AuthMicroflow = %q, want T.Authenticate — the name was discarded", svc.AuthMicroflow)
	}
	if len(svc.AuthenticationTypes) != 1 || svc.AuthenticationTypes[0] != "Microflow" {
		t.Errorf("AuthenticationTypes = %v, want [Microflow]", svc.AuthenticationTypes)
	}
}

// The method list is comma-separated, so the microflow must attach to its own
// entry and not swallow the neighbours.
func TestODataAuth_MicroflowAlongsideOtherMethods(t *testing.T) {
	svc := authService(t, "authentication basic, microflow T.Authenticate, guest")
	if svc.AuthMicroflow != "T.Authenticate" {
		t.Errorf("AuthMicroflow = %q", svc.AuthMicroflow)
	}
	want := []string{"Basic", "Microflow", "Guest"}
	if strings.Join(svc.AuthenticationTypes, ",") != strings.Join(want, ",") {
		t.Errorf("AuthenticationTypes = %v, want %v", svc.AuthenticationTypes, want)
	}
}

// Methods that carry no target must stay unaffected.
func TestODataAuth_NoMicroflowForOtherMethods(t *testing.T) {
	svc := authService(t, "authentication basic, session")
	if svc.AuthMicroflow != "" {
		t.Errorf("AuthMicroflow = %q, want empty", svc.AuthMicroflow)
	}
}

// The name is optional in the grammar, so the incomplete form still parses —
// and is exactly what produced CE0333. Check has to catch it.
func TestODataAuth_FlagsMicroflowWithNoName(t *testing.T) {
	svc := authService(t, "authentication microflow")
	if svc.AuthMicroflow != "" {
		t.Fatalf("AuthMicroflow = %q, want empty for this fixture", svc.AuthMicroflow)
	}
	prog := &ast.Program{Statements: []ast.Statement{svc}}
	vs := ValidateODataAuth(prog)
	if len(vs) != 1 || vs[0].RuleID != "MDL-ODATA04" {
		t.Fatalf("expected one MDL-ODATA04, got %+v", vs)
	}
	if !strings.Contains(vs[0].Suggestion, "CE0333") {
		t.Errorf("the suggestion should name the build error it prevents: %q", vs[0].Suggestion)
	}
}

// A named microflow must not be flagged — otherwise the rule punishes the fix
// it recommends.
func TestODataAuth_SilentWhenTheMicroflowIsNamed(t *testing.T) {
	svc := authService(t, "authentication microflow T.Authenticate")
	if vs := ValidateODataAuth(&ast.Program{Statements: []ast.Statement{svc}}); len(vs) != 0 {
		t.Errorf("a named microflow must not be flagged: %+v", vs)
	}
}

// Nor must a service that never asked for custom authentication.
func TestODataAuth_SilentForOtherMethods(t *testing.T) {
	svc := authService(t, "authentication basic, session")
	if vs := ValidateODataAuth(&ast.Program{Statements: []ast.Statement{svc}}); len(vs) != 0 {
		t.Errorf("basic/session must not be flagged: %+v", vs)
	}
}

// DESCRIBE emits re-executable MDL. Emitting the microflow as a comment
// (`-- Auth Microflow: …`) made the output look complete while replaying into a
// service Mendix rejects — the same shape as §39.
func TestDescribeODataService_RoundTripsTheAuthMicroflow(t *testing.T) {
	got, _, ok := odataAuthenticationProperty(&model.PublishedODataService{
		AuthenticationTypes: []string{"Microflow"},
		AuthMicroflow:       "T.Authenticate",
	})
	if want := "Authentication: (microflow T.Authenticate)"; !ok || got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDescribeODataService_AuthPropertyKeepsOtherMethods(t *testing.T) {
	got, _, _ := odataAuthenticationProperty(&model.PublishedODataService{
		AuthenticationTypes: []string{"Basic", "Microflow"},
		AuthMicroflow:       "T.Authenticate",
	})
	if got != "Authentication: (basic, microflow T.Authenticate)" {
		t.Errorf("got %q", got)
	}
}

// A stored microflow with no matching method is not lost: it is named in a
// comment and the property is left out, so replaying the output keeps it. It
// used to be printed as `Microflow T.Authenticate`, which added the method.
func TestDescribeODataService_AuthMicroflowWithoutAMethodIsKept(t *testing.T) {
	prop, note, ok := odataAuthenticationProperty(&model.PublishedODataService{
		AuthenticationTypes: []string{"Basic"},
		AuthMicroflow:       "T.Authenticate",
	})
	if ok || prop != "" || !strings.Contains(note, "T.Authenticate") {
		t.Errorf("got prop %q note %q ok %v", prop, note, ok)
	}
}

// The whole point: what DESCRIBE prints must parse back to the same service.
func TestDescribeODataService_AuthPropertyReParses(t *testing.T) {
	prop, _, _ := odataAuthenticationProperty(&model.PublishedODataService{
		AuthenticationTypes: []string{"Basic", "Microflow"},
		AuthMicroflow:       "T.Authenticate",
	})
	svc := authServiceWithProperty(t, prop)
	if svc.AuthMicroflow != "T.Authenticate" {
		t.Errorf("round trip lost the microflow: %q", svc.AuthMicroflow)
	}
	if strings.Join(svc.AuthenticationTypes, ",") != "Basic,Microflow" {
		t.Errorf("round trip changed the methods: %v", svc.AuthenticationTypes)
	}
}

// authServiceWithProperty parses a service whose list ends with prop.
func authServiceWithProperty(t *testing.T, prop string) *ast.CreateODataServiceStmt {
	t.Helper()
	prog, errs := visitor.Build("create odata service T.Api (\n  path: 'odata/t/', version: '1.0.0', ODataVersion: OData4,\n  namespace: 'T.Api', ServiceName: 'Api',\n  " + prop + "\n);")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	return prog.Statements[0].(*ast.CreateODataServiceStmt)
}

// describeAction renders one published microflow through the DESCRIBE emitter.
func describeAction(pm *model.PublishedMicroflow) string {
	var b bytes.Buffer
	printPublishedMicroflowMDL(&b, pm)
	return b.String()
}

// mxcli-formula1 §47.1: DESCRIBE emits re-executable MDL, so an OData action it
// read must come back out. Leaving it off — or emitting it as a comment — is the
// same defect class as §39 and §40: output that looks complete and replays into
// a lesser model.
func TestDescribeODataService_EmitsAnAction(t *testing.T) {
	got := describeAction(&model.PublishedMicroflow{
		Microflow: "T.RecordPrediction", ExposedName: "RecordPrediction",
		Parameters: []*model.PublishedMicroflowParameter{
			{MicroflowParameter: "T.RecordPrediction.DriverId", ExposedName: "driverId"},
			{MicroflowParameter: "T.RecordPrediction.Points", ExposedName: "points"},
		},
	})
	want := "  publish microflow T.RecordPrediction as 'RecordPrediction'\n" +
		"    expose ( DriverId as 'driverId', Points as 'points' );\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A parameter whose exposed name equals its own name needs no alias — emitting
// one would be noise that the round trip has to carry forever.
func TestDescribeODataService_ActionOmitsARedundantAlias(t *testing.T) {
	got := describeAction(&model.PublishedMicroflow{
		Microflow: "T.DoThing", ExposedName: "DoThing",
		Parameters: []*model.PublishedMicroflowParameter{
			{MicroflowParameter: "T.DoThing.Note", ExposedName: "Note", CanBeEmpty: true},
		},
	})
	want := "  publish microflow T.DoThing as 'DoThing'\n    expose ( Note (CanBeEmpty) );\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// An action with no parameters is a one-liner.
func TestDescribeODataService_ActionWithNoParameters(t *testing.T) {
	got := describeAction(&model.PublishedMicroflow{
		Microflow: "T.Ping", ExposedName: "Ping",
	})
	if got != "  publish microflow T.Ping as 'Ping';\n" {
		t.Errorf("got %q", got)
	}
}

// The invariant that matters: what DESCRIBE prints must parse back to the same
// action. Asserted against the parser rather than against a hand-written
// expectation about the text.
func TestDescribeODataService_ActionReParses(t *testing.T) {
	for _, pm := range []*model.PublishedMicroflow{
		{Microflow: "T.RecordPrediction", ExposedName: "RecordPrediction",
			Parameters: []*model.PublishedMicroflowParameter{
				{MicroflowParameter: "T.RecordPrediction.DriverId", ExposedName: "driverId"},
				{MicroflowParameter: "T.RecordPrediction.Points", ExposedName: "points"},
			}},
		{Microflow: "T.DoThing", ExposedName: "DoThing",
			Parameters: []*model.PublishedMicroflowParameter{
				{MicroflowParameter: "T.DoThing.Note", ExposedName: "Note", CanBeEmpty: true},
			}},
		{Microflow: "T.Ping", ExposedName: "Ping"},
	} {
		t.Run(pm.Microflow, func(t *testing.T) {
			script := `create odata service T.Api (
  path: 'odata/t/', version: '1.0.0', ODataVersion: OData4,
  namespace: 'T.Api', ServiceName: 'Api'
)
{
` + describeAction(pm) + `};`
			prog, errs := visitor.Build(script)
			if len(errs) > 0 {
				t.Fatalf("DESCRIBE emitted output its own parser rejects:\n%s\nerrors: %v", script, errs)
			}
			svc := prog.Statements[0].(*ast.CreateODataServiceStmt)
			if len(svc.Microflows) != 1 {
				t.Fatalf("re-parsed to %d actions", len(svc.Microflows))
			}
			got := svc.Microflows[0]
			if got.Microflow.String() != pm.Microflow || got.ExposedName != pm.ExposedName {
				t.Errorf("round trip changed the action: %+v", got)
			}
			if len(got.Parameters) != len(pm.Parameters) {
				t.Fatalf("round trip changed the parameter count: %d -> %d",
					len(pm.Parameters), len(got.Parameters))
			}
			for i, want := range pm.Parameters {
				gotName := got.Parameters[i].ExposedName
				if gotName == "" {
					gotName = got.Parameters[i].Name
				}
				if gotName != want.ExposedName {
					t.Errorf("param %d exposed name %q -> %q", i, want.ExposedName, gotName)
				}
				if got.Parameters[i].CanBeEmpty != want.CanBeEmpty {
					t.Errorf("param %d CanBeEmpty %v -> %v", i, want.CanBeEmpty, got.Parameters[i].CanBeEmpty)
				}
			}
		})
	}
}

// The tests above call printPublishedMicroflowMDL directly, which does not
// prove the emitter is WIRED into DESCRIBE. It was not caught by them: reverting
// the loop in outputPublishedODataServiceMDL left every one of them green. This
// goes through the real entry point.
func TestOutputPublishedODataServiceMDL_IncludesActions(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}
	svc := &model.PublishedODataService{
		Name: "Api", Path: "odata/t/", Version: "1.0.0",
		ODataVersion: "OData4", Namespace: "T.Api", ServiceName: "Api",
		Microflows: []*model.PublishedMicroflow{{
			Microflow: "T.RecordPrediction", ExposedName: "RecordPrediction",
			Parameters: []*model.PublishedMicroflowParameter{
				{MicroflowParameter: "T.RecordPrediction.DriverId", ExposedName: "driverId"},
			},
		}},
	}
	if err := outputPublishedODataServiceMDL(ctx, svc, "T", ""); err != nil {
		t.Fatalf("output: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "publish microflow T.RecordPrediction as 'RecordPrediction'") {
		t.Errorf("DESCRIBE dropped the action:\n%s", got)
	}
	if !strings.Contains(got, "expose ( DriverId as 'driverId' );") {
		t.Errorf("DESCRIBE dropped the action's parameters:\n%s", got)
	}
}

// A service whose ONLY published thing is an action must still emit a body —
// the block guard used to key off entity types and entity sets alone.
func TestOutputPublishedODataServiceMDL_ActionOnlyServiceStillEmitsABody(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}
	svc := &model.PublishedODataService{
		Name: "Api", Path: "odata/t/", Version: "1.0.0",
		ODataVersion: "OData4", Namespace: "T.Api", ServiceName: "Api",
		Microflows: []*model.PublishedMicroflow{{Microflow: "T.Ping", ExposedName: "Ping"}},
	}
	if err := outputPublishedODataServiceMDL(ctx, svc, "T", ""); err != nil {
		t.Fatalf("output: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "publish microflow T.Ping") {
		t.Errorf("an action-only service emitted no body:\n%s", got)
	}
}

// The property and `alter … set (Authentication: …)` are held to MDL-ODATA04
// as the clause was.
func TestODataAuth_FlagsThePropertyAndAlter(t *testing.T) {
	for _, src := range []string{
		"create odata service T.Api (path: 'odata/t/', namespace: 'T.Api', Authentication: (basic, microflow));",
		"alter published odata service T.Api set (Authentication: (microflow));",
	} {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parse %q: %v", src, errs)
		}
		if vs := ValidateODataAuth(prog); len(vs) != 1 || vs[0].RuleID != "MDL-ODATA04" {
			t.Errorf("%q: violations %+v, want one MDL-ODATA04", src, vs)
		}
	}
}
