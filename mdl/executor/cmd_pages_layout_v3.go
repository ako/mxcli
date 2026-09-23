// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// buildLayoutV3 converts `create layout Module.Name ( … ) { … }` to the
// semantic model. The codec (mdl/backend/modelsdk/layout_write.go) decides the
// BSON; nothing here knows about Forms$WebLayoutContent.
func (pb *pageBuilder) buildLayoutV3(s *ast.CreateLayoutStmt) (*pages.Layout, error) {
	props := &ast.WidgetV3{Properties: s.Properties}

	// A layout header has exactly one property. Ignoring the rest would let
	// `mainplaceholder: 'Main'` — the obvious thing to reach for, and a real
	// gen property — read as accepted while nothing was written; the metamodel
	// does not declare it and no Studio Pro layout carries it.
	for k := range s.Properties {
		if !strings.EqualFold(k, "LayoutType") && !strings.EqualFold(k, "Class") && !strings.EqualFold(k, "Style") {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"layout %s: unknown property %q (a layout header takes layouttype, class and style; "+
					"which placeholder is \"main\" is set by naming one Main, not by a property)",
				s.Name.String(), k))
		}
	}

	layoutType := pages.LayoutType(props.GetStringProp("LayoutType"))
	if layoutType == "" {
		return nil, mdlerrors.NewValidation(
			"layout needs a layouttype: Responsive, Phone, Tablet or ModalPopup for web; Default or Popup for native")
	}
	// The platform is inferred rather than declared: the two vocabularies are
	// disjoint (measured across all 22 layouts Atlas ships on 11.13.0), so a
	// separate `native:` property could only ever contradict the layout type.
	native, ok := platformForLayoutType(layoutType)
	if !ok {
		return nil, mdlerrors.NewValidation(fmt.Sprintf(
			"layout %s: unknown layouttype %q (want Responsive, Phone, Tablet, ModalPopup, Default or Popup)",
			s.Name.String(), layoutType))
	}

	layout := &pages.Layout{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Forms$Layout",
		},
		ContainerID:   pb.moduleID,
		Name:          s.Name.Name,
		Documentation: s.Documentation,
		LayoutType:    layoutType,
		Native:        native,
		Class:         props.GetStringProp("Class"),
		Style:         props.GetStringProp("Style"),
	}

	expanded, err := pb.expandFragments(s.Widgets)
	if err != nil {
		return nil, err
	}
	for _, astWidget := range expanded {
		w, err := pb.buildWidgetV3(astWidget)
		if err != nil {
			return nil, mdlerrors.NewBackend("build widget", err)
		}
		layout.Widgets = append(layout.Widgets, w)
	}

	return layout, nil
}

// platformForLayoutType maps a layout type onto the platform that uses it.
// Reports false for a value neither platform attests to, so a typo is an error
// rather than a web layout with a meaningless type.
func platformForLayoutType(t pages.LayoutType) (native bool, ok bool) {
	switch {
	case pages.ValidLayoutType(t, false):
		return false, true
	case pages.ValidLayoutType(t, true):
		return true, true
	}
	return false, false
}

