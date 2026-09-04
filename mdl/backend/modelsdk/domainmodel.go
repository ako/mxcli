// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	genRest "github.com/mendixlabs/mxcli/modelsdk/gen/rest"
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
	out := make([]*domainmodel.DomainModel, 0, len(units)+1)
	for _, u := range units {
		out = append(out, domainModelFromGen(u.Element, u.ContainerID))
	}
	b.populateViewEntityOql(out)
	// The System module is virtual (not stored in the project); inject its domain
	// model so platform entities (System.WorkflowUserTask, User, …) resolve.
	out = append(out, buildSystemDomainModel())
	return out, nil
}

// populateViewEntityOql fills OqlQuery for view entities from their separate
// DomainModels$ViewEntitySourceDocument. On Mendix 11.0+ the OQL is stored ONLY
// in that document — the writer's version gate drops the inline
// OqlViewEntitySource.Oml field (serializeOqlViewEntitySource), so entityFromGen
// leaves OqlQuery empty and DESCRIBE VIEW ENTITY would omit the `as (…)` clause.
// Follow the SourceDocumentRef here. Mirrors the legacy reader's
// loadViewEntityOqlQueries. No-op when nothing needs it (pre-11 inline OQL, or no
// view entities).
func (b *Backend) populateViewEntityOql(dms []*domainmodel.DomainModel) {
	need := false
	for _, dm := range dms {
		for _, e := range dm.Entities {
			if e.SourceDocumentRef != "" && e.OqlQuery == "" {
				need = true
			}
		}
	}
	if !need {
		return
	}
	units, err := mprread.ListUnitsWithContainer[*genDm.ViewEntitySourceDocument](b.reader)
	if err != nil {
		return
	}
	oqlByQN := make(map[string]string, len(units))
	for _, u := range units {
		name := u.Element.Name()
		if name == "" {
			continue
		}
		mi, _ := b.reader.GetModule(string(u.ContainerID))
		if mi == nil {
			continue
		}
		oqlByQN[mi.Name+"."+name] = u.Element.Oql()
	}
	for _, dm := range dms {
		for _, e := range dm.Entities {
			if e.OqlQuery == "" && e.SourceDocumentRef != "" {
				if oql, ok := oqlByQN[e.SourceDocumentRef]; ok {
					e.OqlQuery = oql
				}
			}
		}
	}
}

// GetDomainModel returns the domain model whose container is moduleID.
func (b *Backend) GetDomainModel(moduleID model.ID) (*domainmodel.DomainModel, error) {
	units, err := mprread.ListUnitsWithContainer[*genDm.DomainModel](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if u.ContainerID == moduleID {
			dm := domainModelFromGen(u.Element, u.ContainerID)
			b.populateViewEntityOql([]*domainmodel.DomainModel{dm})
			return dm, nil
		}
	}
	// The System module is virtual — its domain model is not stored in the project,
	// so serve the injected one (matching ListDomainModels) so DESCRIBE System.* and
	// any lookup of a platform entity's domain model resolves instead of erroring.
	if sys := buildSystemDomainModel(); sys.ContainerID == moduleID {
		return sys, nil
	}
	// Match the legacy contract: a missing domain model is an error, not (nil, nil).
	// Callers (e.g. finalizeProgramExecution after a module drop) rely on the error
	// to skip — returning (nil, nil) caused a nil-pointer panic downstream.
	return nil, fmt.Errorf("domain model not found for module: %s", moduleID)
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
	// Cross-module associations live in a separate collection (the FROM entity's
	// module owns them, the TO entity is referenced BY_NAME). Without this loop
	// they were invisible to LIST/SHOW ASSOCIATIONS and DESCRIBE MODULE.
	for _, el := range dm.CrossAssociationsItems() {
		if ca, ok := el.(*genDm.CrossAssociation); ok {
			out.CrossAssociations = append(out.CrossAssociations, crossAssocFromGen(ca))
		}
	}
	// The canvas notes. Read so SHOW/DESCRIBE can list them and so an edit can
	// match an existing note rather than appending a duplicate; the WRITE path
	// still mutates the gen elements in place, so nothing unmodelled is lost.
	for _, el := range dm.AnnotationsItems() {
		if a, ok := el.(*genDm.Annotation); ok {
			out.Annotations = append(out.Annotations, annotationFromGen(a))
		}
	}
	return out
}

