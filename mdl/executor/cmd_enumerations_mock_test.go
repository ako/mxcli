// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

func TestShowEnumerations_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	enum := mkEnumeration(mod.ID, "Color", "Red", "Green", "Blue")

	h := mkHierarchy(mod)
	withContainer(h, enum.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listEnumerations(ctx, ""))

	out := buf.String()
	assertContainsStr(t, out, "MyModule.Color")
	assertContainsStr(t, out, "| 3")
	assertContainsStr(t, out, "(1 enumerations)")
}

func TestShowEnumerations_Mock_FilterByModule(t *testing.T) {
	mod1 := mkModule("Alpha")
	mod2 := mkModule("Beta")
	e1 := mkEnumeration(mod1.ID, "Color", "Red")
	e2 := mkEnumeration(mod2.ID, "Size", "S", "M")

	h := mkHierarchy(mod1, mod2)
	withContainer(h, e1.ContainerID, mod1.ID)
	withContainer(h, e2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{e1, e2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, listEnumerations(ctx, "Beta"))

	out := buf.String()
	assertNotContainsStr(t, out, "Alpha.Color")
	assertContainsStr(t, out, "Beta.Size")
	assertContainsStr(t, out, "(1 enumerations)")
}

func TestDescribeEnumeration_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	enum := &model.Enumeration{
		BaseElement: model.BaseElement{ID: nextID("enum")},
		ContainerID: mod.ID,
		Name:        "Status",
		Values: []model.EnumerationValue{
			{BaseElement: model.BaseElement{ID: nextID("ev")}, Name: "Active", Caption: &model.Text{Translations: map[string]string{"en_US": "Active"}}},
			{BaseElement: model.BaseElement{ID: nextID("ev")}, Name: "Inactive", Caption: &model.Text{Translations: map[string]string{"en_US": "Inactive"}}},
		},
	}

	h := mkHierarchy(mod)
	withContainer(h, enum.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, describeEnumeration(ctx, ast.QualifiedName{Module: "MyModule", Name: "Status"}))

	out := buf.String()
	assertContainsStr(t, out, "create or modify enumeration MyModule.Status")
	assertContainsStr(t, out, "Active")
	assertContainsStr(t, out, "Inactive")
}

func TestDescribeEnumeration_Mock_NotFound(t *testing.T) {
	mod := mkModule("MyModule")
	h := mkHierarchy(mod)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return nil, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := describeEnumeration(ctx, ast.QualifiedName{Module: "MyModule", Name: "Missing"})
	assertError(t, err)
}

