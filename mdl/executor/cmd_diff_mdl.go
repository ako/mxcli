// SPDX-License-Identifier: Apache-2.0

// Package executor - MDL generation functions for diff (statement→text and project→text converters)
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ============================================================================
// Statement to MDL Converters
// ============================================================================

// entityStmtToMDL converts a CreateEntityStmt to MDL text
func entityStmtToMDL(ctx *ExecContext, s *ast.CreateEntityStmt) string {
	var lines []string

	// Documentation
	if s.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+s.Documentation)
		lines = append(lines, " */")
	}

	// Position annotation
	if s.Position != nil {
		lines = append(lines, fmt.Sprintf("@Position(%d, %d)", s.Position.X, s.Position.Y))
	}

	// Entity type. EntityKind.String() is upper case for error messages; the
	// project side of the diff writes the keyword in lower case, and an
	// unmodified describe dump was reported as modified purely on that casing
	// (#997, the same two-renderer split as the flow body).
	entityType := strings.ToLower(s.Kind.String())
	lines = append(lines, fmt.Sprintf("create %s entity %s (", entityType, s.Name))

	// Attributes
	for i, attr := range s.Attributes {
		// Attribute documentation
		if attr.Documentation != "" {
			lines = append(lines, fmt.Sprintf("  /** %s */", attr.Documentation))
		}

		typeStr := dataTypeToString(ctx, attr.Type)
		constraints := ""

		if attr.NotNull {
			constraints += " not null"
			if attr.NotNullError != "" {
				constraints += fmt.Sprintf(" error '%s'", attr.NotNullError)
			}
		}
		if attr.Unique {
			constraints += " unique"
			if attr.UniqueError != "" {
				constraints += fmt.Sprintf(" error '%s'", attr.UniqueError)
			}
		}
		if attr.HasDefault {
			defaultVal := fmt.Sprintf("%v", attr.DefaultValue)
			if attr.Type.Kind == ast.TypeString {
				defaultVal = fmt.Sprintf("'%s'", attr.DefaultValue)
			}
			constraints += fmt.Sprintf(" default %s", defaultVal)
		}

		comma := ","
		if i == len(s.Attributes)-1 {
			comma = ""
		}
		lines = append(lines, fmt.Sprintf("  %s: %s%s%s", attr.Name, typeStr, constraints, comma))
	}

	lines = append(lines, ")")

	// Indexes
	for _, idx := range s.Indexes {
		var cols []string
		for _, col := range idx.Columns {
			colStr := col.Name
			if col.Descending {
				colStr += " desc"
			}
			cols = append(cols, colStr)
		}
		lines = append(lines, fmt.Sprintf("index (%s)", strings.Join(cols, ", ")))
	}

	lines = append(lines, ";")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// viewEntityStmtToMDL converts a CreateViewEntityStmt to MDL text
func viewEntityStmtToMDL(ctx *ExecContext, s *ast.CreateViewEntityStmt) string {
	var lines []string

	if s.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+s.Documentation)
		lines = append(lines, " */")
	}

	if s.Position != nil {
		lines = append(lines, fmt.Sprintf("@Position(%d, %d)", s.Position.X, s.Position.Y))
	}

	lines = append(lines, fmt.Sprintf("create view entity %s (", s.Name))

	for i, attr := range s.Attributes {
		typeStr := dataTypeToString(ctx, attr.Type)
		comma := ","
		if i == len(s.Attributes)-1 {
			comma = ""
		}
		lines = append(lines, fmt.Sprintf("  %s: %s%s", attr.Name, typeStr, comma))
	}

	lines = append(lines, ") as (")
	// Indent OQL query
	for line := range strings.SplitSeq(s.Query.RawQuery, "\n") {
		lines = append(lines, "  "+line)
	}
	lines = append(lines, ");")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// enumerationStmtToMDL converts a CreateEnumerationStmt to MDL text
func enumerationStmtToMDL(ctx *ExecContext, s *ast.CreateEnumerationStmt) string {
	var lines []string

	if s.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+s.Documentation)
		lines = append(lines, " */")
	}

	lines = append(lines, fmt.Sprintf("create enumeration %s (", s.Name))

	for i, v := range s.Values {
		comma := ","
		if i == len(s.Values)-1 {
			comma = ""
		}
		lines = append(lines, fmt.Sprintf("  %s '%s'%s", v.Name, v.Caption, comma))
	}

	lines = append(lines, ");")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// associationStmtToMDL converts a CreateAssociationStmt to MDL text
