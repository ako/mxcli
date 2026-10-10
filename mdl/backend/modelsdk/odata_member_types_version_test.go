// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// PublishedAttribute.EdmType and PublishedAssociationEnd.IsMany exist from
// Mendix 11.12.0 (measured with `mx convert`: 11.11.0 strips both). The writer
// derives their values, so on an older project they are left out rather than
// refused by the storage layer — which would refuse the whole service.
func TestPublishedMemberTypes_ByVersion(t *testing.T) {
	attr := &model.PublishedMember{Kind: "attribute", Name: "Name", ExposedName: "Name", EdmType: "Edm.String"}
	assoc := &model.PublishedMember{Kind: "association", Name: "Order_Customer", ExposedName: "Customer", IsMany: true}
	for _, c := range []struct {
		memberTypes bool
	}{{true}, {false}} {
		a := encodeToMap(t, publishedMemberToGen(attr, "Shop.Order", c.memberTypes))
		if _, ok := a["EdmType"]; ok != c.memberTypes {
			t.Errorf("memberTypes=%v: EdmType present = %v", c.memberTypes, ok)
		}
		e := encodeToMap(t, publishedMemberToGen(assoc, "Shop.Order", c.memberTypes))
		if _, ok := e["IsMany"]; ok != c.memberTypes {
			t.Errorf("memberTypes=%v: IsMany present = %v", c.memberTypes, ok)
		}
		// Control: the rest of the member is written either way.
		if a["ExposedName"] != "Name" {
			t.Errorf("memberTypes=%v: ExposedName = %v", c.memberTypes, a["ExposedName"])
		}
	}
}
