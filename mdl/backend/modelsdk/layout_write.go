// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// layoutToGen builds a Forms$Layout.
//
// The shape is measured against Atlas_Core.Atlas_Default on 11.13.0, whose
// top-level keys are exactly $ID, $Type, Appearance, CanvasHeight, CanvasWidth,
// Content, Documentation, Excluded, ExportLevel, Name. Two things follow that a
// reading of gen alone would get wrong:
//
//   - The widget tree hangs off Content, a Forms$WebLayoutContent (or
//     Forms$NativeLayoutContent), never off the layout directly.
//   - LayoutType is a property of that wrapper. gen exposes Layout.LayoutType()
//     bound to a key the document does not carry, so setting it there writes a
//     property Mendix ignores and reads back "" — which is how DESCRIBE LAYOUT
//     came to report every layout as Responsive.
//
// Those ten keys are also the whole type: generated/metamodel's PagesLayout
// declares exactly them. gen additionally exposes a family of placeholder
// properties on Layout — MainPlaceholderName, AcceptPlaceholderName,
// UseMainPlaceholderForPopups and four more — that no Atlas layout carries and
// the metamodel does not declare. Writing one produces a document mxbuild
// accepts (measured: 0 errors) and Studio Pro refuses to open, because it
// resolves every stored property against the type's list. Which placeholder is
// "main" is a naming convention instead: 22 of 22 Atlas layouts name one Main,
// and a page binds to a placeholder by qualified name anyway
// (Atlas_Core.Atlas_Default.Main), so there is nothing for the document to say.
func layoutToGen(l *pages.Layout) (*genPg.Layout, error) {
	if l.Name == "" {
		return nil, fmt.Errorf("layout needs a name")
	}
	if l.LayoutType == "" {
		return nil, fmt.Errorf("layout %q needs a layout type (%s)", l.Name, layoutTypeChoices(l.Native))
	}
	// Validated against the platform rather than one flat list: Responsive on a
	// native layout is as meaningless as Default on a web one, and Mendix does
	// not report either.
	if !pages.ValidLayoutType(l.LayoutType, l.Native) {
		return nil, fmt.Errorf("layout %q: %q is not a %s layout type (%s)",
			l.Name, l.LayoutType, platformWord(l.Native), layoutTypeChoices(l.Native))
	}

	out := genPg.NewLayout()
	if l.ID != "" {
		out.SetID(element.ID(l.ID))
	}
	assignID(out)
	out.SetName(l.Name)
	out.SetDocumentation(l.Documentation)
	out.SetExportLevel("Hidden")
	out.SetCanvasWidth(1280)
	out.SetCanvasHeight(800)
	out.SetAppearance(newAppearance("", "", "", nil))
	// Written explicitly rather than left to the load-time default, so the
	// document carries the same ten keys Studio Pro writes.
	out.SetExcluded(false)

	// A layout with no placeholder can host no page: there is nowhere for the
	// page's content to go, and the page's Forms$FormCallArgument has nothing to
	// name. No Atlas layout is shaped that way and no page could use one, so it
	// is refused rather than written and discovered later.
	if !hasAnyPlaceholder(l.Widgets) {
		return nil, fmt.Errorf("layout %q declares no placeholder: a page's content has nowhere to go. "+
			"Add `placeholder Main` to the region that should hold page content", l.Name)
	}

	content, err := layoutContentToGen(l)
	if err != nil {
		return nil, err
	}
	out.SetContent(content)
	return out, nil
}

// layoutContentToGen builds the wrapper the widget tree hangs off.
func layoutContentToGen(l *pages.Layout) (element.Element, error) {
	widgets := make([]element.Element, 0, len(l.Widgets))
	for _, w := range l.Widgets {
		wg, err := widgetToGen(w)
		if err != nil {
			return nil, err
		}
		widgets = append(widgets, wg)
	}

	if l.Native {
		c := genPg.NewNativeLayoutContent()
		assignID(c)
		c.SetLayoutType(string(l.LayoutType))
		for _, w := range widgets {
			c.AddWidgets(w)
		}
		return c, nil
	}
	c := genPg.NewWebLayoutContent()
	assignID(c)
	c.SetLayoutType(string(l.LayoutType))
	for _, w := range widgets {
		c.AddWidgets(w)
	}
	return c, nil
}

// hasAnyPlaceholder walks the tree for a Forms$Placeholder.
func hasAnyPlaceholder(widgets []pages.Widget) bool {
	for _, w := range widgets {
		switch x := w.(type) {
		case *pages.LayoutPlaceholder:
			return true
		case *pages.ScrollContainer:
			for _, r := range x.Regions {
				if hasAnyPlaceholder(r.Widgets) {
					return true
				}
			}
			if hasAnyPlaceholder(x.Widgets) {
				return true
			}
		case *pages.Container:
			if hasAnyPlaceholder(x.Widgets) {
				return true
			}
		case *pages.GroupBox:
			if hasAnyPlaceholder(x.Widgets) {
				return true
			}
		}
	}
	return false
}

func platformWord(native bool) string {
	if native {
		return "native"
	}
	return "web"
}

func layoutTypeChoices(native bool) string {
	set := pages.WebLayoutTypes
	if native {
		set = pages.NativeLayoutTypes
	}
	out := ""
	for i, v := range set {
		if i > 0 {
			out += ", "
		}
		out += string(v)
	}
	return out
}

// CreateLayout inserts a new Forms$Layout unit.
func (b *Backend) CreateLayout(layout *pages.Layout) error {
	if layout == nil {
		return fmt.Errorf("CreateLayout: nil layout")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateLayout: not connected for writing")
	}
	if layout.ID == "" {
		layout.ID = model.ID(mmpr.GenerateID())
	}
	g, err := layoutToGen(layout)
	if err != nil {
		return err
	}
	g.SetID(element.ID(layout.ID))
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("CreateLayout: encode: %w", err)
	}
	if err := b.writer.InsertUnit(string(layout.ID), string(layout.ContainerID), "Documents", "Forms$Layout", contents); err != nil {
		return fmt.Errorf("CreateLayout: insert: %w", err)
	}
	return nil
}

// DeleteLayout removes a Forms$Layout unit. CREATE OR REPLACE LAYOUT is a
// delete followed by a create, so this is on the write path, not a convenience.
func (b *Backend) DeleteLayout(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteLayout: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}
