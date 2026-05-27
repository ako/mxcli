// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// ============================================================================
// Consumed OData Service (OData Client) — Rest$ConsumedODataService
// ============================================================================

// CreateConsumedODataService creates a new consumed OData service (client) document.
func (w *Writer) CreateConsumedODataService(svc *model.ConsumedODataService) error {
	if svc.ID == "" {
		svc.ID = model.ID(generateUUID())
	}
	svc.TypeName = "Rest$ConsumedODataService"

	contents, err := w.serializeConsumedODataService(svc)
	if err != nil {
		return fmt.Errorf("failed to serialize consumed OData service: %w", err)
	}

	return w.insertUnit(string(svc.ID), string(svc.ContainerID), "Documents", "Rest$ConsumedODataService", contents)
}

// UpdateConsumedODataService updates an existing consumed OData service.
func (w *Writer) UpdateConsumedODataService(svc *model.ConsumedODataService) error {
	contents, err := w.serializeConsumedODataService(svc)
	if err != nil {
		return fmt.Errorf("failed to serialize consumed OData service: %w", err)
	}

	return w.updateUnit(string(svc.ID), contents)
}

// DeleteConsumedODataService deletes a consumed OData service by ID.
func (w *Writer) DeleteConsumedODataService(id model.ID) error {
	return w.deleteUnit(string(id))
}

// serializeConsumedODataService converts a ConsumedODataService to BSON bytes.
func (w *Writer) serializeConsumedODataService(svc *model.ConsumedODataService) ([]byte, error) {
	doc := bson.M{
		"$ID":                  idToBsonBinary(string(svc.ID)),
		"$Type":                "Rest$ConsumedODataService",
		"Name":                 svc.Name,
		"Documentation":        svc.Documentation,
		"Version":              svc.Version,
		"ServiceName":          svc.ServiceName,
		"ODataVersion":         svc.ODataVersion,
		"MetadataUrl":          svc.MetadataUrl,
		"TimeoutExpression":    svc.TimeoutExpression,
		"ProxyType":            svc.ProxyType,
		"Description":          svc.Description,
		"Validated":            svc.Validated,
		"Excluded":             svc.Excluded,
		"ExportLevel":          "Hidden",
		"Metadata":             svc.Metadata,
		"MetadataHash":         svc.MetadataHash,
		"MetadataReferences":   bson.A{int32(0)}, // empty BSON array marker
		"ValidatedEntities":    bson.A{int32(0)}, // empty BSON array marker
		"LastUpdated":          "",
		"UseQuerySegment":      false,
		"MinimumMxVersion":     "",
		"RecommendedMxVersion": "",
	}

	// Microflow references (BY_NAME). BSON storage names verified by
	// diffing Studio Pro-saved samples for both dropdown options:
	//   - "Configuration microflow" -> ConfigurationMicroflow
	//   - "Headers microflow"       -> HeadersMicroflow
	// Earlier mxcli attempts used `ConfigurationEntityMicroflow` and
	// `HeaderListMicroflow`; Studio Pro doesn't recognise either, and
	// the dropdown silently falls back to "Constants only".
	if svc.ConfigurationMicroflow != "" {
		doc["ConfigurationMicroflow"] = svc.ConfigurationMicroflow
	}
	if svc.HeadersMicroflow != "" {
		doc["HeadersMicroflow"] = svc.HeadersMicroflow
	}
	if svc.ErrorHandlingMicroflow != "" {
		doc["ErrorHandlingMicroflow"] = svc.ErrorHandlingMicroflow
	}

	// Proxy constant references (BY_NAME)
	if svc.ProxyHost != "" {
		doc["ProxyHost"] = svc.ProxyHost
	}
	if svc.ProxyPort != "" {
		doc["ProxyPort"] = svc.ProxyPort
	}
	if svc.ProxyUsername != "" {
		doc["ProxyUsername"] = svc.ProxyUsername
	}
	if svc.ProxyPassword != "" {
		doc["ProxyPassword"] = svc.ProxyPassword
	}

	// Mendix Catalog integration (optional)
	if svc.ApplicationId != "" {
		doc["ApplicationId"] = svc.ApplicationId
	}
	if svc.EndpointId != "" {
		doc["EndpointId"] = svc.EndpointId
	}
	if svc.CatalogUrl != "" {
		doc["CatalogUrl"] = svc.CatalogUrl
	}
	if svc.EnvironmentType != "" {
		doc["EnvironmentType"] = svc.EnvironmentType
	}

	// HTTP configuration (required nested part)
	doc["HttpConfiguration"] = serializeHttpConfiguration(svc.HttpConfiguration)

	return bson.Marshal(doc)
}

