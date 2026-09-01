// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// parseImportMapping parses an ImportMappings$ImportMapping unit from BSON.
func (r *Reader) parseImportMapping(unitID, containerID string, contents []byte) (*model.ImportMapping, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	im := &model.ImportMapping{}
	im.ID = model.ID(unitID)
	im.TypeName = "ImportMappings$ImportMapping"
	im.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		im.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		im.Documentation = doc
	}
	if excluded, ok := raw["Excluded"].(bool); ok {
		im.Excluded = excluded
	}
	if exportLevel, ok := raw["ExportLevel"].(string); ok {
		im.ExportLevel = exportLevel
	}
	if v, ok := raw["JsonStructure"].(string); ok {
		im.JsonStructure = v
	}
	if v, ok := raw["XmlSchema"].(string); ok {
		im.XmlSchema = v
	}
	if v, ok := raw["MessageDefinition"].(string); ok {
		im.MessageDefinition = v
	}
	im.WebServiceSource = parseWebServiceSource(raw)
	// MessageDefinition2 is version-introduced (11.10+) and carried, not derived:
	// nil means the stored document does not have the key (ako/mxcli#279).
	if v, ok := raw["MessageDefinition2"].(string); ok {
		im.MessageDefinition2 = &v
	}
	// The mapping's input object (#265). Only DataTypes$ObjectType carries an
	// entity — the DataTypes$UnknownType marker an unparameterised mapping
	// stores means "none".
	if pt, ok := raw["ParameterType"].(map[string]any); ok {
		if t, _ := pt["$Type"].(string); t == "DataTypes$ObjectType" {
			if e, _ := pt["Entity"].(string); e != "" {
				im.ParameterEntity = e
			}
		}
	}

	// Parse top-level mapping elements (may start with int32 version prefix)
	if elements, ok := raw["Elements"].(bson.A); ok {
		for _, e := range elements {
			if elemMap, ok := e.(map[string]any); ok {
				elem := parseImportMappingElement(elemMap)
				if elem != nil {
					im.Elements = append(im.Elements, elem)
				}
			}
		}
	}

	return im, nil
}

// parseImportMappingElement dispatches to the correct parser based on $Type.
func parseImportMappingElement(raw map[string]any) *model.ImportMappingElement {
	typeName, _ := raw["$Type"].(string)
	switch typeName {
	case "ImportMappings$ObjectMappingElement":
		return parseImportObjectMappingElement(raw)
	case "ImportMappings$ValueMappingElement":
		return parseImportValueMappingElement(raw)
	default:
		return nil
	}
}

func parseImportObjectMappingElement(raw map[string]any) *model.ImportMappingElement {
	elem := &model.ImportMappingElement{Kind: "Object"}

	if id := extractBsonID(raw["$ID"]); id != "" {
		elem.ID = model.ID(id)
	}
	elem.TypeName = "ImportMappings$ObjectMappingElement"

	if v, ok := raw["Entity"].(string); ok {
		elem.Entity = v
	}
	if v, ok := raw["ExposedName"].(string); ok {
		elem.ExposedName = v
	}
	if v, ok := raw["JsonPath"].(string); ok {
		elem.JsonPath = v
	}
	if v, ok := raw["XmlPath"].(string); ok {
		elem.XmlPath = v
	}
	if v, ok := raw["Converter"].(string); ok {
		elem.Converter = v
	}
	if v, ok := raw["ObjectHandling"].(string); ok {
		elem.ObjectHandling = v
		if v == "Find" {
			if backup, ok := raw["ObjectHandlingBackup"].(string); ok && backup == "Create" {
				elem.ObjectHandling = "FindOrCreate"
			}
		}
	}
	// The backup is what the element does when the object is NOT found, and it
	// is carried in its own right now that MDL can say `or ignore` / `or error`
	// (#261). FindOrCreate above stays as the shorthand for Find + Create.
	if v, ok := raw["ObjectHandlingBackup"].(string); ok {
		elem.ObjectHandlingBackup = v
	}
	if v, ok := raw["ObjectHandlingBackupAllowOverride"].(bool); ok {
		elem.BackupAllowOverride = v
	}
	if v, ok := raw["Association"].(string); ok {
		elem.Association = v
	}
	elem.MinOccurs = extractInt(raw["MinOccurs"])
	elem.MaxOccurs = extractInt(raw["MaxOccurs"])

	// Parse children recursively (mix of object and value elements)
	if children, ok := raw["Children"].(bson.A); ok {
		for _, c := range children {
			if childMap, ok := c.(map[string]any); ok {
				child := parseImportMappingElement(childMap)
				if child != nil {
					elem.Children = append(elem.Children, child)
				}
			}
		}
	}

	return elem
}

