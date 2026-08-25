// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// Mappings over a MESSAGE DEFINITION (#263).
//
// A message definition is not a payload sample — it is derived from the DOMAIN
// MODEL. Every node names an entity or an attribute, and carries two names: the
// node's own (`Emails`) and the singular one its items take (`Email`). A mapping
// over it therefore stores BOTH path families, which is the thing that makes
// this its own builder rather than a branch of the JSON one:
//
//	value  attr=AgentCore.Email.From   json=(Array)|(Object)|From   xml=Emails|Email|From
//
// Both are computed here, from the definition. Measured on AgentCore.MesDef_Email
// / .Email, AgentCore.MesDef_WorkflowDetails / .workflow and
// Email_Connector.MD_EmailTemplate (EnquiriesManagement demo app, Mendix 11.13):
//
//	unbounded node   xml += ExposedName|ExposedItemName    json += ExposedName|(Object)
//	single node      xml += ExposedName                    json += ExposedName
//	root, unbounded  xml  = ExposedName|ExposedItemName    json  = (Array)|(Object)
//
// The root is the one asymmetry: its own name does not appear in the JSON
// projection, exactly as an array-rooted JSON structure's does not.
//
// The body is NOT optional, even though the definition fixes entity, attributes
// and names. Measured across the ten message-definition mappings in the demo
// apps, six map every exposed node and four map a strict subset (58 of 96, 27 of
// 30, 28 of 30, 26 of 27) — so a "map everything" shorthand would be lossy for
// 40% of real mappings.

// messageDefRef is a parsed Module.Collection.Definition reference.
type messageDefRef struct {
	Module     string
	Collection string
	Definition string
}

func parseMessageDefRef(ref string) (messageDefRef, bool) {
	parts := strings.Split(ref, ".")
	if len(parts) != 3 {
		return messageDefRef{}, false
	}
	for _, p := range parts {
		if p == "" {
			return messageDefRef{}, false
		}
	}
	return messageDefRef{Module: parts[0], Collection: parts[1], Definition: parts[2]}, true
}

// findMessageDefinition resolves a Module.Collection.Definition reference.
//
// An unresolvable reference is REFUSED and the available definitions listed, the
// shape #882 established for members: a reference written through unchecked
// reaches mxbuild as CE1613 and leaves the mapping bound to nothing (#259).
func findMessageDefinition(b backend.FullBackend, ref string) (*model.MessageDefinition, error) {
	parsed, ok := parseMessageDefRef(ref)
	if !ok {
		return nil, fmt.Errorf("%q is not a message definition reference — it names a "+
			"definition INSIDE a collection document, so it has three parts: "+
			"Module.Collection.Definition", ref)
	}
	collections, err := b.ListMessageDefinitionCollections()
	if err != nil {
		return nil, err
	}
	var known []string
	for _, c := range collections {
		for _, d := range c.Definitions {
			qn := fmt.Sprintf("%s.%s", c.Name, d.Name)
			known = append(known, qn)
			if strings.EqualFold(c.Name, parsed.Collection) &&
				strings.EqualFold(d.Name, parsed.Definition) {
				return d, nil
			}
		}
	}
	sort.Strings(known)
	if len(known) == 0 {
		return nil, fmt.Errorf("message definition %q not found — this project has no "+
			"message definitions", ref)
	}
	return nil, fmt.Errorf("message definition %q not found; available: %s",
		ref, strings.Join(known, ", "))
}

// messageDefChild resolves a member name against a definition node, accepting
// the exposed name or (for an unbounded node) its item name — DESCRIBE emits the
// exposed name, and both spellings identify the same node.
func messageDefChild(node *model.MessageDefinitionElement, name string) *model.MessageDefinitionElement {
	if node == nil {
		return nil
	}
	for _, c := range node.Children {
		if c.ExposedName == name {
			return c
		}
	}
	for _, c := range node.Children {
		if strings.EqualFold(c.ExposedName, name) || strings.EqualFold(c.ExposedItemName, name) {
			return c
		}
	}
	return nil
}

// messageDefMemberNames lists the spellings that would have resolved, so a
// rejection can name them instead of leaving the author to guess.
func messageDefMemberNames(node *model.MessageDefinitionElement) []string {
	var out []string
	if node == nil {
		return out
	}
	for _, c := range node.Children {
		out = append(out, c.ExposedName)
	}
	sort.Strings(out)
	return out
}