// serializeHttpConfiguration converts an HttpConfiguration to a BSON map.
// If cfg is nil, a minimal default configuration is created.
func serializeHttpConfiguration(cfg *model.HttpConfiguration) bson.M {
	cfgID := generateUUID()
	if cfg != nil && cfg.ID != "" {
		cfgID = string(cfg.ID)
	}

	doc := bson.M{
		"$ID":                        idToBsonBinary(cfgID),
		"$Type":                      "Microflows$HttpConfiguration",
		"UseHttpAuthentication":      false,
		"HttpAuthenticationUserName": "",
		"HttpAuthenticationPassword": "",
		"HttpMethod":                 "Post",
		"OverrideLocation":           false,
		"CustomLocation":             "",
		"ClientCertificate":          "",
	}

	if cfg != nil {
		doc["UseHttpAuthentication"] = cfg.UseAuthentication
		doc["HttpAuthenticationUserName"] = cfg.Username
		doc["HttpAuthenticationPassword"] = cfg.Password
		if cfg.HttpMethod != "" {
			doc["HttpMethod"] = cfg.HttpMethod
		}
		doc["OverrideLocation"] = cfg.OverrideLocation
		doc["CustomLocation"] = cfg.CustomLocation
		doc["ClientCertificate"] = cfg.ClientCertificate

		// Serialize header entries
		if len(cfg.HeaderEntries) > 0 {
			headers := bson.A{int32(3)}
			for _, h := range cfg.HeaderEntries {
				hID := string(h.ID)
				if hID == "" {
					hID = generateUUID()
				}
				headers = append(headers, bson.M{
					"$ID":   idToBsonBinary(hID),
					"$Type": "Microflows$HttpHeaderEntry",
					"Key":   h.Key,
					"Value": h.Value,
				})
			}
			doc["HttpHeaderEntries"] = headers
		}
	}

	return doc
}

// ============================================================================
// Published OData Service — ODataPublish$PublishedODataService2
// ============================================================================

// CreatePublishedODataService creates a new published OData service document.
func (w *Writer) CreatePublishedODataService(svc *model.PublishedODataService) error {
	if svc.ID == "" {
		svc.ID = model.ID(generateUUID())
	}
	svc.TypeName = "ODataPublish$PublishedODataService2"

	contents, err := w.serializePublishedODataService(svc)
	if err != nil {
		return fmt.Errorf("failed to serialize published OData service: %w", err)
	}

	return w.insertUnit(string(svc.ID), string(svc.ContainerID), "Documents", "ODataPublish$PublishedODataService2", contents)
}

// UpdatePublishedODataService updates an existing published OData service.
func (w *Writer) UpdatePublishedODataService(svc *model.PublishedODataService) error {
	contents, err := w.serializePublishedODataService(svc)
	if err != nil {
		return fmt.Errorf("failed to serialize published OData service: %w", err)
	}

	return w.updateUnit(string(svc.ID), contents)
}

// DeletePublishedODataService deletes a published OData service by ID.
func (w *Writer) DeletePublishedODataService(id model.ID) error {
	return w.deleteUnit(string(id))
}

