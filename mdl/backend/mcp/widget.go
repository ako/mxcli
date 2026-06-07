// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// comboboxWidgetID is the only pluggable widget the MCP backend maps so far —
// the Mendix 11 association/enumeration selector (the reference/dropdown
// replacement). Other pluggable widgets (DataGrid 2, Gallery) are rejected in
// mapCustomWidget until their pg `object` shapes are mapped.
const comboboxWidgetID = "com.mendix.widget.web.combobox.Combobox"

// mapCustomWidget emits the pg CustomWidgets$CustomWidget for a pluggable widget
// built this session. Only ComboBox is mapped so far; other pluggable widgets,
// or a ComboBox that relied on a property operation we don't translate, are
// rejected rather than written with missing properties.
func (b *Backend) mapCustomWidget(wd *pages.CustomWidget) (map[string]any, error) {
	cw := b.customWidgets[wd.ID]
	if cw == nil {
		return nil, fmt.Errorf("custom widget %q has no recorded properties (the MCP backend builds pluggable widgets via the widget engine)", wd.Name)
	}
	if cw.widgetID != comboboxWidgetID {
		return nil, fmt.Errorf("pluggable widget %q (%s) is not yet supported by the MCP backend — only ComboBox", wd.Name, cw.widgetID)
	}
	if len(cw.unsupported) > 0 {
		return nil, fmt.Errorf("combobox %q uses properties not yet supported by the MCP backend: %v", wd.Name, cw.unsupported)
	}
	// The combobox def.json enum mode maps only attributeEnumeration; the MPR
	// template carries optionsSourceType's default, which the MCP path lacks. pg
	// defaults an unset optionsSourceType to "association" and then prunes the
	// (now irrelevant) attributeEnumeration, so the enum binding is lost unless
	// we set the source type explicitly. Association mode already sets it via the
	// primitive mapping.
	if _, ok := cw.object["optionsSourceType"]; !ok {
		if _, isEnum := cw.object["attributeEnumeration"]; isEnum {
			cw.object["optionsSourceType"] = "enumeration"
		}
	}
	return map[string]any{
		"$Type":      "CustomWidgets$CustomWidget",
		"name":       wd.Name,
		"appearance": pageAppearance(wd.Class, wd.Style),
		"widgetId":   cw.widgetID,
		"object":     cw.object,
	}, nil
}

// mcpCustomWidget is the recorded high-level pg form of one pluggable widget:
// its widget id and the `object` property bag that pg_write_page expands into a
// full widget (Studio Pro fills every default, so only the meaningful, MDL-
// derived properties need to be set — this is what sidesteps the CE0463
// template-mismatch class of bugs that the BSON writer path hits).
type mcpCustomWidget struct {
	widgetID    string
	object      map[string]any
	unsupported []string // property ops recorded for not-yet-supported widget shapes
}

// LoadWidgetTemplate returns an MCP-specific pluggable-widget builder. Unlike the
// MPR builder (which mutates an embedded BSON template), this one records the
// engine's semantic property operations into a high-level pg `object` map. No
// template BSON is needed — Studio Pro owns serialization over pg_write_page —
// so the projectPath is ignored and the CE0463 template-mismatch class of bugs
// does not arise on the MCP path.
func (b *Backend) LoadWidgetTemplate(widgetID string, _ string) (backend.WidgetObjectBuilder, error) {
	return &mcpWidgetBuilder{backend: b, widgetID: widgetID, object: map[string]any{}}, nil
}

// mcpWidgetBuilder implements backend.WidgetObjectBuilder by translating each
// semantic Set* operation into the corresponding pg `object` field. Operations
// not needed by the supported widgets are recorded in `unsupported` so
// mapCustomWidget can refuse a widget whose shape we can't faithfully emit
// rather than silently dropping properties.
type mcpWidgetBuilder struct {
	backend     *Backend
	widgetID    string
	object      map[string]any
	unsupported []string
}

var _ backend.WidgetObjectBuilder = (*mcpWidgetBuilder)(nil)

func (w *mcpWidgetBuilder) note(op string) { w.unsupported = append(w.unsupported, op) }

func (w *mcpWidgetBuilder) SetAttribute(propertyKey, attributePath string) {
	if attributePath == "" {
		return
	}
	w.object[propertyKey] = map[string]any{
		"$Type":     "DomainModels$AttributeRef",
		"attribute": attributePath,
	}
}

