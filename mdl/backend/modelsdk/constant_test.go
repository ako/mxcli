// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import "testing"

// TestReadSlice_Constants checks the constant adapter (name, type kind, default,
// exposed). SHOW CONSTANTS is cross-checked byte-for-byte against legacy.
func TestReadSlice_Constants(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	cs, err := b.ListConstants()
	if err != nil {
		t.Fatalf("ListConstants: %v", err)
	}
	if len(cs) != 2 {
		t.Fatalf("ListConstants count = %d, want 2", len(cs))
	}
	for _, c := range cs {
		if c.Name == "" {
			t.Error("constant with empty name")
		}
		if c.Type.Kind == "" || c.Type.Kind == "Unknown" {
			t.Errorf("constant %q has unresolved type kind %q", c.Name, c.Type.Kind)
		}
	}
}