// serializePublishedODataService converts a PublishedODataService to BSON bytes.
func (w *Writer) serializePublishedODataService(svc *model.PublishedODataService) ([]byte, error) {
	// Authentication types array (versioned: starts with int32(3))
	authTypes := bson.A{int32(3)}
	for _, at := range svc.AuthenticationTypes {
		authTypes = append(authTypes, at)
	}

	// Serialize entity types and build ID map for entity set pointers.
	// Issue #595: key by qualified entity name (et.Entity), not ExposedName.
	// PublishedEntitySet.EntityTypeName holds the qualified name, so keying
	// by ExposedName made the lookup return "" and EntityTypePointer was
	// never written. Studio Pro's EntitySet.Check then NREs dereferencing
	// the missing pointer and aborts the whole project checker.
	//
	// Versioned BSON arrays in Mendix start with an int32 storage marker
	// (typically 3). Without it Mendix treats the array as malformed and
	// silently drops elements after the first — observed in CE6585 firing
	// on the second entity in a multi-entity service.
	entityTypeIDMap := make(map[string]string) // qualified entity name -> entity type ID
	entityTypes := bson.A{int32(3)}
	for _, et := range svc.EntityTypes {
		etID := string(et.ID)
		if etID == "" {
			etID = generateUUID()
			et.ID = model.ID(etID)
		}
		entityTypeIDMap[et.Entity] = etID
		entityTypes = append(entityTypes, serializePublishedEntityType(et))
	}

	// Serialize entity sets with BY_ID pointers to entity types
	entitySets := bson.A{int32(3)}
	for _, es := range svc.EntitySets {
		esID := string(es.ID)
		if esID == "" {
			esID = generateUUID()
			es.ID = model.ID(esID)
		}
		// Resolve EntityTypeName to EntityType ID
		entityTypeID := entityTypeIDMap[es.EntityTypeName]
		entitySets = append(entitySets, serializePublishedEntitySet(es, entityTypeID))
	}

	doc := bson.M{
		"$ID":                     idToBsonBinary(string(svc.ID)),
		"$Type":                   "ODataPublish$PublishedODataService2",
		"Name":                    svc.Name,
		"Documentation":           svc.Documentation,
		"Path":                    svc.Path,
		"Namespace":               svc.Namespace,
		"ServiceName":             svc.ServiceName,
		"Version":                 svc.Version,
		"ODataVersion":            svc.ODataVersion,
		"Summary":                 svc.Summary,
		"Description":             svc.Description,
		"PublishAssociations":     svc.PublishAssociations,
		"UseGeneralization":       svc.UseGeneralization,
		"AuthenticationMicroflow": svc.AuthMicroflow,
		"AuthenticationTypes":     authTypes,
		"EntityTypes":             entityTypes,
		"EntitySets":              entitySets,
		"Excluded":                svc.Excluded,
		// Empty collection markers required by Studio Pro 11.10. Without
		// these fields Mendix can resolve the first entity's key but fails
		// to resolve the second's (CE6585) — observed when comparing a
		// Studio Pro-authored multi-entity service against ours.
		"Enumerations":             bson.A{int32(3)},
		"Microflows":               bson.A{int32(3)},
		"IncludeMetadataByDefault": true,
		"ReplaceIllegalChars":      false,
		"SupportsGraphQL":          false,
	}
	return bson.Marshal(doc)
}

// serializePublishedEntityType converts a PublishedEntityType to a BSON map.
func serializePublishedEntityType(et *model.PublishedEntityType) bson.M {
	// Serialize child members. Pass the owning entity's qualified name so
	// the writer can emit fully-qualified Attribute / Association BSON
	// references (Module.Entity.AttributeName) — Studio Pro and mx check
	// require these to be qualified, and using bare names made the second
	// entity's members silently fail to link in a multi-entity service.
	// Like EntityTypes / EntitySets, ChildMembers is a Mendix versioned
	// array and must start with the int32(3) storage marker.
	members := bson.A{int32(3)}
	for _, m := range et.Members {
		members = append(members, serializePublishedMember(m, et.Entity))
	}

	return bson.M{
		"$ID":          idToBsonBinary(string(et.ID)),
		"$Type":        "ODataPublish$EntityType",
		"Entity":       et.Entity,
		"ExposedName":  et.ExposedName,
		"Summary":      et.Summary,
		"Description":  et.Description,
		"ChildMembers": members,
	}
}

