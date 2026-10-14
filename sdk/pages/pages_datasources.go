// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"github.com/mendixlabs/mxcli/model"
)

// Data Sources

// DataSource represents a data source for widgets.
type DataSource interface {
	isDataSource()
}

// EntityPathSource retrieves data via an entity path.
type EntityPathSource struct {
	model.BaseElement
	EntityPath string `json:"entityPath"`
}

func (EntityPathSource) isDataSource() {}

// DatabaseSource retrieves data from the database.
type DatabaseSource struct {
	model.BaseElement
	EntityID        model.ID    `json:"entityId"`
	EntityName      string      `json:"entityName,omitempty"` // Qualified name e.g. "Module.Entity"
	XPathConstraint string      `json:"xPathConstraint,omitempty"`
	Sorting         []*GridSort `json:"sorting,omitempty"`
	// SearchAttributes are the attributes a List View's search bar filters on.
	// Stored as Forms$ListViewSearch.SearchRefs, a list of
	// DomainModels$AttributeRef — the same element a sort item carries, pinned
	// against a Studio Pro-authored Forms$GridSortItem in a blank 11.12.2 app.
	//
	// Only a List View source has one: Forms$ListViewXPathSource declares Search,
	// and the grid sources do not (ako/mxcli#512).
	SearchAttributes []string `json:"searchAttributes,omitempty"`
	// EntitySteps, when set, reach EntityName from a context object over
	// associations: Studio Pro's "Database" source on a List View inside a data
	// container, stored as a Forms$ListViewXPathSource whose EntityRef is an
	// IndirectEntityRef of these hops. It is still a database retrieve, with
	// XPath, sort and search — not a Forms$AssociationSource (ako/mxcli#721 L5).
	// Only the List View source is written this way.
	EntitySteps []AttributeRefStep `json:"entitySteps,omitempty"`
	// ContextVariable names the page or snippet parameter the steps start from
	// (without $); empty means the enclosing container's object.
	ContextVariable    string `json:"contextVariable,omitempty"`
	IsSnippetParameter bool   `json:"isSnippetParameter,omitempty"`
	// UseConstraints selects Studio Pro 9's "Database" source on a pluggable
	// widget, CustomWidgets$CustomWidgetDatabaseSource: Constraints in place of
	// XPathConstraint, which is then empty. Mendix 10 merged it into the XPath
	// source, so it is only written to a 9.x project (mendixlabs/mxcli#1399).
	UseConstraints bool                 `json:"useConstraints,omitempty"`
	Constraints    []DatabaseConstraint `json:"constraints,omitempty"`
}

func (DatabaseSource) isDataSource() {}

// DatabaseConstraint is one Forms$DatabaseConstraint row of a 9.x database
// source: an attribute of the source entity (qualified), an operator
// (Equals, NotEquals, LessThan, LessThanOrEquals, GreaterThan,
// GreaterThanOrEquals, Contains, NotContains) and the value as untyped text.
type DatabaseConstraint struct {
	Attribute string `json:"attribute"`
	Operator  string `json:"operator"`
	Value     string `json:"value"`
}

// GridSort represents sorting configuration.
type GridSort struct {
	model.BaseElement
	AttributePath string `json:"attributePath"`
	// AttributeRefSteps carries the association hops when the sort navigates to
	// another entity. Mendix stores them as the AttributeRef's EntityRef; an
	// attribute path naming a far entity without them is CE7247 "Cannot sort on
	// attribute …" (mendixlabs/mxcli#1152).
	AttributeRefSteps []AttributeRefStep `json:"attributeRefSteps,omitempty"`
	Direction         SortDirection      `json:"direction"`
}

// SortDirection represents the sort direction.
type SortDirection string

const (
	SortDirectionAscending  SortDirection = "Ascending"
	SortDirectionDescending SortDirection = "Descending"
)

// MicroflowSource retrieves data from a microflow.
type MicroflowSource struct {
	model.BaseElement
	MicroflowID model.ID `json:"microflowId"`
	Microflow   string   `json:"microflow"` // Qualified name (e.g., "Module.MicroflowName")
	// Argument bindings for a parameterized source microflow. Mendix requires an
	// argument for every parameter; dropping these builds to CE1571 "No argument
	// has been selected for parameter 'X'" (#835).
	ParameterMappings []*MicroflowParameterMapping `json:"parameterMappings,omitempty"`
}

func (MicroflowSource) isDataSource() {}

// NanoflowSource retrieves data from a nanoflow.
type NanoflowSource struct {
	model.BaseElement
	NanoflowID model.ID `json:"nanoflowId"`
	Nanoflow   string   `json:"nanoflow"` // Qualified name (e.g., "Module.NanoflowName")
	// Argument bindings for a parameterized source nanoflow — same rule as
	// MicroflowSource above (#835).
	ParameterMappings []*MicroflowParameterMapping `json:"parameterMappings,omitempty"`
}

func (NanoflowSource) isDataSource() {}

// ListenToWidgetSource listens to another widget (selection datasource).
// BSON type is Forms$ListenTargetSource with ListenTarget as widget name.
type ListenToWidgetSource struct {
	model.BaseElement
	WidgetID   model.ID `json:"widgetId"`             // Widget ID for internal reference
	WidgetName string   `json:"widgetName,omitempty"` // Widget name for BSON serialization (ListenTarget field)
}

func (ListenToWidgetSource) isDataSource() {}

// AssociationSource retrieves data via association.
type AssociationSource struct {
	model.BaseElement
	EntityPath      string `json:"entityPath"`                // "Module.Assoc/Module.DestEntity", one pair per hop joined by "/"
	ContextVariable string `json:"contextVariable,omitempty"` // page or snippet parameter name (without $) — empty for $currentObject
	// IsSnippetParameter puts ContextVariable in the SnippetParameter slot of
	// the source's Forms$PageVariable instead of PageParameter.
	IsSnippetParameter bool `json:"isSnippetParameter,omitempty"`
}

func (AssociationSource) isDataSource() {}

// DataViewSource is the datasource for DataView widgets using page/snippet parameters.
type DataViewSource struct {
	model.BaseElement
	EntityID           model.ID `json:"entityId,omitempty"`           // Entity reference (qualified name stored here)
	EntityName         string   `json:"entityName,omitempty"`         // Qualified entity name for BSON serialization
	ParameterName      string   `json:"parameterName,omitempty"`      // Name of page/snippet parameter (without $)
	IsSnippetParameter bool     `json:"isSnippetParameter,omitempty"` // True if this is a snippet parameter, false for page parameter
}

func (DataViewSource) isDataSource() {}