func associationStmtToMDL(ctx *ExecContext, s *ast.CreateAssociationStmt) string {
	var lines []string

	if s.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+s.Documentation)
		lines = append(lines, " */")
	}

	lines = append(lines, fmt.Sprintf("create association %s", s.Name))
	lines = append(lines, fmt.Sprintf("from %s to %s", s.Parent, s.Child))

	assocType := "Reference"
	if s.Type == ast.AssocReferenceSet {
		assocType = "ReferenceSet"
	}
	lines = append(lines, fmt.Sprintf("type %s", assocType))

	owner := "Default"
	if s.Owner == ast.OwnerBoth {
		owner = "Both"
	}
	lines = append(lines, fmt.Sprintf("owner %s", owner))

	deleteBehavior := "DELETE_BUT_KEEP_REFERENCES"
	switch s.DeleteBehavior {
	case ast.DeleteCascade:
		deleteBehavior = "DELETE_AND_REFERENCES"
	case ast.DeleteIfNoReferences:
		deleteBehavior = "DELETE_IF_NO_REFERENCES"
	}
	lines = append(lines, fmt.Sprintf("delete_behavior %s;", deleteBehavior))
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// ============================================================================
// Project to MDL Converters
// ============================================================================

// entityToMDL converts a project entity to MDL text
func entityToMDL(ctx *ExecContext, moduleName string, entity *domainmodel.Entity, dm *domainmodel.DomainModel) string {
	var lines []string

	// Documentation
	if entity.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+entity.Documentation)
		lines = append(lines, " */")
	}

	// Position
	lines = append(lines, fmt.Sprintf("@Position(%d, %d)", entity.Location.X, entity.Location.Y))

	// Entity type
	entityType := "persistent"
	if strings.Contains(entity.Source, "OqlView") {
		entityType = "view"
	} else if !entity.Persistable {
		entityType = "non-persistent"
	}

	lines = append(lines, fmt.Sprintf("create %s entity %s.%s (", entityType, moduleName, entity.Name))

	// Build validation rules map
	validationsByAttr := make(map[model.ID][]*domainmodel.ValidationRule)
	validationsByName := make(map[string][]*domainmodel.ValidationRule)
	for _, vr := range entity.ValidationRules {
		validationsByAttr[vr.AttributeID] = append(validationsByAttr[vr.AttributeID], vr)
		attrName := extractAttrNameFromQualified(string(vr.AttributeID))
		if attrName != "" {
			validationsByName[attrName] = append(validationsByName[attrName], vr)
		}
	}

	// Attributes
	for i, attr := range entity.Attributes {
		// Documentation
		if attr.Documentation != "" {
			lines = append(lines, fmt.Sprintf("  /** %s */", attr.Documentation))
		}

		typeStr := formatAttributeType(attr.Type)
		var constraints strings.Builder

		// Check for validation rules
		attrValidations := validationsByAttr[attr.ID]
		if len(attrValidations) == 0 {
			attrValidations = validationsByName[attr.Name]
		}
		for _, vr := range attrValidations {
			if vr.Type == "Required" {
				constraints.WriteString(" not null")
				if vr.ErrorMessage != nil {
					errMsg := vr.ErrorMessage.GetTranslation("en_US")
					if errMsg != "" {
						constraints.WriteString(fmt.Sprintf(" error '%s'", errMsg))
					}
				}
			}
			if vr.Type == "Unique" {
				constraints.WriteString(" unique")
				if vr.ErrorMessage != nil {
					errMsg := vr.ErrorMessage.GetTranslation("en_US")
					if errMsg != "" {
						constraints.WriteString(fmt.Sprintf(" error '%s'", errMsg))
					}
				}
			}
		}

		// Default value
		if attr.Value != nil && attr.Value.DefaultValue != "" {
			defaultVal := attr.Value.DefaultValue
			if _, ok := attr.Type.(*domainmodel.StringAttributeType); ok {
				defaultVal = fmt.Sprintf("'%s'", defaultVal)
			}
			constraints.WriteString(fmt.Sprintf(" default %s", defaultVal))
		}

		comma := ","
		if i == len(entity.Attributes)-1 {
			comma = ""
		}
		lines = append(lines, fmt.Sprintf("  %s: %s%s%s", attr.Name, typeStr, constraints.String(), comma))
	}

	lines = append(lines, ")")

	// Build attr name map for indexes
	attrNames := make(map[model.ID]string)
	for _, attr := range entity.Attributes {
		attrNames[attr.ID] = attr.Name
	}

	// Indexes
	for _, idx := range entity.Indexes {
		var cols []string
		for _, ia := range idx.Attributes {
			colName := attrNames[ia.AttributeID]
			if !ia.Ascending {
				colName += " desc"
			}
			cols = append(cols, colName)
		}
		if len(cols) > 0 {
			lines = append(lines, fmt.Sprintf("index (%s)", strings.Join(cols, ", ")))
		}
	}

	lines = append(lines, ";")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// viewEntityFromProjectToMDL converts a view entity from project to MDL
