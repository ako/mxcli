// SPDX-License-Identifier: Apache-2.0

package ast

// A message definition collection is a selection over the domain model: every
// element names an entity, an attribute or an association. That is what makes it
// authorable where a mapping's other two non-JSON sources are not — an XML
// schema holds an imported .xsd and an imported web service holds a WSDL.
//
// It is the source for 74 of the 327 mappings in the demo corpus (22.6%), and
// was the only one of the four a script could not create.

// CreateMessageDefinitionCollectionStmt represents:
//
//	CREATE [OR MODIFY] MESSAGE DEFINITION COLLECTION Module.Name
//	  [FOLDER 'path']
//	( definition Name for Module.Entity [as 'Exposed'] ( members ) , ... );
type CreateMessageDefinitionCollectionStmt struct {
	Name           QualifiedName
	Folder         string
	CreateOrModify bool
	Definitions    []*MessageDefinitionDef
}

func (s *CreateMessageDefinitionCollectionStmt) isStatement() {}

// MessageDefinitionDef is one `definition <Name> for <Entity>` inside a
// collection.
//
// Name and ExposedName are independent: measured, 19 of 56 real definitions are
// named something other than their entity, and 52 of 56 root elements carry an
// ExposedName that differs from the entity's name.
type MessageDefinitionDef struct {
	Name        string
	Entity      QualifiedName
	ExposedName string // "" means: use the entity's own name
	Members     []*MessageMemberDef
}

// MessageMemberDef is an exposed attribute or an exposed association.
//
// Association is empty for an attribute. When it is set, Entity names the
// association's TARGET — spelled out rather than inferred, because the stored
// MaxOccurs tracks the DIRECTION of traversal and not the association's type
// (measured: all 927 resolvable associations in the corpus are `Reference`, yet
// 526 store MaxOccurs 1 and 401 store -1).
type MessageMemberDef struct {
	// Attribute is the member's name on the holding entity, for a value member.
	// An INHERITED attribute is named exactly like an own one — 398 of 3,697
	// real exposed attributes are inherited — and the executor resolves it to
	// the entity that DECLARES it, which is what Mendix stores.
	Attribute string

	Association QualifiedName // empty for an attribute
	Entity      QualifiedName // the association's target

	ExposedName string // "" means: use the member's own name
	// Example is author-set sample text. Rare (1 of 4,707 elements measured)
	// but real, and describe emitting nothing for it is what would make
	// describe -> exec lossy.
	Example string
	Members []*MessageMemberDef
}

// IsAssociation reports whether the member exposes an association rather than an
// attribute.
func (m *MessageMemberDef) IsAssociation() bool { return m.Association.Name != "" }

// DropMessageDefinitionCollectionStmt represents:
// DROP MESSAGE DEFINITION COLLECTION Module.Name
type DropMessageDefinitionCollectionStmt struct {
	Name QualifiedName
}

func (s *DropMessageDefinitionCollectionStmt) isStatement() {}

// AlterMessageDefinitionCollectionStmt adds, drops or renames a DEFINITION
// within a collection.
type AlterMessageDefinitionCollectionStmt struct {
	Name QualifiedName
	// Op is "ADD", "DROP" or "RENAME".
	Op string
	// Definition is the definition being added (ADD) or its name (DROP/RENAME).
	Definition *MessageDefinitionDef
	Target     string // DROP / RENAME: the definition's name
	NewName    string // RENAME only
	IfExists   bool
	IfNotExist bool
}

func (s *AlterMessageDefinitionCollectionStmt) isStatement() {}

// AlterMessageDefinitionStmt adds, drops or renames a MEMBER within one
// definition, addressed as Module.Collection.Definition — the same three-part
// reference `WITH MESSAGE DEFINITION` takes.
type AlterMessageDefinitionStmt struct {
	// Collection and Definition come from the three-part name.
	Collection QualifiedName
	Definition string
	// Op is "ADD", "DROP" or "SET".
	Op string
	// Member is the member being added (ADD).
	Member *MessageMemberDef
	// Target is the member's name for DROP and SET.
	Target string
	// ExposedName is the new exposed name for SET. SET changes only this — it is
	// not a model rename, which is why the keyword is SET and not RENAME.
	ExposedName string
	// Path addresses a nested member, in exposed names. Members nest to depth 7
	// in the corpus, so this is not an edge case.
	Path       []string
	IfExists   bool
	IfNotExist bool
}

func (s *AlterMessageDefinitionStmt) isStatement() {}
