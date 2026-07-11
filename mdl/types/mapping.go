// SPDX-License-Identifier: Apache-2.0

package types

import "github.com/mendixlabs/mxcli/model"

// JsonStructure represents a JSON structure document.
type JsonStructure struct {
	model.BaseElement
	ContainerID   model.ID       `json:"containerId"`
	Name          string         `json:"name"`
	Documentation string         `json:"documentation,omitempty"`
	JsonSnippet   string         `json:"jsonSnippet,omitempty"`
	Elements      []*JsonElement `json:"elements,omitempty"`
	Excluded      bool           `json:"excluded,omitempty"`
	ExportLevel   string         `json:"exportLevel,omitempty"`
}

// GetName returns the JSON structure's name.
func (js *JsonStructure) GetName() string { return js.Name }

// GetContainerID returns the container ID.
func (js *JsonStructure) GetContainerID() model.ID { return js.ContainerID }

// JsonElement represents a single element in a JSON structure (recursive).
type JsonElement struct {
	ExposedName     string         `json:"exposedName"`
	ExposedItemName string         `json:"exposedItemName,omitempty"`
	Path            string         `json:"path"`
	ElementType     string         `json:"elementType"`
	PrimitiveType   string         `json:"primitiveType"`
	MinOccurs       int            `json:"minOccurs"`
	MaxOccurs       int            `json:"maxOccurs"`
	Nillable        bool           `json:"nillable,omitempty"`
	IsDefaultType   bool           `json:"isDefaultType,omitempty"`
	MaxLength       int            `json:"maxLength"`
	FractionDigits  int            `json:"fractionDigits"`
	TotalDigits     int            `json:"totalDigits"`
	OriginalValue   string         `json:"originalValue,omitempty"`
	Children        []*JsonElement `json:"children,omitempty"`
}

// ImageCollection represents an image collection document.
type ImageCollection struct {
	model.BaseElement
	ContainerID   model.ID `json:"containerId"`
	Name          string   `json:"name"`
	ExportLevel   string   `json:"exportLevel,omitempty"`
	Documentation string   `json:"documentation,omitempty"`
	Images        []Image  `json:"images,omitempty"`
}

// GetName returns the image collection's name.
func (ic *ImageCollection) GetName() string { return ic.Name }

// GetContainerID returns the container ID.
func (ic *ImageCollection) GetContainerID() model.ID { return ic.ContainerID }

// IconCollection represents a CustomIcons$CustomIconCollection — an icon *set*
// (e.g. Atlas_Core.Atlas_Filled). Its icons are referenced from a widget as
// `Module.CollectionName.IconName` (the button `icon:` property). Read-only in
// mxcli (SHOW / DESCRIBE ICON COLLECTION); icon collections are authored via the
// theme/Atlas, not MDL.
type IconCollection struct {
	model.BaseElement
	ContainerID   model.ID   `json:"containerId"`
	Name          string     `json:"name"`
	Prefix        string     `json:"prefix,omitempty"`
	ExportLevel   string     `json:"exportLevel,omitempty"`
	Documentation string     `json:"documentation,omitempty"`
	Icons         []IconItem `json:"icons,omitempty"`
}

// GetName returns the icon collection's name.
func (ic *IconCollection) GetName() string { return ic.Name }

// GetContainerID returns the container ID.
func (ic *IconCollection) GetContainerID() model.ID { return ic.ContainerID }

// IconItem is a single named icon within an icon collection.
type IconItem struct {
	Name          string   `json:"name"`
	CharacterCode int      `json:"characterCode,omitempty"`
	Tags          []string `json:"tags,omitempty"`
}

// Image represents a single image in an image collection.
type Image struct {
	ID     model.ID `json:"id"`
	Name   string   `json:"name"`
	Data   []byte   `json:"data,omitempty"`
	Format string   `json:"format,omitempty"`
}
