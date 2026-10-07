// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// isTabControl reports whether a stored widget is a tab container.
func isTabControl(widget bson.D) bool {
	switch widgetTypeName(widget) {
	case "Forms$TabControl", "Pages$TabControl":
		return true
	}
	return false
}

// InsertTabPages adds tab pages to an existing tab container.
//
// A tab page lives in the control's TabPages list, not in a Widgets list, and
// is not a widget — the same reason list view templates and DataGrid2 columns
// have their own paths. INSERT INTO <tabcontainer> appends; INSERT BEFORE/AFTER
// <tabpage> places the new pages next to that sibling. Anything else would put
// a tab page where only widgets belong, or a widget where only tab pages do.
//
// The pages are serialized by wrapping them in a throwaway tab container and
// taking its TabPages back out, so they get exactly the shape CREATE PAGE
// writes. The wrapper's DefaultPagePointer is discarded: the control keeps the
// default page it already had, and only an empty control gains one.
func (m *Mutator) InsertTabPages(targetRef string, position backend.InsertPosition, tabPages []*pages.TabPage) error {
	result := m.widgetFinder(m.rawData, targetRef)
	if result == nil {
		return m.widgetNotFoundError(targetRef)
	}
	into := strings.EqualFold(string(position), "into")
	isPage := widgetTypeName(result.widget) == tabPageType
	switch {
	case into && !isTabControl(result.widget):
		hint := ""
		if isPage {
			hint = fmt.Sprintf(" — to add a tab page next to %q, use `insert after %s { tabpage … }`", targetRef, targetRef)
		}
		return fmt.Errorf("cannot insert a tab page into %q (%s): a tab page can only be added to a tab container%s",
			targetRef, widgetTypeName(result.widget), hint)
	case !into && !isPage:
		return fmt.Errorf("cannot insert a tab page %s %q (%s): a tab page's siblings are tab pages — "+
			"use `insert %s <tabpage> { tabpage … }` or `insert into <tabcontainer> { tabpage … }`",
			strings.ToLower(string(position)), targetRef, widgetTypeName(result.widget), strings.ToLower(string(position)))
	}

	newDocs, err := m.serializeTabPages(tabPages)
	if err != nil {
		return err
	}

	if !into {
		// result.parentArr is the control's TabPages list and result.index the
		// sibling's slot in it, marker included.
		insertIdx := result.index
		if strings.EqualFold(string(position), "after") {
			insertIdx++
		}
		out := make([]any, 0, len(result.parentArr)+len(newDocs))
		out = append(out, result.parentArr[:insertIdx]...)
		out = append(out, newDocs...)
		out = append(out, result.parentArr[insertIdx:]...)
		bsonnav.DSetArray(result.parentDoc, result.parentKey, out)
		return nil
	}

	control := result.widget
	stored := bsonnav.ToBsonA(bsonnav.DGet(control, "TabPages"))
	var out bson.A
	switch {
	case len(stored) > 0 && isListMarker(stored[0]):
		out = append(out, stored...)
	case len(stored) == 0:
		out = append(out, int32(3)) // TabPages' list marker as Studio Pro writes it
	default:
		out = append(out, stored...)
	}
	hadPages := len(out) > 1 || (len(out) == 1 && !isListMarker(out[0]))
	out = append(out, newDocs...)

	if !bsonnav.DSet(control, "TabPages", out) {
		control = append(control, bson.E{Key: "TabPages", Value: out})
	}
	// An empty control has no default page; the first tab page is Studio Pro's
	// default, so give it one rather than leave the pointer null.
	if !hadPages {
		firstID := bsonnav.DGet(newDocs[0].(bson.D), "$ID")
		if !bsonnav.DSet(control, "DefaultPagePointer", firstID) {
			control = append(control, bson.E{Key: "DefaultPagePointer", Value: firstID})
		}
	}
	if result.parentArr == nil || result.index < 0 || result.index >= len(result.parentArr) {
		return fmt.Errorf("cannot add tab pages to %q: its parent slot could not be located", targetRef)
	}
	result.parentArr[result.index] = control
	bsonnav.DSetArray(result.parentDoc, result.parentKey, result.parentArr)
	return nil
}

// serializeTabPages returns the stored form of each tab page, without a list
// marker, by serializing them inside a wrapper tab container.
func (m *Mutator) serializeTabPages(tabPages []*pages.TabPage) ([]any, error) {
	if len(tabPages) == 0 {
		return nil, fmt.Errorf("no tab pages to insert")
	}
	wrapper := &pages.TabContainer{TabPages: tabPages}
	wrapper.TypeName = "Forms$TabControl"
	doc := m.deps.SerializeWidget(wrapper)
	var out []any
	for _, el := range bsonnav.DGetArrayElements(bsonnav.DGet(doc, "TabPages")) {
		if d, ok := el.(bson.D); ok {
			out = append(out, d)
		}
	}
	if len(out) != len(tabPages) {
		return nil, fmt.Errorf("serialize tab pages: got %d stored tab pages for %d inserted", len(out), len(tabPages))
	}
	return out, nil
}
