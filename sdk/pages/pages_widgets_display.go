// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"github.com/mendixlabs/mxcli/model"
)

// Text and Display Widgets

// Text represents a static text widget.
type Text struct {
	BaseWidget
	Caption    *model.Text    `json:"caption,omitempty"`
	RenderMode TextRenderMode `json:"renderMode,omitempty"`
}

// TextRenderMode represents how text is rendered.
type TextRenderMode string

const (
	TextRenderModeText      TextRenderMode = "Text"
	TextRenderModeH1        TextRenderMode = "H1"
	TextRenderModeH2        TextRenderMode = "H2"
	TextRenderModeH3        TextRenderMode = "H3"
	TextRenderModeH4        TextRenderMode = "H4"
	TextRenderModeH5        TextRenderMode = "H5"
	TextRenderModeH6        TextRenderMode = "H6"
	TextRenderModeParagraph TextRenderMode = "Paragraph"
)

// ClientTemplate represents a text template with parameters.
// Used for dynamic text content in widgets like DynamicText and ActionButton captions.
type ClientTemplate struct {
	model.BaseElement
	Template   *model.Text                `json:"template,omitempty"`
	Parameters []*ClientTemplateParameter `json:"parameters,omitempty"`
	Fallback   *model.Text                `json:"fallback,omitempty"`
}

// AttributeRefStep is one hop of an association-navigated attribute reference
// (DomainModels$EntityRefStep). A ClientTemplateParameter that binds an
// attribute *over* an association (e.g. {1} = Order_Customer/Name) carries one
// step per association hop; the final attribute lives in AttributeRef.
type AttributeRefStep struct {
	Association       string `json:"association,omitempty"`       // qualified association name, e.g. "Sales.Order_Customer"
	DestinationEntity string `json:"destinationEntity,omitempty"` // qualified entity reached by this hop, e.g. "Sales.Customer"
}

// ClientTemplateParameter represents a parameter in a client template.
// Used to substitute values for placeholders like {1}, {2} in template text.
type ClientTemplateParameter struct {
	model.BaseElement
	AttributeRef       string             `json:"attributeRef,omitempty"`       // Qualified attribute path like "Module.Entity.Attribute"
	AttributeRefSteps  []AttributeRefStep `json:"attributeRefSteps,omitempty"`  // association hops when the attribute is navigated over associations (AttributeRef.EntityRef)
	Expression         string             `json:"expression,omitempty"`         // Literal expression like "'Hello'"
	SourceVariable     string             `json:"sourceVariable,omitempty"`     // Variable name (no $ prefix)
	SourceVariableKind string             `json:"sourceVariableKind,omitempty"` // "" (default = page parameter), "local" (page-level Variables entry), or "snippet"
	FormattingInfo     *FormattingInfo    `json:"formattingInfo,omitempty"`
}

// DynamicText represents dynamic text based on an attribute.
type DynamicText struct {
	BaseWidget
	AttributePath string          `json:"attributePath,omitempty"`
	Content       *ClientTemplate `json:"content,omitempty"`
	RenderMode    TextRenderMode  `json:"renderMode,omitempty"`
}

// Label represents a label widget.
type Label struct {
	BaseWidget
	Caption *model.Text `json:"caption,omitempty"`
	ForID   model.ID    `json:"forId,omitempty"`
}

// Title represents a page title widget.
type Title struct {
	BaseWidget
	Caption *model.Text `json:"caption,omitempty"`
}

// DynamicImage represents a dynamic image widget.
type DynamicImage struct {
	BaseWidget
	// DataSource is the entity holding the image, stored as the EntityRef of a
	// Forms$ImageViewerSource. Without it mxbuild refuses the widget outright —
	// CE0489 "Select an entity for the data source of this dynamic image" — so
	// this is the one field the widget cannot be written without.
	DataSource DataSource `json:"dataSource,omitempty"`
	// DefaultImageName is the fallback image shown when the object has none, as
	// the three-part qualified name of an image-collection entry
	// (Module.Collection.Image). Forms$ImageViewer.DefaultImage is a by-name
	// reference to Images$Image, so a NAME is what Mendix stores — the
	// DefaultImage (model.ID) field that used to stand here was never filled by
	// anything and named the wrong thing.
	DefaultImageName string    `json:"defaultImageName,omitempty"`
	Width            int       `json:"width,omitempty"`
	WidthUnit        WidthUnit `json:"widthUnit,omitempty"`
	Height           int       `json:"height,omitempty"`
	// HeightUnit is the sibling of WidthUnit, which had no field while the
	// writer hardcoded both to "Auto". Empty means Auto (Mendix's default).
	HeightUnit WidthUnit `json:"heightUnit,omitempty"`
	// ShowAsThumbnail and OnClickEnlarge were both hardcoded false by the
	// writer, so neither was reachable from MDL.
	ShowAsThumbnail bool         `json:"showAsThumbnail,omitempty"`
	OnClickEnlarge  bool         `json:"onClickEnlarge,omitempty"`
	Responsive      bool         `json:"responsive"`
	OnClickAction   ClientAction `json:"onClickAction,omitempty"`
}

// StaticImage represents a static image widget.
type StaticImage struct {
	BaseWidget
	// ImageName is the image this widget shows, as the three-part qualified
	// name of an entry in an image collection (Module.Collection.Image).
	// Forms$StaticImageViewer.Image is a by-name reference to Images$Image, so
	// a NAME is what Mendix stores — the ImageID (model.ID) field that used to
	// stand here was never filled by anything and named the wrong thing
	// (mendixlabs/mxcli#1057).
	ImageName string    `json:"imageName,omitempty"`
	Width     int       `json:"width,omitempty"`
	WidthUnit WidthUnit `json:"widthUnit,omitempty"`
	Height    int       `json:"height,omitempty"`
	// HeightUnit is the sibling of WidthUnit, which had no field while the
	// writer hardcoded both to "Auto". Empty means Auto (Studio Pro's default).
	HeightUnit    WidthUnit    `json:"heightUnit,omitempty"`
	Responsive    bool         `json:"responsive"`
	OnClickAction ClientAction `json:"onClickAction,omitempty"`
}