func parseImportValueMappingElement(raw map[string]any) *model.ImportMappingElement {
	elem := &model.ImportMappingElement{Kind: "Value"}

	if id := extractBsonID(raw["$ID"]); id != "" {
		elem.ID = model.ID(id)
	}
	elem.TypeName = "ImportMappings$ValueMappingElement"

	if v, ok := raw["Attribute"].(string); ok {
		elem.Attribute = v
	}
	if v, ok := raw["ExposedName"].(string); ok {
		elem.ExposedName = v
	}
	if v, ok := raw["JsonPath"].(string); ok {
		elem.JsonPath = v
	}
	if v, ok := raw["XmlPath"].(string); ok {
		elem.XmlPath = v
	}
	if v, ok := raw["Converter"].(string); ok {
		elem.Converter = v
	}
	if v, ok := raw["IsKey"].(bool); ok {
		elem.IsKey = v
	}
	elem.MinOccurs = extractInt(raw["MinOccurs"])
	elem.MaxOccurs = extractInt(raw["MaxOccurs"])

	// Extract the primitive type from the nested Type object
	if typeObj, ok := raw["Type"].(map[string]any); ok {
		elem.DataType = extractPrimitiveTypeName(typeObj)
	}

	return elem
}

// extractPrimitiveTypeName converts a DataTypes$* BSON type object to a simple type string.
func extractPrimitiveTypeName(typeObj map[string]any) string {
	typeName, _ := typeObj["$Type"].(string)
	switch typeName {
	case "DataTypes$StringType":
		return "String"
	case "DataTypes$IntegerType":
		return "Integer"
	case "DataTypes$LongType":
		return "Long"
	case "DataTypes$DecimalType":
		return "Decimal"
	case "DataTypes$BooleanType":
		return "Boolean"
	case "DataTypes$DateTimeType":
		return "DateTime"
	case "DataTypes$BinaryType":
		return "Binary"
	default:
		return "String"
	}
}

// parseWebServiceSource reads a mapping's SOAP binding.
//
// Read-only, and read for one reason: a rewrite that dropped these keys turned a
// working integration into CE6896 + CE0270. ImportedWebService is stored under
// `wsdlFile`'s SDK name — the BSON key is ImportedWebService — and the root
// element under RootElementName (`xsdRootElementName` in the SDK).
func parseWebServiceSource(raw map[string]any) model.WebServiceMappingSource {
	var w model.WebServiceMappingSource
	if v, ok := raw["ImportedWebService"].(string); ok {
		w.ImportedWebService = v
	}
	if v, ok := raw["ServiceName"].(string); ok {
		w.ServiceName = v
	}
	if v, ok := raw["OperationName"].(string); ok {
		w.OperationName = v
	}
	if v, ok := raw["RootElementName"].(string); ok {
		w.RootElementName = v
	}
	if v, ok := raw["ParameterName"].(string); ok {
		w.ParameterName = v
	}
	if v, ok := raw["IsHeader"].(bool); ok {
		w.IsHeader = v
	}
	return w
}
