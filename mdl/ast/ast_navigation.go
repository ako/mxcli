// SPDX-License-Identifier: Apache-2.0

package ast

// AlterNavigationStmt represents: CREATE [OR REPLACE] NAVIGATION <profile> [clauses...]
// This is a full-replacement command: omitted clauses clear that section.
type AlterNavigationStmt struct {
	ProfileName    string           // e.g. "Responsive"
	HomePages      []NavHomePageDef // HOME PAGE/MICROFLOW ... [FOR role]
	LoginPage      *QualifiedName   // LOGIN PAGE ...
	NotFoundPage   *QualifiedName   // NOT FOUND PAGE ...
	MenuItems      []NavMenuItemDef // MENU (...) block
	HasMenuBlock   bool             // true if MENU (...) was present (even if empty → clears menu)
	CreateOrModify bool             // true if CREATE OR REPLACE/MODIFY was used
}

func (s *AlterNavigationStmt) isStatement() {}

// NavHomePageDef represents a HOME PAGE or HOME MICROFLOW clause.
type NavHomePageDef struct {
	IsPage  bool           // true = PAGE, false = MICROFLOW
	Target  QualifiedName  // the page or microflow qualified name
	ForRole *QualifiedName // nil for default home, set for role-based
}

// NavMenuItemDef represents a MENU ITEM or MENU sub-menu definition.
type NavMenuItemDef struct {
	Caption   string           // from STRING_LITERAL
	Page      *QualifiedName   // PAGE target
	Microflow *QualifiedName   // MICROFLOW target
	SignOut   bool             // SIGN_OUT — the third action a menu item can carry
	Icon      string           // ICON 'Module.Collection.name', empty for none
	Items     []NavMenuItemDef // Sub-items (for MENU 'caption' (...))
}

// CreateMenuStmt is `create [or modify] menu Module.Name ( <items> )` — a
// standalone Menus$MenuDocument, not the menu inside a navigation profile.
// Items reuse NavMenuItemDef so both constructs share one item syntax.
//
// Like CREATE NAVIGATION, this is a full replacement: the item list given is the
// document's complete contents, so an omitted item is a removed item.
type CreateMenuStmt struct {
	Folder           string // Folder path within module (empty = leave placement alone)
	Name             QualifiedName
	Items            []NavMenuItemDef
	CreateOrModify   bool // CREATE OR MODIFY / OR REPLACE
	Documentation    string
	DocumentationSet bool // see mendixlabs/mxcli#1018: absent preserves, empty clears
}

func (s *CreateMenuStmt) isStatement() {}

// DropMenuStmt is `drop menu Module.Name`.
type DropMenuStmt struct {
	Name QualifiedName
}

func (s *DropMenuStmt) isStatement() {}