// serializePublishedEntitySet converts a PublishedEntitySet to a BSON map.
func serializePublishedEntitySet(es *model.PublishedEntitySet, entityTypeID string) bson.M {
	doc := bson.M{
		"$ID":                    idToBsonBinary(string(es.ID)),
		"$Type":                  "ODataPublish$EntitySet",
		"ExposedName":            es.ExposedName,
		"AlternativeExposedName": "",
		"UsePaging":              es.UsePaging,
		"PageSize":               int64(es.PageSize),
		// QueryOptions is required by Studio Pro's BSON shape for the
		// entity set to be considered valid. Without it the second
		// published entity in a multi-entity service fails to resolve
		// its key (CE6585) — see Studio Pro reference dump.
		"QueryOptions": bson.M{
			"$ID":           idToBsonBinary(generateUUID()),
			"$Type":         "ODataPublish$QueryOptions",
			"Countable":     true,
			"SkipSupported": true,
			"TopSupported":  true,
		},
	}

	// EntityTypePointer is a BY_ID reference
	if entityTypeID != "" {
		doc["EntityTypePointer"] = idToBsonBinary(entityTypeID)
	}

	// Serialize mode objects
	if es.ReadMode != "" {
		doc["ReadMode"] = serializeReadMode(es.ReadMode)
	}
	if es.InsertMode != "" {
		doc["InsertMode"] = serializeChangeMode(es.InsertMode)
	}
	if es.UpdateMode != "" {
		doc["UpdateMode"] = serializeChangeMode(es.UpdateMode)
	}
	if es.DeleteMode != "" {
		doc["DeleteMode"] = serializeChangeMode(es.DeleteMode)
	}

	return doc
}

// serializePublishedMember converts a PublishedMember to a BSON map.
// `ownerQN` is the qualified name (Module.Entity) of the EntityType this
// member belongs to. Mendix expects PublishedAttribute.Attribute and
// PublishedAssociationEnd.Association BSON values to be fully qualified —
// "Module.Entity.AttributeName" for attributes and "Module.AssociationName"
// for associations. If the AST already supplied a qualified name (contains
// a dot), it's used as-is; otherwise the owner is prepended.
func serializePublishedMember(m *model.PublishedMember, ownerQN string) bson.M {
	memberID := string(m.ID)
	if memberID == "" {
		memberID = generateUUID()
	}

	// Base fields written by Studio Pro for both attribute and association
	// members. Description/Summary stay empty in this writer; CanBeEmpty
	// defaults to !IsPartOfKey (keys are required to have a value) which
	// matches Studio Pro's convention and is required for Mendix to
	// recognise the attribute as a valid OData key (CE6585).
	doc := bson.M{
		"$ID":         idToBsonBinary(memberID),
		"ExposedName": m.ExposedName,
		"CanBeEmpty":  !m.IsPartOfKey,
		"Description": "",
		"Summary":     "",
	}

	switch m.Kind {
	case "attribute":
		doc["$Type"] = "ODataPublish$PublishedAttribute"
		doc["Attribute"] = qualifyMemberName(m.Name, ownerQN)
		doc["Filterable"] = m.Filterable
		doc["Sortable"] = m.Sortable
		doc["IsPartOfKey"] = m.IsPartOfKey
		doc["EnumerationAsString"] = false
		doc["StringAsGuid"] = false
	case "association":
		doc["$Type"] = "ODataPublish$PublishedAssociationEnd"
		// Associations live at module scope (Module.AssocName), so prepend
		// only the module portion of the owner.
		doc["Association"] = qualifyAssociationName(m.Name, ownerQN)
		// AssociationEnd carries the target entity and a separate
		// ExposedAssociationName (typically the bare assoc name). Both
		// are required by Studio Pro's BSON shape.
		doc["Entity"] = m.AssociationTargetEntity
		doc["ExposedAssociationName"] = m.ExposedAssociationName
	case "id":
		doc["$Type"] = "ODataPublish$PublishedId"
		doc["Attribute"] = qualifyMemberName(m.Name, ownerQN)
		doc["Filterable"] = m.Filterable
		doc["Sortable"] = m.Sortable
		doc["IsPartOfKey"] = m.IsPartOfKey
	default:
		// Default to attribute for unknown kinds
		doc["$Type"] = "ODataPublish$PublishedAttribute"
		doc["Attribute"] = qualifyMemberName(m.Name, ownerQN)
		doc["Filterable"] = m.Filterable
		doc["Sortable"] = m.Sortable
		doc["IsPartOfKey"] = m.IsPartOfKey
		doc["EnumerationAsString"] = false
		doc["StringAsGuid"] = false
	}

	return doc
}