func viewEntityFromProjectToMDL(ctx *ExecContext, moduleName string, entity *domainmodel.Entity, dm *domainmodel.DomainModel) string {
	var lines []string

	if entity.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+entity.Documentation)
		lines = append(lines, " */")
	}

	lines = append(lines, fmt.Sprintf("@Position(%d, %d)", entity.Location.X, entity.Location.Y))
	lines = append(lines, fmt.Sprintf("create view entity %s.%s (", moduleName, entity.Name))

	for i, attr := range entity.Attributes {
		typeStr := formatAttributeType(attr.Type)
		comma := ","
		if i == len(entity.Attributes)-1 {
			comma = ""
		}
		lines = append(lines, fmt.Sprintf("  %s: %s%s", attr.Name, typeStr, comma))
	}

	lines = append(lines, ") as (")
	if entity.OqlQuery != "" {
		for line := range strings.SplitSeq(entity.OqlQuery, "\n") {
			lines = append(lines, "  "+line)
		}
	}
	lines = append(lines, ");")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// enumerationToMDL converts a project enumeration to MDL text
func enumerationToMDL(ctx *ExecContext, moduleName string, enum *model.Enumeration) string {
	var lines []string

	if enum.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+enum.Documentation)
		lines = append(lines, " */")
	}

	lines = append(lines, fmt.Sprintf("create enumeration %s.%s (", moduleName, enum.Name))

	for i, v := range enum.Values {
		comma := ","
		if i == len(enum.Values)-1 {
			comma = ""
		}
		caption := ""
		if v.Caption != nil {
			caption = v.Caption.GetTranslation("en_US")
		}
		lines = append(lines, fmt.Sprintf("  %s '%s'%s", v.Name, caption, comma))
	}

	lines = append(lines, ");")
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// associationToMDL converts a project association to MDL text
func associationToMDL(ctx *ExecContext, moduleName string, assoc *domainmodel.Association, dm *domainmodel.DomainModel) string {
	var lines []string

	// Build entity name map
	entityNames := make(map[model.ID]string)
	for _, entity := range dm.Entities {
		entityNames[entity.ID] = entity.Name
	}

	if assoc.Documentation != "" {
		lines = append(lines, "/**")
		lines = append(lines, " * "+assoc.Documentation)
		lines = append(lines, " */")
	}

	fromEntity := entityNames[assoc.ParentID]
	toEntity := entityNames[assoc.ChildID]

	lines = append(lines, fmt.Sprintf("create association %s.%s", moduleName, assoc.Name))
	lines = append(lines, fmt.Sprintf("from %s.%s to %s.%s", moduleName, fromEntity, moduleName, toEntity))

	assocType := "Reference"
	if assoc.Type == domainmodel.AssociationTypeReferenceSet {
		assocType = "ReferenceSet"
	}
	lines = append(lines, fmt.Sprintf("type %s", assocType))

	owner := "Default"
	if assoc.Owner == domainmodel.AssociationOwnerBoth {
		owner = "Both"
	}
	lines = append(lines, fmt.Sprintf("owner %s", owner))

	deleteBehavior := "DELETE_BUT_KEEP_REFERENCES"
	if assoc.ChildDeleteBehavior != nil {
		switch assoc.ChildDeleteBehavior.Type {
		case domainmodel.DeleteBehaviorTypeDeleteMeAndReferences:
			deleteBehavior = "DELETE_AND_REFERENCES"
		case domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences:
			deleteBehavior = "DELETE_IF_NO_REFERENCES"
		}
	}
	lines = append(lines, fmt.Sprintf("delete_behavior %s;", deleteBehavior))
	lines = append(lines, "/")

	return strings.Join(lines, "\n")
}

// ============================================================================
// Helper Functions
// ============================================================================

// dataTypeToString converts a DataType to its string representation
func dataTypeToString(_ *ExecContext, dt ast.DataType) string {
	switch dt.Kind {
	case ast.TypeString:
		if dt.Length > 0 {
			return fmt.Sprintf("String(%d)", dt.Length)
		}
		return "String"
	case ast.TypeInteger:
		return "Integer"
	case ast.TypeLong:
		return "Long"
	case ast.TypeDecimal:
		return "Decimal"
	case ast.TypeBoolean:
		return "Boolean"
	case ast.TypeDateTime:
		return "DateTime"
	case ast.TypeDate:
		return "Date"
	case ast.TypeAutoNumber:
		return "AutoNumber"
	case ast.TypeBinary:
		return "Binary"
	case ast.TypeEnumeration:
		if dt.EnumRef != nil {
			return fmt.Sprintf("Enumeration(%s)", dt.EnumRef.String())
		}
		return "Enumeration"
	case ast.TypeEntity:
		if dt.EntityRef != nil {
			return dt.EntityRef.String()
		}
		return "Object"
	case ast.TypeListOf:
		if dt.EntityRef != nil {
			return fmt.Sprintf("List of %s", dt.EntityRef.String())
		}
		return "List"
	case ast.TypeVoid:
		return "Void"
	default:
		return "Unknown"
	}
}
