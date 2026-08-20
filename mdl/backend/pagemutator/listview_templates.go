// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// listViewTemplateType is the stored $Type of a List View specialization
// template. The entity it renders is stored under "Entity" — the SDK calls that
// property Specialization, but no document does; see pages.ListViewTemplate.
const listViewTemplateType = "Forms$ListViewTemplate"

// findListView locates a List View by name and returns it with its parent slot,
// so a caller that has to append a missing field can store the grown document
// back (a bson.D is a slice: appending to it reallocates).
func (m *Mutator) findListView(listViewRef string) (*bsonWidgetResult, error) {
	result := m.widgetFinder(m.rawData, listViewRef)
	if result == nil {
		return nil, fmt.Errorf("widget %q not found", listViewRef)
	}
	if t := widgetTypeName(result.widget); t != "Forms$ListView" && t != "Pages$ListView" {
		return nil, fmt.Errorf("widget %q is a %s, not a list view — specialization templates "+
			"exist only on a list view (a gallery's `template <name>` is a named content slot, "+
			"a different construct)", listViewRef, t)
	}
	return result, nil
}

// storedTemplateEntities returns the specializations a list view already renders,
// in stored order.
func storedTemplateEntities(listView bson.D) []string {
	var out []string
	for _, el := range bsonnav.DGetArrayElements(bsonnav.DGet(listView, "Templates")) {
		if doc, ok := el.(bson.D); ok {
			out = append(out, bsonnav.DGetString(doc, "Entity"))
		}
	}
	return out
}

// InsertListViewTemplates appends specialization templates to a list view.
//
// They go in the Templates array, never in Widgets: Widgets holds the default
// body, and a Forms$ListViewTemplate is not a widget. Appending one to the
// widget list produces a document Studio Pro cannot open, which is the same
// reason DataGrid2 columns have their own path rather than going through
// InsertWidget.
func (m *Mutator) InsertListViewTemplates(listViewRef string, templates []*pages.ListViewTemplate) error {
	result, err := m.findListView(listViewRef)
	if err != nil {
		return err
	}

	// One template per specialization, matching what CREATE PAGE enforces. Mendix
	// renders the first match, so a second template for the same entity is dead
	// weight the author cannot see.
	existing := storedTemplateEntities(result.widget)
	for _, tpl := range templates {
		for _, have := range existing {
			if strings.EqualFold(have, tpl.Specialization) {
				return fmt.Errorf("list view %q already has a template for %s", listViewRef, tpl.Specialization)
			}
		}
		existing = append(existing, tpl.Specialization)
	}

	newDocs := make([]any, 0, len(templates))
	for _, tpl := range templates {
		children, err := m.serializeWidgets(tpl.Widgets)
		if err != nil {
			return err
		}
		// The child list carries Mendix's list marker like any other widget list:
		// 3 when empty, 2 when populated.
		childArr := bson.A{int32(3)}
		if len(children) > 0 {
			childArr = bson.A{int32(2)}
			childArr = append(childArr, children...)
		}
		id := string(tpl.ID)
		if id == "" {
			id = types.GenerateID()
		}
		// Key order matches Studio Pro's documents.
		newDocs = append(newDocs, bson.D{
			{Key: "$ID", Value: bsonutil.IDToBsonBinary(id)},
			{Key: "$Type", Value: listViewTemplateType},
			{Key: "Entity", Value: tpl.Specialization},
			{Key: "Widgets", Value: childArr},
		})
	}

	stored := bsonnav.ToBsonA(bsonnav.DGet(result.widget, "Templates"))
	var out bson.A
	switch {
	case len(stored) > 0 && isListMarker(stored[0]):
		// Appending to an empty list flips the marker from 3 to 2, the same
		// distinction Studio Pro makes.
		out = append(out, int32(2))
		out = append(out, stored[1:]...)
		out = append(out, newDocs...)
	case len(stored) == 0:
		out = append(out, int32(2))
		out = append(out, newDocs...)
	default:
		out = append(out, stored...)
		out = append(out, newDocs...)
	}

	if bsonnav.DSet(result.widget, "Templates", out) {
		return nil
	}
	// The field was absent: appending grows the bson.D, so the widget has to be
	// stored back into its parent slot or the change is written to a copy.
	grown := append(result.widget, bson.E{Key: "Templates", Value: out})
	if result.parentArr == nil || result.index < 0 || result.index >= len(result.parentArr) {
		return fmt.Errorf("cannot add templates to list view %q: its parent slot could not be located", listViewRef)
	}
	result.parentArr[result.index] = grown
	bsonnav.DSetArray(result.parentDoc, result.parentKey, result.parentArr)
	return nil
}

// DropListViewTemplate removes the template rendering a given specialization.
//
// A template has no name, so it is addressed by entity. An entity that matches
// nothing is an error naming what IS there: dropping nothing and reporting
// success is how a typo in a specialization name becomes a silent no-op.
func (m *Mutator) DropListViewTemplate(listViewRef, specialization string) error {
	result, err := m.findListView(listViewRef)
	if err != nil {
		return err
	}

	stored := bsonnav.ToBsonA(bsonnav.DGet(result.widget, "Templates"))
	var out bson.A
	var marker []any
	rest := []any(stored)
	if len(stored) > 0 && isListMarker(stored[0]) {
		marker = []any{stored[0]}
		rest = stored[1:]
	}

	dropped := false
	for _, el := range rest {
		doc, ok := el.(bson.D)
		if ok && strings.EqualFold(bsonnav.DGetString(doc, "Entity"), specialization) {
			dropped = true
			continue
		}
		out = append(out, el)
	}
	if !dropped {
		have := storedTemplateEntities(result.widget)
		if len(have) == 0 {
			return fmt.Errorf("list view %q has no specialization templates", listViewRef)
		}
		return fmt.Errorf("list view %q has no template for %s (it has: %s)",
			listViewRef, specialization, strings.Join(have, ", "))
	}

	// An emptied list reverts to Mendix's empty-list marker.
	final := bson.A{}
	if len(marker) > 0 && len(out) == 0 {
		final = bson.A{int32(3)}
	} else {
		final = append(final, marker...)
		final = append(final, out...)
	}
	bsonnav.DSetArray(result.parentDoc, result.parentKey, result.parentArr)
	if !bsonnav.DSet(result.widget, "Templates", final) {
		return fmt.Errorf("list view %q has no Templates array to drop from", listViewRef)
	}
	return nil
}