// annotationFromGen converts a canvas note to the semantic type.
//
// Location is the string "x;y" — the same convention an entity's Location uses,
// and what every Studio Pro-authored annotation stores. gen types it as a string
// and generated/metamodel types it as a Point; the documents agree with gen.
func annotationFromGen(a *genDm.Annotation) *domainmodel.Annotation {
	out := &domainmodel.Annotation{
		Caption: a.Caption(),
		Width:   int(a.Width()),
	}
	out.ID = model.ID(a.ID())
	if x, y, ok := parsePointString(a.Location()); ok {
		out.Location = model.Point{X: x, Y: y}
	}
	return out
}

// parsePointString reads Mendix's "x;y" position encoding.
func parsePointString(s string) (int, int, bool) {
	before, after, found := strings.Cut(s, ";")
	if !found {
		return 0, 0, false
	}
	x, errX := strconv.Atoi(strings.TrimSpace(before))
	y, errY := strconv.Atoi(strings.TrimSpace(after))
	if errX != nil || errY != nil {
		return 0, 0, false
	}
	return x, y, true
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

	// An entity's Source says where its data comes from. Surface each flavour so
	// read-modify-write paths (MOVE ENTITY reparenting a source doc, ALTER ENTITY
	// flipping a remote capability) can see it — without this an external entity
	// reads back looking local, DESCRIBE EXTERNAL ENTITY rejects it, and an update
	// rebuilds it with no source at all (mendixlabs/mxcli#782). Mirrors the legacy
	// parser in sdk/mpr/parser_domainmodel.go.
	switch src := e.Source().(type) {
	case *genDm.OqlViewEntitySource:
		out.Source = "DomainModels$OqlViewEntitySource"
		out.SourceObjectID = model.ID(src.ID())
		out.SourceDocumentRef = src.SourceDocumentQualifiedName()
		out.OqlQuery = src.Oql()

	case *genRest.ODataRemoteEntitySource:
		// Top-level external entity: has its own entity set, so it carries the
		// CRUD/paging capabilities and the local-changes flag.
		out.Source = "Rest$ODataRemoteEntitySource"
		out.SourceObjectID = model.ID(src.ID())
		out.RemoteServiceName = src.SourceDocumentQualifiedName()
		out.RemoteEntitySet = src.EntitySet()
		out.RemoteEntityName = src.RemoteName()
		out.Countable = src.Countable()
		out.Creatable = src.Creatable()
		out.Deletable = src.Deletable()
		out.SkipSupported = src.SkipSupported()
		out.TopSupported = src.TopSupported()
		out.CreateChangeLocally = src.CreateChangeLocally()
		out.RemoteKeyParts = odataKeyFromGen(src.Key())
		// Updatable has no gen accessor because the storage type has no such
		// field — updatability is per attribute (Rest$ODataMappedValue). The
		// write path does not emit it either, so leaving it zero is symmetric.

	case *genRest.ODataEntityTypeSource:
		// Derived / abstract / contained target type: no entity set, no capabilities.
		out.Source = "Rest$ODataEntityTypeSource"
		out.SourceObjectID = model.ID(src.ID())
		out.RemoteServiceName = src.SourceDocumentQualifiedName()
		out.RemoteEntityName = src.EntityTypeName()
		out.IsOpen = src.IsOpen()
		out.RemoteKeyParts = odataKeyFromGen(src.Key())

	case *genRest.ODataPrimitiveCollectionEntitySource:
		// NPE generated for a Collection(Edm.*) property; carries only the service.
		out.Source = "Rest$ODataPrimitiveCollectionEntitySource"
		out.SourceObjectID = model.ID(src.ID())
		out.RemoteServiceName = src.SourceDocumentQualifiedName()
	}

	for _, el := range e.AttributesItems() {
		if a, ok := el.(*genDm.Attribute); ok {
			out.Attributes = append(out.Attributes, attributeFromGen(a))
		}
	}
	for _, el := range e.AccessRulesItems() {
		if ar, ok := el.(*genDm.AccessRule); ok {
			out.AccessRules = append(out.AccessRules, accessRuleFromGen(ar))
		}
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
		if eh, ok := el.(*genDm.EventHandler); ok {
			out.EventHandlers = append(out.EventHandlers, eventHandlerFromGen(eh))
		}
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
	switch v := a.Value().(type) {
	case *genDm.StoredValue:
		attr.Value = &domainmodel.AttributeValue{DefaultValue: v.DefaultValue()}
	case *genDm.CalculatedValue:
		// Reading the binding back is what makes a read-modify-write safe: without
		// it every calculated attribute comes back as a plain value and the
		// writer's CalculatedValue arm never fires, so an unrelated ALTER on the
		// same entity silently converts the attribute to a stored one (#917).
		attr.Value = &domainmodel.AttributeValue{
			Type:          "CalculatedValue",
			MicroflowName: v.MicroflowQualifiedName(),
			PassEntity:    v.PassEntity(),
		}
	case *genDm.OqlViewValue:
		// View-entity attribute: the OQL column reference must survive a
		// read-modify-write (e.g. MOVE ENTITY) or the view goes out of sync (CE6770).
		attr.Value = &domainmodel.AttributeValue{ViewReference: v.Reference()}
	case *genRest.ODataMappedValue:
		// External-entity attribute: the mapping to the remote OData property.
		// Reading it back is what makes a read-modify-write safe — without it
		// every attribute of an external entity comes back unmapped, and the
		// writer's `isExternal && a.RemoteName != ""` arm falls through to a
		// plain StoredValue. The entity then no longer matches the contract:
		// "Attribute 'year' of external entity 'Stg_Season' is not supported."
		attr.RemoteName = v.RemoteName()
		attr.RemoteType = v.RemoteType()
		attr.Filterable = v.Filterable()
		attr.Sortable = v.Sortable()
		attr.Creatable = v.Creatable()
		attr.Updatable = v.Updatable()
	case *genRest.ODataMappedPrimitiveCollectionValue:
		// The single attribute of a primitive-collection NPE (issue #718).
		attr.RemoteName = v.RemoteName()
		attr.RemoteType = v.RemoteType()
		attr.IsPrimitiveCollection = true
	}
	return attr
}

// eventHandlerFromGen converts a gen EventHandler to a lossless
// domainmodel.EventHandler so ALTER ENTITY preserves entity events on round-trip.
// The gen reads the correct storage keys (Type, SendInputParameter) after the
// storage-name override; the microflow is a by-name reference.
func eventHandlerFromGen(eh *genDm.EventHandler) *domainmodel.EventHandler {
	out := &domainmodel.EventHandler{
		Moment:            domainmodel.EventMoment(eh.Moment()),
		Event:             domainmodel.EventType(eh.Event()),
		MicroflowName:     eh.MicroflowQualifiedName(),
		RaiseErrorOnFalse: eh.RaiseErrorOnFalse(),
		PassEventObject:   eh.PassEventObject(),
	}
	out.ID = model.ID(eh.ID())
	return out
}

// accessRuleFromGen converts a gen AccessRule to a lossless domainmodel.AccessRule
// (module roles by qualified name, allow flags, default member rights, XPath, and
// per-member accesses) so ALTER ENTITY preserves entity security on round-trip.
// AllowRead/AllowWrite are intentionally absent — Mendix stores read/write per
// member (MemberAccess.AccessRights), not at the rule level.
func accessRuleFromGen(ar *genDm.AccessRule) *domainmodel.AccessRule {
	out := &domainmodel.AccessRule{
		ModuleRoleNames:           ar.ModuleRolesQualifiedNames(),
		AllowCreate:               ar.AllowCreate(),
		AllowDelete:               ar.AllowDelete(),
		DefaultMemberAccessRights: domainmodel.MemberAccessRights(ar.DefaultMemberAccessRights()),
		XPathConstraint:           ar.XPathConstraint(),
	}
	out.ID = model.ID(ar.ID())
	for _, el := range ar.MemberAccessesItems() {
		if ma, ok := el.(*genDm.MemberAccess); ok {
			out.MemberAccesses = append(out.MemberAccesses, memberAccessFromGen(ma))
		}
	}
	return out
}

// memberAccessFromGen converts a gen MemberAccess (attribute/association ref by
// qualified name + access rights) to a domainmodel.MemberAccess.
func memberAccessFromGen(ma *genDm.MemberAccess) *domainmodel.MemberAccess {
	out := &domainmodel.MemberAccess{
		AttributeName:   ma.AttributeQualifiedName(),
		AssociationName: ma.AssociationQualifiedName(),
		AccessRights:    domainmodel.MemberAccessRights(ma.AccessRights()),
	}
	out.ID = model.ID(ma.ID())
	return out
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
	out.Rule = ruleInfoFromGen(vr.RuleInfo())
	return out
}

// ruleInfoFromGen carries the rule's payload back onto the model, so a
// read-modify-write can rebuild it. Without this the reader reported the right
// TYPE and dropped everything that made the rule mean something, and the writer
// then had no choice but to refuse (see ruleInfoToGen).
//
// A type with no case here reads as a nil payload, which the writer treats as a
// refusal — the safe direction.
func ruleInfoFromGen(ri element.Element) domainmodel.ValidationRuleInfo {
	switch info := ri.(type) {
	case *genDm.RegExRuleInfo:
		out := &domainmodel.RegexValidationRuleInfo{
			RegularExpressionQualifiedName: info.RegularExpressionQualifiedName(),
		}
		out.ID = model.ID(info.ID())
		return out
	case *genDm.RangeRuleInfo:
		out := &domainmodel.RangeValidationRuleInfo{
			UseMinValue:               info.UseMinValue(),
			UseMaxValue:               info.UseMaxValue(),
			MinAttributeQualifiedName: info.MinAttributeQualifiedName(),
			MaxAttributeQualifiedName: info.MaxAttributeQualifiedName(),
		}
		if v := info.MinValue(); v != "" {
			out.MinValue = &v
		}
		if v := info.MaxValue(); v != "" {
			out.MaxValue = &v
		}
		out.ID = model.ID(info.ID())
		return out
	case *genDm.RequiredRuleInfo:
		out := &domainmodel.RequiredValidationRuleInfo{}
		out.ID = model.ID(info.ID())
		return out
	case *genDm.UniqueRuleInfo:
		out := &domainmodel.UniqueValidationRuleInfo{}
		out.ID = model.ID(info.ID())
		return out
	default:
		return nil
	}
}

// ruleTypeFromGen maps a gen RuleInfo element back to the domainmodel rule-type
// string.
//
// It reports the type it actually found rather than collapsing everything to
// "Required". The old default did exactly that, and because ALTER ENTITY
// round-trips an entity through this, a stored RegEx rule was READ as Required
// and WRITTEN BACK as Required — the regex reference silently gone, the field
// merely mandatory. Any type the writers cannot reproduce is refused at the
// write, not quietly rewritten (see ruleInfoToGen).
func ruleTypeFromGen(ri element.Element) string {
	if ri == nil {
		return "Required"
	}
	// "DomainModels$RegExRuleInfo" -> "RegEx"
	name := strings.TrimSuffix(strings.TrimPrefix(ri.TypeName(), "DomainModels$"), "RuleInfo")
	if name == "" {
		return "Required"
	}
	return name
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
// odataKeyFromGen converts a Rest$ODataKey to the semantic remote-key parts. The
// inverse of odataKeyToGen in domainmodel_write.go.
func odataKeyFromGen(key element.Element) []*domainmodel.RemoteKeyPart {
	k, ok := key.(*genRest.ODataKey)
	if !ok || k == nil {
		return nil
	}
	var parts []*domainmodel.RemoteKeyPart
	for _, el := range k.PartsItems() {
		p, ok := el.(*genRest.ODataKeyPart)
		if !ok {
			continue
		}
		kp := &domainmodel.RemoteKeyPart{
			Name:       p.EntityKeyPartName(),
			RemoteName: p.Name(),
			RemoteType: p.RemoteType(),
		}
		if t := p.Type(); t != nil {
			kp.Type = attributeTypeFromGen(t)
		}
		parts = append(parts, kp)
	}
	return parts
}

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
		Name:          a.Name(),
		Documentation: a.Documentation(),
		ParentID:      model.ID(a.ParentRefID()), // FROM entity (owns the FK)
		ChildID:       model.ID(a.ChildRefID()),  // TO entity
		// Type/Owner/StorageFormat/DeleteBehavior MUST be read here: the executor's
		// CREATE OR MODIFY ASSOCIATION path re-serializes the whole domain model via
		// UpdateDomainModel→assocToGen, which reads these off the semantic model.
		// Dropping them here wiped Type/Owner on every *other* association (Studio
		// Pro then fails to load the domain model: "cannot destructure property
		// 'child' from null").
		Type:          domainmodel.AssociationType(a.Type()),
		Owner:         domainmodel.AssociationOwner(a.Owner()),
		StorageFormat: domainmodel.AssociationStorageFormat(a.StorageFormat()),
		// Line anchors, for the same reason: assocToGen rebuilds the element from
		// this struct, so anything not read here is destroyed on the next write.
		ParentConnection: domainmodel.ParseConnectionPoint(a.ParentConnection()),
		ChildConnection:  domainmodel.ParseConnectionPoint(a.ChildConnection()),
	}
	out.ID = model.ID(a.ID())
	if db, ok := a.DeleteBehavior().(*genDm.AssociationDeleteBehavior); ok && db != nil {
		out.ParentDeleteBehavior = &domainmodel.DeleteBehavior{Type: domainmodel.DeleteBehaviorType(db.ParentDeleteBehavior())}
		out.ChildDeleteBehavior = &domainmodel.DeleteBehavior{
			Type: domainmodel.DeleteBehaviorType(db.ChildDeleteBehavior()),
			// Read the refusal message back too, or DESCRIBE cannot emit it and a
			// describe -> exec round trip writes an association whose runtime will
			// not start (CapTrackV2 §1).
			ErrorMessage: deleteErrorMessageFromGen(db.ChildErrorMessage()),
		}
	}

	// Read the external (OData) source back. RemoteParentNavigationProperty in
	// particular is the only durable link from an association to the OData
	// navigation property it was generated from, and CREATE EXTERNAL ENTITIES
	// dedupes on it. Dropping it here meant a re-import could not recognise an
	// association it had itself created once the name carried a numeric suffix
	// (module-wide uniqueness turns a second `season` into `season_2`), so every
	// re-run appended two more — unbounded, and invisible to `mx check`.
	if src, ok := a.Source().(*genRest.ODataRemoteAssociationSource); ok && src != nil {
		out.Source = "Rest$ODataRemoteAssociationSource"
		out.Navigability2 = src.Navigability2()
		out.RemoteParentNavigationProperty = src.RemoteParentNavigationProperty()
		out.RemoteChildNavigationProperty = src.RemoteChildNavigationProperty()
		out.CreatableFromParent = src.CreatableFromParent()
		out.CreatableFromChild = src.CreatableFromChild()
		out.UpdatableFromParent = src.UpdatableFromParent()
		out.UpdatableFromChild = src.UpdatableFromChild()
	}
	return out
}