func (w *mcpWidgetBuilder) SetAssociation(propertyKey, assocPath, entityName string) {
	if assocPath == "" {
		return
	}
	w.object[propertyKey] = map[string]any{
		"$Type": "DomainModels$IndirectEntityRef",
		"steps": []any{map[string]any{
			"$Type":             "DomainModels$EntityRefStep",
			"association":       assocPath,
			"destinationEntity": entityName,
		}},
	}
}

func (w *mcpWidgetBuilder) SetPrimitive(propertyKey, value string) {
	w.object[propertyKey] = value
}

func (w *mcpWidgetBuilder) SetDataSource(propertyKey string, ds pages.DataSource) {
	if src := customWidgetXPathSource(ds); src != nil {
		w.object[propertyKey] = src
	}
}

// customWidgetXPathSource maps a database/entity data source onto a
// CustomWidgets$CustomWidgetXPathSource (the options source for an association
// combobox). Microflow/other sources are not yet mapped.
func customWidgetXPathSource(ds pages.DataSource) map[string]any {
	var entity string
	switch s := ds.(type) {
	case *pages.DatabaseSource:
		entity = s.EntityName
	case *pages.DataViewSource:
		entity = s.EntityName
	}
	if entity == "" {
		return nil
	}
	return map[string]any{
		"$Type":            "CustomWidgets$CustomWidgetXPathSource",
		"entityRef":        map[string]any{"$Type": "DomainModels$DirectEntityRef", "entity": entity},
		"forceFullObjects": false,
	}
}

// Operations not needed by ComboBox — recorded so an unsupported widget that
// relies on them is rejected rather than emitted with missing properties.
func (w *mcpWidgetBuilder) SetSelection(propertyKey, _ string)  { w.note("selection:" + propertyKey) }
func (w *mcpWidgetBuilder) SetExpression(propertyKey, _ string) { w.note("expression:" + propertyKey) }
func (w *mcpWidgetBuilder) SetChildWidgets(propertyKey string, _ []pages.Widget) {
	w.note("childWidgets:" + propertyKey)
}
func (w *mcpWidgetBuilder) SetTextTemplate(propertyKey, _ string) {
	w.note("textTemplate:" + propertyKey)
}
func (w *mcpWidgetBuilder) SetTextTemplateWithParams(propertyKey, _ string, _ string) {
	w.note("textTemplate:" + propertyKey)
}
func (w *mcpWidgetBuilder) SetAction(propertyKey string, _ pages.ClientAction) {
	w.note("action:" + propertyKey)
}
func (w *mcpWidgetBuilder) SetAttributeObjects(propertyKey string, _ []string) {
	w.note("attributeObjects:" + propertyKey)
}
func (w *mcpWidgetBuilder) SetObjectList(propertyKey string, _ []backend.ObjectListItemSpec) {
	w.note("objectList:" + propertyKey)
}
func (w *mcpWidgetBuilder) CloneGallerySelectionProperty(propertyKey, _ string) {
	w.note("gallerySelection:" + propertyKey)
}

// PropertyTypeIDs returns an empty map — the MCP path needs no template metadata
// (Studio Pro expands the object), and the engine's auto-datasource/child-slot
// passes simply find nothing to do.
func (w *mcpWidgetBuilder) PropertyTypeIDs() map[string]pages.PropertyTypeIDEntry {
	return map[string]pages.PropertyTypeIDEntry{}
}

func (w *mcpWidgetBuilder) EnsureRequiredObjectLists()                             {}
func (w *mcpWidgetBuilder) ApplyPropertyVisibility(_ []types.WidgetVisibilityRule) {}

// Finalize builds the CustomWidget shell and registers its recorded pg object on
// the backend, keyed by the widget's ID so mapPageWidget can find it.
func (w *mcpWidgetBuilder) Finalize(id model.ID, name, label, editable string) *pages.CustomWidget {
	if w.backend.customWidgets == nil {
		w.backend.customWidgets = make(map[model.ID]*mcpCustomWidget)
	}
	w.backend.customWidgets[id] = &mcpCustomWidget{
		widgetID:    w.widgetID,
		object:      w.object,
		unsupported: w.unsupported,
	}
	return &pages.CustomWidget{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{ID: id, TypeName: "CustomWidgets$CustomWidget"},
			Name:        name,
		},
		Label:    label,
		Editable: editable,
	}
}
