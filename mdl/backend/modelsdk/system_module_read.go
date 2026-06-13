// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// buildSystemDomainModel builds the virtual System-module domain model from the
// platform entity definitions in modelsdk/meta. The System module is not stored
// in the project (mprcontents), so without this its entities (WorkflowUserTask,
// User, FileDocument, …) can't be resolved — mirrors the legacy reader's
// BuildSystemModule. Entity IDs are synthetic (System entities are referenced by
// qualified name, never by ID, in serialized output).
func buildSystemDomainModel() *domainmodel.DomainModel {
	dm := &domainmodel.DomainModel{ContainerID: model.ID(meta.SystemModuleID)}
	dm.ID = model.ID(meta.SystemDomainModelID)
	for _, e := range meta.SystemEntities {
		ent := &domainmodel.Entity{
			Name:              e.Name,
			Persistable:       e.Persistable,
			GeneralizationRef: e.Generalization,
		}
		ent.ID = model.ID("System." + e.Name)
		for _, a := range e.Attributes {
			attr := &domainmodel.Attribute{
				Name: a.Name,
				Type: systemAttrType(a),
			}
			// Synthesize a stable, unique ID per attribute. The System module is
			// virtual (no BSON), so these IDs aren't stored — but the catalog keys
			// attributes_data on Id, so empty IDs collide (UNIQUE constraint on
			// attributes_data.Id). Mirror the entity ID scheme.
			attr.ID = model.ID("System." + e.Name + "." + a.Name)
			ent.Attributes = append(ent.Attributes, attr)
		}
		dm.Entities = append(dm.Entities, ent)
	}
	return dm
}

// systemAttrType maps a System attribute definition to a domainmodel attribute type.
func systemAttrType(a meta.SystemAttrDef) domainmodel.AttributeType {
	switch a.Type {
	case "Integer":
		return &domainmodel.IntegerAttributeType{}
	case "Long":
		return &domainmodel.LongAttributeType{}
	case "Decimal":
		return &domainmodel.DecimalAttributeType{}
	case "Boolean":
		return &domainmodel.BooleanAttributeType{}
	case "DateTime":
		return &domainmodel.DateTimeAttributeType{}
	case "Enumeration":
		return &domainmodel.EnumerationAttributeType{EnumerationRef: a.EnumQN}
	case "AutoNumber":
		return &domainmodel.AutoNumberAttributeType{}
	case "Binary":
		return &domainmodel.BinaryAttributeType{}
	case "HashedString":
		return &domainmodel.HashedStringAttributeType{}
	default: // String and anything unmapped
		return &domainmodel.StringAttributeType{Length: a.Length}
	}
}