// qualifyMemberName prepends the owning entity's qualified name (Module.Entity)
// to a bare attribute name. If `name` already contains a dot (already qualified)
// or `ownerQN` is empty, the original is returned unchanged.
func qualifyMemberName(name, ownerQN string) string {
	if name == "" || ownerQN == "" || strings.Contains(name, ".") {
		return name
	}
	return ownerQN + "." + name
}

// qualifyAssociationName prepends the owning entity's module to a bare
// association name. Associations live at module scope, so only the module
// portion of `ownerQN` is used.
func qualifyAssociationName(name, ownerQN string) string {
	if name == "" || ownerQN == "" || strings.Contains(name, ".") {
		return name
	}
	if idx := strings.IndexByte(ownerQN, '.'); idx > 0 {
		return ownerQN[:idx] + "." + name
	}
	return name
}

// serializeReadMode converts a read mode string to a BSON mode object.
// Accepts both parsed format ("ReadFromDatabase") and MDL format ("SOURCE").
func serializeReadMode(mode string) bson.M {
	modeID := idToBsonBinary(generateUUID())

	switch {
	case strings.EqualFold(mode, "ReadFromDatabase") || strings.EqualFold(mode, "SOURCE"):
		return bson.M{
			"$ID":   modeID,
			"$Type": "ODataPublish$ReadSource",
		}
	case strings.HasPrefix(mode, "CallMicroflow:"):
		return bson.M{
			"$ID":       modeID,
			"$Type":     "ODataPublish$CallMicroflowToRead",
			"Microflow": strings.TrimPrefix(mode, "CallMicroflow:"),
		}
	case strings.HasPrefix(mode, "MICROFLOW "):
		return bson.M{
			"$ID":       modeID,
			"$Type":     "ODataPublish$CallMicroflowToRead",
			"Microflow": strings.TrimPrefix(mode, "MICROFLOW "),
		}
	default:
		// Unknown mode — store as ReadSource
		return bson.M{
			"$ID":   modeID,
			"$Type": "ODataPublish$ReadSource",
		}
	}
}

// serializeChangeMode converts a change mode string to a BSON mode object.
// Accepts both parsed format ("ChangeFromDatabase", "NotSupported") and MDL format ("SOURCE", "NOT_SUPPORTED").
func serializeChangeMode(mode string) bson.M {
	modeID := idToBsonBinary(generateUUID())

	switch {
	case strings.EqualFold(mode, "ChangeFromDatabase") || strings.EqualFold(mode, "SOURCE"):
		return bson.M{
			"$ID":   modeID,
			"$Type": "ODataPublish$ChangeSource",
		}
	case strings.EqualFold(mode, "NotSupported") || strings.EqualFold(mode, "NOT_SUPPORTED"):
		return bson.M{
			"$ID":   modeID,
			"$Type": "ODataPublish$ChangeNotSupported",
		}
	case strings.HasPrefix(mode, "CallMicroflow:"):
		return bson.M{
			"$ID":       modeID,
			"$Type":     "ODataPublish$CallMicroflowToChange",
			"Microflow": strings.TrimPrefix(mode, "CallMicroflow:"),
		}
	case strings.HasPrefix(mode, "MICROFLOW "):
		return bson.M{
			"$ID":       modeID,
			"$Type":     "ODataPublish$CallMicroflowToChange",
			"Microflow": strings.TrimPrefix(mode, "MICROFLOW "),
		}
	default:
		// Unknown mode — store as ChangeNotSupported
		return bson.M{
			"$ID":   modeID,
			"$Type": "ODataPublish$ChangeNotSupported",
		}
	}
}
