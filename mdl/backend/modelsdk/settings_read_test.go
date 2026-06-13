// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import "testing"

// TestGetProjectSettings exercises the codec-native ProjectSettings read against
// the vendored fixture: the "Settings" array must decode into typed parts (the
// gen binding fix) and convert to the semantic model. A regression here means the
// SettingsParts→Settings storage-key fix or a part alias was lost.
func TestGetProjectSettings(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect(%s): %v", fixture, err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	ps, err := b.GetProjectSettings()
	if err != nil {
		t.Fatalf("GetProjectSettings: %v", err)
	}
	if ps == nil || ps.ID == "" {
		t.Fatalf("GetProjectSettings returned empty settings: %+v", ps)
	}

	// The "Settings" array must have decoded into typed parts. Every Mendix
	// project carries a Model (runtime) and Configuration settings part.
	if ps.Model == nil {
		t.Error("ProjectSettings.Model is nil — Settings array did not decode into RuntimeSettings (storage-key/alias regression)")
	}
	if ps.Configuration == nil {
		t.Fatal("ProjectSettings.Configuration is nil — Settings array did not decode")
	}
	// (A blank fixture may carry zero server configurations, so we don't require any.)
	// The Convention part exercises the second storage alias.
	if ps.Convention == nil {
		t.Error("ProjectSettings.Convention is nil — Settings$ConventionSettings alias regression")
	}
}
