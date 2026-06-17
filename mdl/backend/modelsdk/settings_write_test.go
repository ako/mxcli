// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"
)

// TestUpdateProjectSettings_OverlayRoundTrip reads the project settings, mutates a
// Model field, writes via the raw-part overlay, and confirms the change persists
// while the untouched parts survive (RawParts is repopulated and the document
// re-reads cleanly).
func TestUpdateProjectSettings_OverlayRoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	ps, err := b.GetProjectSettings()
	if err != nil {
		t.Fatalf("GetProjectSettings: %v", err)
	}
	if len(ps.RawParts) == 0 {
		t.Fatalf("RawParts not populated on read")
	}
	if ps.Model == nil {
		t.Fatalf("Model settings not read")
	}
	partCount := len(ps.RawParts)
	ps.Model.HashAlgorithm = "BCrypt"
	if err := b.UpdateProjectSettings(ps); err != nil {
		t.Fatalf("UpdateProjectSettings: %v", err)
	}

	// Re-read from a fresh connection: the mutated field persists and every part
	// is preserved (overlay didn't drop any).
	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	ps2, err := b2.GetProjectSettings()
	if err != nil {
		t.Fatalf("GetProjectSettings(2): %v", err)
	}
	if ps2.Model == nil || ps2.Model.HashAlgorithm != "BCrypt" {
		t.Errorf("HashAlgorithm not round-tripped: %+v", ps2.Model)
	}
	if len(ps2.RawParts) != partCount {
		t.Errorf("part count changed: was %d, now %d (overlay dropped a part)", partCount, len(ps2.RawParts))
	}
}
