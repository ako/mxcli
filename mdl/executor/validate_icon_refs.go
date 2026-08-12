// SPDX-License-Identifier: Apache-2.0

// Reference validation for icon-collection references.
//
// `icon: 'Atlas_Core.Atlas_Filled.pencil'` names an icon inside an icon
// collection document. Nothing resolved it: the name was written through to
// BSON verbatim, `mxcli check` passed, and the first sign of a typo was MxBuild:
//
//	[error] [CE1613] "The selected custom icon
//	'Atlas_Core.Atlas_Filled.no-such-icon' no longer exists." at Action button 'btnBad'
//
// The collections live in the project (Atlas_Core ships three, ~770 icons), so
// this needs -p — it runs in the --references pass alongside the other
// project-resolved references.
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// iconIndex holds the project's icon collections, keyed by qualified collection
// name (Module.Collection) → set of icon names.
type iconIndex struct {
	collections map[string]map[string]bool
	// order preserves a stable listing for error messages.
	order []string
}

// buildIconIndex reads the project's icon collections once per validation run.
// Returns nil when the project exposes none, which disables the check rather
// than reporting every icon as unknown.
func buildIconIndex(ctx *ExecContext) *iconIndex {
	cols, err := ctx.Backend.ListIconCollections()
	if err != nil || len(cols) == 0 {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	idx := &iconIndex{collections: make(map[string]map[string]bool, len(cols))}
	for _, c := range cols {
		qn := iconCollectionQualifiedName(h, c)
		if qn == "" {
			continue
		}
		names := make(map[string]bool, len(c.Icons))
		for _, ic := range c.Icons {
			names[ic.Name] = true
		}
		idx.collections[qn] = names
		idx.order = append(idx.order, qn)
	}
	sort.Strings(idx.order)
	if len(idx.collections) == 0 {
		return nil
	}
	return idx
}

func iconCollectionQualifiedName(h *ContainerHierarchy, c *types.IconCollection) string {
	mod := h.GetModuleName(h.FindModuleID(c.ContainerID))
	if mod == "" || c.Name == "" {
		return ""
	}
	return mod + "." + c.Name
}

// validateIconRefs resolves every icon reference in the program against the
// project's icon collections.
func validateIconRefs(ctx *ExecContext, prog *ast.Program) []error {
	if !ctx.Connected() {
		return nil
	}
	idx := buildIconIndex(ctx)
	if idx == nil {
		return nil
	}

	var errs []error
	for i, stmt := range prog.Statements {
		for _, ref := range iconRefsInStatement(stmt) {
			if err := idx.check(ref); err != nil {
				errs = append(errs, fmt.Errorf("statement %d: %w", i+1, err))
			}
		}
	}
	return errs
}

// iconRef is one icon reference and where it was written, for the message.
type iconRef struct {
	value string // as authored, e.g. Atlas_Core.Atlas_Filled.pencil
	where string // e.g. `button "btnSave"` or `menu item 'Home'`
}

// iconRefsInStatement collects icon references from the statements that can
// carry one: page/snippet widget trees and navigation menus.
func iconRefsInStatement(stmt ast.Statement) []iconRef {
	var out []iconRef
	switch s := stmt.(type) {
	case *ast.CreatePageStmtV3:
		for _, w := range s.Widgets {
			out = append(out, iconRefsInWidget(w)...)
		}
	case *ast.AlterPageStmt:
		for _, op := range s.Operations {
			switch o := op.(type) {
			case *ast.SetPropertyOp:
				if v := iconPropValue(o.Properties); v != "" {
					out = append(out, iconRef{value: v, where: "widget " + quoteName(o.Target.Name())})
				}
			case *ast.InsertWidgetOp:
				for _, w := range o.Widgets {
					out = append(out, iconRefsInWidget(w)...)
				}
			case *ast.ReplaceWidgetOp:
				for _, w := range o.NewWidgets {
					out = append(out, iconRefsInWidget(w)...)
				}
			}
		}
	case *ast.AlterNavigationStmt:
		for _, item := range s.MenuItems {
			out = append(out, iconRefsInMenu(item)...)
		}
	}
	return out
}

func iconRefsInWidget(w *ast.WidgetV3) []iconRef {
	if w == nil {
		return nil
	}
	var out []iconRef
	if v := iconPropValue(w.Properties); v != "" {
		out = append(out, iconRef{value: v, where: strings.ToLower(w.Type) + " " + quoteName(w.Name)})
	}
	for _, c := range w.Children {
		out = append(out, iconRefsInWidget(c)...)
	}
	return out
}

func iconRefsInMenu(item ast.NavMenuItemDef) []iconRef {
	var out []iconRef
	if ref := normalizeIconRef(item.Icon); ref != "" {
		out = append(out, iconRef{value: ref, where: "menu item " + quoteName(item.Caption)})
	}
	for _, sub := range item.Items {
		out = append(out, iconRefsInMenu(sub)...)
	}
	return out
}

// iconPropValue pulls the icon property out of a widget property map. MDL
// property keys are case-insensitive, so both `Icon:` and `icon:` are accepted.
func iconPropValue(props map[string]any) string {
	if props == nil {
		return ""
	}
	for k, v := range props {
		if !strings.EqualFold(k, "icon") {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		return normalizeIconRef(s)
	}
	return ""
}

// normalizeIconRef strips the quoting MDL allows around an icon reference.
func normalizeIconRef(s string) string {
	return strings.Trim(strings.TrimSpace(s), "'\"")
}

func quoteName(s string) string {
	if s == "" {
		return "(unnamed)"
	}
	return "'" + s + "'"
}

// check resolves one reference, distinguishing an unknown collection from an
// unknown icon within a known one — the two need different fixes.
func (idx *iconIndex) check(ref iconRef) error {
	// Module.Collection.IconName — the icon name is the last segment, and the
	// collection is everything before it. Splitting from the right keeps working
	// if a module name ever contains a dot.
	dot := strings.LastIndex(ref.value, ".")
	if dot <= 0 || dot == len(ref.value)-1 {
		return mdlerrors.NewValidation(fmt.Sprintf(
			"%s: icon %q is not a qualified icon reference — write Module.Collection.IconName, "+
				"for example 'Atlas_Core.Atlas_Filled.pencil'.\n  Collections in this project: %s",
			ref.where, ref.value, strings.Join(idx.order, ", ")))
	}
	collection, icon := ref.value[:dot], ref.value[dot+1:]

	icons, known := idx.collections[collection]
	if !known {
		return mdlerrors.NewValidation(fmt.Sprintf(
			"%s: unknown icon collection %q in icon reference %q.\n  Collections in this project: %s",
			ref.where, collection, ref.value, strings.Join(idx.order, ", ")))
	}
	if icons[icon] {
		return nil
	}

	msg := fmt.Sprintf("%s: icon %q does not exist in collection %q (MxBuild reports this as CE1613)",
		ref.where, icon, collection)
	if near := nearestIcons(icons, icon); len(near) > 0 {
		msg += ".\n  Did you mean: " + strings.Join(near, ", ")
	}
	msg += fmt.Sprintf(".\n  List the collection's icons with: describe icon collection %s", collection)
	return mdlerrors.NewValidation(msg)
}

// nearestIcons suggests up to five icons whose names are close to the one
// written. Substring matching in both directions covers the common typos
// (a truncated name, an extra qualifier) without needing an edit-distance table.
func nearestIcons(icons map[string]bool, want string) []string {
	lower := strings.ToLower(want)
	var hits []string
	for name := range icons {
		l := strings.ToLower(name)
		if l == lower {
			continue
		}
		if strings.Contains(l, lower) || strings.Contains(lower, l) {
			hits = append(hits, name)
		}
	}
	sort.Strings(hits)
	if len(hits) > 5 {
		hits = hits[:5]
	}
	return hits
}
