// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/modelsdk/property"
)

func init() {
	// Consumed OData service: MetadataReferences / ValidatedEntities are empty
	// marker-0 arrays; the HttpConfiguration's header entries use marker 3.
	codec.RegisterTypeDefaults("Rest$ConsumedODataService", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"MetadataReferences": 0, "ValidatedEntities": 0},
	})
	codec.RegisterListMarker("Microflows$HttpHeaderEntry", 3)

	// Published OData service: AuthenticationTypes / EntityTypes / EntitySets /
	// Enumerations / Microflows are all marker-3 arrays; ChildMembers (mixed
	// attribute/association/id) is marker 3.
	codec.RegisterListMarker("ODataPublish$EntityType", 3)
	codec.RegisterListMarker("ODataPublish$EntitySet", 3)
	codec.RegisterListMarker("ODataPublish$PublishedAttribute", 3)
	codec.RegisterListMarker("ODataPublish$PublishedAssociationEnd", 3)
	codec.RegisterListMarker("ODataPublish$PublishedId", 3)
	// Published microflows (OData actions) and their parameters. Marker 3 like
	// every other part list in this document.
	codec.RegisterListMarker("ODataPublish$PublishedMicroflow", 3)
	codec.RegisterListMarker("ODataPublish$PublishedMicroflowParameter", 3)
	// AllowedModuleRoles empties as marker 1, not 3 — it is a BY_NAME role list,
	// not a part list. Without the entry, a service with no grants omits the key
	// entirely, and since the document is serialized wholesale that DELETES the
	// stored grants on the next modify (CE0307). An empty list is only reached
	// when the property itself wrote nothing, because ByNameRefList marks dirty
	// on Append.
	codec.RegisterTypeDefaults("ODataPublish$PublishedODataService2", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"AllowedModuleRoles": 1, "AuthenticationTypes": 3, "EntityTypes": 3, "EntitySets": 3, "Enumerations": 3, "Microflows": 3},
	})
	codec.RegisterTypeDefaults("ODataPublish$EntityType", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"ChildMembers": 3},
	})
}

// ---------------------------------------------------------------------------
// Consumed OData service (Rest$ConsumedODataService)
// ---------------------------------------------------------------------------

func (b *Backend) CreateConsumedODataService(svc *model.ConsumedODataService) error {
	if svc == nil {
		return fmt.Errorf("CreateConsumedODataService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateConsumedODataService: not connected for writing")
	}
	if svc.ID == "" {
		svc.ID = model.ID(mmpr.GenerateID())
	}
	svc.TypeName = "Rest$ConsumedODataService"
	contents, err := b.encodeConsumedODataService(svc)
	if err != nil {
		return fmt.Errorf("CreateConsumedODataService: encode: %w", err)
	}
	return b.writer.InsertUnit(string(svc.ID), string(svc.ContainerID), "Documents", "Rest$ConsumedODataService", contents)
}

