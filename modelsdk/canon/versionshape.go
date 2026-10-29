// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/version"
)

// LowerToProjectShape rewrites the properties whose storage shape changed at a
// Mendix version into the shape a project of version pv stores (ako/mxcli#1209).
//
// The writers build every document in one shape — the current one — and the
// metamodel keeps the older shapes as different properties of the same type, so
// for these the property floor (StripUndeclaredProperties) can only refuse the
// newer key: it cannot know that the older key holds the same fact. Measured on
// Studio Pro-authored 9.24.30 pages and in mendixmodelsdk:
//
//   - Forms$DesignPropertyValue: a typed Value sub-document from 10.2.0; before,
//     Type ("Toggle"/"DropDown"), StringValue and BooleanValue on the entry
//     itself. Studio Pro 9.24 writes `{BooleanValue: true, Key, StringValue: "",
//     Type: "Toggle"}` and `{BooleanValue: false, Key, StringValue: "Outer
//     small", Type: "DropDown"}`, keys in that order.
//   - Forms$DataView: Editability ("Always"/"Never"/"Conditional") from 10.7.0;
//     before, Editable as a boolean.
//   - Forms$TextBox/TextArea/DatePicker: PlaceholderTemplate (a
//     Forms$ClientTemplate) from 10.11.0; before, Placeholder, the template's
//     Texts$Text. Declared on the abstract AttributeWidgetWithPlaceholder, which
//     the floor does not look up, so it passed through unrefused and the key the
//     version reads was never written.
//   - Forms$DataViewSource: SourceVariable (a Forms$PageVariable, declared on
//     EntityPathSource from 10.0.0); before, the PageParameter and
//     SnippetParameter names themselves. Without them 9.24 sees no parameter:
//     CE6050 "The selected entity must be configured with a page parameter".
//   - ForceFullObjects, declared on the abstract Forms$DataSource from 10.8.0:
//     every data source carried it, and the floor never saw it.
//   - Settings$ConstantValue: a Settings$SharedValue under SharedOrPrivateValue
//     from 10.9.0; before, the value as a flat Value string, `{$ID, $Type,
//     ConstantId, Value}` (`mx convert` 9.24.30, which empties the nested form
//     it does not know).
//   - Forms$MicroflowParameterMapping / NanoflowParameterMapping: an argument
//     as Expression text from 10.10.0; before, only a Variable (a required
//     Forms$PageVariable), and an argument that is the enclosing object is not
//     stored at all — Studio Pro 9.24 infers it (Workflow Commons 2.6.0: 77
//     flow calls taking parameters, no mapping on any). mxcli writes that
//     argument as Expression `$currentObject` with Variable null, which 9.24
//     cannot even load.
//   - Workflows$ PersistentId, from 10.21.0 on the workflow and on the abstract
//     WorkflowActivity, Outcome and BoundaryEvent (ako/mxcli#1163). Workflows
//     exist from 9.0; `mx convert` 9.24.30 strips every one. The workflow's
//     own was refused; the others slipped past the floor.
//   - Workflows$EndOfParallelSplitPathActivity, a TYPE introduced in 10.21.0,
//     which ends every parallel split path mxcli writes (ako/mxcli#1163).
//     Before, a path is its activities and nothing more (Studio Pro 9.4.0, the
//     Marketplace's "Example project workflow extension" 2.0.0), and a project
//     holding the marker does not load (TypeCacheUnknownTypeException). The
//     floor checks property versions, never a type's own.
//   - Workflows$ user task targeting: UserTargeting from 11.2.0, declared on
//     the abstract UserTaskActivity; before, UserSource (ako/mxcli#1771). See
//     lowerUserTargeting.
//   - Workflows$SingleUserTaskActivity and MultiUserTaskActivity, TYPES
//     introduced in 10.12.0; before, a Workflows$UserTask whose
//     UserTaskCompletion says which (ako/mxcli#1771). See lowerUserTaskType.
//
// It runs on every unit write, before the floor, so no writer needs to know
// these shapes. What the older shape cannot hold — a design property value
// that is neither a toggle nor an option, conditional data view editability, a
// placeholder with parameters — is refused, naming it, rather than dropped.
func LowerToProjectShape(contents []byte, pv *version.Version) ([]byte, error) {
	if pv == nil || pv.IsZero() {
		return contents, nil
	}
	l := shapeLowering{
		pv:              *pv,
		flatDesignProps: !PropertyDeclared("Forms$DesignPropertyValue", "Value", *pv),
		boolEditable:    !PropertyDeclared("Forms$DataView", "Editability", *pv),
		textPlaceholder: !PropertyDeclared("Forms$AttributeWidgetWithPlaceholder", "PlaceholderTemplate", *pv),
		namedParameter:  !PropertyDeclared("Forms$EntityPathSource", "SourceVariable", *pv),
		noFullObjects:   !PropertyDeclared("Forms$DataSource", "ForceFullObjects", *pv),
		noPersistentID:  !PropertyDeclared("Workflows$Workflow", "PersistentId", *pv),
		flatConstant:    !PropertyDeclared("Settings$ConstantValue", "SharedOrPrivateValue", *pv),
		variableArgs:    !PropertyDeclared("Forms$MicroflowParameterMapping", "Expression", *pv),
		unmarkedPaths:   pv.Compare(endOfParallelSplitPathSince) < 0,
		userSource:      !PropertyDeclared("Workflows$UserTaskActivity", "UserTargeting", *pv),
		taskBoundaries:  !PropertyDeclared("Workflows$UserTaskActivity", "BoundaryEvents", *pv),
		userTaskType:    pv.Compare(userTaskActivitySince) < 0,
	}
	if !l.flatDesignProps && !l.boolEditable && !l.textPlaceholder && !l.namedParameter && !l.noFullObjects &&
		!l.noPersistentID && !l.flatConstant && !l.variableArgs && !l.unmarkedPaths && !l.userSource &&
		!l.taskBoundaries && !l.userTaskType {
		return contents, nil
	}
	var doc bson.D
	if err := bson.Unmarshal(contents, &doc); err != nil {
		return nil, err
	}
	l.root = doc
	out, err := l.walk(doc)
	if err != nil {
		return nil, err
	}
	if !l.changed {
		return contents, nil
	}
	return bson.Marshal(out)
}

