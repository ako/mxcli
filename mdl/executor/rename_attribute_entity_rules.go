// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// renameAttributeInEntityRules moves an entity's own references to one of its
// attributes onto the attribute's new name.
//
// Access rules and validation rules both point at an attribute by qualified name
// string rather than by element ID, so renaming the attribute in the model
// leaves them behind. That matters more than it looks: UpdateEntity re-derives a
// rule's members from the attributes it can match, so a stale member is not
// merely stale — a `READ *` rule comes back with a member for the new name *and*
// the orphaned old one, which mxbuild reports as CE0066 "Entity access is out of
// date". Fixing them here, before the write, is what keeps the rule intact.
//
// The project-wide reference scan would rewrite the same strings afterwards, but
// by then the damage is done, and a scan cannot tell a duplicate from a
// legitimate second member.
func renameAttributeInEntityRules(entity *domainmodel.Entity, entityQN, oldAttr, newAttr string) {
	oldQN := entityQN + "." + oldAttr
	newQN := entityQN + "." + newAttr

	for _, rule := range entity.AccessRules {
		if rule == nil {
			continue
		}
		for _, ma := range rule.MemberAccesses {
			if ma != nil && ma.AttributeName == oldQN {
				ma.AttributeName = newQN
			}
		}
	}

	for _, vr := range entity.ValidationRules {
		if vr == nil {
			continue
		}
		// AttributeID holds either a UUID (an entity built in this run, where the
		// name is looked up at serialization time and needs no help) or the
		// qualified name read from disk.
		if string(vr.AttributeID) == oldQN {
			vr.AttributeID = model.ID(newQN)
		}
		// A range rule can be bounded by another attribute of the same entity,
		// including the one being renamed. MDL cannot author that form, but a
		// stored rule survives a round trip and must survive a rename too.
		if rng, ok := vr.Rule.(*domainmodel.RangeValidationRuleInfo); ok {
			if rng.MinAttributeQualifiedName == oldQN {
				rng.MinAttributeQualifiedName = newQN
			}
			if rng.MaxAttributeQualifiedName == oldQN {
				rng.MaxAttributeQualifiedName = newQN
			}
		}
	}
}
