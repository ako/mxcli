// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	genTexts "github.com/mendixlabs/mxcli/modelsdk/gen/texts"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// This file is the gen→domainmodel READ adapter. engalar's fork changed the
// DomainModelBackend interface to traffic in *genDm types and deleted
// sdk/domainmodel, so there is no converter to port — keeping main's executor
// and domainmodel types canonical means we own this translation. Phase 1 covers
// the breadth SHOW ENTITIES needs (names, persistability, generalization, and
// faithful member counts); full attribute-type / association-detail fidelity
// (DESCRIBE level) is a later phase.

// ListDomainModels reads every domain model through the codec engine.
func (b *Backend) ListDomainModels() ([]*domainmodel.DomainModel, error) {
	units, err := mprread.ListUnitsWithContainer[*genDm.DomainModel](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*domainmodel.DomainModel, 0, len(units))
	for _, u := range units {
		out = append(out, domainModelFromGen(u.Element, u.ContainerID))
	}
	return out, nil
}

// GetDomainModel returns the domain model whose container is moduleID.
func (b *Backend) GetDomainModel(moduleID model.ID) (*domainmodel.DomainModel, error) {
	units, err := mprread.ListUnitsWithContainer[*genDm.DomainModel](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if u.ContainerID == moduleID {
			return domainModelFromGen(u.Element, u.ContainerID), nil
		}
	}
	return nil, nil
}

func domainModelFromGen(dm *genDm.DomainModel, containerID model.ID) *domainmodel.DomainModel {
	out := &domainmodel.DomainModel{ContainerID: containerID}
	out.ID = model.ID(dm.ID())
	for _, el := range dm.EntitiesItems() {
		if e, ok := el.(*genDm.Entity); ok {
			out.Entities = append(out.Entities, entityFromGen(e))
		}
	}
	for _, el := range dm.AssociationsItems() {
		if a, ok := el.(*genDm.Association); ok {
			out.Associations = append(out.Associations, assocFromGen(a))
		}
	}
	return out
}

func entityFromGen(e *genDm.Entity) *domainmodel.Entity {
	out := &domainmodel.Entity{
		Name:          e.Name(),
		Documentation: e.Documentation(),
		Persistable:   true, // default; NoGeneralization overrides below
	}
	out.ID = model.ID(e.ID())

	// Generalization element is either NoGeneralization (carries persistability
	// + system-attribute flags) or Generalization (extends a parent entity).
	switch g := e.Generalization().(type) {
	case *genDm.NoGeneralization:
		out.Persistable = g.Persistable()
		out.HasOwner = g.HasOwner()
		out.HasChangedBy = g.HasChangedBy()
		out.HasChangedDate = g.HasChangedDate()
		out.HasCreatedDate = g.HasCreatedDate()
	case *genDm.Generalization:
		out.GeneralizationRef = g.GeneralizationQualifiedName()
		// Persistability is inherited from the parent chain; default true
		// matches legacy (sdk/mpr parser_domainmodel.go).
	}

	out.Location = parseLocation(e.Location())

	for _, el := range e.AttributesItems() {
		if a, ok := el.(*genDm.Attribute); ok {
			out.Attributes = append(out.Attributes, attributeFromGen(a))
		}
	}
	for _, el := range e.AccessRulesItems() {
		ar := &domainmodel.AccessRule{}
		ar.ID = model.ID(el.ID())
		out.AccessRules = append(out.AccessRules, ar)
	}
	for _, el := range e.IndexesItems() {
		if idx, ok := el.(*genDm.Index); ok {
			out.Indexes = append(out.Indexes, indexFromGen(idx))
		}
	}
	for _, el := range e.ValidationRulesItems() {
		if vr, ok := el.(*genDm.ValidationRule); ok {
			out.ValidationRules = append(out.ValidationRules, validationRuleFromGen(vr))
		}
	}
	for _, el := range e.EventHandlersItems() {
		eh := &domainmodel.EventHandler{}
		eh.ID = model.ID(el.ID())
		out.EventHandlers = append(out.EventHandlers, eh)
	}
	return out
}

// parseLocation converts a gen "X;Y" location string to a model.Point.
func parseLocation(s string) model.Point {
	var p model.Point
	if s == "" {
		return p
	}
	if _, err := fmt.Sscanf(s, "%d;%d", &p.X, &p.Y); err != nil {
		return model.Point{}
	}
	return p
}