type shapeLowering struct {
	pv                                             version.Version
	flatDesignProps, boolEditable, textPlaceholder bool
	namedParameter, noFullObjects, noPersistentID  bool
	flatConstant, variableArgs, unmarkedPaths      bool
	userSource, taskBoundaries, userTaskType       bool
	changed                                        bool
	// root is the unit, and used the $IDs it holds, for an element the
	// lowering adds (see lowerUserTaskType).
	root bson.D
	used map[string]bool
}

// endOfParallelSplitPathSince is the version that introduced the type
// Workflows$EndOfParallelSplitPathActivity (mendixmodelsdk 4.117.0:
// EndOfParallelSplitPathActivity.versionInfo introduced "10.21.0"). It is
// written down here because the generated version data holds property
// versions only, never a type's own.
var endOfParallelSplitPathSince = version.Version{Major: 10, Minor: 21}

var placeholderWidgets = map[string]bool{"Forms$TextBox": true, "Forms$TextArea": true, "Forms$DatePicker": true}

// walk returns v with every element below it in the project's shape.
func (l *shapeLowering) walk(v any) (any, error) {
	switch x := v.(type) {
	case bson.D:
		typ, _ := shapeString(x, "$Type")
		var err error
		switch {
		case l.flatDesignProps && typ == "Forms$DesignPropertyValue":
			if x, err = l.lowerDesignProperty(x); err != nil {
				return nil, err
			}
		case typ == "Forms$DataView":
			if l.boolEditable {
				if err = l.lowerEditability(x); err != nil {
					return nil, err
				}
			}
			if l.namedParameter {
				if err = l.lowerDataViewSource(x); err != nil {
					return nil, err
				}
			}
		case l.textPlaceholder && placeholderWidgets[typ]:
			if err = l.lowerPlaceholder(x, typ); err != nil {
				return nil, err
			}
		case l.flatConstant && typ == "Settings$ConstantValue":
			if x, err = l.lowerConstantValue(x); err != nil {
				return nil, err
			}
		case userTaskTypes[typ]:
			if x, err = l.lowerUserTask(x, typ); err != nil {
				return nil, err
			}
		}
		if l.noFullObjects {
			if x, err = l.dropForceFullObjects(x, typ); err != nil {
				return nil, err
			}
		}
		if l.noPersistentID && strings.HasPrefix(typ, "Workflows$") {
			x = l.dropPersistentID(x)
		}
		if l.unmarkedPaths && typ == "Workflows$Flow" {
			l.dropEndOfPathMarkers(x)
		}
		if l.variableArgs {
			if err = l.lowerFlowArguments(x); err != nil {
				return nil, err
			}
		}
		for i := range x {
			if x[i].Value, err = l.walk(x[i].Value); err != nil {
				return nil, err
			}
		}
		return x, nil
	case bson.A:
		for i := range x {
			var err error
			if x[i], err = l.walk(x[i]); err != nil {
				return nil, err
			}
		}
		return x, nil
	}
	return v, nil
}

