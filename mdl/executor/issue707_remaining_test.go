// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/security"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ako/mxcli#707, the items #728 left open: a user role's description and
// check-security flag, a user role with no module roles, workflow annotations,
// REST client header expressions and image-collection file paths. Each test
// feeds describe output to the real visitor and, where the loss was on the
// write side, executes it and compares what is stored.

// --- user roles ------------------------------------------------------------

// userRoleCtx serves a project security document whose user roles the mock
// write methods really change, so describe → exec → describe can be compared.
func userRoleCtx(t *testing.T, roles ...*security.UserRole) (*ExecContext, *bytes.Buffer, *security.ProjectSecurity) {
	t.Helper()
	ps := &security.ProjectSecurity{
		BaseElement: model.BaseElement{ID: nextID("ps")},
		UserRoles:   roles,
	}
	find := func(name string) *security.UserRole {
		for _, ur := range ps.UserRoles {
			if ur.Name == name {
				return ur
			}
		}
		t.Fatalf("user role %s not found", name)
		return nil
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:        func() bool { return true },
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) { return ps, nil },
		AddUserRoleFunc: func(_ model.ID, name string, mrs []string, manageAll bool) error {
			ps.UserRoles = append(ps.UserRoles, &security.UserRole{Name: name, ModuleRoles: mrs, ManageAllRoles: manageAll})
			return nil
		},
		AlterUserRoleModuleRolesFunc: func(_ model.ID, name string, add bool, mrs []string) error {
			ur := find(name)
			for _, m := range mrs {
				found := false
				for _, have := range ur.ModuleRoles {
					found = found || have == m
				}
				if !found {
					ur.ModuleRoles = append(ur.ModuleRoles, m)
				}
			}
			return nil
		},
		SetUserRolePropertiesFunc: func(_ model.ID, name string, p backend.UserRoleProperties) error {
			ur := find(name)
			if p.Description != nil {
				ur.Description = *p.Description
			}
			if p.ManageAllRoles != nil {
				ur.ManageAllRoles = *p.ManageAllRoles
			}
			if p.ManageUsersWithoutRoles != nil {
				ur.ManageUsersWithoutRoles = *p.ManageUsersWithoutRoles
			}
			if p.CheckSecurity != nil {
				ur.CheckSecurity = *p.CheckSecurity
			}
			if p.ManageableRoles != nil {
				ur.ManageableRoles = p.ManageableRoles
			}
			return nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	return ctx, buf, ps
}

// The description and the check-security flag were printed as `--` comments
// after the statement, so a replay dropped them; the manageable roles were not
// printed at all. Replayed onto a project without the role, the output now
// builds the same role.
func TestIssue707_UserRolePropertiesSurviveReplay(t *testing.T) {
	stored := &security.UserRole{
		Name:                    "Manager",
		Description:             "Can't delete, can approve",
		ModuleRoles:             []string{"Shop.Manager", "Administration.User"},
		ManageableRoles:         []string{"Clerk", "Viewer"},
		ManageUsersWithoutRoles: true,
		CheckSecurity:           true,
	}
	src, srcBuf, _ := userRoleCtx(t, stored)
	assertNoError(t, describeUserRole(src, ast.QualifiedName{Name: "Manager"}))
	out := srcBuf.String()
	prog := reparse(t, out)
	if len(prog.Deprecations) != 0 {
		t.Errorf("describe output uses a deprecated spelling: %+v\n%s", prog.Deprecations, out)
	}
	stmt := findStmt[*ast.CreateUserRoleStmt](t, prog, out)

	// The manageable roles must exist where the replay lands: exec resolves
	// them before writing.
	dst, dstBuf, ps := userRoleCtx(t, &security.UserRole{Name: "Clerk"}, &security.UserRole{Name: "Viewer"})
	assertNoError(t, execCreateUserRole(dst, stmt))
	if len(ps.UserRoles) != 3 {
		t.Fatalf("user roles after replay = %d, want 3", len(ps.UserRoles))
	}
	got := ps.UserRoles[2]
	if !reflect.DeepEqual(got, stored) {
		t.Errorf("replay stored\n  %+v\nwant\n  %+v\n%s", got, stored, out)
	}

	dstBuf.Reset()
	assertNoError(t, describeUserRole(dst, ast.QualifiedName{Name: "Manager"}))
	if again := dstBuf.String(); again != out {
		t.Errorf("describe is not a fixed point:\n--- first ---\n%s\n--- second ---\n%s", out, again)
	}
}

// A user role with no module roles described as `create user role X;` in the
// old form's words, which did not parse: the positional list was required.
func TestIssue707_UserRoleWithoutModuleRolesReparses(t *testing.T) {
	ctx, buf, _ := userRoleCtx(t, &security.UserRole{Name: "Empty"})
	assertNoError(t, describeUserRole(ctx, ast.QualifiedName{Name: "Empty"}))
	out := buf.String()
	stmt := findStmt[*ast.CreateUserRoleStmt](t, reparse(t, out), out)
	if stmt.Name != "Empty" || len(stmt.ModuleRoles) != 0 || stmt.Description != nil || stmt.CheckSecurity != nil {
		t.Errorf("empty role re-parses as %+v\n%s", stmt, out)
	}
}

// Control: `create or modify` states a property only when the statement does,
// so a replay of the old positional form onto an existing role keeps the
// stored description rather than clearing it.
func TestIssue707_UserRoleModifyKeepsUnstatedProperties(t *testing.T) {
	ctx, _, ps := userRoleCtx(t, &security.UserRole{
		Name: "Clerk", Description: "Front desk", CheckSecurity: true, ModuleRoles: []string{"Shop.User"},
	})
	prog, errs := visitor.Build("create or modify user role Clerk (Shop.User, Shop.Viewer);")
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	assertNoError(t, execCreateUserRole(ctx, prog.Statements[0].(*ast.CreateUserRoleStmt)))
	ur := ps.UserRoles[0]
	if ur.Description != "Front desk" || !ur.CheckSecurity {
		t.Errorf("unstated properties changed: %+v", ur)
	}
	if !reflect.DeepEqual(ur.ModuleRoles, []string{"Shop.User", "Shop.Viewer"}) {
		t.Errorf("module roles = %v", ur.ModuleRoles)
	}
	if len(prog.Deprecations) != 1 || prog.Deprecations[0].Code != deprecation.UserRolePositional {
		t.Errorf("positional form recorded %+v, want %s", prog.Deprecations, deprecation.UserRolePositional)
	}
}

// --- workflow annotations --------------------------------------------------

// An activity's note, the workflow's own note and an event sub-process's note
// were printed as `-- annotation:` comments, so a replay dropped all three.
func TestIssue707_WorkflowAnnotationsSurvive(t *testing.T) {
	mod := mkModule("Sales")
	wf := mkWorkflow(mod.ID, "Approve")
	wf.Annotation = "Starts when an order is placed; it's audited"
	wf.Parameter = &workflows.WorkflowParameter{EntityRef: "Sales.Order"}
	note := &workflows.NotificationActivity{}
	note.Name = "Ready"
	note.Caption = "Ready"
	note.Annotation = "Line one\nLine two"
	wf.Flow = &workflows.Flow{Activities: []workflows.WorkflowActivity{&workflows.StartWorkflowActivity{}, note, &workflows.EndWorkflowActivity{}}}
	start := &workflows.EventSubProcessStartActivity{Interrupting: true}
	start.Name = "Cancel"
	wf.EventSubProcesses = []*workflows.EventSubProcess{{
		Name:       "OnCancel",
		Annotation: "Runs on cancel",
		Flow:       &workflows.Flow{Activities: []workflows.WorkflowActivity{start}},
	}}

	h := mkHierarchy(mod)
	withContainer(h, wf.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListWorkflowsFunc: func() ([]*workflows.Workflow, error) { return []*workflows.Workflow{wf}, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeWorkflow(ctx, ast.QualifiedName{Module: "Sales", Name: "Approve"}))

	out := buf.String()
	if strings.Contains(out, "-- annotation") {
		t.Errorf("an annotation is still a comment:\n%s", out)
	}
	stmt := findStmt[*ast.CreateWorkflowStmt](t, reparse(t, out), out)
	if stmt.Annotation != wf.Annotation {
		t.Errorf("workflow annotation: stored %q, re-exec stores %q\n%s", wf.Annotation, stmt.Annotation, out)
	}
	var got string
	for _, a := range stmt.Activities {
		if n, ok := a.(*ast.WorkflowNotificationNode); ok {
			got = n.Annotation
			// The builder carries it into the activity it writes.
			act := buildWorkflowActivity(n)
			if b, ok := act.(*workflows.NotificationActivity); !ok || b.Annotation != note.Annotation {
				t.Errorf("built activity %+v, want annotation %q", act, note.Annotation)
			}
		}
	}
	if got != note.Annotation {
		t.Errorf("activity annotation: stored %q, re-exec stores %q\n%s", note.Annotation, got, out)
	}
	if len(stmt.EventSubProcesses) != 1 || stmt.EventSubProcesses[0].Annotation != "Runs on cancel" {
		t.Errorf("event sub-process annotation lost: %+v\n%s", stmt.EventSubProcesses, out)
	}
}

// Anything but one `@annotation '…'` before a workflow activity is refused:
// MDL writes no position for one, and Mendix attaches one note.
func TestIssue707_WorkflowActivityRefusesOtherAnnotations(t *testing.T) {
	for _, src := range []string{
		"create workflow M.W parameter $WorkflowContext: M.E begin @position(1, 2) notification N; end workflow;",
		"create workflow M.W parameter $WorkflowContext: M.E begin @annotation 'a' @annotation 'b' notification N; end workflow;",
	} {
		if _, errs := visitor.Build(src); len(errs) == 0 {
			t.Errorf("accepted: %s", src)
		}
	}
}

// --- REST client header expressions ----------------------------------------

// `'Bearer ' + $Token` stored "Bearer " and dropped the token, and describe then
// printed the literal 'Bearer '. A header value is a template, `{P}` naming the
// operation parameter P, so the old spelling now builds `Bearer {Token}`.
func TestIssue707_RestHeaderExpressionKeepsTheParameter(t *testing.T) {
	const old = "create consumed rest service M.Api (BaseUrl: 'https://x', Authentication: none) " +
		"{ operation Get (Method: get, Path: '/a', Parameters: ($Token: String, $Key: String), " +
		"Headers: ('Authorization' = 'Bearer ' + $Token, 'X-Key' = $Key), Response: none) };"
	prog, errs := visitor.Build(old)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	stmt := prog.Statements[0].(*ast.CreateRestClientStmt)
	op, err := buildRestClientOperation(stmt.Operations[0])
	assertNoError(t, err)
	want := map[string]string{"Authorization": "Bearer {Token}", "X-Key": "{Key}"}
	for _, h := range op.Headers {
		if w, ok := want[h.Name]; ok && h.Value != w {
			t.Errorf("header %s stored %q, want %q", h.Name, h.Value, w)
		}
	}
	codes := 0
	for _, d := range prog.Deprecations {
		if d.Code == deprecation.RestHeaderConcat {
			codes++
		}
	}
	if codes != 2 {
		t.Errorf("recorded %d uses of %s, want 2: %+v", codes, deprecation.RestHeaderConcat, prog.Deprecations)
	}

	// Describe prints the stored template, which re-parses to itself.
	svc := &model.ConsumedRestService{Name: "Api", BaseUrl: "https://x", Operations: []*model.RestClientOperation{op}}
	ctx, buf := newMockCtx(t)
	assertNoError(t, outputConsumedRestServiceMDL(ctx, svc, "M"))
	out := buf.String()
	re := findStmt[*ast.CreateRestClientStmt](t, reparse(t, out), out)
	for _, h := range re.Operations[0].Headers {
		if w, ok := want[h.Name]; ok && h.Value != w {
			t.Errorf("describe → parse: header %s = %q, want %q\n%s", h.Name, h.Value, w, out)
		}
	}
}

// --- image collections -----------------------------------------------------

// Describe wrote each image to /tmp/mxcli-preview and printed that path, so its
// output replayed only on the machine that described it, until /tmp was
// cleared. The image is now in the statement.
func TestIssue707_ImageCollectionDescribeCarriesTheImage(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	odd := []byte("not an image signature")
	mod := mkModule("Shop")
	ic := &types.ImageCollection{
		BaseElement: model.BaseElement{ID: nextID("ic")},
		ContainerID: mod.ID,
		Name:        "Icons",
		Images: []types.Image{
			{Name: "Logo", Data: png, Format: "Png"},
			{Name: "Odd", Data: odd, Format: "Svg"},
		},
	}
	h := mkHierarchy(mod)
	withContainer(h, ic.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ListImageCollectionsFunc: func() ([]*types.ImageCollection, error) { return []*types.ImageCollection{ic}, nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeImageCollection(ctx, ast.QualifiedName{Module: "Shop", Name: "Icons"}))
	out := buf.String()
	if strings.Contains(out, "/tmp") || strings.Contains(out, "File:") {
		t.Errorf("describe names a file:\n%s", out)
	}
	stmt := findStmt[*ast.CreateImageCollectionStmt](t, reparse(t, out), out)
	if len(stmt.Images) != 2 {
		t.Fatalf("images = %d, want 2\n%s", len(stmt.Images), out)
	}

	var written *types.ImageCollection
	mb.CreateImageCollectionFunc = func(c *types.ImageCollection) error { written = c; return nil }
	mb.ListImageCollectionsFunc = func() ([]*types.ImageCollection, error) { return nil, nil }
	mb.ListModulesFunc = func() ([]*model.Module, error) { return []*model.Module{mod}, nil }
	assertNoError(t, execCreateImageCollection(ctx, stmt))
	if written == nil {
		t.Fatal("no image collection written")
	}
	for i, img := range written.Images {
		want := ic.Images[i]
		if img.Name != want.Name || !bytes.Equal(img.Data, want.Data) || img.Format != want.Format {
			t.Errorf("image %d: replay stored %s/%q/%s, want %s/%q/%s\n%s",
				i, img.Name, img.Data, img.Format, want.Name, want.Data, want.Format, out)
		}
	}
}
