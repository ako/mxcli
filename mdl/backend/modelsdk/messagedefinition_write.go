// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMsg "github.com/mendixlabs/mxcli/modelsdk/gen/messagedefinitions"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// Writing a message definition collection (ako/mxcli#272).
//
// Almost every stored property is a constant or is derived — measured across
// all 36 collections / 56 definitions / 4,686 elements in the demo corpus, with
// no exceptions:
//
//	MinOccurs        0            always
//	Nillable         true         always
//	IsDefaultType    false        always
//	MaxLength        -1           always
//	FractionDigits   -1           always
//	TotalDigits      -1           always
//	Documentation    ""           always (as are ErrorMessage/WarningMessage)
//
// Example is the exception and is CARRIED: it is author-set free text, rare (1
// of 4,707 elements across the corpus and ako/TestApp) but real, and hardcoding
// it empty would silently drop the one that exists.
//	ElementType      Object/Value derived from the node's kind
//	PrimitiveType    Unknown for an object, the attribute's type for a value
//	OriginalName     the entity / attribute / target entity's own name
//	Path             the position in the tree — see messagePathSegment
//	ExposedItemName  OriginalName when MaxOccurs is -1, "" otherwise (461/461)
//
// So the caller supplies only what a person chooses, and this fills in the rest.

func init() {
	// Studio Pro writes typed-array marker 2 on these lists; the codec's default
	// is 3. Measured across the demo corpus and ako/TestApp — 37 definition
	// lists and 575 child lists, all marker 2 — and caught by the round-trip
	// test against a real document, which is the only thing that would have.
	codec.RegisterListMarker("MessageDefinitions$EntityMessageDefinition", 2)
	codec.RegisterListMarker("MessageDefinitions$ExposedEntity", 2)
	codec.RegisterListMarker("MessageDefinitions$ExposedAssociation", 2)
	codec.RegisterListMarker("MessageDefinitions$ExposedAttribute", 2)

	// Every exposed element serializes Children even when it has none — a leaf
	// attribute stores the bare [2]. Same MandatoryLists rule as a rule
	// document's Flows and a custom handler's ParameterMappings; omitting it is
	// a diff against every stored document, which is exactly what the
	// round-trip against ako/TestApp reported before this was here.
	for _, t := range []string{
		"MessageDefinitions$ExposedEntity",
		"MessageDefinitions$ExposedAssociation",
		"MessageDefinitions$ExposedAttribute",
	} {
		codec.RegisterTypeDefaults(t, codec.TypeDefaults{
			MandatoryListMarkers: map[string]int32{"Children": 2},
		})
	}
}

// element constants, named rather than repeated so the measurement above has one
// place to be wrong.
const (
	mdMinOccurs      = 0
	mdUnbounded      = -1
	mdSingle         = 1
	mdNillable       = true
	mdIsDefaultType  = false
	mdUnsetPrecision = -1
)

// CreateMessageDefinitionCollection writes a new collection.
func (b *Backend) CreateMessageDefinitionCollection(c *model.MessageDefinitionCollection) error {
	if c == nil {
		return fmt.Errorf("CreateMessageDefinitionCollection: nil collection")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateMessageDefinitionCollection: not connected for writing")
	}
	if c.ID == "" {
		c.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := encodeMessageDefinitionCollection(c)
	if err != nil {
		return err
	}
	return b.writer.InsertUnit(string(c.ID), string(c.ContainerID), "Documents",
		"MessageDefinitions$MessageDefinitionCollection", contents)
}

// UpdateMessageDefinitionCollection rewrites a collection in place, preserving
// its ID — mappings reference the collection by qualified name, and a fresh
// document would break every `WITH MESSAGE DEFINITION`.
func (b *Backend) UpdateMessageDefinitionCollection(c *model.MessageDefinitionCollection) error {
	if c == nil {
		return fmt.Errorf("UpdateMessageDefinitionCollection: nil collection")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateMessageDefinitionCollection: not connected for writing")
	}
	contents, err := encodeMessageDefinitionCollection(c)
	if err != nil {
		return err
	}
	return b.writer.UpdateRawUnit(string(c.ID), contents)
}

// DeleteMessageDefinitionCollection removes a collection by ID.
func (b *Backend) DeleteMessageDefinitionCollection(id string) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteMessageDefinitionCollection: not connected for writing")
	}
	return b.writer.DeleteUnit(id)
}

func encodeMessageDefinitionCollection(c *model.MessageDefinitionCollection) ([]byte, error) {
	g, err := messageCollectionToGen(c)
	if err != nil {
		return nil, err
	}
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return nil, fmt.Errorf("message definition collection %s: encode: %w", c.Name, err)
	}
	return contents, nil
}

func messageCollectionToGen(c *model.MessageDefinitionCollection) (*genMsg.MessageDefinitionCollection, error) {
	g := genMsg.NewMessageDefinitionCollection()
	g.SetID(element.ID(c.ID))
	g.SetName(c.Name)
	g.SetDocumentation(c.Documentation)
	g.SetExcluded(c.Excluded)
	exportLevel := c.ExportLevel
	if exportLevel == "" {
		exportLevel = "Hidden" // what every collection in the corpus stores
	}
	g.SetExportLevel(exportLevel)

	for _, def := range c.Definitions {
		if def == nil {
			continue
		}
		d := genMsg.NewEntityMessageDefinition()
		d.SetName(def.Name)
		d.SetDocumentation("")
		root, err := messageNodeToGen(def.Root, "")
		if err != nil {
			return nil, err
		}
		if root == nil {
			return nil, fmt.Errorf("message definition %s has no root element", def.Name)
		}
		d.SetExposedEntity(root)
		g.AddMessageDefinitions(d)
	}
	return g, nil
}

