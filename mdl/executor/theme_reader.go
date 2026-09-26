// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// ThemeProperty represents a single design property definition from design-properties.json.
type ThemeProperty struct {
	Name        string        `json:"name"`
	Type        string        `json:"type"` // "Toggle", "Dropdown", "ColorPicker", "ToggleButtonGroup"
	Description string        `json:"description"`
	Class       string        `json:"class"`   // For Toggle type: the CSS class toggled
	Options     []ThemeOption `json:"options"` // For Dropdown/ColorPicker/ToggleButtonGroup
	// Property is the CSS property (or custom property) a ColorPicker writes a
	// CUSTOM colour to — "border-color", "--layoutgrid-column-bg". Without one a
	// custom colour has nowhere to go: mxbuild accepts it and the compiled page
	// carries neither a class nor a style for it (measured on Atlas Core 4.1.3's
	// Label "Style", Mendix 11.13.0). With one, the value is emitted verbatim as
	// an inline style (`style:{borderColor:"#ff0000"}`).
	Property string `json:"property"`
	// MultiSelect marks a property whose value is a SET of the declared options
	// rather than one of them — Atlas declares it on `Hide on` (Phone/Tablet/
	// Desktop). It changes the STORED SHAPE, not just the arity: Mendix writes a
	// Forms$CompoundDesignPropertyValue whose Properties hold one
	// Forms$DesignPropertyValue per selected option, each valued with a bare
	// Forms$ToggleDesignPropertyValue. Measured by decoding a Studio Pro-authored
	// Atlas page in a blank 11.12.2 project.
	//
	// Without it a flat `'Hide on': 'Phone'` serialized as a plain option, which
	// mxbuild refuses with CE6084 "Expected design property Hide on to be of type
	// Toggle button group, but found Option" (ako/mxcli#511).
	MultiSelect bool `json:"multiSelect"`
	// OldNames are the keys this property had in earlier theme versions. A
	// page authored against one still stores the old key, and mxbuild reports
	// CE6087 "Design properties have been renamed in your theme and need to be
	// updated" on it unless the page is excluded (measured on 11.13.0 with Atlas
	// Core 4.1.3, where "Align content" became "Align content (deprecated)").
	OldNames []string `json:"oldNames"`
	// Margin and Padding are the steps of a `"type": "Spacing"` property. Its
	// old names live on each side of each step, spelled "<old key>::<old
	// value>": Atlas's one Spacing property replaced the per-side dropdowns
	// "Spacing top" … "Spacing left", so an old key maps to one side.
	Margin  []ThemeSpacingStep `json:"margin"`
	Padding []ThemeSpacingStep `json:"padding"`
}

// ThemeOption represents a single option within a dropdown/picker design property.
type ThemeOption struct {
	Name  string `json:"name"`
	Class string `json:"class"`
	// OldNames are earlier names of this option. On an ordinary property they
	// are old VALUES ("Left align as row"); on a multi-select property they are
	// the separate toggles the option replaced ("Hide on phone" became Hide on:
	// Phone), i.e. old KEYS.
	OldNames []string `json:"oldNames"`
}

// ThemeSpacingStep is one step ("None", "S", "M", …) of a Spacing property.
type ThemeSpacingStep struct {
	Name   string            `json:"name"`
	Top    *ThemeSpacingSide `json:"top"`
	Right  *ThemeSpacingSide `json:"right"`
	Bottom *ThemeSpacingSide `json:"bottom"`
	Left   *ThemeSpacingSide `json:"left"`
}

// ThemeSpacingSide is one side of a Spacing step.
type ThemeSpacingSide struct {
	Class    string   `json:"class"`
	OldNames []string `json:"oldNames"`
}

// ThemeRegistry holds all design property definitions loaded from the project's themesource.
type ThemeRegistry struct {
	// WidgetProperties maps design-properties.json widget type key to its properties.
	// Keys: "Widget", "DivContainer", "Button", "DataGrid", pluggable widget IDs, etc.
	WidgetProperties map[string][]ThemeProperty
}

