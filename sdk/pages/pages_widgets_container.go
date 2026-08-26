// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"github.com/mendixlabs/mxcli/model"
)

// Container Widgets

// LayoutGrid represents a layout grid container.
type LayoutGrid struct {
	BaseWidget
	Rows []*LayoutGridRow `json:"rows,omitempty"`
}

// LayoutGridRow represents a row in a layout grid.
type LayoutGridRow struct {
	model.BaseElement
	Columns []*LayoutGridColumn `json:"columns,omitempty"`
}

// LayoutGridColumn represents a column in a layout grid.
type LayoutGridColumn struct {
	model.BaseElement
	Weight       int      `json:"weight"`
	TabletWeight int      `json:"tabletWeight"`
	PhoneWeight  int      `json:"phoneWeight"`
	Widgets      []Widget `json:"widgets,omitempty"`
}

// Container represents a generic container widget.
type Container struct {
	BaseWidget
	Widgets       []Widget            `json:"widgets,omitempty"`
	RenderMode    ContainerRenderMode `json:"renderMode,omitempty"`
	OnClickAction ClientAction        `json:"onClickAction,omitempty"` // optional "On click" action (clickable container)
}

// ContainerRenderMode represents how a container is rendered.
type ContainerRenderMode string

const (
	ContainerRenderModeDiv  ContainerRenderMode = "Div"
	ContainerRenderModeForm ContainerRenderMode = "Form"
)

// GroupBox represents a group box container.
type GroupBox struct {
	BaseWidget
	Caption     *ClientTemplate `json:"captionTemplate,omitempty"`
	Collapsible string          `json:"collapsible"` // "No", "YesInitiallyCollapsed", "YesInitiallyExpanded"
	HeaderMode  string          `json:"headerMode"`  // "Div", "H1"-"H6"
	Widgets     []Widget        `json:"widgets,omitempty"`
}

// TabContainer represents a tab container.
type TabContainer struct {
	BaseWidget
	TabPages      []*TabPage `json:"tabPages,omitempty"`
	DefaultPageID model.ID   `json:"defaultPageId,omitempty"`
}

// TabPage represents a page within a tab container.
type TabPage struct {
	model.BaseElement
	Name          string      `json:"name"`
	Caption       *model.Text `json:"caption,omitempty"`
	Widgets       []Widget    `json:"widgets,omitempty"`
	RefreshOnShow bool        `json:"refreshOnShow,omitempty"`
}

// GetName returns the tab page's name.
func (tp *TabPage) GetName() string {
	return tp.Name
}

// ScrollContainer represents a scrollable container.
//
// Its children live in five named region slots, not in one list: a layout's
// topbar is in Top, its navigation in Left, its page content in Center. Widgets
// is the flat form some older documents use and is kept for reading them.
type ScrollContainer struct {
	BaseWidget
	ScrollBehavior ScrollBehavior           `json:"scrollBehavior"`
	Regions        []*ScrollContainerRegion `json:"regions,omitempty"`
	Widgets        []Widget                 `json:"widgets,omitempty"`
}

// ScrollContainerRegion is one of a ScrollContainer's five slots.
//
// Slot is the position, not a free name: the region has no Name of its own in
// the BSON, so which slot it occupies is its identity. The BSON key for the
// centre slot is "CenterRegion" while the other four are bare positions —
// ScrollContainerSlot spells the MDL names and the codec maps them.
type ScrollContainerRegion struct {
	model.BaseElement
	Slot     ScrollContainerSlot `json:"slot"`
	Size     int                 `json:"size,omitempty"`
	SizeMode string              `json:"sizeMode,omitempty"`
	Class    string              `json:"class,omitempty"`
	Widgets  []Widget            `json:"widgets,omitempty"`
}

// ScrollContainerSlot names one of the five regions.
type ScrollContainerSlot string

const (
	ScrollSlotTop    ScrollContainerSlot = "top"
	ScrollSlotRight  ScrollContainerSlot = "right"
	ScrollSlotBottom ScrollContainerSlot = "bottom"
	ScrollSlotLeft   ScrollContainerSlot = "left"
	ScrollSlotCenter ScrollContainerSlot = "center"
)

// ScrollBehavior represents how scrolling behaves.
type ScrollBehavior string

const (
	ScrollBehaviorVertical   ScrollBehavior = "Vertical"
	ScrollBehaviorHorizontal ScrollBehavior = "Horizontal"
	ScrollBehaviorBoth       ScrollBehavior = "Both"
)
