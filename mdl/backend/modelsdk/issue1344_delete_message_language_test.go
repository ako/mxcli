// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	genTexts "github.com/mendixlabs/mxcli/modelsdk/gen/texts"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// mendixlabs/mxcli#1344: "on delete restrict error_message '…'" stored the
// message as a Texts$Text whose one translation was always en_US, so a nl_NL app
// had no message in its own language when a delete was refused — and mx check
// reported nothing. deleteErrorText hardcoded the language code.

func restrictMessageTranslations(t *testing.T, db *domainmodel.DeleteBehavior) map[string]string {
	t.Helper()
	gen := assocToGen(&domainmodel.Association{Name: "Order_Customer", ChildDeleteBehavior: db})
	gdb, ok := gen.DeleteBehavior().(*genDm.AssociationDeleteBehavior)
	if !ok || gdb == nil {
		t.Fatalf("no AssociationDeleteBehavior on the generated association")
	}
	txt, ok := gdb.ChildErrorMessage().(*genTexts.Text)
	if !ok || txt == nil {
		t.Fatal("ChildErrorMessage is not a Texts$Text — the runtime will not start")
	}
	return textFromGen(txt).Translations
}

// The reported case: the message lands under the language the executor names.
func TestDeleteErrorMessage_StoredInTheGivenLanguage(t *testing.T) {
	got := restrictMessageTranslations(t, &domainmodel.DeleteBehavior{
		Type:                 domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences,
		ErrorMessage:         "Een klant met orders kan niet verwijderd worden",
		ErrorMessageLanguage: "nl_NL",
	})
	if len(got) != 1 || got["nl_NL"] != "Een klant met orders kan niet verwijderd worden" {
		t.Errorf("translations = %v, want exactly nl_NL", got)
	}
}

// CONTROL: no language named keeps en_US, so an en_US project and every caller
// that predates the field are unchanged.
func TestDeleteErrorMessage_NoLanguageKeepsEnUS(t *testing.T) {
	got := restrictMessageTranslations(t, &domainmodel.DeleteBehavior{
		Type:         domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences,
		ErrorMessage: "blocked",
	})
	if len(got) != 1 || got["en_US"] != "blocked" {
		t.Errorf("translations = %v, want exactly en_US", got)
	}
}

// The read side hands back every translation, so DESCRIBE can choose the
// project's default instead of en_US.
func TestDeleteErrorMessage_ReadBackCarriesEveryTranslation(t *testing.T) {
	gen := assocToGen(&domainmodel.Association{Name: "A", ChildDeleteBehavior: &domainmodel.DeleteBehavior{
		Type:                 domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences,
		ErrorMessage:         "Nee",
		ErrorMessageLanguage: "nl_NL",
	}})
	back := assocFromGen(gen)
	if back.ChildDeleteBehavior == nil {
		t.Fatal("no child delete behaviour read back")
	}
	if got := back.ChildDeleteBehavior.ErrorMessageTranslations; len(got) != 1 || got["nl_NL"] != "Nee" {
		t.Errorf("ErrorMessageTranslations = %v, want nl_NL: Nee", got)
	}
}

// The cross-module alter path edits the message in place: it must compare and
// write in the statement's language, and keep the other languages' translations
// rather than replacing the whole text.
func TestPatchCrossDeleteErrorMessage_EditsOneLanguageKeepsTheOthers(t *testing.T) {
	db := genDm.NewAssociationDeleteBehavior()
	db.SetChildErrorMessage(textToGen(&model.Text{Translations: map[string]string{
		"en_US": "Cannot delete", "nl_NL": "Oud",
	}}))

	patchCrossDeleteErrorMessage(db, &domainmodel.DeleteBehavior{
		Type:                 domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences,
		ErrorMessage:         "Nieuw",
		ErrorMessageLanguage: "nl_NL",
	})

	got := deleteErrorTranslationsFromGen(db.ChildErrorMessage())
	if got["nl_NL"] != "Nieuw" {
		t.Errorf("nl_NL = %q, want the statement's message", got["nl_NL"])
	}
	if got["en_US"] != "Cannot delete" {
		t.Errorf("en_US = %q, want the stored translation kept", got["en_US"])
	}
}

// CONTROL for the comparison: the same message re-set in nl_NL is a no-op even
// though the en_US translation differs — reading en_US first would see a change
// on every run and rewrite the text.
func TestPatchCrossDeleteErrorMessage_UnchangedInItsLanguageIsANoOp(t *testing.T) {
	db := genDm.NewAssociationDeleteBehavior()
	before := textToGen(&model.Text{Translations: map[string]string{
		"en_US": "Cannot delete", "nl_NL": "Nee",
	}})
	db.SetChildErrorMessage(before)

	patchCrossDeleteErrorMessage(db, &domainmodel.DeleteBehavior{
		Type:                 domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences,
		ErrorMessage:         "Nee",
		ErrorMessageLanguage: "nl_NL",
	})
	if db.ChildErrorMessage() != before {
		t.Error("an unchanged nl_NL message replaced the stored text")
	}
}
