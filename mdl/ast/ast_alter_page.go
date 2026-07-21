// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// ALTER PAGE / ALTER SNIPPET — in-place widget tree modification
// ============================================================================

// AlterPageStmt represents: ALTER PAGE/SNIPPET Module.Name { operations }
type AlterPageStmt struct {
	ContainerType string        // "PAGE" or "SNIPPET"
	PageName      QualifiedName // page or snippet qualified name
	Operations    []AlterPageOperation
}

func (s *AlterPageStmt) isStatement() {}

// AlterPageOperation is the interface for individual ALTER PAGE operations.
type AlterPageOperation interface {
	isAlterPageOperation()
}

// WidgetRef represents a widget reference, optionally with a sub-element path.
// Plain: "btnSave" (Widget="btnSave", Column="")
// Dotted: "dgProducts.Name" (Widget="dgProducts", Column="Name")
type WidgetRef struct {
	Widget string // widget name (always set)
	Column string // column name within widget (empty for plain widget refs)
}

// Name returns the full reference string for error messages.
func (r WidgetRef) Name() string {
	if r.Column != "" {
		return r.Widget + "." + r.Column
	}
	return r.Widget
}

// IsColumn returns true if this is a column reference (dotted path).
func (r WidgetRef) IsColumn() bool {
	return r.Column != ""
}

// SetPropertyOp represents: SET prop = value ON widgetRef
// or SET prop = value (page-level, Target.Widget empty)
type SetPropertyOp struct {
	Target     WidgetRef              // empty Widget for page-level SET
	Properties map[string]interface{} // property name -> value
}

func (s *SetPropertyOp) isAlterPageOperation() {}

// InsertWidgetOp represents: INSERT AFTER/BEFORE/INTO widgetRef { widgets }
type InsertWidgetOp struct {
	Position string    // "AFTER", "BEFORE", or "INTO" (append as children of the container)
	Target   WidgetRef // widget to insert relative to, or container to insert into
	Widgets  []*WidgetV3
}

func (s *InsertWidgetOp) isAlterPageOperation() {}

// DropWidgetOp represents: DROP WIDGET ref1, ref2, ...
type DropWidgetOp struct {
	Targets []WidgetRef
}

func (s *DropWidgetOp) isAlterPageOperation() {}

// ReplaceWidgetOp represents: REPLACE widgetRef WITH { widgets }
type ReplaceWidgetOp struct {
	Target     WidgetRef
	NewWidgets []*WidgetV3
}

func (s *ReplaceWidgetOp) isAlterPageOperation() {}

// AddVariableOp represents: ADD Variables $name: Type = 'default'
type AddVariableOp struct {
	Variable PageVariable
}

func (s *AddVariableOp) isAlterPageOperation() {}

// DropVariableOp represents: DROP Variables $name
type DropVariableOp struct {
	VariableName string // without $ prefix
}

func (s *DropVariableOp) isAlterPageOperation() {}

// SetLayoutOp represents: SET Layout = Module.LayoutName [MAP (Old -> New, ...)]
type SetLayoutOp struct {
	NewLayout QualifiedName     // New layout qualified name
	Mappings  map[string]string // Old placeholder -> New placeholder (nil = auto-map)
}

func (s *SetLayoutOp) isAlterPageOperation() {}

// LayoutMapping represents a single placeholder mapping: Old -> New
type LayoutMapping struct {
	From string
	To   string
}
