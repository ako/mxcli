// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import "testing"

// TestMicroflowFromGen_ReadsAllowedModuleRoles guards the "grant execute on
// microflow … to …" line in DESCRIBE MICROFLOW. The allowed module roles are
// BY_NAME references the gen exposes via AllowedModuleRolesQualifiedNames();
// the adapter must surface them as Microflow.AllowedModuleRoles or DESCRIBE
// drops the grant line that the legacy engine emits.
func TestMicroflowFromGen_ReadsAllowedModuleRoles(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mfs, err := b.ListMicroflows()
	if err != nil {
		t.Fatalf("ListMicroflows: %v", err)
	}
	var roles []string
	for _, mf := range mfs {
		if mf.Name == "ChangeMyPassword" {
			for _, r := range mf.AllowedModuleRoles {
				roles = append(roles, string(r))
			}
		}
	}
	if roles == nil {
		t.Fatal("ChangeMyPassword not found or has no AllowedModuleRoles")
	}
	want := map[string]bool{"Administration.Administrator": false, "Administration.User": false}
	for _, r := range roles {
		if _, ok := want[r]; ok {
			want[r] = true
		}
	}
	for role, seen := range want {
		if !seen {
			t.Errorf("AllowedModuleRoles missing %q (got %v)", role, roles)
		}
	}
}
