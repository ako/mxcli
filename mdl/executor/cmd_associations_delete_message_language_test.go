// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// mendixlabs/mxcli#1344: "on delete restrict error_message '…'" (and
// "delete_behavior prevent error_message '…'") stored the message under en_US
// whatever the project's language. On a nl_NL project the app had no message in
// its own language when a delete was refused, and mx check reported nothing. The
// create, alter and describe handlers never asked for authoringLanguage, so
// fixing the backend alone would still have written en_US.

// withProjectLanguage makes ctx's mock backend report code as the project's
// DefaultLanguageCode. It must run before anything resolves the language: the
// answer is cached on first use.
func withProjectLanguage(t *testing.T, ctx *ExecContext, code string) {
	t.Helper()
	mb, ok := ctx.Backend.(*mock.MockBackend)
	if !ok {
		t.Fatalf("backend is %T, want *mock.MockBackend", ctx.Backend)
	}
	mb.GetProjectSettingsFunc = func() (*model.ProjectSettings, error) {
		return &model.ProjectSettings{Language: &model.LanguageSettings{DefaultLanguageCode: code}}, nil
	}
	t.Cleanup(func() { model.SetAuthoringLanguage("") })
}

func restrictStmt(msg string, createOrModify bool) *ast.CreateAssociationStmt {
	return &ast.CreateAssociationStmt{
		Name:               ast.QualifiedName{Module: "M", Name: "Child_Parent"},
		Parent:             ast.QualifiedName{Module: "M", Name: "Child"},
		Child:              ast.QualifiedName{Module: "M", Name: "Parent"},
		Type:               ast.AssocReference,
		DeleteBehavior:     ast.DeleteIfNoReferences,
		DeleteErrorMessage: msg,
		CreateOrModify:     createOrModify,
	}
}

func assertMessageLanguage(t *testing.T, db *domainmodel.DeleteBehavior, wantLang, wantMsg string) {
	t.Helper()
	if db == nil {
		t.Fatal("no delete behaviour written")
	}
	if db.ErrorMessage != wantMsg {
		t.Errorf("ErrorMessage = %q, want %q", db.ErrorMessage, wantMsg)
	}
	if db.ErrorMessageLanguage != wantLang {
		t.Errorf("ErrorMessageLanguage = %q, want %q — the message is stored in a language the app is not in",
			db.ErrorMessageLanguage, wantLang)
	}
}

// The reported statement, as the first statement of a script: CREATE and
// CREATE OR MODIFY, same-module, on a nl_NL project and (control) an en_US one.
func TestCreateAssociation_DeleteMessageUsesProjectLanguage(t *testing.T) {
	for _, lang := range []string{"nl_NL", "en_US"} {
		t.Run("create-or-modify/"+lang, func(t *testing.T) {
			ctx, assoc := assocFixture(t)
			withProjectLanguage(t, ctx, lang)
			assertNoError(t, execCreateAssociation(ctx, restrictStmt("Kan niet", true)))
			assertMessageLanguage(t, assoc.ChildDeleteBehavior, lang, "Kan niet")
		})

		t.Run("create/"+lang, func(t *testing.T) {
			ctx, _ := assocFixture(t)
			withProjectLanguage(t, ctx, lang)
			var created *domainmodel.Association
			ctx.Backend.(*mock.MockBackend).CreateAssociationFunc = func(_ model.ID, a *domainmodel.Association) error {
				created = a
				return nil
			}
			s := restrictStmt("Kan niet", false)
			s.Name.Name = "Child_Parent2"
			assertNoError(t, execCreateAssociation(ctx, s))
			if created == nil {
				t.Fatal("CreateAssociation was not called")
			}
			assertMessageLanguage(t, created.ChildDeleteBehavior, lang, "Kan niet")
		})
	}
}