// execCreateLayout handles CREATE [OR REPLACE|MODIFY] LAYOUT.
func execCreateLayout(ctx *ExecContext, s *ast.CreateLayoutStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return mdlerrors.NewBackend(fmt.Sprintf("find module %s", s.Name.Module), err)
	}

	// A layout in a marketplace module is not ours to rewrite: an upgrade
	// replaces the module wholesale and the edit is gone, which is the whole
	// reason Mendix's own guidance is to put custom layouts in a module of your
	// own. Refusing here is cheaper than discovering it at upgrade time.
	if isMarketplaceModule(ctx, module) {
		return mdlerrors.NewValidation(fmt.Sprintf(
			"layout %s: %s is a marketplace module — a layout written there is overwritten by the next module update. "+
				"Create the layout in a module of your own (Mendix's own guidance) and point pages at it with ALTER PAGE … SET LAYOUT.",
			s.Name.String(), s.Name.Module))
	}

	// The stored layout this statement rewrites, if there is one, plus any
	// duplicates of the same name to clear out.
	existing, _ := ctx.Backend.ListLayouts()
	var stored *pages.Layout
	var duplicates []model.ID
	for _, l := range existing {
		modName := getModuleName(ctx, getModuleID(ctx, l.ContainerID))
		if modName != s.Name.Module || l.Name != s.Name.Name {
			continue
		}
		if !s.IsReplace && !s.IsModify {
			return mdlerrors.NewAlreadyExists("layout", s.Name.String())
		}
		if stored == nil {
			stored = l
			continue
		}
		duplicates = append(duplicates, l.ID)
	}

	pb := &pageBuilder{
		ctx:              ctx,
		backend:          ctx.Backend,
		moduleID:         module.ID,
		moduleName:       s.Name.Module,
		widgetScope:      make(map[string]model.ID),
		paramScope:       make(map[string]model.ID),
		paramEntityNames: make(map[string]string),
		execCache:        ctx.Cache,
		fragments:        ctx.Fragments,
		themeRegistry:    ctx.GetThemeRegistry(),
		widgetBackend:    ctx.Backend,
		// The root of a document that this pass walks in full: there is no
		// enclosing data widget, so there is no context object. #1029.
		argCtx: atDocumentRoot(),
	}

	// Built before the old one is deleted: a build failure must leave the
	// project as it was, not layout-less.
	layout, err := pb.buildLayoutV3(s)
	if err != nil {
		return err
	}

	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if stored != nil {
		layout.Documentation = carriedDocumentation(s.DocumentationSet, s.Documentation, stored.Documentation)
	}

	// A duplicate of the same name is cleared; the FIRST stored layout is
	// rewritten rather than deleted (see below), so it is not in this list.
	for _, id := range duplicates {
		if err := ctx.Backend.DeleteLayout(id); err != nil {
			return mdlerrors.NewBackend("delete existing layout", err)
		}
	}

	verb := "Created"
	if stored != nil {
		verb = "Replaced"
		// REWRITE THE STORED UNIT, do not replace it. A delete followed by a
		// create goes through InsertUnit under a freshly minted id, so an
		// identical re-run replaced the layout's unit under a NEW GUID every
		// time — measured on 11.14.0, three runs produced three different
		// .mxunit files, each run a delete plus an untracked add, and the tree
		// never came back clean (ako/mxcli#600). The storage layer's net for
		// delete+insert recreates (#556) keys on the unit id and so cannot
		// catch a path that re-mints it; this has to be decided here.
		//
		// Carrying the stored container is the other half: there is no FOLDER
		// clause on CREATE LAYOUT, so the rebuilt layout always names the
		// module root, and an insert would file a foldered layout back into it
		// (the defect #932 fixed for REST clients). UpdateLayout does not touch
		// the unit's row at all.
		layout.ID = stored.ID
		layout.ContainerID = stored.ContainerID
		if err := ctx.Backend.UpdateLayout(layout); err != nil {
			return mdlerrors.NewBackend("update layout", err)
		}
	} else if err := ctx.Backend.CreateLayout(layout); err != nil {
		return mdlerrors.NewBackend("create layout", err)
	}

	invalidateHierarchy(ctx)

	// Through ReportMutation: the rewrite now reaches canon.Reconcile, so a
	// statement that matches what is stored is elided and must say so rather
	// than claiming a replacement that did not happen.
	ctx.ReportMutation(verb, "layout %s", s.Name.String())
	return nil
}

// isMarketplaceModule reports whether the module came from the Marketplace.
//
// Both stored signals count. FromAppStore is the flag Studio Pro sets and
// SHOW MODULES reports; AppStoreGuid is the version UUID the importer writes.
// A real import carries both, but checking only one makes the guard depend on
// which field a given path happened to populate — and a marketplace guard that
// quietly stops firing is worse than no guard, because the refusal is what
// stops an edit an update will erase. The module *name* is not a signal: a user
// may name a module Atlas_Core.
func isMarketplaceModule(ctx *ExecContext, module *model.Module) bool {
	if module == nil {
		return false
	}
	return module.FromAppStore || strings.TrimSpace(module.AppStoreGuid) != ""
}
