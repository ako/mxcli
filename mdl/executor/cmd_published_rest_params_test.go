// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// publishedRestFixture is a module RestQ holding the microflows of
// mendixlabs/mxcli#1206's repro, with the given stored services.
type publishedRestFixture struct {
	mod     *model.Module
	stored  []*model.PublishedRestService
	created *model.PublishedRestService
	updated *model.PublishedRestService
}

func newPublishedRestFixture(t *testing.T, stored ...*model.PublishedRestService) (*publishedRestFixture, *ExecContext, *strings.Builder) {
	t.Helper()
	f := &publishedRestFixture{mod: mkModule("RestQ"), stored: stored}
	for _, s := range stored {
		s.ContainerID = f.mod.ID
	}
	param := func(name string, dt microflows.DataType) *microflows.MicroflowParameter {
		return &microflows.MicroflowParameter{Name: name, Type: dt}
	}
	mf := func(name string, params ...*microflows.MicroflowParameter) *microflows.Microflow {
		return &microflows.Microflow{
			BaseElement: model.BaseElement{ID: nextID("mf")},
			ContainerID: f.mod.ID, Name: name, Parameters: params,
		}
	}
	mfs := []*microflows.Microflow{
		mf("GetStatus",
			param("orderNumber", &microflows.StringType{}),
			param("count", &microflows.IntegerType{}),
			param("httpRequest", &microflows.ObjectType{EntityQualifiedName: "System.HttpRequest"})),
		mf("GetById",
			param("id", &microflows.IntegerType{}),
			param("verbose", &microflows.BooleanType{})),
		mf("PutFile", param("file", &microflows.ObjectType{EntityQualifiedName: "RestQ.Upload"})),
		mf("PutMany", param("items", &microflows.ListType{EntityQualifiedName: "RestQ.Item"})),
	}
	// Custom-authentication microflows, the shapes measured with mx check on
	// 11.14.0 (mendixlabs/mxcli#1331).
	user := &microflows.ObjectType{EntityQualifiedName: "System.User"}
	authMf := func(name string, ret microflows.DataType, params ...*microflows.MicroflowParameter) *microflows.Microflow {
		m := mf(name, params...)
		m.ReturnType = ret
		return m
	}
	mfs = append(mfs,
		authMf("Authenticate", user, param("HttpRequest", &microflows.ObjectType{EntityQualifiedName: "System.HttpRequest"})),
		authMf("AuthenticateBoth", user,
			param("Req", &microflows.ObjectType{EntityQualifiedName: "System.HttpRequest"}),
			param("Resp", &microflows.ObjectType{EntityQualifiedName: "System.HttpResponse"})),
		authMf("AuthenticateNoParams", user),
		authMf("AuthenticateBool", &microflows.BooleanType{}, param("HttpRequest", &microflows.ObjectType{EntityQualifiedName: "System.HttpRequest"})),
		authMf("AuthenticateToken", user, param("Token", &microflows.StringType{})),
	)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{f.mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return mfs, nil
		},
		ListPublishedRestServicesFunc: func() ([]*model.PublishedRestService, error) { return f.stored, nil },
		CreatePublishedRestServiceFunc: func(svc *model.PublishedRestService) error {
			f.created = svc
			return nil
		},
		UpdatePublishedRestServiceFunc: func(svc *model.PublishedRestService) error {
			f.updated = svc
			return nil
		},
	}
	h := mkHierarchy(f.mod)
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	out := &strings.Builder{}
	ctx.Output = out
	return f, ctx, out
}