func (b *Backend) UpdateConsumedODataService(svc *model.ConsumedODataService) error {
	if svc == nil {
		return fmt.Errorf("UpdateConsumedODataService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateConsumedODataService: not connected for writing")
	}
	contents, err := b.encodeConsumedODataService(svc)
	if err != nil {
		return fmt.Errorf("UpdateConsumedODataService: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(svc.ID), contents)
}

func (b *Backend) DeleteConsumedODataService(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteConsumedODataService: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// encodeConsumedODataService serializes the service with version-appropriate
// microflow BSON keys.
func (b *Backend) encodeConsumedODataService(svc *model.ConsumedODataService) ([]byte, error) {
	configKey, headersKey := b.microflowKeys()
	return (&codec.Encoder{}).Encode(consumedODataServiceToGen(svc, configKey, headersKey))
}

// microflowKeys returns the version-appropriate BSON field names for the
// consumed OData service's configuration and headers microflows (issue #728).
// Defaults to the pre-11.10 keys when the project version is unknown.
func (b *Backend) microflowKeys() (configKey, headersKey string) {
	if pv := b.ProjectVersion(); pv != nil {
		return model.ODataConfigMicroflowBSONKey(pv.MajorVersion, pv.MinorVersion),
			model.ODataHeadersMicroflowBSONKey(pv.MajorVersion, pv.MinorVersion)
	}
	return "ConfigurationMicroflow", "ConfigurationMicroflow"
}

func consumedODataServiceToGen(svc *model.ConsumedODataService, configMicroflowKey, headersMicroflowKey string) element.Element {
	g := newElem("Rest$ConsumedODataService", string(svc.ID))
	addStr(g, "Name", svc.Name)
	addStr(g, "Documentation", svc.Documentation)
	addStr(g, "Version", svc.Version)
	addStr(g, "ServiceName", svc.ServiceName)
	addStr(g, "ODataVersion", svc.ODataVersion)
	addStr(g, "MetadataUrl", svc.MetadataUrl)
	addStr(g, "TimeoutExpression", svc.TimeoutExpression)
	addStr(g, "ProxyType", svc.ProxyType)
	addStr(g, "Description", svc.Description)
	addBool(g, "Validated", svc.Validated)
	addBool(g, "Excluded", svc.Excluded)
	addStr(g, "ExportLevel", "Hidden")
	addStr(g, "Metadata", svc.Metadata)
	addStr(g, "MetadataHash", svc.MetadataHash)
	// MetadataReferences / ValidatedEntities: empty marker-0 (via TypeDefaults).
	addStr(g, "LastUpdated", "")
	addBool(g, "UseQuerySegment", false)
	addStr(g, "MinimumMxVersion", "")
	addStr(g, "RecommendedMxVersion", "")

	// Optional by-name / constant references — only emitted when set, matching the
	// legacy writer (Studio Pro omits empties).
	addStrIf(g, configMicroflowKey, svc.ConfigurationMicroflow)
	addStrIf(g, headersMicroflowKey, svc.HeadersMicroflow)
	addStrIf(g, "ErrorHandlingMicroflow", svc.ErrorHandlingMicroflow)
	addStrIf(g, "ProxyHost", svc.ProxyHost)
	addStrIf(g, "ProxyPort", svc.ProxyPort)
	addStrIf(g, "ProxyUsername", svc.ProxyUsername)
	addStrIf(g, "ProxyPassword", svc.ProxyPassword)
	addStrIf(g, "ApplicationId", svc.ApplicationId)
	addStrIf(g, "EndpointId", svc.EndpointId)
	addStrIf(g, "CatalogUrl", svc.CatalogUrl)
	addStrIf(g, "EnvironmentType", svc.EnvironmentType)

	addPart(g, "HttpConfiguration", httpConfigurationToGen(svc.HttpConfiguration))
	return g
}

func httpConfigurationToGen(cfg *model.HttpConfiguration) element.Element {
	id := ""
	if cfg != nil {
		id = string(cfg.ID)
	}
	g := newElem("Microflows$HttpConfiguration", id)
	if cfg == nil {
		addBool(g, "UseHttpAuthentication", false)
		addStr(g, "HttpAuthenticationUserName", "")
		addStr(g, "HttpAuthenticationPassword", "")
		addStr(g, "HttpMethod", "Post")
		addBool(g, "OverrideLocation", false)
		addStr(g, "CustomLocation", "")
		addStr(g, "ClientCertificate", "")
		return g
	}
	addBool(g, "UseHttpAuthentication", cfg.UseAuthentication)
	addStr(g, "HttpAuthenticationUserName", cfg.Username)
	addStr(g, "HttpAuthenticationPassword", cfg.Password)
	addStr(g, "HttpMethod", orDefault(cfg.HttpMethod, "Post"))
	addBool(g, "OverrideLocation", cfg.OverrideLocation)
	addStr(g, "CustomLocation", cfg.CustomLocation)
	addStr(g, "ClientCertificate", cfg.ClientCertificate)
	if len(cfg.HeaderEntries) > 0 {
		headers := make([]element.Element, 0, len(cfg.HeaderEntries))
		for _, h := range cfg.HeaderEntries {
			he := newElem("Microflows$HttpHeaderEntry", string(h.ID))
			addStr(he, "Key", h.Key)
			addStr(he, "Value", h.Value)
			headers = append(headers, he)
		}
		addPartList(g, "HttpHeaderEntries", headers)
	}
	return g
}

// ---------------------------------------------------------------------------
// Published OData service (ODataPublish$PublishedODataService2)
// ---------------------------------------------------------------------------

func (b *Backend) CreatePublishedODataService(svc *model.PublishedODataService) error {
	if svc == nil {
		return fmt.Errorf("CreatePublishedODataService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("CreatePublishedODataService: not connected for writing")
	}
	if svc.ID == "" {
		svc.ID = model.ID(mmpr.GenerateID())
	}
	svc.TypeName = "ODataPublish$PublishedODataService2"
	contents, err := (&codec.Encoder{}).Encode(publishedODataServiceToGen(svc))
	if err != nil {
		return fmt.Errorf("CreatePublishedODataService: encode: %w", err)
	}
	return b.writer.InsertUnit(string(svc.ID), string(svc.ContainerID), "Documents", "ODataPublish$PublishedODataService2", contents)
}

func (b *Backend) UpdatePublishedODataService(svc *model.PublishedODataService) error {
	if svc == nil {
		return fmt.Errorf("UpdatePublishedODataService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdatePublishedODataService: not connected for writing")
	}
	contents, err := (&codec.Encoder{}).Encode(publishedODataServiceToGen(svc))
	if err != nil {
		return fmt.Errorf("UpdatePublishedODataService: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(svc.ID), contents)
}

func (b *Backend) DeletePublishedODataService(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeletePublishedODataService: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

func publishedODataServiceToGen(svc *model.PublishedODataService) element.Element {
	g := newElem("ODataPublish$PublishedODataService2", string(svc.ID))
	addStr(g, "Name", svc.Name)
	addStr(g, "Documentation", svc.Documentation)
	addStr(g, "Path", svc.Path)
	addStr(g, "Namespace", svc.Namespace)
	addStr(g, "ServiceName", svc.ServiceName)
	addStr(g, "Version", svc.Version)
	addStr(g, "ODataVersion", svc.ODataVersion)
	addStr(g, "Summary", svc.Summary)
	addStr(g, "Description", svc.Description)
	addBool(g, "PublishAssociations", svc.PublishAssociations)
	addBool(g, "SupportsGraphQL", svc.SupportsGraphQL)
	addBool(g, "UseGeneralization", svc.UseGeneralization)
	addStr(g, "AuthenticationMicroflow", svc.AuthMicroflow)
	// AllowedModuleRoles is written unconditionally, marker 1, matching
	// sdk/mpr/writer_odata.go. This document is serialized wholesale, so a key
	// the serializer omits is not left alone — it is deleted, silently revoking
	// the service's access on every `create or modify`. Grants are made by a
	// separate statement (`grant access on odata service …`) and cannot be
	// restated in the create script, so nothing in the script can put them back;
	// the next build fails CE0307 "At least one allowed role must be selected".
	//
	// The §26 fix went into the mpr writer only, and this one had no reference to
	// the field at all, so the bug survived here for a full release with a green
	// suite (mxcli-formula1 §41).
	addByNameRefList(g, "AllowedModuleRoles", "", svc.AllowedModuleRoles)
	if len(svc.AuthenticationTypes) > 0 {
		addByNameRefListV3(g, "AuthenticationTypes", svc.AuthenticationTypes)
	}

	// Pre-assign entity-type IDs so entity sets can point at them by ID.
	entityTypeIDByName := make(map[string]string, len(svc.EntityTypes))
	etElems := make([]element.Element, 0, len(svc.EntityTypes))
	for _, et := range svc.EntityTypes {
		if et.ID == "" {
			et.ID = model.ID(mmpr.GenerateID())
		}
		entityTypeIDByName[et.Entity] = string(et.ID)
		etElems = append(etElems, publishedEntityTypeToGen(et))
	}
	if len(etElems) > 0 {
		addPartList(g, "EntityTypes", etElems)
	}

	esElems := make([]element.Element, 0, len(svc.EntitySets))
	for _, es := range svc.EntitySets {
		esElems = append(esElems, publishedEntitySetToGen(es, entityTypeIDByName[es.EntityTypeName]))
	}
	if len(esElems) > 0 {
		addPartList(g, "EntitySets", esElems)
	}

	if len(svc.Microflows) > 0 {
		mfElems := make([]element.Element, 0, len(svc.Microflows))
		for _, pm := range svc.Microflows {
			mfElems = append(mfElems, publishedMicroflowToGen(pm))
		}
		addPartList(g, "Microflows", mfElems)
	}

	addBool(g, "Excluded", svc.Excluded)
	// Enumerations: empty marker-3 (via TypeDefaults). Microflows too when the
	// service publishes no actions.
	addBool(g, "IncludeMetadataByDefault", true)
	addBool(g, "ReplaceIllegalChars", false)
	addBool(g, "SupportsGraphQL", false)
	return g
}

// publishedMicroflowToGen builds an ODataPublish$PublishedMicroflow — the
// element Mendix turns into an ActionImport.
//
// Property names and kinds come from the generated metamodel
// (modelsdk/gen/odatapublish): ExposedName, AlternativeExposedName, a BY_NAME
// Microflow ref, a Parameters part list, a ReturnType part, Summary and
// Description. The MicroflowParameter ref is Module.Microflow.Param — the shape
// the published-REST writer already ships.
func publishedMicroflowToGen(pm *model.PublishedMicroflow) element.Element {
	g := newElem("ODataPublish$PublishedMicroflow", string(pm.ID))
	addStr(g, "ExposedName", pm.ExposedName)
	addStr(g, "AlternativeExposedName", "")
	addStr(g, "Microflow", pm.Microflow)
	if len(pm.Parameters) > 0 {
		params := make([]element.Element, 0, len(pm.Parameters))
		for _, p := range pm.Parameters {
			params = append(params, publishedMicroflowParameterToGen(p))
		}
		addPartList(g, "Parameters", params)
	}
	if rt := dataTypeElement(pm.ReturnTypeKind, pm.ReturnTypeRef); rt != nil {
		addPart(g, "ReturnType", rt)
	}
	addStr(g, "Summary", pm.Summary)
	addStr(g, "Description", pm.Description)
	return g
}

func publishedMicroflowParameterToGen(p *model.PublishedMicroflowParameter) element.Element {
	g := newElem("ODataPublish$PublishedMicroflowParameter", string(p.ID))
	addStr(g, "ExposedName", p.ExposedName)
	addStr(g, "MicroflowParameter", p.MicroflowParameter)
	if dt := dataTypeElement(p.DataTypeKind, p.DataTypeRef); dt != nil {
		addPart(g, "DataType", dt)
	}
	addBool(g, "CanBeEmpty", p.CanBeEmpty)
	addStr(g, "Summary", p.Summary)
	addStr(g, "Description", p.Description)
	return g
}

// dataTypeElement builds a DataTypes$* element from a kind and, where the kind
// names something, its qualified name. The kind strings are the ones
// microflows.DataType.GetTypeName() produces, so the two stay in step.
func dataTypeElement(kind, ref string) element.Element {
	if kind == "" {
		return nil
	}
	switch kind {
	case "Object":
		e := newElem("DataTypes$ObjectType", "")
		addStr(e, "Entity", ref)
		return e
	case "List":
		e := newElem("DataTypes$ListType", "")
		addStr(e, "Entity", ref)
		return e
	case "Enumeration":
		e := newElem("DataTypes$EnumerationType", "")
		addStr(e, "Enumeration", ref)
		return e
	default:
		// Boolean, String, Integer, Long, Decimal, DateTime, Binary, Void …
		return newElem("DataTypes$"+kind+"Type", "")
	}
}

func publishedEntityTypeToGen(et *model.PublishedEntityType) element.Element {
	g := newElem("ODataPublish$EntityType", string(et.ID))
	addStr(g, "Entity", et.Entity)
	addStr(g, "ExposedName", et.ExposedName)
	addStr(g, "Summary", et.Summary)
	addStr(g, "Description", et.Description)
	members := make([]element.Element, 0, len(et.Members))
	for _, m := range et.Members {
		members = append(members, publishedMemberToGen(m, et.Entity))
	}
	if len(members) > 0 {
		addPartList(g, "ChildMembers", members)
	}
	return g
}

func publishedEntitySetToGen(es *model.PublishedEntitySet, entityTypeID string) element.Element {
	g := newElem("ODataPublish$EntitySet", string(es.ID))
	addStr(g, "ExposedName", es.ExposedName)
	addStr(g, "AlternativeExposedName", "")
	addBool(g, "UsePaging", es.UsePaging)
	addInt64(g, "PageSize", int64(es.PageSize))
	// Query options default to true (Mendix's own defaults) and are only turned
	// off when the author says so. Countable is not free: it forces the read
	// microflow to take a System.ODataResponse parameter and compute a count.
	// (mxcli-formula1 findings #10.3.)
	qo := newElem("ODataPublish$QueryOptions", "")
	addBool(qo, "Countable", boolOrDefault(es.Countable, true))
	addBool(qo, "SkipSupported", boolOrDefault(es.SkipSupported, true))
	addBool(qo, "TopSupported", boolOrDefault(es.TopSupported, true))
	addPart(g, "QueryOptions", qo)
	if entityTypeID != "" {
		addIDRef(g, "EntityTypePointer", model.ID(entityTypeID))
	}
	if es.ReadMode != "" {
		addPart(g, "ReadMode", odataReadModeToGen(es.ReadMode))
	}
	if es.InsertMode != "" {
		addPart(g, "InsertMode", odataChangeModeToGen(es.InsertMode))
	}
	if es.UpdateMode != "" {
		addPart(g, "UpdateMode", odataChangeModeToGen(es.UpdateMode))
	}
	if es.DeleteMode != "" {
		addPart(g, "DeleteMode", odataChangeModeToGen(es.DeleteMode))
	}
	return g
}

func publishedMemberToGen(m *model.PublishedMember, ownerQN string) element.Element {
	memberID := string(m.ID)
	switch m.Kind {
	case "association":
		g := newElem("ODataPublish$PublishedAssociationEnd", memberID)
		addStr(g, "ExposedName", m.ExposedName)
		addBool(g, "CanBeEmpty", !m.IsPartOfKey)
		addStr(g, "Description", "")
		addStr(g, "Summary", "")
		addStr(g, "Association", qualifyAssociationName(m.Name, ownerQN))
		addStr(g, "Entity", m.AssociationTargetEntity)
		// IsMany is the exposed navigation's multiplicity; without it Studio Pro
		// reports CE5022 ("changed multiplicity"). Verified against Studio Pro BSON.
		addBool(g, "IsMany", m.IsMany)
		addStr(g, "ExposedAssociationName", m.ExposedAssociationName)
		return g
	case "id":
		g := newElem("ODataPublish$PublishedId", memberID)
		addStr(g, "ExposedName", m.ExposedName)
		addBool(g, "CanBeEmpty", !m.IsPartOfKey)
		addStr(g, "Description", "")
		addStr(g, "Summary", "")
		addStr(g, "Attribute", qualifyMemberName(m.Name, ownerQN))
		addBool(g, "Filterable", m.Filterable)
		addBool(g, "Sortable", m.Sortable)
		addBool(g, "IsPartOfKey", m.IsPartOfKey)
		return g
	default: // attribute
		g := newElem("ODataPublish$PublishedAttribute", memberID)
		addStr(g, "ExposedName", m.ExposedName)
		addBool(g, "CanBeEmpty", !m.IsPartOfKey)
		addStr(g, "Description", "")
		addStr(g, "Summary", "")
		addStr(g, "Attribute", qualifyMemberName(m.Name, ownerQN))
		// EdmType is the published OData type; without it Studio Pro reports
		// CE5016 ("published as ."). Verified against Studio Pro's corrected BSON.
		addStr(g, "EdmType", m.EdmType)
		addBool(g, "Filterable", m.Filterable)
		addBool(g, "Sortable", m.Sortable)
		addBool(g, "IsPartOfKey", m.IsPartOfKey)
		addBool(g, "EnumerationAsString", m.EnumerationAsString)
		addBool(g, "StringAsGuid", false)
		return g
	}
}

func odataReadModeToGen(mode string) element.Element {
	switch {
	case strings.HasPrefix(mode, "CallMicroflow:"):
		g := newElem("ODataPublish$CallMicroflowToRead", "")
		addStr(g, "Microflow", strings.TrimPrefix(mode, "CallMicroflow:"))
		return g
	case strings.HasPrefix(mode, "MICROFLOW "):
		g := newElem("ODataPublish$CallMicroflowToRead", "")
		addStr(g, "Microflow", strings.TrimPrefix(mode, "MICROFLOW "))
		return g
	default: // ReadFromDatabase / SOURCE / unknown
		return newElem("ODataPublish$ReadSource", "")
	}
}

func odataChangeModeToGen(mode string) element.Element {
	switch {
	case strings.EqualFold(mode, "ChangeFromDatabase") || strings.EqualFold(mode, "SOURCE"):
		return newElem("ODataPublish$ChangeSource", "")
	case strings.HasPrefix(mode, "CallMicroflow:"):
		g := newElem("ODataPublish$CallMicroflowToChange", "")
		addStr(g, "Microflow", strings.TrimPrefix(mode, "CallMicroflow:"))
		return g
	case strings.HasPrefix(mode, "MICROFLOW "):
		g := newElem("ODataPublish$CallMicroflowToChange", "")
		addStr(g, "Microflow", strings.TrimPrefix(mode, "MICROFLOW "))
		return g
	default: // NotSupported / NOT_SUPPORTED / unknown
		return newElem("ODataPublish$ChangeNotSupported", "")
	}
}

// qualifyMemberName prepends the owning entity's qualified name to a bare
// attribute name (Module.Entity.Attr). Already-qualified names pass through.
func qualifyMemberName(name, ownerQN string) string {
	if name == "" || ownerQN == "" || strings.Contains(name, ".") {
		return name
	}
	return ownerQN + "." + name
}

// qualifyAssociationName prepends the owning entity's module to a bare
// association name (associations live at module scope).
func qualifyAssociationName(name, ownerQN string) string {
	if name == "" || ownerQN == "" || strings.Contains(name, ".") {
		return name
	}
	if idx := strings.IndexByte(ownerQN, '.'); idx > 0 {
		return ownerQN[:idx] + "." + name
	}
	return name
}

// addStrIf adds a string property only when the value is non-empty.
func addStrIf(b *element.Base, name, val string) {
	if val != "" {
		addStr(b, name, val)
	}
}

// addByNameRefListV3 adds a marker-3 reference-string list (qualified names),
// the form Mendix uses for a published OData service's AuthenticationTypes.
func addByNameRefListV3(b *element.Base, name string, qnames []string) {
	p := property.NewByNameRefListV3[element.Element](name, "")
	b.AddProperty(p, uint(len(b.Properties())))
	for _, qn := range qnames {
		p.Append(qn)
	}
}

// boolOrDefault resolves an optional bool: nil means "not specified".
func boolOrDefault(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}