// lowerDesignProperty returns the flat entry Studio Pro 9.24 stores for a
// design property entry holding a typed Value.
func (l *shapeLowering) lowerDesignProperty(d bson.D) (bson.D, error) {
	value, ok := shapeValue(d, "Value")
	if !ok {
		return d, nil // already flat
	}
	key, _ := shapeString(d, "Key")
	valueDoc, _ := value.(bson.D)
	valueType, _ := shapeString(valueDoc, "$Type")
	var typ, option string
	var on bool
	switch valueType {
	case "Forms$ToggleDesignPropertyValue":
		typ, on = "Toggle", true
	case "Forms$OptionDesignPropertyValue":
		typ = "DropDown"
		option, _ = shapeString(valueDoc, "Option")
	default:
		return nil, fmt.Errorf("design property %q holds a %s, which has no form before Mendix 10.2 "+
			"(this project is %s): only a toggle or a single option can be stored", key, valueType, l.pv)
	}
	id, _ := shapeValue(d, "$ID")
	l.changed = true
	return bson.D{
		{Key: "$ID", Value: id},
		{Key: "$Type", Value: "Forms$DesignPropertyValue"},
		{Key: "BooleanValue", Value: on},
		{Key: "Key", Value: key},
		{Key: "StringValue", Value: option},
		{Key: "Type", Value: typ},
	}, nil
}

// lowerEditability replaces a data view's Editability with the boolean
// Editable older versions store.
func (l *shapeLowering) lowerEditability(d bson.D) error {
	i := shapeIndex(d, "Editability")
	if i < 0 {
		return nil
	}
	name, _ := shapeString(d, "Name")
	editability, _ := d[i].Value.(string)
	var editable bool
	switch editability {
	case "Always", "":
		editable = true
	case "Never":
		editable = false
	default:
		return fmt.Errorf("data view %q is editable %q, which Mendix introduced in 10.7 (this project is %s): "+
			"a data view there is either editable or not", name, editability, l.pv)
	}
	if ces, _ := shapeValue(d, "ConditionalEditabilitySettings"); ces != nil {
		return fmt.Errorf("data view %q has conditional editability, which Mendix introduced in 10.7 (this project is %s)",
			name, l.pv)
	}
	d[i] = bson.E{Key: "Editable", Value: editable}
	l.changed = true
	return nil
}

// lowerPlaceholder replaces a PlaceholderTemplate with the Placeholder text
// older versions store: the template's own Texts$Text.
func (l *shapeLowering) lowerPlaceholder(d bson.D, typ string) error {
	i := shapeIndex(d, "PlaceholderTemplate")
	if i < 0 {
		return nil
	}
	tmpl, _ := d[i].Value.(bson.D)
	if params, _ := shapeValue(tmpl, "Parameters"); len(listItems(params)) > 0 {
		name, _ := shapeString(d, "Name")
		return fmt.Errorf("%s %q has a placeholder with parameters, which Mendix introduced in 10.11 (this project is %s)",
			typ, name, l.pv)
	}
	text, _ := shapeValue(tmpl, "Template")
	d[i] = bson.E{Key: "Placeholder", Value: text}
	l.changed = true
	return nil
}