// messageNodeToGen converts one node, filling in everything derived.
//
// parentPath is the enclosing node's Path.
func messageNodeToGen(n *model.MessageDefinitionElement, parentPath string) (element.Element, error) {
	if n == nil {
		return nil, nil
	}
	path := messagePath(parentPath, n)

	if n.Kind == "Attribute" {
		a := genMsg.NewExposedAttribute()
		a.SetAttributeQualifiedName(n.Attribute)
		applyExposedCommon(a, n, path)
		a.SetElementType("Value")
		a.SetPrimitiveType(n.PrimitiveType)
		a.SetMaxOccurs(mdSingle) // a value never repeats: 3697/3697
		a.SetExposedItemName("")
		return a, nil
	}

	// An object node is an ExposedEntity at the root of a definition and an
	// ExposedAssociation everywhere else — the association is what reaches it.
	if n.Association == "" {
		e := genMsg.NewExposedEntity()
		e.SetEntityQualifiedName(n.Entity)
		applyExposedCommon(e, n, path)
		e.SetElementType("Object")
		e.SetPrimitiveType("Unknown")
		e.SetMaxOccurs(mdUnbounded) // a definition root repeats: 56/56
		e.SetExposedItemName(n.ExposedItemName)
		if err := addMessageChildren(e.AddChildren, n, path); err != nil {
			return nil, err
		}
		return e, nil
	}

	a := genMsg.NewExposedAssociation()
	a.SetAssociationQualifiedName(n.Association)
	a.SetEntityQualifiedName(n.Entity)
	applyExposedCommon(a, n, path)
	a.SetElementType("Object")
	a.SetPrimitiveType("Unknown")
	// MaxOccurs is the caller's, because it depends on the DIRECTION of
	// traversal and not on the association: measured, all 927 resolvable
	// associations in the corpus are `Reference`, yet 526 store 1 and 401 store
	// -1. The executor resolves it against the domain model.
	a.SetMaxOccurs(int32(n.MaxOccurs))
	a.SetExposedItemName(n.ExposedItemName)
	if err := addMessageChildren(a.AddChildren, n, path); err != nil {
		return nil, err
	}
	return a, nil
}

func addMessageChildren(add func(element.Element), n *model.MessageDefinitionElement, path string) error {
	for _, c := range n.Children {
		child, err := messageNodeToGen(c, path)
		if err != nil {
			return err
		}
		if child != nil {
			add(child)
		}
	}
	return nil
}

// messagePath builds a node's stored Path.
//
// Two things about it are easy to get wrong, and both were, until ako/TestApp's
// hand-authored collection showed the real shape (then confirmed at 4,707 of
// 4,707 elements across it and the demo corpus):
//
//   - the chain is of ORIGINAL names, not exposed ones. A root exposed as
//     "Orders" contributes "Order".
//
//   - an ASSOCIATION contributes TWO segments — the association's own name and
//     then the target entity's:
//
//     Order|OrderLine_Order|OrderLine|Amount
//     ^root ^association    ^entity   ^attribute
func messagePath(parentPath string, n *model.MessageDefinitionElement) string {
	seg := messagePathSegment(n)
	if parentPath == "" {
		return seg
	}
	return parentPath + "|" + seg
}

func messagePathSegment(n *model.MessageDefinitionElement) string {
	if n.Association != "" {
		return shortName(n.Association) + "|" + n.OriginalName
	}
	return n.OriginalName
}

// shortName drops the module from a qualified name.
func shortName(qualified string) string {
	if i := strings.LastIndex(qualified, "."); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}

// exposedCommon is the property set every node type shares.
type exposedCommon interface {
	SetPath(string)
	SetOriginalName(string)
	SetExposedName(string)
	SetMinOccurs(int32)
	SetNillable(bool)
	SetIsDefaultType(bool)
	SetMaxLength(int32)
	SetFractionDigits(int32)
	SetTotalDigits(int32)
	SetDocumentation(string)
	SetExample(string)
	SetErrorMessage(string)
	SetWarningMessage(string)
}

func applyExposedCommon(e exposedCommon, n *model.MessageDefinitionElement, path string) {
	e.SetPath(path)
	e.SetOriginalName(n.OriginalName)
	e.SetExposedName(n.ExposedName)
	e.SetMinOccurs(mdMinOccurs)
	e.SetNillable(mdNillable)
	e.SetIsDefaultType(mdIsDefaultType)
	e.SetMaxLength(mdUnsetPrecision)
	e.SetFractionDigits(mdUnsetPrecision)
	e.SetTotalDigits(mdUnsetPrecision)
	e.SetDocumentation("")
	e.SetExample(n.Example)
	e.SetErrorMessage("")
	e.SetWarningMessage("")
}

var _ backend.MappingBackend = (*Backend)(nil)
