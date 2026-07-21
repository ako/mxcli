// SPDX-License-Identifier: Apache-2.0

package widgets

import "github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"

// GenerateFromMPK builds a complete WidgetTemplate from a parsed MPK WidgetDefinition.
// All $IDs are placeholder IDs (aa000000... prefix). loader.go's collectIDs remaps them
// to real UUIDs before BSON serialisation — matching the lifecycle of embedded templates.
//
// System properties (<systemProperty> — Name, TabIndex, Visibility, Label,
// Editability) ARE emitted, in their declared document position, as System-typed
// PropertyTypes with no corresponding Object WidgetProperty (they map to the outer
// CustomWidget's Name/TabIndex/visibility fields). mxbuild's update-widgets emits
// them and CE0463 checks the Type's PropertyTypes, so a generated widget that omits
// them (e.g. any Charts widget, which has no embedded template) drifts → CE0463.
func GenerateFromMPK(def *mpk.WidgetDefinition) *WidgetTemplate {
	typeID := placeholderID()
	objTypeID := placeholderID()

	propTypes := []any{float64(2)} // Mendix array version marker
	objProps := []any{float64(2)}

	// Iterate the full declared order (regular + system interleaved) so system
	// PropertyTypes land at their .mpk-declared position; fall back to the regular
	// list if the ordered view is unavailable.
	source := def.AllTopLevel
	if len(source) == 0 {
		source = def.Properties
	}
	for _, p := range source {
		if p.IsSystem {
			propTypes = append(propTypes, createSystemPropertyType(p))
			continue // system props have no Object WidgetProperty
		}
		bsonType := xmlTypeToBSONType(p.Type)
		if bsonType == "" {
			continue // unknown XML type — skip silently
		}
		pt, prop := createPropertyPair(p, bsonType)
		if pt != nil {
			propTypes = append(propTypes, pt)
		}
		if prop != nil {
			objProps = append(objProps, prop)
		}
	}

	platform := def.SupportedPlatform
	if platform == "" {
		platform = "Web"
	}

	typeMap := map[string]any{
		"$ID":                      typeID,
		"$Type":                    "CustomWidgets$CustomWidgetType",
		"HelpUrl":                  def.HelpURL,
		"OfflineCapable":           def.OfflineCapable,
		"StudioCategory":           def.StudioCategory,
		"StudioProCategory":        def.StudioProCategory,
		"SupportedPlatform":        platform,
		"WidgetDescription":        def.Description,
		"WidgetId":                 def.ID,
		"WidgetName":               def.Name,
		"WidgetNeedsEntityContext": def.NeedsEntityContext,
		"WidgetPluginWidget":       def.IsPluggable,
		"ObjectType": map[string]any{
			"$ID":           objTypeID,
			"$Type":         "CustomWidgets$WidgetObjectType",
			"PropertyTypes": propTypes,
		},
	}

	objectMap := map[string]any{
		"$ID":         placeholderID(),
		"$Type":       "CustomWidgets$WidgetObject",
		"TypePointer": objTypeID,
		"Properties":  objProps,
	}

	tmpl := &WidgetTemplate{
		WidgetID:  def.ID,
		Name:      def.Name,
		Version:   def.Version,
		Generated: true,
		Type:      typeMap,
		Object:    objectMap,
	}

	// createDefaultValueType emits a minimal ValueType (empty enum/returnType/etc.)
	// — for template-based widgets augment's reconcile pass fills these in from the
	// .mpk, but a generated widget skips augment. Run the same reconciliation here so
	// each PropertyType's schema-derived fields (enum option sets, expression
	// returnType, ValueType scalars) match the installed widget; otherwise a
	// generated widget (e.g. any Charts widget) drifts within-key → CE0463.
	byKey := mpkPropDefsByKey(def)
	reconcileEnumValues(tmpl.Type, mpkEnumValuesByKey(def))
	reconcilePropertyMetadata(tmpl.Type, byKey)
	reconcileValueTypesFromMPK(tmpl, byKey)
	completeValueTypeEnvelope(tmpl.Type)

	return tmpl
}

// createSystemPropertyType builds a System-typed CustomWidgets$WidgetPropertyType for
// a <systemProperty> (Name/TabIndex/Visibility/Label/Editability), matching mxbuild's
// output: Caption "<system:Key>", Category from the enclosing group chain, and a
// ValueType with Type "System" and the standard empty envelope (array markers as
// Studio Pro emits them). The placeholder $IDs are remapped by the loader's ID phase.
func createSystemPropertyType(p mpk.PropertyDef) map[string]any {
	return map[string]any{
		"$ID":         placeholderID(),
		"$Type":       "CustomWidgets$WidgetPropertyType",
		"Caption":     "<system:" + p.Key + ">",
		"Category":    p.Category,
		"Description": "",
		"IsDefault":   false,
		"PropertyKey": p.Key,
		"ValueType": map[string]any{
			"$ID":                         placeholderID(),
			"$Type":                       "CustomWidgets$WidgetValueType",
			"ActionVariables":             []any{float64(2)},
			"AllowNonPersistableEntities": false,
			"AllowUpload":                 false,
			"AllowedTypes":                []any{float64(1)},
			"AssociationTypes":            []any{float64(1)},
			"DataSourceProperty":          "",
			"DefaultType":                 "None",
			"DefaultValue":                "",
			"EntityProperty":              "",
			"EnumerationValues":           []any{float64(2)},
			"IsLinked":                    false,
			"IsList":                      false,
			"IsMetaData":                  false,
			"IsPath":                      "No",
			"Multiline":                   false,
			"ObjectType":                  nil,
			"OnChangeProperty":            "",
			"ParameterIsList":             false,
			"PathType":                    "None",
			"Required":                    false,
			"ReturnType":                  nil,
			"SelectableObjectsProperty":   "",
			"SelectionTypes":              []any{float64(1)},
			"SetLabel":                    false,
			"Translations":                []any{float64(2)},
			"Type":                        "System",
		},
	}
}
