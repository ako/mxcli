// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"github.com/mendixlabs/mxcli/model"
)

// DataView represents a data view widget.
type DataView struct {
	BaseWidget
	DataSource      DataSource      `json:"dataSource,omitempty"`
	Editable        bool            `json:"editable"`
	ReadOnly        bool            `json:"readOnly,omitempty"`
	ShowFooter      bool            `json:"showFooter"`
	Widgets         []Widget        `json:"widgets,omitempty"`
	FooterWidgets   []Widget        `json:"footerWidgets,omitempty"`
	NoEntityMessage *model.Text     `json:"noEntityMessage,omitempty"`
	FormOrientation FormOrientation `json:"formOrientation,omitempty"`
	LabelWidth      *int            `json:"labelWidth,omitempty"`
}

// FormOrientation controls label placement inside a DataView. Mendix
// stores this as DataView.LabelWidth: 0 = Vertical (label above input),
// >0 = Horizontal (label beside input, taking LabelWidth/12 of the row).
type FormOrientation string

const (
	FormOrientationHorizontal FormOrientation = "Horizontal"
	FormOrientationVertical   FormOrientation = "Vertical"
)

// DefaultLabelWidth is Mendix's metamodel default for DataView.LabelWidth — the
// Horizontal form orientation, label beside the input taking 3/12 of the row.
const DefaultLabelWidth = 3

// ResolvedLabelWidth is the LabelWidth a writer must emit for this DataView.
//
// Studio Pro's "Form orientation" radio has no BSON field of its own: it *is*
// LabelWidth (0 = Vertical, >0 = Horizontal). An explicit LabelWidth is the more
// specific statement and therefore wins over FormOrientation.
//
// This lives on the model so both writers share one definition. Previously only the
// legacy writer performed the translation and the modelsdk one emitted LabelWidth
// only when it was set explicitly, so `FormOrientation: Vertical` was parsed into the
// model and then silently dropped on the default engine (mendixlabs/mxcli#762).
func (dv *DataView) ResolvedLabelWidth() int {
	if dv.LabelWidth != nil {
		return *dv.LabelWidth
	}
	if dv.FormOrientation == FormOrientationVertical {
		return 0
	}
	return DefaultLabelWidth
}

// ListView represents a list view widget.
type ListView struct {
	BaseWidget
	DataSource  DataSource          `json:"dataSource,omitempty"`
	Editable    bool                `json:"editable"`
	ClickAction ClientAction        `json:"clickAction,omitempty"`
	PageSize    int                 `json:"pageSize,omitempty"`
	Widgets     []Widget            `json:"widgets,omitempty"`
	Templates   []*ListViewTemplate `json:"templates,omitempty"`
}

// ListViewTemplate represents a template in a list view: the body rendered for
// one specialization of the list view's entity.
//
// The tag is the STORAGE name, matching generated/metamodel — Mendix's SDK name
// for this property is Specialization but every Studio Pro document stores it
// under "Entity". A template carries nothing else: Studio Pro writes exactly
// {$ID, $Type, Entity, Widgets}.
type ListViewTemplate struct {
	model.BaseElement
	Specialization string   `json:"entity,omitempty"`
	Widgets        []Widget `json:"widgets,omitempty"`
}

// TemplateGrid represents a template grid widget.
type TemplateGrid struct {
	BaseWidget
	DataSource        DataSource    `json:"dataSource,omitempty"`
	NumberOfColumns   int           `json:"numberOfColumns"`
	NumberOfRows      int           `json:"numberOfRows"`
	SelectionMode     SelectionMode `json:"selectionMode"`
	SelectFirst       bool          `json:"selectFirst"`
	Widgets           []Widget      `json:"widgets,omitempty"`
	ControlBarWidgets []Widget      `json:"controlBarWidgets,omitempty"`
}

// DataGrid represents a data grid widget.
type DataGrid struct {
	BaseWidget
	DataSource        DataSource        `json:"dataSource,omitempty"`
	Columns           []*DataGridColumn `json:"columns,omitempty"`
	SelectionMode     SelectionMode     `json:"selectionMode"`
	SelectFirst       bool              `json:"selectFirst"`
	ShowPagingButtons bool              `json:"showPagingButtons"`
	ShowEmptyRows     bool              `json:"showEmptyRows,omitempty"`
	WidthUnit         WidthUnit         `json:"widthUnit,omitempty"`
	ControlBarWidgets []Widget          `json:"controlBarWidgets,omitempty"`
}

// DataGridColumn represents a column in a data grid.
type DataGridColumn struct {
	model.BaseElement
	Name             string            `json:"name,omitempty"`
	Caption          *model.Text       `json:"caption,omitempty"`
	AttributePath    string            `json:"attributePath,omitempty"`
	Editable         bool              `json:"editable"`
	Aggregate        AggregateFunction `json:"aggregate,omitempty"`
	AggregateCaption *model.Text       `json:"aggregateCaption,omitempty"`
	ShowTooltip      bool              `json:"showTooltip,omitempty"`
}

// AggregateFunction represents an aggregate function for columns.
type AggregateFunction string

const (
	AggregateFunctionNone    AggregateFunction = "None"
	AggregateFunctionAverage AggregateFunction = "Average"
	AggregateFunctionCount   AggregateFunction = "Count"
	AggregateFunctionMaximum AggregateFunction = "Maximum"
	AggregateFunctionMinimum AggregateFunction = "Minimum"
	AggregateFunctionSum     AggregateFunction = "Sum"
)

// SelectionMode represents how selection works.
type SelectionMode string

const (
	SelectionModeNone   SelectionMode = "None"
	SelectionModeSingle SelectionMode = "Single"
	SelectionModeMulti  SelectionMode = "Multi"
)

// WidthUnit represents the unit for widths.
type WidthUnit string

const (
	WidthUnitPercentage WidthUnit = "Percentage"
	WidthUnitPixels     WidthUnit = "Pixels"
)