func execPublishedRest(t *testing.T, ctx *ExecContext, src string) error {
	t.Helper()
	prog := parseMDL(t, src)
	for _, stmt := range prog.Statements {
		var err error
		switch s := stmt.(type) {
		case *ast.CreatePublishedRestServiceStmt:
			err = execCreatePublishedRestService(ctx, s)
		case *ast.AlterPublishedRestServiceStmt:
			err = execAlterPublishedRestService(ctx, s)
		default:
			t.Fatalf("unexpected statement %T", stmt)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func paramsString(op *model.PublishedRestOperation) string {
	var parts []string
	for _, p := range op.OperationParameters {
		s := p.ParameterType + " " + p.Name + ":" + p.DataType
		if p.QualifiedName != "" {
			s += "(" + p.QualifiedName + ")"
		}
		s += "->" + p.MicroflowParameter
		parts = append(parts, s)
	}
	return strings.Join(parts, "; ")
}

// TestCreatePublishedRestService_DerivesParameters is mendixlabs/mxcli#1206:
// only the path's placeholders were written, each a String, so every query and
// body parameter failed mx check with CE0350 and an Integer {id} with CE6539.
func TestCreatePublishedRestService_DerivesParameters(t *testing.T) {
	f, ctx, _ := newPublishedRestFixture(t)
	assertNoError(t, execPublishedRest(t, ctx, `create published rest service RestQ.Orders (Path: 'rest/orders/v1') {
  resource 'orders' {
    get 'status' microflow RestQ.GetStatus;
    get 'items/{id}' microflow RestQ.GetById;
    post 'upload' microflow RestQ.PutFile;
    put 'many' microflow RestQ.PutMany;
  }
};`))
	if f.created == nil {
		t.Fatal("nothing created")
	}
	ops := f.created.Resources[0].Operations
	want := []string{
		"Query orderNumber:String->RestQ.GetStatus.orderNumber; Query count:Integer->RestQ.GetStatus.count",
		"Path id:Integer->RestQ.GetById.id; Query verbose:Boolean->RestQ.GetById.verbose",
		"Body file:Object(RestQ.Upload)->RestQ.PutFile.file",
		"Body items:List(RestQ.Item)->RestQ.PutMany.items",
	}
	for i, w := range want {
		if got := paramsString(ops[i]); got != w {
			t.Errorf("operation %d parameters:\n got %s\nwant %s", i, got, w)
		}
	}
}

// TestCreatePublishedRestService_WritesMappingBindings is ako/mxcli#571: the
// import mapping, export mapping and commit clauses parsed and were thrown away.
func TestCreatePublishedRestService_WritesMappingBindings(t *testing.T) {
	f, ctx, _ := newPublishedRestFixture(t)
	assertNoError(t, execPublishedRest(t, ctx, `create published rest service RestQ.Orders (Path: 'rest/orders/v1') {
  resource 'orders' {
    post 'upload' microflow RestQ.PutFile import mapping RestQ.IMM_Upload export mapping "RestQ"."EMM_Result" commit yeswithoutevents;
    get 'status' microflow RestQ.GetStatus;
  }
};`))
	op := f.created.Resources[0].Operations[0]
	if op.ImportMapping != "RestQ.IMM_Upload" || op.ExportMapping != "RestQ.EMM_Result" || op.Commit != "YesWithoutEvents" {
		t.Errorf("bindings = import %q export %q commit %q, want RestQ.IMM_Upload, RestQ.EMM_Result, YesWithoutEvents",
			op.ImportMapping, op.ExportMapping, op.Commit)
	}
	if other := f.created.Resources[0].Operations[1]; other.ImportMapping != "" || other.ExportMapping != "" || other.Commit != "" {
		t.Errorf("an operation without clauses got bindings: %+v", other)
	}
}

// TestCreatePublishedRestService_RefusesUnknownCommit: `commit Maybe` parsed
// and was thrown away; there is nothing correct to write, so it is refused by
// exec and by check (MDL-REST03).
func TestCreatePublishedRestService_RefusesUnknownCommit(t *testing.T) {
	src := `create published rest service RestQ.Orders (Path: 'rest/orders/v1') {
  resource 'orders' { post 'upload' microflow RestQ.PutFile import mapping RestQ.IMM commit Maybe; }
};`
	f, ctx, _ := newPublishedRestFixture(t)
	err := execPublishedRest(t, ctx, src)
	if err == nil || !strings.Contains(err.Error(), "commit Maybe") {
		t.Fatalf("exec error = %v, want a refusal naming commit Maybe", err)
	}
	if f.created != nil {
		t.Error("the service was written despite the refusal")
	}
	v := ValidatePublishedRestCommit(parseMDL(t, src))
	if len(v) != 1 || v[0].RuleID != "MDL-REST03" {
		t.Errorf("check = %+v, want one MDL-REST03", v)
	}
	if v := ValidatePublishedRestCommit(parseMDL(t, strings.Replace(src, "Maybe", "No", 1))); len(v) != 0 {
		t.Errorf("control: commit No flagged: %+v", v)
	}
}

// TestCreateOrModifyPublishedRestService_KeepsStoredParameters: a parameter
// Studio Pro lets the user rename, describe or turn into a header has no MDL
// spelling; create or modify of the same operation keeps it, as it keeps the
// summary, documentation and object handling, while the type follows the
// microflow and a new microflow parameter is derived.
func TestCreateOrModifyPublishedRestService_KeepsStoredParameters(t *testing.T) {
	stored := &model.PublishedRestService{
		BaseElement: model.BaseElement{ID: nextID("prs")},
		Name:        "Orders",
		Path:        "rest/orders/v1",
		Resources: []*model.PublishedRestResource{{
			Name: "orders",
			Operations: []*model.PublishedRestOperation{{
				HTTPMethod: "Get", Path: "status", Microflow: "RestQ.GetStatus",
				Summary: "Order status", Documentation: "docs", ObjectHandlingBackup: "Error", Commit: "No",
				OperationParameters: []*model.PublishedRestOperationParameter{
					{Name: "X-Count", ParameterType: "Header", MicroflowParameter: "RestQ.GetStatus.count", DataType: "String", Description: "how many"},
				},
			}},
		}},
	}
	f, ctx, out := newPublishedRestFixture(t, stored)
	assertNoError(t, execPublishedRest(t, ctx, `create or modify published rest service RestQ.Orders (Path: 'rest/orders/v1') {
  resource 'orders' { get 'status' microflow RestQ.GetStatus commit No; }
};`))
	if f.updated == nil {
		t.Fatalf("nothing updated; output:\n%s", out)
	}
	op := f.updated.Resources[0].Operations[0]
	if got, want := paramsString(op), "Header X-Count:Integer->RestQ.GetStatus.count; Query orderNumber:String->RestQ.GetStatus.orderNumber"; got != want {
		t.Errorf("parameters:\n got %s\nwant %s", got, want)
	}
	if op.OperationParameters[0].Description != "how many" {
		t.Errorf("description lost: %+v", op.OperationParameters[0])
	}
	if op.Summary != "Order status" || op.Documentation != "docs" || op.ObjectHandlingBackup != "Error" {
		t.Errorf("unstated operation properties not carried: %+v", op)
	}
}

// TestAlterPublishedRestService_DerivesAddedOperations: ALTER writes every
// operation again, so an added resource derives its parameters and keeps its
// bindings, and the untouched operations keep theirs.
func TestAlterPublishedRestService_DerivesAddedOperations(t *testing.T) {
	stored := &model.PublishedRestService{
		BaseElement: model.BaseElement{ID: nextID("prs")},
		Name:        "Orders",
		Path:        "rest/orders/v1",
		Resources: []*model.PublishedRestResource{{
			Name: "orders",
			Operations: []*model.PublishedRestOperation{{
				HTTPMethod: "Post", Path: "upload", Microflow: "RestQ.PutFile",
				ImportMapping: "RestQ.IMM_Upload", Commit: "No",
				OperationParameters: []*model.PublishedRestOperationParameter{
					{Name: "file", ParameterType: "Body", MicroflowParameter: "RestQ.PutFile.file", DataType: "Object", QualifiedName: "RestQ.Upload"},
				},
			}},
		}},
	}
	f, ctx, _ := newPublishedRestFixture(t, stored)
	assertNoError(t, execPublishedRest(t, ctx, `alter published rest service RestQ.Orders
  add resource 'items' { get '{id}' microflow RestQ.GetById export mapping RestQ.EMM_Item; };`))
	if f.updated == nil {
		t.Fatal("nothing updated")
	}
	kept := f.updated.Resources[0].Operations[0]
	if kept.ImportMapping != "RestQ.IMM_Upload" || kept.Commit != "No" || paramsString(kept) != "Body file:Object(RestQ.Upload)->RestQ.PutFile.file" {
		t.Errorf("untouched operation changed: %+v %s", kept, paramsString(kept))
	}
	added := f.updated.Resources[1].Operations[0]
	if added.ExportMapping != "RestQ.EMM_Item" {
		t.Errorf("added operation export mapping = %q", added.ExportMapping)
	}
	if got, want := paramsString(added), "Path id:Integer->RestQ.GetById.id; Query verbose:Boolean->RestQ.GetById.verbose"; got != want {
		t.Errorf("added operation parameters:\n got %s\nwant %s", got, want)
	}
}

// TestCreatePublishedRestService_MissingMicroflowWarns: without the microflow
// only the path is known; the operation keeps today's path-only parameters and
// exec says what mx check will report.
func TestCreatePublishedRestService_MissingMicroflowWarns(t *testing.T) {
	f, ctx, out := newPublishedRestFixture(t)
	assertNoError(t, execPublishedRest(t, ctx, `create published rest service RestQ.Orders (Path: 'rest/orders/v1') {
  resource 'orders' { get '{id}' microflow RestQ.NotYet; }
};`))
	if len(f.created.Resources[0].Operations[0].OperationParameters) != 0 {
		t.Errorf("parameters derived for a missing microflow: %s", paramsString(f.created.Resources[0].Operations[0]))
	}
	if !strings.Contains(out.String(), "microflow RestQ.NotYet not found") {
		t.Errorf("no warning; output:\n%s", out)
	}
}

// TestDescribePublishedRestService_PrintsBindings: describe prints the
// bindings exec now writes, so executing its output restates them, and names
// the parameters MDL cannot state.
func TestDescribePublishedRestService_PrintsBindings(t *testing.T) {
	stored := &model.PublishedRestService{
		BaseElement: model.BaseElement{ID: nextID("prs")},
		Name:        "Orders",
		Path:        "rest/orders/v1",
		Resources: []*model.PublishedRestResource{{
			Name: "orders",
			Operations: []*model.PublishedRestOperation{
				{
					HTTPMethod: "Post", Path: "upload", Microflow: "RestQ.PutFile",
					ImportMapping: "RestQ.IMM_Upload", ExportMapping: "RestQ.EMM_Result", Commit: "YesWithoutEvents",
				},
				{
					HTTPMethod: "Get", Path: "status", Microflow: "RestQ.GetStatus", Commit: "Yes",
					OperationParameters: []*model.PublishedRestOperationParameter{
						{Name: "X-Count", ParameterType: "Header", MicroflowParameter: "RestQ.GetStatus.count", DataType: "Integer"},
						{Name: "orderNumber", ParameterType: "Query", MicroflowParameter: "RestQ.GetStatus.orderNumber", DataType: "String"},
					},
				},
			},
		}},
	}
	_, ctx, out := newPublishedRestFixture(t, stored)
	assertNoError(t, describePublishedRestService(ctx, ast.QualifiedName{Module: "RestQ", Name: "Orders"}))
	got := out.String()
	assertContainsStr(t, got, "post 'upload' microflow RestQ.PutFile import mapping RestQ.IMM_Upload export mapping RestQ.EMM_Result commit YesWithoutEvents;")
	assertContainsStr(t, got, "get 'status' microflow RestQ.GetStatus;")
	assertContainsStr(t, got, "-- parameter X-Count: header parameter, bound to $count")
	if strings.Contains(got, "parameter orderNumber") {
		t.Errorf("a derivable parameter was flagged:\n%s", got)
	}
	// The output re-parses, and its bindings parse back to what is stored.
	prog := parseMDL(t, got)
	op := prog.Statements[0].(*ast.CreatePublishedRestServiceStmt).Resources[0].Operations[0]
	if op.ImportMapping != "RestQ.IMM_Upload" || op.ExportMapping != "RestQ.EMM_Result" || op.Commit != "YesWithoutEvents" {
		t.Errorf("describe output parses back to %+v", op)
	}
}