// loadThemeRegistry reads and merges all design-properties.json files from the project's
// themesource directories (themesource/*/web/design-properties.json).
func loadThemeRegistry(projectDir string) (*ThemeRegistry, error) {
	registry := &ThemeRegistry{
		WidgetProperties: make(map[string][]ThemeProperty),
	}

	themesourceDir := filepath.Join(projectDir, "themesource")
	if _, err := os.Stat(themesourceDir); os.IsNotExist(err) {
		return registry, nil // No themesource directory — return empty registry
	}

	// Walk themesource/*/web/design-properties.json
	entries, err := os.ReadDir(themesourceDir)
	if err != nil {
		return nil, mdlerrors.NewBackend("read themesource directory", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dpPath := filepath.Join(themesourceDir, entry.Name(), "web", "design-properties.json")
		if _, err := os.Stat(dpPath); os.IsNotExist(err) {
			continue
		}

		data, err := os.ReadFile(dpPath)
		if err != nil {
			continue // Skip unreadable files
		}

		fileProps, err := parseDesignPropertiesJSON(data)
		if err != nil {
			continue // Skip malformed files
		}

		// Merge into registry
		for widgetType, props := range fileProps {
			registry.WidgetProperties[widgetType] = append(registry.WidgetProperties[widgetType], props...)
		}
	}

	return registry, nil
}

// parseDesignPropertiesJSON decodes one design-properties.json into its widget
// groups. Split out from the directory walk so the decoding — in particular that
// `multiSelect` is read at all — is testable without a project on disk.
func parseDesignPropertiesJSON(data []byte) (map[string][]ThemeProperty, error) {
	var fileProps map[string][]ThemeProperty
	if err := json.Unmarshal(data, &fileProps); err != nil {
		return nil, err
	}
	return fileProps, nil
}

// GetPropertiesForWidget returns properties applicable to a widget type,
// including inherited "Widget" properties that apply to all widget types.
func (r *ThemeRegistry) GetPropertiesForWidget(widgetTypeKey string) []ThemeProperty {
	var result []ThemeProperty

	// Add inherited "Widget" properties first (apply to all widgets)
	if widgetProps, ok := r.WidgetProperties["Widget"]; ok {
		result = append(result, widgetProps...)
	}

	// Add type-specific properties
	if widgetTypeKey != "Widget" {
		if typeProps, ok := r.WidgetProperties[widgetTypeKey]; ok {
			result = append(result, typeProps...)
		}
	}

	return result
}

// mdlKeywordStorageType maps each MDL keyword that builds a NATIVE widget to the
// $Type the builder writes for it. The theme key is then read off that $Type
// (bsonTypeToDesignPropsKey), so an inline widget and a stored one resolve
// through the same table and cannot disagree about which group applies.
//
// It used to map keywords straight to keys — a second hand-written copy of the
// same concept — and it drifted: `groupbox`, `tabcontainer`, `navigationtree`,
// `menubar`, `simplemenubar`, `button`, `row` and `column` had no entry, so the
// keyword fell through as-is, no design-properties.json defines "groupbox", the
// validator skipped the widget, and the builder saw only the "Widget" base group
// — a custom colour on a group box's ColorPicker "Style" was written as an
// option and mxbuild refused it with CE6085. `radiobuttons` and `snippetcall`
// named keys mxbuild does not apply ("RadioButtons", "SnippetCall" — CE6083 when
// a theme declares them), and `header`/`footer` named groups for widgets the
// builder writes as a Forms$DivContainer.
//
// A keyword is a widget only where buildWidgetV3 builds it. `row`, `column` and
// `footer` are also SLOTS of a layoutgrid / row / dataview, where no widget of
// this $Type exists — validateDesignPropsSubtree tells the two apart.
//
// TestKeywordStorageTypesMatchBuilder builds every entry and compares the $Type;
// TestKeywordStorageTypesCoverBuilderDispatch holds it to buildWidgetV3's switch.
var mdlKeywordStorageType = map[string]string{
	"container":       "Forms$DivContainer",
	"customcontainer": "Forms$DivContainer",
	// Top-level `row` / `column` build a Forms$DivContainer holding a one-row
	// layout grid; their design properties land on that container.
	"row":        "Forms$DivContainer",
	"column":     "Forms$DivContainer",
	"header":     "Forms$DivContainer",
	"footer":     "Forms$DivContainer",
	"controlbar": "Forms$DivContainer",
	"template":   "Forms$DivContainer",
	"filter":     "Forms$DivContainer",

	"actionbutton":    "Forms$ActionButton",
	"linkbutton":      "Forms$ActionButton",
	"button":          "Forms$ActionButton",
	"textbox":         "Forms$TextBox",
	"textarea":        "Forms$TextArea",
	"datepicker":      "Forms$DatePicker",
	"checkbox":        "Forms$CheckBox",
	"radiobuttons":    "Forms$RadioButtonGroup",
	"dropdown":        "Forms$DropDown",
	"dataview":        "Forms$DataView",
	"listview":        "Forms$ListView",
	"layoutgrid":      "Forms$LayoutGrid",
	"dynamictext":     "Forms$DynamicText",
	"label":           "Forms$Label",
	"title":           "Forms$Title",
	"staticimage":     "Forms$StaticImageViewer",
	"dynamicimage":    "Forms$ImageViewer",
	"navigationlist":  "Forms$NavigationList",
	"snippetcall":     "Forms$SnippetCallWidget",
	"tabcontainer":    "Forms$TabControl",
	"groupbox":        "Forms$GroupBox",
	"scrollcontainer": "Forms$ScrollContainer",
	"navigationtree":  "Forms$NavigationTree",
	"menubar":         "Forms$MenuBar",
	"simplemenubar":   "Forms$SimpleMenuBar",
	"placeholder":     "Forms$Placeholder",
}

// pluggableKeywordIDs maps an MDL keyword to the pluggable widget id it writes,
// built once from the two places that already decide it: keywordDispatchTable
// (version-aware keywords, today just DATAGRID) and the embedded widget
// definitions (COMBOBOX, GALLERY, IMAGE, the DataGrid filters, …).
//
// Deriving it rather than listing it is the point. The hand-written table above
// named DataGrid for `datagrid`, which is Atlas Core's DEPRECATED data grid,
// while MDL's `datagrid` has always written Data grid 2 from the DataWidgets
// module. The two have disjoint design properties, so MDL-WIDGET11 warned that
// Compact / Hover / Striped were "not defined for this widget type" — they are
// exactly its properties — and suggested Style and Row size, which mxbuild then
// refuses with CE6083 "not supported by your theme". Taking the tool's advice
// turned 16 warnings into 17 build errors (ako/CapTrackV4 010).
//
// The other three were wrong in the quieter direction: `combobox`, `gallery` and
// `image` named keys no web design-properties.json defines, so the registry
// lookup missed and validateWidgetDesignProps skipped those widgets entirely.
// Silence read as approval.
var pluggableKeywordIDs = sync.OnceValue(func() map[string]string {
	out := map[string]string{}
	for _, mapping := range keywordDispatchTable {
		for _, b := range mapping.Bindings {
			if b.Kind == bindingKindPluggable && b.WidgetID != "" {
				out[strings.ToLower(mapping.Keyword)] = b.WidgetID
				break
			}
		}
	}
	// A definition's own MDLName is what the builder dispatches on, so this is
	// the same answer the writer gives. A registry that fails to load leaves the
	// dispatch-table entries, which is the case that matters most.
	if reg, err := NewWidgetRegistry(); err == nil && reg != nil {
		for _, def := range reg.All() {
			if def.MDLName != "" && def.WidgetID != "" {
				out[strings.ToLower(def.MDLName)] = def.WidgetID
			}
		}
	}
	return out
})

// resolveDesignPropsKey converts an MDL widget type keyword (e.g., "container",
// "CONTAINER") to the design-properties.json key (e.g., "DivContainer").
//
// A keyword that writes a PLUGGABLE widget resolves to that widget's id, which
// is how design-properties.json keys them. A native keyword resolves through the
// $Type it writes (mdlKeywordStorageType) to the key a stored widget of that
// type has. An unrecognised type falls through as-is — a pluggable id written
// directly is already the right key.
func resolveDesignPropsKey(mdlKeyword string) string {
	lower := strings.ToLower(mdlKeyword)
	if id, ok := pluggableKeywordIDs()[lower]; ok {
		return id
	}
	if storage, ok := mdlKeywordStorageType[lower]; ok {
		if key, ok := bsonTypeToDesignPropsKey[storage]; ok {
			return key
		}
	}
	return mdlKeyword
}

// storageTypeThemeKeys maps a stored widget $Type, without its "Forms$" /
// "Pages$" prefix, to the design-properties.json group Studio Pro applies to it.
//
// A theme key is a Mendix CLASS name — the qualified name, NOT the storage name.
// Measured with probe groups added to a copy of PedApp's theme (Mendix 11.13.0):
// mxbuild accepts a "TabContainer", "RadioButtonGroup" or "SnippetCallWidget"
// design property on Forms$TabControl / Forms$RadioButtonGroup /
// Forms$SnippetCallWidget, and refuses "TabControl", "RadioButtons" and
// "SnippetCall" with CE6083 "not supported by your theme". Studio Pro also
// applies the groups of ANCESTOR classes — "Widget" for every widget, and
// "Button" (Atlas's key) as well as "ActionButton" for an action button. This
// table names the one group per type that Atlas declares; GetPropertiesForWidget
// adds "Widget".
//
// TestDesignPropsKeysAreMetamodelClassNames holds every entry to the generated
// metamodel's class for its storage name, so a storage/qualified mix-up like the
// ones above fails a test instead of a build.
var storageTypeThemeKeys = map[string]string{
	"DivContainer":       "DivContainer",
	"ActionButton":       "Button",
	"TextBox":            "TextBox",
	"TextArea":           "TextArea",
	"DatePicker":         "DatePicker",
	"CheckBox":           "CheckBox",
	"RadioButtonGroup":   "RadioButtonGroup",
	"ReferenceSelector":  "ReferenceSelector",
	"DropDown":           "DropDown",
	"DataGrid":           "DataGrid",
	"DataView":           "DataView",
	"ListView":           "ListView",
	"LayoutGrid":         "LayoutGrid",
	"DynamicText":        "DynamicText",
	"Label":              "Label",
	"Title":              "Title",
	"StaticImageViewer":  "StaticImageViewer",
	"ImageViewer":        "DynamicImageViewer", // DynamicImageViewer's storage name
	"DynamicImageViewer": "DynamicImageViewer",
	"NavigationList":     "NavigationList",
	"TabControl":         "TabContainer", // TabContainer's storage name
	"GroupBox":           "GroupBox",
	"ScrollContainer":    "ScrollContainer",
	"NavigationTree":     "NavigationTree",
	"MenuBar":            "MenuBar",
	"SimpleMenuBar":      "SimpleMenuBar",
	"Placeholder":        "Placeholder",
	"SnippetCallWidget":  "SnippetCallWidget",
}

// bsonTypeToDesignPropsKey maps BSON $Type values to design-properties.json keys:
// storageTypeThemeKeys under both prefixes.
var bsonTypeToDesignPropsKey = func() map[string]string {
	out := make(map[string]string, 2*len(storageTypeThemeKeys))
	for storage, key := range storageTypeThemeKeys {
		out["Forms$"+storage] = key
		out["Pages$"+storage] = key
	}
	return out
}()

// widgetTypeDisplayName maps BSON $Type to a short display name for output.
var widgetTypeDisplayName = map[string]string{
	"Forms$DivContainer":         "Container",
	"Pages$DivContainer":         "Container",
	"Forms$ActionButton":         "ActionButton",
	"Pages$ActionButton":         "ActionButton",
	"Forms$TextBox":              "TextBox",
	"Pages$TextBox":              "TextBox",
	"Forms$TextArea":             "TextArea",
	"Pages$TextArea":             "TextArea",
	"Forms$DatePicker":           "DatePicker",
	"Pages$DatePicker":           "DatePicker",
	"Forms$CheckBox":             "CheckBox",
	"Pages$CheckBox":             "CheckBox",
	"Forms$RadioButtons":         "RadioButtons",
	"Pages$RadioButtons":         "RadioButtons",
	"Forms$ReferenceSelector":    "ReferenceSelector",
	"Pages$ReferenceSelector":    "ReferenceSelector",
	"Forms$DropDown":             "DropDown",
	"Pages$DropDown":             "DropDown",
	"Forms$DataGrid":             "DataGrid",
	"Pages$DataGrid":             "DataGrid",
	"Forms$DataView":             "DataView",
	"Pages$DataView":             "DataView",
	"Forms$ListView":             "ListView",
	"Pages$ListView":             "ListView",
	"Forms$LayoutGrid":           "LayoutGrid",
	"Pages$LayoutGrid":           "LayoutGrid",
	"Forms$DynamicText":          "DynamicText",
	"Pages$DynamicText":          "DynamicText",
	"Forms$Title":                "Title",
	"Pages$Title":                "Title",
	"Forms$Text":                 "StaticText",
	"Pages$Text":                 "StaticText",
	"Forms$Label":                "Label",
	"Pages$Label":                "Label",
	"Forms$StaticImageViewer":    "StaticImage",
	"Pages$StaticImageViewer":    "StaticImage",
	"Forms$DynamicImageViewer":   "DynamicImage",
	"Pages$DynamicImageViewer":   "DynamicImage",
	"Forms$Gallery":              "Gallery",
	"Pages$Gallery":              "Gallery",
	"Forms$NavigationList":       "NavigationList",
	"Pages$NavigationList":       "NavigationList",
	"Forms$SnippetCallWidget":    "SnippetCall",
	"Pages$SnippetCallWidget":    "SnippetCall",
	"CustomWidgets$CustomWidget": "CustomWidget",
}

// getWidgetDisplayName returns a short display name for a BSON widget $Type.
func getWidgetDisplayName(bsonType string) string {
	if name, ok := widgetTypeDisplayName[bsonType]; ok {
		return name
	}
	return bsonType
}
