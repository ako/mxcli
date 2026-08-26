// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMenus "github.com/mendixlabs/mxcli/modelsdk/gen/menus"
	genPages "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	genTexts "github.com/mendixlabs/mxcli/modelsdk/gen/texts"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// menuDocumentType is the BSON storage name of a standalone menu document.
const menuDocumentType = "Menus$MenuDocument"

// CreateMenuDocument writes a new Menus$MenuDocument unit.
//
// This goes through gen + codec rather than hand-built BSON, which is what makes
// the typed-array markers come out right: the codec emits the default marker 3
// for a PartList, and Studio Pro's own menu documents carry 3 on both the
// collection's Items and each item's sub-Items (verified against Atlas_Core's
// Phone_Menu / Tablet_Menu). The navigation writers build their menu items by
// hand and use 1 — that difference is deliberate here and not copied.
func (b *Backend) CreateMenuDocument(md *types.MenuDocument) error {
	if md == nil {
		return fmt.Errorf("CreateMenuDocument: nil menu")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateMenuDocument: not connected for writing")
	}
	if md.ID == "" {
		md.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := encodeMenuDocument(md)
	if err != nil {
		return fmt.Errorf("CreateMenuDocument: encode: %w", err)
	}
	return b.writer.InsertUnit(string(md.ID), string(md.ContainerID), "Documents", menuDocumentType, contents)
}

// UpdateMenuDocument rebuilds a menu document and rewrites its unit. The document
// is small and rebuilt wholesale, matching how enumerations are updated.
func (b *Backend) UpdateMenuDocument(md *types.MenuDocument) error {
	if md == nil {
		return fmt.Errorf("UpdateMenuDocument: nil menu")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateMenuDocument: not connected for writing")
	}
	if md.ID == "" {
		return fmt.Errorf("UpdateMenuDocument: menu %q has no ID", md.Name)
	}
	contents, err := encodeMenuDocument(md)
	if err != nil {
		return fmt.Errorf("UpdateMenuDocument: encode: %w", err)
	}
	return b.writer.UpdateRawUnit(string(md.ID), contents)
}

// DeleteMenuDocument removes a menu document unit by ID.
func (b *Backend) DeleteMenuDocument(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteMenuDocument: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

func encodeMenuDocument(md *types.MenuDocument) ([]byte, error) {
	g := genMenus.NewMenuDocument()
	g.SetID(element.ID(md.ID))
	g.SetName(md.Name)
	g.SetDocumentation(md.Documentation)
	g.SetExcluded(md.Excluded)
	exportLevel := md.ExportLevel
	if exportLevel == "" {
		exportLevel = "Hidden"
	}
	g.SetExportLevel(exportLevel)

	coll := genMenus.NewMenuItemCollection()
	coll.SetID(element.ID(mmpr.GenerateID()))
	for _, item := range md.Items {
		coll.AddItems(menuItemToGen(item))
	}
	g.SetItemCollection(coll)

	return (&codec.Encoder{}).Encode(g)
}

// menuItemToGen converts one semantic menu item (and its sub-items) to gen. It is
// the inverse of navMenuItemFromGen, and deliberately mirrors the same three
// concerns: caption, icon, action.
func menuItemToGen(item *types.NavMenuItem) element.Element {
	g := genMenus.NewMenuItem()
	g.SetID(element.ID(mmpr.GenerateID()))
	g.SetCaption(menuCaptionToGen(item.Caption))
	g.SetIcon(menuIconToGen(item))
	g.SetAction(menuActionToGen(item))
	for _, sub := range item.Items {
		g.AddItems(menuItemToGen(sub))
	}
	return g
}

// menuCaptionToGen builds the Texts$Text / Texts$Translation pair a caption is
// stored as. en_US matches what the navigation writers emit.
func menuCaptionToGen(caption string) element.Element {
	t := genTexts.NewText()
	t.SetID(element.ID(mmpr.GenerateID()))
	tr := genTexts.NewTranslation()
	tr.SetID(element.ID(mmpr.GenerateID()))
	tr.SetLanguageCode(model.AuthoringLanguage())
	tr.SetText(caption)
	t.AddTranslations(tr)
	return t
}

// menuIconToGen emits only Forms$IconCollectionIcon, the one variant MDL can
// name. A glyph icon carries a numeric code and an image icon points into an
// image collection; neither is expressible in the ICON clause, so an item that
// had one keeps no icon rather than getting a wrong one. DESCRIBE flags those on
// the way out, so the loss is visible rather than silent.
func menuIconToGen(item *types.NavMenuItem) element.Element {
	if item.Icon == "" {
		return nil
	}
	icon := genPages.NewIconCollectionIcon()
	icon.SetID(element.ID(mmpr.GenerateID()))
	icon.SetImageQualifiedName(item.Icon)
	return icon
}

// menuActionToGen builds the item's client action. The gen type names are the SDK
// names; their storage names are what Mendix writes — PageClientAction is
// Forms$FormAction, NoClientAction is Forms$NoAction.
func menuActionToGen(item *types.NavMenuItem) element.Element {
	switch {
	case item.Page != "":
		a := genPages.NewPageClientAction()
		a.SetID(element.ID(mmpr.GenerateID()))
		ps := genPages.NewPageSettings()
		ps.SetID(element.ID(mmpr.GenerateID()))
		ps.SetPageQualifiedName(item.Page)
		a.SetPageSettings(ps)
		return a
	case item.Microflow != "":
		a := genPages.NewMicroflowClientAction()
		a.SetID(element.ID(mmpr.GenerateID()))
		ms := genPages.NewMicroflowSettings()
		ms.SetID(element.ID(mmpr.GenerateID()))
		ms.SetMicroflowQualifiedName(item.Microflow)
		a.SetMicroflowSettings(ms)
		return a
	default:
		a := genPages.NewNoClientAction()
		a.SetID(element.ID(mmpr.GenerateID()))
		return a
	}
}