// messageDefPaths returns the (jsonPath, xmlPath) a mapping element gets for a
// definition node reached from the given parent paths.
func messageDefPaths(node *model.MessageDefinitionElement, jsonParent, xmlParent string, isRoot bool) (string, string) {
	join := func(parent, seg string) string {
		if parent == "" {
			return seg
		}
		return parent + "|" + seg
	}
	if isRoot {
		if node.Unbounded() {
			return "(Array)|(Object)", join(node.ExposedName, node.ExposedItemName)
		}
		return "(Object)", node.ExposedName
	}
	if node.Unbounded() {
		return join(join(jsonParent, node.ExposedName), "(Object)"),
			join(join(xmlParent, node.ExposedName), node.ExposedItemName)
	}
	return join(jsonParent, node.ExposedName), join(xmlParent, node.ExposedName)
}

// buildImportMappingFromMessageDefinition builds an import mapping's element
// tree against a message definition.
func buildImportMappingFromMessageDefinition(moduleName string, def *ast.ImportMappingElementDef,
	node *model.MessageDefinitionElement, jsonParent, xmlParent string, isRoot bool,
	b backend.FullBackend,
) (*model.ImportMappingElement, error) {
	jsonPath, xmlPath := messageDefPaths(node, jsonParent, xmlParent, isRoot)

	elem := &model.ImportMappingElement{
		BaseElement:    model.BaseElement{ID: model.ID(types.GenerateID())},
		ExposedName:    node.ExposedName,
		JsonPath:       jsonPath,
		XmlPath:        xmlPath,
		MinOccurs:      node.MinOccurs,
		MaxOccurs:      node.MaxOccurs,
		Nillable:       true,
		FractionDigits: -1,
		TotalDigits:    -1,
		MaxLength:      -1,
	}

	if def.Entity == "" {
		if node.Kind != "Attribute" {
			return nil, fmt.Errorf("%q is an object in the message definition, not a value — "+
				"give it an entity: create Assoc/Module.Entity = %s { ... }",
				node.ExposedName, node.ExposedName)
		}
		elem.Kind = "Value"
		elem.TypeName = "ImportMappings$ValueMappingElement"
		elem.IsKey = def.IsKey
		elem.DataType = node.PrimitiveType
		if elem.DataType == "" {
			elem.DataType = "String"
		}
		// The definition already names the attribute, qualified against the
		// entity that DECLARES it — use it rather than re-deriving, which is
		// what gets inherited members wrong (#703).
		elem.Attribute = node.Attribute
		if err := setMappingConverter(&elem.Converter, def.Converter, moduleName, b); err != nil {
			return nil, err
		}
		return elem, nil
	}

	if node.Kind != "Entity" {
		return nil, fmt.Errorf("%q is a value in the message definition, not an object — "+
			"map it as an attribute: Attr = %s", node.ExposedName, node.ExposedName)
	}

	entity := def.Entity
	if !strings.Contains(entity, ".") {
		entity = moduleName + "." + entity
	}
	// The definition says which entity it exposes; a statement that disagrees is
	// authoring a mapping that cannot bind.
	if node.Entity != "" && !strings.EqualFold(node.Entity, entity) {
		return nil, fmt.Errorf("%q exposes %s in the message definition, but the mapping says %s",
			node.ExposedName, node.Entity, entity)
	}
	assoc := def.Association
	if assoc != "" && !strings.Contains(assoc, ".") {
		assoc = moduleName + "." + assoc
	}
	handling := def.ObjectHandling
	if handling == "" {
		handling = "Create"
	}
	// The same rule as the JSON path: `find` must say what happens when the
	// object is not found (#261).
	if handling == "Find" && def.Backup == "" && def.CustomHandler == nil {
		return nil, fmt.Errorf("`find %s` does not say what to do when the object is not "+
			"found — add one of: or create, or ignore, or error", def.Entity)
	}

	elem.Kind = "Object"
	elem.TypeName = "ImportMappings$ObjectMappingElement"
	elem.Entity = entity
	elem.Association = assoc
	elem.ObjectHandling = handling
	elem.ObjectHandlingBackup = def.Backup
	elem.BackupAllowOverride = def.BackupOverridable

	for _, child := range def.Children {
		childNode := messageDefChild(node, child.JsonName)
		if childNode == nil {
			return nil, fmt.Errorf("%q is not exposed by the message definition at %q; available: %s",
				child.JsonName, node.ExposedName,
				strings.Join(messageDefMemberNames(node), ", "))
		}
		c, err := buildImportMappingFromMessageDefinition(moduleName, child, childNode,
			jsonPath, xmlPath, false, b)
		if err != nil {
			return nil, err
		}
		elem.Children = append(elem.Children, c)
	}
	return elem, nil
}