// ALTER … SET DELETE_BEHAVIOR PREVENT ERROR_MESSAGE, same-module and
// cross-module, read back from a copy-on-read store.
func TestAlterAssociation_DeleteMessageUsesProjectLanguage(t *testing.T) {
	for _, cross := range []bool{false, true} {
		ctx := storeLikeAssocFixture(t, cross, true)
		withProjectLanguage(t, ctx, "nl_NL")
		assertNoError(t, execAlterAssociation(ctx, &ast.AlterAssociationStmt{
			Name:               ast.QualifiedName{Module: "M", Name: "Child_Parent"},
			Operation:          ast.AlterAssociationSetDeleteBehavior,
			DeleteBehavior:     ast.DeleteIfNoReferences,
			DeleteErrorMessage: "Kan niet",
		}))
		dm, _ := ctx.Backend.GetDomainModel("")
		var db *domainmodel.DeleteBehavior
		if cross {
			db = dm.CrossAssociations[0].ChildDeleteBehavior
		} else {
			db = dm.Associations[0].ChildDeleteBehavior
		}
		assertMessageLanguage(t, db, "nl_NL", "Kan niet")
	}
}

// The alter's read-back must compare in the language it wrote. A real backend
// carries the stored en_US translation onto the rewritten text and reports
// ErrorMessage en_US-first, so comparing that string failed every nl_NL alter
// of a message that already had an English one with "change not persisted".
func TestAlterAssociation_VerifiesTheMessageInTheWrittenLanguage(t *testing.T) {
	ctx := storeLikeAssocFixture(t, false, true)
	withProjectLanguage(t, ctx, "nl_NL")
	mb := ctx.Backend.(*mock.MockBackend)
	persist := mb.UpdateDomainModelFunc
	mb.UpdateDomainModelFunc = func(dm *domainmodel.DomainModel) error {
		db := dm.Associations[0].ChildDeleteBehavior
		db.ErrorMessageTranslations = map[string]string{"en_US": "Stale English", db.ErrorMessageLanguage: db.ErrorMessage}
		db.ErrorMessage = "Stale English" // what the backend reads first
		return persist(dm)
	}
	assertNoError(t, execAlterAssociation(ctx, &ast.AlterAssociationStmt{
		Name:               ast.QualifiedName{Module: "M", Name: "Child_Parent"},
		Operation:          ast.AlterAssociationSetDeleteBehavior,
		DeleteBehavior:     ast.DeleteIfNoReferences,
		DeleteErrorMessage: "Kan niet",
	}))
}

// DESCRIBE reads back the language CREATE writes, or describe -> exec stops
// round-tripping on a project whose default is not en_US. The stored text holds
// both, so picking en_US first would emit the stale English.
func TestDescribeAssociation_DeleteMessageInProjectLanguage(t *testing.T) {
	for _, tc := range []struct{ lang, want string }{
		{"nl_NL", "Kan niet"},
		{"en_US", "Cannot"}, // control
	} {
		t.Run(tc.lang, func(t *testing.T) {
			ctx, assoc := assocFixture(t)
			withProjectLanguage(t, ctx, tc.lang)
			assoc.ChildDeleteBehavior = &domainmodel.DeleteBehavior{
				Type:                     domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences,
				ErrorMessage:             "Cannot",
				ErrorMessageTranslations: map[string]string{"en_US": "Cannot", "nl_NL": "Kan niet"},
			}
			var buf bytes.Buffer
			ctx.Output = &buf
			assertNoError(t, describeAssociation(ctx, ast.QualifiedName{Module: "M", Name: "Child_Parent"}))
			prog, errs := visitor.Build(buf.String())
			if len(errs) > 0 {
				t.Fatalf("DESCRIBE emitted MDL the parser rejects: %v\n%s", errs, buf.String())
			}
			stmt := prog.Statements[0].(*ast.CreateAssociationStmt)
			if stmt.DeleteErrorMessage != tc.want {
				t.Errorf("described message = %q, want %q\n%s", stmt.DeleteErrorMessage, tc.want, buf.String())
			}
		})
	}
}