// crossAssocFromGen converts a gen CrossAssociation (cross-module: local FROM
// entity by ID, remote TO entity by qualified name) to the semantic model.
func crossAssocFromGen(ca *genDm.CrossAssociation) *domainmodel.CrossModuleAssociation {
	out := &domainmodel.CrossModuleAssociation{
		Name:          ca.Name(),
		Documentation: ca.Documentation(),
		ParentID:      model.ID(ca.ParentRefID()), // local FROM entity (owns the FK)
		ChildRef:      ca.ChildQualifiedName(),    // remote TO entity, BY_NAME
		Type:          domainmodel.AssociationType(ca.Type()),
		Owner:         domainmodel.AssociationOwner(ca.Owner()),
		StorageFormat: domainmodel.AssociationStorageFormat(ca.StorageFormat()),
	}
	out.ID = model.ID(ca.ID())
	if db, ok := ca.DeleteBehavior().(*genDm.AssociationDeleteBehavior); ok && db != nil {
		out.ParentDeleteBehavior = &domainmodel.DeleteBehavior{Type: domainmodel.DeleteBehaviorType(db.ParentDeleteBehavior())}
		out.ChildDeleteBehavior = &domainmodel.DeleteBehavior{
			Type: domainmodel.DeleteBehaviorType(db.ChildDeleteBehavior()),
			// Read the refusal message back too, or DESCRIBE cannot emit it and a
			// describe -> exec round trip writes an association whose runtime will
			// not start (CapTrackV2 §1).
			ErrorMessage: deleteErrorMessageFromGen(db.ChildErrorMessage()),
		}
	}
	return out
}

// deleteErrorMessageFromGen reads the en_US text out of a delete behaviour's
// error message, or "" when there is none. The message is a Texts$Text like any
// caption; MDL carries one string, and CarryTranslations puts the other
// languages back on a rewrite.
func deleteErrorMessageFromGen(el element.Element) string {
	txt, ok := el.(*genTexts.Text)
	if !ok || txt == nil {
		return ""
	}
	t := textFromGen(txt)
	if t == nil {
		return ""
	}
	if v, ok := t.Translations["en_US"]; ok {
		return v
	}
	for _, v := range t.Translations {
		return v
	}
	return ""
}
