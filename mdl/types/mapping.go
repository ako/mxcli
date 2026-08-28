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

// XmlSchema represents an XML schema document — the other thing a mapping's
// `with` clause can name.
//
// Read-only, and deliberately shallow: mxcli cannot author an XML schema (there
// is no `create xml schema`, because the document holds an imported .xsd), and
// it does not read the element tree either, so a mapping over one has nothing to
// validate its members against. What it IS used for is resolving the reference,
// which mxbuild otherwise reports as CE1613 "The selected XML schema 'X' no
// longer exists" at the far end of a build (ako/mxcli#259).
type XmlSchema struct {
	model.BaseElement
	ContainerID model.ID `json:"containerId"`
	// Module is the owning module's name, resolved by the backend. It is carried
	// on the document rather than left to the caller because resolving it needs
	// the container hierarchy — a document's ContainerID may be a FOLDER inside
	// the module — and only the backends can walk that.
	Module        string `json:"module,omitempty"`
	Name          string `json:"name"`
	Documentation string `json:"documentation,omitempty"`
	// FilePath is the .xsd the document was imported from. Kept because it is
	// the one field that tells a reader where the schema came from; an empty one
	// is what mxbuild calls CE0292 "Please import an XSD file."
	FilePath string `json:"filePath,omitempty"`
}

// GetName returns the XML schema's name.
func (xs *XmlSchema) GetName() string { return xs.Name }

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
	// Excluded mirrors Studio Pro's "Exclude from project". Reads must supply
	// it so a rewrite can carry it forward instead of clearing it (#914).
	Excluded bool    `json:"excluded,omitempty"`
	Images   []Image `json:"images,omitempty"`
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