// buildExportMappingFromMessageDefinition is the export twin. The element shapes
// differ from the import side only in the object handling Mendix stores
// (Parameter at the root, Find below it) and in the model type.
func buildExportMappingFromMessageDefinition(moduleName string, def *ast.ExportMappingElementDef,
	node *model.MessageDefinitionElement, jsonParent, xmlParent string, isRoot bool,
	b backend.FullBackend,
) (*model.ExportMappingElement, error) {
	jsonPath, xmlPath := messageDefPaths(node, jsonParent, xmlParent, isRoot)

	elem := &model.ExportMappingElement{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		ExposedName: node.ExposedName,
		JsonPath:    jsonPath,
		XmlPath:     xmlPath,
		MaxOccurs:   node.MaxOccurs,
	}

	if def.Entity == "" {
		if node.Kind != "Attribute" {
			return nil, fmt.Errorf("%q is an object in the message definition, not a value — "+
				"give it an entity: Assoc/Module.Entity as %s { ... }",
				node.ExposedName, node.ExposedName)
		}
		elem.Kind = "Value"
		elem.TypeName = "ExportMappings$ValueMappingElement"
		elem.DataType = node.PrimitiveType
		if elem.DataType == "" {
			elem.DataType = "String"
		}
		elem.Attribute = node.Attribute
		if err := setMappingConverter(&elem.Converter, def.Converter, moduleName, b); err != nil {
			return nil, err
		}
		return elem, nil
	}

	if node.Kind != "Entity" {
		return nil, fmt.Errorf("%q is a value in the message definition, not an object — "+
			"map it as an attribute: %s = Attr", node.ExposedName, node.ExposedName)
	}

	entity := def.Entity
	if !strings.Contains(entity, ".") {
		entity = moduleName + "." + entity
	}
	if node.Entity != "" && !strings.EqualFold(node.Entity, entity) {
		return nil, fmt.Errorf("%q exposes %s in the message definition, but the mapping says %s",
			node.ExposedName, node.Entity, entity)
	}
	assoc := def.Association
	if assoc != "" && !strings.Contains(assoc, ".") {
		assoc = moduleName + "." + assoc
	}

	elem.Kind = "Object"
	elem.TypeName = "ExportMappings$ObjectMappingElement"
	elem.Entity = entity
	elem.Association = assoc
	elem.ObjectHandling = "Find"
	if isRoot {
		elem.ObjectHandling = "Parameter"
	}

	for _, child := range def.Children {
		childNode := messageDefChild(node, child.JsonName)
		if childNode == nil {
			return nil, fmt.Errorf("%q is not exposed by the message definition at %q; available: %s",
				child.JsonName, node.ExposedName,
				strings.Join(messageDefMemberNames(node), ", "))
		}
		c, err := buildExportMappingFromMessageDefinition(moduleName, child, childNode,
			jsonPath, xmlPath, false, b)
		if err != nil {
			return nil, err
		}
		elem.Children = append(elem.Children, c)
	}
	return elem, nil
}

// setMappingConverter resolves a value element's converter microflow (#266).
//
// The reference is REFUSED when the microflow does not exist, rather than being
// written through: mxbuild reports an unresolvable converter as CE1613 and the
// mapping silently loses the transform, which is the same failure mode as an
// unresolvable schema source (#259).
func setMappingConverter(dst *string, converter, moduleName string, b backend.FullBackend) error {
	if converter == "" {
		return nil
	}
	if !strings.Contains(converter, ".") {
		converter = moduleName + "." + converter
	}
	parts := strings.SplitN(converter, ".", 2)
	mfs, err := b.ListMicroflows()
	if err != nil {
		// No microflow list to check against — take the name at face value
		// rather than refusing something that may well be right.
		*dst = converter
		return nil
	}
	for _, mf := range mfs {
		if strings.EqualFold(mf.Name, parts[1]) {
			*dst = converter
			return nil
		}
	}
	return fmt.Errorf("converter microflow %q not found", converter)
}
