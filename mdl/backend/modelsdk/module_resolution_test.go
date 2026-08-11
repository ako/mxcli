// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import "testing"

// Documents in the fixture that live inside a folder rather than directly under
// their module. Studio Pro puts most documents in folders, and marketplace
// modules almost always do (the FeedbackModule ships _Docs/, Pages/, Private/…),
// so folder nesting is the normal case rather than an edge case.
//
// moduleNameFor resolved a unit's module by reading its *direct* container, so
// any document one or more folders deep resolved to "" and every by-qualified-
// name lookup built on it reported "not found". `describe import mapping
// FeedbackModule.IMM_PostResponse` was the reported symptom; the same helper
// backs the CREATE OR MODIFY existence check and DROP for these types, so the
// failure mode there is worse — an existence check that answers "no" turns a
// modify into an attempted create.
func TestByQualifiedNameFindsDocumentsInsideFolders(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect(%s): %v", fixture, err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	t.Run("import mapping", func(t *testing.T) {
		im, err := b.GetImportMappingByQualifiedName("FeedbackModule", "IMM_PostResponse")
		if err != nil {
			t.Fatalf("GetImportMappingByQualifiedName: %v", err)
		}
		if im.Name != "IMM_PostResponse" {
			t.Errorf("Name = %q, want IMM_PostResponse", im.Name)
		}
	})

	t.Run("export mapping", func(t *testing.T) {
		em, err := b.GetExportMappingByQualifiedName("FeedbackModule", "EXM_PostFeedback")
		if err != nil {
			t.Fatalf("GetExportMappingByQualifiedName: %v", err)
		}
		if em.Name != "EXM_PostFeedback" {
			t.Errorf("Name = %q, want EXM_PostFeedback", em.Name)
		}
	})

	t.Run("json structure", func(t *testing.T) {
		js, err := b.GetJsonStructureByQualifiedName("FeedbackModule", "JSON_AppInsightsRequest")
		if err != nil {
			t.Fatalf("GetJsonStructureByQualifiedName: %v", err)
		}
		if js.Name != "JSON_AppInsightsRequest" {
			t.Errorf("Name = %q, want JSON_AppInsightsRequest", js.Name)
		}
	})
}

// TestByQualifiedNameRejectsWrongModule guards the other direction: walking up
// the container chain must stop at the first enclosing module, not resolve a
// name against any module that happens to contain a like-named document.
func TestByQualifiedNameRejectsWrongModule(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect(%s): %v", fixture, err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	if _, err := b.GetImportMappingByQualifiedName("Administration", "IMM_PostResponse"); err == nil {
		t.Error("expected an error for a mapping that lives in a different module, got nil")
	}
}