// attributeFromGen converts a gen Attribute to a lossless domainmodel.Attribute
// (name, documentation, full type, and default value) so a read-modify-write
// round-trip (ALTER ENTITY) reproduces the attribute faithfully.
func attributeFromGen(a *genDm.Attribute) *domainmodel.Attribute {
	attr := &domainmodel.Attribute{
		Name:          a.Name(),
		Documentation: a.Documentation(),
		Type:          attributeTypeFromGen(a.Type()),
	}
	attr.ID = model.ID(a.ID())
	if sv, ok := a.Value().(*genDm.StoredValue); ok {
		attr.Value = &domainmodel.AttributeValue{DefaultValue: sv.DefaultValue()}
	}
	return attr
}

// validationRuleFromGen converts a gen ValidationRule to a lossless
// domainmodel.ValidationRule (attribute ref by qualified name, rule type, and
// error message text) so ALTER ENTITY preserves validations on round-trip.
func validationRuleFromGen(vr *genDm.ValidationRule) *domainmodel.ValidationRule {
	out := &domainmodel.ValidationRule{
		AttributeID: model.ID(vr.AttributeQualifiedName()), // qualified name; ruleInfoToGen handles it
		Type:        ruleTypeFromGen(vr.RuleInfo()),
	}
	out.ID = model.ID(vr.ID())
	if txt, ok := vr.ErrorMessage().(*genTexts.Text); ok {
		out.ErrorMessage = textFromGen(txt)
	}
	return out
}

// ruleTypeFromGen maps a gen RuleInfo element back to the domainmodel rule-type
// string (reverse of ruleInfoToGen, which today emits Unique/Required).
func ruleTypeFromGen(ri element.Element) string {
	if ri == nil {
		return "Required"
	}
	switch ri.TypeName() {
	case "DomainModels$UniqueRuleInfo":
		return "Unique"
	default:
		return "Required"
	}
}

// textFromGen converts a gen Text (translations) back to a model.Text.
func textFromGen(t *genTexts.Text) *model.Text {
	out := &model.Text{Translations: map[string]string{}}
	for _, el := range t.TranslationsItems() {
		if tr, ok := el.(*genTexts.Translation); ok {
			out.Translations[tr.LanguageCode()] = tr.Text()
		}
	}
	return out
}

// indexFromGen converts a gen EntityIndex to a lossless domainmodel.Index so an
// ALTER ENTITY round-trip preserves the index (segment attribute + sort order).
func indexFromGen(idx *genDm.Index) *domainmodel.Index {
	out := &domainmodel.Index{}
	out.ID = model.ID(idx.ID())
	for _, el := range idx.AttributesItems() {
		if ia, ok := el.(*genDm.IndexedAttribute); ok {
			seg := &domainmodel.IndexAttribute{
				AttributeID: model.ID(ia.AttributeRefID()),
				Ascending:   ia.Ascending(),
			}
			seg.ID = model.ID(ia.ID())
			out.Attributes = append(out.Attributes, seg)
		}
	}
	return out
}

// attributeTypeFromGen is the reverse of attributeTypeToGen: a gen attribute-type
// element back to a domainmodel.AttributeType (with Length / enumeration ref).
func attributeTypeFromGen(t element.Element) domainmodel.AttributeType {
	switch at := t.(type) {
	case *genDm.StringAttributeType:
		return &domainmodel.StringAttributeType{Length: int(at.Length())}
	case *genDm.IntegerAttributeType:
		return &domainmodel.IntegerAttributeType{}
	case *genDm.LongAttributeType:
		return &domainmodel.LongAttributeType{}
	case *genDm.DecimalAttributeType:
		return &domainmodel.DecimalAttributeType{}
	case *genDm.BooleanAttributeType:
		return &domainmodel.BooleanAttributeType{}
	case *genDm.DateTimeAttributeType:
		return &domainmodel.DateTimeAttributeType{}
	case *genDm.AutoNumberAttributeType:
		return &domainmodel.AutoNumberAttributeType{}
	case *genDm.BinaryAttributeType:
		return &domainmodel.BinaryAttributeType{}
	case *genDm.HashedStringAttributeType:
		return &domainmodel.HashedStringAttributeType{}
	case *genDm.EnumerationAttributeType:
		return &domainmodel.EnumerationAttributeType{EnumerationRef: at.EnumerationQualifiedName()}
	default:
		return &domainmodel.StringAttributeType{}
	}
}

func assocFromGen(a *genDm.Association) *domainmodel.Association {
	out := &domainmodel.Association{
		Name:     a.Name(),
		ParentID: model.ID(a.ParentRefID()), // FROM entity (owns the FK)
		ChildID:  model.ID(a.ChildRefID()),  // TO entity
	}
	out.ID = model.ID(a.ID())
	return out
}