// lowerDataViewSource replaces a data view source's SourceVariable with the
// parameter names older versions store on the source itself.
func (l *shapeLowering) lowerDataViewSource(dv bson.D) error {
	srcAny, _ := shapeValue(dv, "DataSource")
	src, _ := srcAny.(bson.D)
	if t, _ := shapeString(src, "$Type"); t != "Forms$DataViewSource" {
		return nil
	}
	i := shapeIndex(src, "SourceVariable")
	if i < 0 {
		return nil
	}
	name, _ := shapeString(dv, "Name")
	variable, _ := src[i].Value.(bson.D)
	page, _ := shapeString(variable, "PageParameter")
	snippet, _ := shapeString(variable, "SnippetParameter")
	widget, _ := shapeString(variable, "Widget")
	all, _ := shapeValue(variable, "UseAllPages")
	if widget != "" || all == true || (page == "" && snippet == "" && variable != nil) {
		return fmt.Errorf("data view %q takes its object from another widget's or from every page, which Mendix "+
			"introduced in 10.0 (this project is %s): a data view there takes a page or snippet parameter", name, l.pv)
	}
	named := bson.D{}
	if PropertyDeclared("Forms$DataViewSource", "PageParameter", l.pv) {
		named = append(named, bson.E{Key: "PageParameter", Value: page})
	}
	if PropertyDeclared("Forms$DataViewSource", "SnippetParameter", l.pv) {
		named = append(named, bson.E{Key: "SnippetParameter", Value: snippet})
	}
	out := append(append(append(bson.D{}, src[:i]...), named...), src[i+1:]...)
	dvSrc := shapeIndex(dv, "DataSource")
	dv[dvSrc].Value = out
	l.changed = true
	return nil
}

// dropForceFullObjects removes a data source's ForceFullObjects below 10.8,
// where no data source declares it; set, it is refused.
func (l *shapeLowering) dropForceFullObjects(d bson.D, typ string) (bson.D, error) {
	i := shapeIndex(d, "ForceFullObjects")
	if i < 0 {
		return d, nil
	}
	if on, _ := d[i].Value.(bool); on {
		return nil, fmt.Errorf("%s forces full objects, which Mendix introduced in 10.8 (this project is %s)", typ, l.pv)
	}
	l.changed = true
	return append(append(bson.D{}, d[:i]...), d[i+1:]...), nil
}

// lowerConstantValue replaces a constant override's nested SharedOrPrivateValue
// with the flat Value older versions store, in its place. A flat Value already
// beside it (the overlay clears one to "" when it writes the nested form) takes
// the nested value. A private value — kept on the developer's workstation, which
// the flat form cannot say — is refused.
func (l *shapeLowering) lowerConstantValue(d bson.D) (bson.D, error) {
	i := shapeIndex(d, "SharedOrPrivateValue")
	if i < 0 {
		return d, nil
	}
	inner, _ := d[i].Value.(bson.D)
	if t, _ := shapeString(inner, "$Type"); t == "Settings$PrivateValue" {
		constant, _ := shapeString(d, "ConstantId")
		return nil, fmt.Errorf("constant %q has a private value in this configuration, which Mendix introduced in 10.9 "+
			"(this project is %s): a constant value there is shared, stored in the model", constant, l.pv)
	}
	value, _ := shapeString(inner, "Value")
	l.changed = true
	if j := shapeIndex(d, "Value"); j >= 0 {
		d[j].Value = value
		return append(append(bson.D{}, d[:i]...), d[i+1:]...), nil
	}
	d[i] = bson.E{Key: "Value", Value: value}
	return d, nil
}

