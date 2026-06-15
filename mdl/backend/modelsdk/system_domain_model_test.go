// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import "testing"

// TestGetDomainModel_VirtualSystemModule guards DESCRIBE System.*: the System
// module is virtual (no stored domain-model unit), so GetDomainModel must serve
// the injected System domain model for its container ID rather than erroring
// "domain model not found" (GetDomainModel errors on truly-missing modules, which
// the drop-module finalize path relies on — System must be the documented exception).
func TestGetDomainModel_VirtualSystemModule(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	sys := buildSystemDomainModel()
	dm, err := b.GetDomainModel(sys.ContainerID)
	if err != nil {
		t.Fatalf("GetDomainModel(System) errored: %v", err)
	}
	if dm == nil || len(dm.Entities) == 0 {
		t.Fatalf("System domain model empty: %+v", dm)
	}
	// A non-existent module must still error (drop-module finalize relies on it).
	if _, err := b.GetDomainModel("ffffffff-0000-0000-0000-000000000000"); err == nil {
		t.Error("GetDomainModel(bogus) should error, not return nil,nil")
	}
}
