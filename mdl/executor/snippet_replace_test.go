// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-ledger #143: `create or replace snippet` destroyed the document's
// translations. Replaying one snippet file reset a translated label to its
// source language in all six languages — silently: the run reported "Created
// snippet", mx check reported 0 errors, and the app rendered in one language.
//
// The cause was that the replace path was an unconditional delete-then-create.
// Nothing canon.Reconcile does can survive that: by the time the create runs
// there is no stored document left to transplant identities from or to compare
// against, so both the preservation and the unchanged-elision have nothing to
// work from. Pages already did the right thing, which is the control that made
// this a snippet defect rather than a fact about replacing documents — measured
// on one project in one run:
//
//	page    "Unchanged page"     translations survive
//	snippet "Created snippet"    translation destroyed
package executor

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// snippetReplaceCalls records which backend verb the replace path used.
type snippetReplaceCalls struct {
	created []model.ID
	updated []model.ID
	deleted []model.ID
}

func runSnippetReplace(t *testing.T, mod *model.Module, existing []*pages.Snippet) *snippetReplaceCalls {
	t.Helper()
	calls := &snippetReplaceCalls{}
	mb := &mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc:   func() ([]*types.FolderInfo, error) { return nil, nil },
		ListSnippetsFunc:  func() ([]*pages.Snippet, error) { return existing, nil },
		CreateSnippetFunc: func(s *pages.Snippet) error { calls.created = append(calls.created, s.ID); return nil },
		UpdateSnippetFunc: func(s *pages.Snippet) error { calls.updated = append(calls.updated, s.ID); return nil },
		DeleteSnippetFunc: func(id model.ID) error { calls.deleted = append(calls.deleted, id); return nil },
	}
	ctx := &ExecContext{Backend: mb, Cache: &executorCache{}, Output: &bytes.Buffer{}}
	stmt := &ast.CreateSnippetStmtV3{
		Name:      ast.QualifiedName{Module: "MyModule", Name: "SNIPPET_Zeb"},
		IsReplace: true,
	}
	if err := execCreateSnippetV3(ctx, stmt); err != nil {
		t.Fatalf("execCreateSnippetV3: %v", err)
	}
	return calls
}

func TestReplaceSnippetUpdatesInPlace(t *testing.T) {
	mod := mkModule("MyModule")
	existing := mkSnippet(mod.ID, "SNIPPET_Zeb")
	calls := runSnippetReplace(t, mod, []*pages.Snippet{existing})

	// The whole fix: an existing snippet is UPDATED, never deleted and recreated.
	// A delete+create is what discarded the translations (#143) — and it also
	// churns the file in git, which is the RevStatusCache crash the page path
	// already documents.
	if len(calls.updated) != 1 {
		t.Errorf("expected 1 UpdateSnippet, got %d (created=%d deleted=%d) — a replace that "+
			"deletes and recreates cannot preserve translations (ako/mxcli-ledger #143)",
			len(calls.updated), len(calls.created), len(calls.deleted))
	}
	if len(calls.created) != 0 {
		t.Errorf("expected no CreateSnippet on a replace, got %d", len(calls.created))
	}
	if len(calls.deleted) != 0 {
		t.Errorf("expected no DeleteSnippet for a one-for-one replace, got %d", len(calls.deleted))
	}
	// The stored unit's identity must be reused, or Studio Pro sees a delete+add.
	if len(calls.updated) == 1 && calls.updated[0] != existing.ID {
		t.Errorf("update used ID %v, want the stored %v", calls.updated[0], existing.ID)
	}
}

func TestCreateSnippetStillCreatesWhenAbsent(t *testing.T) {
	// The control: with nothing stored, the path must still create. Without this
	// a fix that always calls Update would pass the test above and break creation.
	calls := runSnippetReplace(t, mkModule("MyModule"), nil)
	if len(calls.created) != 1 {
		t.Errorf("expected 1 CreateSnippet for a new snippet, got %d", len(calls.created))
	}
	if len(calls.updated) != 0 {
		t.Errorf("expected no UpdateSnippet when nothing is stored, got %d", len(calls.updated))
	}
}

func TestReplaceSnippetDeletesOnlyExtraDuplicates(t *testing.T) {
	// Duplicates of the same name are genuine garbage: the first is reused, the
	// rest deleted — the same rule the page path applies.
	mod := mkModule("MyModule")
	a := mkSnippet(mod.ID, "SNIPPET_Zeb")
	b := mkSnippet(mod.ID, "SNIPPET_Zeb")
	calls := runSnippetReplace(t, mod, []*pages.Snippet{a, b})
	if len(calls.updated) != 1 || calls.updated[0] != a.ID {
		t.Errorf("expected the FIRST stored snippet to be updated, got %v", calls.updated)
	}
	if len(calls.deleted) != 1 || calls.deleted[0] != b.ID {
		t.Errorf("expected only the duplicate to be deleted, got %v", calls.deleted)
	}
}