// Issue #391 — DROP ENUMERATION with an unqualified name must error when
// the name matches enumerations in multiple modules, not silently drop one.
func TestDropEnumeration_AmbiguousUnqualified_Issue391(t *testing.T) {
	mod1 := mkModule("Mod1")
	mod2 := mkModule("Mod2")
	e1 := mkEnumeration(mod1.ID, "Status", "Active", "Inactive")
	e2 := mkEnumeration(mod2.ID, "Status", "Open", "Closed")

	h := mkHierarchy(mod1, mod2)
	withContainer(h, e1.ContainerID, mod1.ID)
	withContainer(h, e2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod1, mod2}, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{e1, e2}, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execDropEnumeration(ctx, &ast.DropEnumerationStmt{
		Name: ast.QualifiedName{Name: "Status"}, // unqualified
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "ambiguous")
}

func TestDropEnumeration_Qualified_Success(t *testing.T) {
	mod1 := mkModule("Mod1")
	mod2 := mkModule("Mod2")
	e1 := mkEnumeration(mod1.ID, "Status", "Active")
	e2 := mkEnumeration(mod2.ID, "Status", "Open")
	deleted := ""

	h := mkHierarchy(mod1, mod2)
	withContainer(h, e1.ContainerID, mod1.ID)
	withContainer(h, e2.ContainerID, mod2.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod1, mod2}, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{e1, e2}, nil },
		DeleteEnumerationFunc: func(id model.ID) error {
			deleted = string(id)
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execDropEnumeration(ctx, &ast.DropEnumerationStmt{
		Name: ast.QualifiedName{Module: "Mod1", Name: "Status"},
	})
	assertNoError(t, err)
	if deleted != string(e1.ID) {
		t.Errorf("expected Mod1.Status (id=%s) to be deleted, got %s", e1.ID, deleted)
	}
}

// Issue #390 — CREATE ENUMERATION must reject duplicate value names.
func TestCreateEnumeration_DuplicateValueName_Issue390(t *testing.T) {
	mod := mkModule("M")
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return nil, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execCreateEnumeration(ctx, &ast.CreateEnumerationStmt{
		Name: ast.QualifiedName{Module: "M", Name: "E"},
		Values: []ast.EnumValue{
			{Name: "A", Caption: "First"},
			{Name: "A", Caption: "Second"},
		},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "duplicate")
}

// ALTER ENUMERATION ... MODIFY VALUE X CAPTION '...' changes an existing value's
// caption in place, without dropping/recreating the enum (which fails while it is
// referenced by an attribute). The value keeps its ID; only the en_US caption
// changes.
func TestAlterEnumeration_ModifyValueCaption_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	enum := mkEnumeration(mod.ID, "Status", "Active", "Inactive") // captions start nil

	h := mkHierarchy(mod)
	withContainer(h, enum.ContainerID, mod.ID)

	var updated *model.Enumeration
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
		UpdateEnumerationFunc: func(e *model.Enumeration) error {
			updated = e
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	origID := enum.Values[0].ID
	err := execAlterEnumeration(ctx, &ast.AlterEnumerationStmt{
		Name:      ast.QualifiedName{Module: "MyModule", Name: "Status"},
		Operation: ast.AlterEnumModifyCaption,
		ValueName: "Active",
		Caption:   "Currently Active",
	})
	assertNoError(t, err)
	if updated == nil {
		t.Fatal("UpdateEnumeration was not called")
	}
	if got := updated.Values[0].Caption.GetTranslation("en_US"); got != "Currently Active" {
		t.Errorf("Active caption = %q, want 'Currently Active'", got)
	}
	if updated.Values[0].ID != origID {
		t.Errorf("value ID changed (%s -> %s); MODIFY CAPTION must preserve the value identity", origID, updated.Values[0].ID)
	}
	if updated.Values[0].Name != "Active" {
		t.Errorf("value name changed to %q; MODIFY CAPTION must not rename", updated.Values[0].Name)
	}
	if updated.Values[1].Caption != nil && updated.Values[1].Caption.GetTranslation("en_US") != "" {
		t.Errorf("sibling value 'Inactive' caption was altered: %q", updated.Values[1].Caption.GetTranslation("en_US"))
	}
}

func TestAlterEnumeration_ModifyValueCaption_ValueNotFound_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	enum := mkEnumeration(mod.ID, "Status", "Active")

	h := mkHierarchy(mod)
	withContainer(h, enum.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
		UpdateEnumerationFunc: func(e *model.Enumeration) error {
			t.Fatal("UpdateEnumeration must not be called when the value is missing")
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execAlterEnumeration(ctx, &ast.AlterEnumerationStmt{
		Name:      ast.QualifiedName{Module: "MyModule", Name: "Status"},
		Operation: ast.AlterEnumModifyCaption,
		ValueName: "Missing",
		Caption:   "x",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
}

// Backend error: cmd_error_mock_test.go (TestShowEnumerations_Mock_BackendError)
// JSON: cmd_json_mock_test.go (TestShowEnumerations_Mock_JSON)

// `alter enumeration ... add value` had no IF NOT EXISTS, so a script that adds
// one was not re-runnable: the second run errored and `exec` STOPPED THERE,
// leaving every later statement unapplied. One already-present value silently
// truncated the rest of the script (ako/mxcli-rest FINDINGS #60).
//
// Same guard pair as ALTER ENTITY's ADD ATTRIBUTE / ADD INDEX, and for the same
// reason: a defensive drop-then-add cannot be re-run either, since the drop
// fails when the value is absent and the add when it is present.
func TestAlterEnumeration_AddValueIfNotExists_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	enum := mkEnumeration(mod.ID, "Status", "Active", "Inactive")
	h := mkHierarchy(mod)
	withContainer(h, enum.ContainerID, mod.ID)

	updates := 0
	mb := &mock.MockBackend{
		IsConnectedFunc:       func() bool { return true },
		ListEnumerationsFunc:  func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
		UpdateEnumerationFunc: func(*model.Enumeration) error { updates++; return nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))

	stmt := &ast.AlterEnumerationStmt{
		Name:        ast.QualifiedName{Module: "MyModule", Name: "Status"},
		Operation:   ast.AlterEnumAdd,
		ValueName:   "Active",
		Caption:     "Active",
		IfNotExists: true,
	}
	if err := execAlterEnumeration(ctx, stmt); err != nil {
		t.Fatalf("add value if not exists errored on an existing value: %v", err)
	}
	if updates != 0 {
		t.Error("the enumeration was rewritten for a value that was already there")
	}
	if !strings.Contains(buf.String(), "skipped") {
		t.Errorf("the skip was silent: %q", buf.String())
	}

	// CONTROL: the bare form must still error, and must say how to make the
	// script re-runnable. Without this, a guard applied unconditionally would
	// pass the assertion above and silently swallow a real duplicate.
	bare := *stmt
	bare.IfNotExists = false
	err := execAlterEnumeration(ctx, &bare)
	if err == nil {
		t.Fatal("the unguarded add accepted a duplicate value")
	}
	if !strings.Contains(err.Error(), "if not exists") {
		t.Errorf("the error should name the guard: %v", err)
	}

	// CONTROL: the guard must not stop a value that is genuinely new.
	if err := execAlterEnumeration(ctx, &ast.AlterEnumerationStmt{
		Name:        ast.QualifiedName{Module: "MyModule", Name: "Status"},
		Operation:   ast.AlterEnumAdd,
		ValueName:   "Archived",
		IfNotExists: true,
	}); err != nil {
		t.Fatalf("add value if not exists rejected a new value: %v", err)
	}
	if updates != 1 {
		t.Errorf("UpdateEnumeration called %d times, want 1 — the new value was not written", updates)
	}
}

// The DROP twin: a re-run finds the value already gone.
func TestAlterEnumeration_DropValueIfExists_Mock(t *testing.T) {
	mod := mkModule("MyModule")
	enum := mkEnumeration(mod.ID, "Status", "Active", "Inactive")
	h := mkHierarchy(mod)
	withContainer(h, enum.ContainerID, mod.ID)

	updates := 0
	mb := &mock.MockBackend{
		IsConnectedFunc:       func() bool { return true },
		ListEnumerationsFunc:  func() ([]*model.Enumeration, error) { return []*model.Enumeration{enum}, nil },
		UpdateEnumerationFunc: func(*model.Enumeration) error { updates++; return nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))

	if err := execAlterEnumeration(ctx, &ast.AlterEnumerationStmt{
		Name:      ast.QualifiedName{Module: "MyModule", Name: "Status"},
		Operation: ast.AlterEnumDrop,
		ValueName: "NeverThere",
		IfExists:  true,
	}); err != nil {
		t.Fatalf("drop value if exists errored on a missing value: %v", err)
	}
	if updates != 0 {
		t.Error("the enumeration was rewritten for a value that was not there")
	}
	if !strings.Contains(buf.String(), "skipped") {
		t.Errorf("the skip was silent: %q", buf.String())
	}

	// CONTROL: the bare form still reports a missing value.
	if err := execAlterEnumeration(ctx, &ast.AlterEnumerationStmt{
		Name:      ast.QualifiedName{Module: "MyModule", Name: "Status"},
		Operation: ast.AlterEnumDrop,
		ValueName: "NeverThere",
	}); err == nil {
		t.Fatal("the unguarded drop accepted a value that does not exist")
	}
}
