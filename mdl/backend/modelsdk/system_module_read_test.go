// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import "testing"

// TestSystemDomainModel_UniqueAttrIDs guards the catalog PK invariant: the
// virtual System module's attributes must each have a unique, non-empty ID.
// Empty IDs collide on attributes_data.Id and break every catalog query.
func TestSystemDomainModel_UniqueAttrIDs(t *testing.T) {
	dm := buildSystemDomainModel()
	seen := map[string]string{}
	for _, e := range dm.Entities {
		for _, a := range e.Attributes {
			id := string(a.ID)
			if id == "" {
				t.Errorf("System.%s.%s has empty ID", e.Name, a.Name)
				continue
			}
			if prev, ok := seen[id]; ok {
				t.Errorf("duplicate System attr ID %q: %s and System.%s.%s", id, prev, e.Name, a.Name)
			}
			seen[id] = "System." + e.Name + "." + a.Name
		}
	}
}