var flowParameterMappings = map[string]bool{
	"Forms$MicroflowParameterMapping": true, "Forms$NanoflowParameterMapping": true,
}

// lowerFlowArguments rewrites the flow call arguments in d's ParameterMappings
// list, if it has one, into what a version before 10.10 stores: an argument
// bound through Variable keeps it (its empty Expression is the floor's to
// drop); an argument that is the enclosing object (`$currentObject`) — or one
// that binds nothing — is left out, which is how Studio Pro 9.24 stores it.
// Any other expression has no Variable form and is refused, naming the
// parameter.
func (l *shapeLowering) lowerFlowArguments(d bson.D) error {
	i := shapeIndex(d, "ParameterMappings")
	if i < 0 {
		return nil
	}
	list, _ := d[i].Value.(bson.A)
	out := bson.A{}
	dropped := false
	for _, el := range list {
		m, ok := el.(bson.D)
		typ, _ := shapeString(m, "$Type")
		if !ok || !flowParameterMappings[typ] {
			out = append(out, el)
			continue
		}
		if v, _ := shapeValue(m, "Variable"); v != nil {
			out = append(out, el)
			continue
		}
		expr, _ := shapeString(m, "Expression")
		if e := strings.TrimSpace(expr); e != "" && !strings.EqualFold(e, "$currentObject") {
			param, _ := shapeString(m, "Parameter")
			return fmt.Errorf("the argument for %s is the expression %s, which Mendix introduced in 10.10 (this project is %s): "+
				"an argument there is the enclosing object, or a page parameter, snippet parameter or widget", param, expr, l.pv)
		}
		dropped = true
	}
	if dropped {
		d[i].Value = out
		l.changed = true
	}
	return nil
}

// dropPersistentID removes a workflow element's PersistentId below 10.21, where
// none declares it. Whatever it holds is dropped, not refused: a PersistentId is
// minted by the writer (a fresh GUID on every build), never stated by a script,
// and a workflow Studio Pro 9.24 stores has none.
func (l *shapeLowering) dropPersistentID(d bson.D) bson.D {
	i := shapeIndex(d, "PersistentId")
	if i < 0 {
		return d
	}
	l.changed = true
	return append(append(bson.D{}, d[:i]...), d[i+1:]...)
}

// dropEndOfPathMarkers removes the Workflows$EndOfParallelSplitPathActivity
// from a flow's activities below 10.21, where the type does not exist. The
// list keeps its leading type marker, so a path that held only the end-of-path
// activity stores `Activities: [3]`, as Studio Pro 9 stores an empty flow.
// Nothing is refused:
// the marker is the writer's, never something a script states, and a path
// before 10.21 is its activities alone (Studio Pro 9.4.0's parallel split).
func (l *shapeLowering) dropEndOfPathMarkers(flow bson.D) {
	i := shapeIndex(flow, "Activities")
	if i < 0 {
		return
	}
	list, _ := flow[i].Value.(bson.A)
	out := make(bson.A, 0, len(list))
	for _, el := range list {
		if d, ok := el.(bson.D); ok {
			if t, _ := shapeString(d, "$Type"); t == "Workflows$EndOfParallelSplitPathActivity" {
				continue
			}
		}
		out = append(out, el)
	}
	if len(out) != len(list) {
		flow[i].Value = out
		l.changed = true
	}
}

// listItems returns the elements of a stored list, without its type marker.
func listItems(v any) bson.A {
	a, _ := v.(bson.A)
	if len(a) > 0 {
		switch a[0].(type) {
		case int32, int64:
			return a[1:]
		}
	}
	return a
}

func shapeIndex(d bson.D, key string) int {
	for i, e := range d {
		if e.Key == key {
			return i
		}
	}
	return -1
}

func shapeValue(d bson.D, key string) (any, bool) {
	if i := shapeIndex(d, key); i >= 0 {
		return d[i].Value, true
	}
	return nil, false
}

func shapeString(d bson.D, key string) (string, bool) {
	v, ok := shapeValue(d, key)
	s, _ := v.(string)
	return s, ok
}
