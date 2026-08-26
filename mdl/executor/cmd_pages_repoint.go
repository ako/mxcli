// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// findLayoutByQName resolves Module.Name to the stored layout.
func findLayoutByQName(ctx *ExecContext, qn ast.QualifiedName) (*pages.Layout, error) {
	layouts, err := ctx.Backend.ListLayouts()
	if err != nil {
		return nil, mdlerrors.NewBackend("list layouts", err)
	}
	for _, l := range layouts {
		if l.Name != qn.Name {
			continue
		}
		if qn.Module == "" || getModuleName(ctx, getModuleID(ctx, l.ContainerID)) == qn.Module {
			return l, nil
		}
	}
	return nil, mdlerrors.NewNotFound("layout", qn.String())
}

// checkRepointPlaceholders refuses a repoint that would leave a page bound to a
// placeholder the new layout does not declare.
//
// Without it the rewrite happily produces `NewLayout.HeaderLeft` for a layout
// with no HeaderLeft. mxbuild does catch that — CE1613, "The selected
// placeholder … no longer exists" — but at the far end of a build, naming the
// page rather than the statement that broke it, and only if someone builds. It
// matters more for the bulk form, where one page out of forty using an extra
// placeholder is exactly what a caller cannot check by eye.
//
// The check runs after MAP is applied, because MAP exists precisely to rename a
// binding onto a placeholder the new layout does have.
func checkRepointPlaceholders(ctx *ExecContext, mutator backend.PageMutator, op *ast.SetLayoutOp, newLayoutQN string) error {
	layout, err := findLayoutByQName(ctx, op.NewLayout)
	if err != nil {
		return err
	}
	declared, err := ctx.Backend.LayoutPlaceholders(layout.ID)
	if err != nil {
		return mdlerrors.NewBackend("read placeholders of "+newLayoutQN, err)
	}
	return checkRepointPlaceholdersAgainst(mutator, op, newLayoutQN, declared)
}

// checkRepointPlaceholdersAgainst is the check itself, split out so the bulk
// form reads the target layout's placeholders once rather than once per page.
func checkRepointPlaceholdersAgainst(mutator backend.PageMutator, op *ast.SetLayoutOp, newLayoutQN string, declared []string) error {
	// A layout that reports no placeholders is one the backend could not read
	// (MCP does not expose them), not one that has none — CREATE LAYOUT refuses
	// a placeholder-less layout. Refusing every repoint on that basis would be
	// wrong, so an unreadable target skips the check.
	if len(declared) == 0 {
		return nil
	}
	have := make(map[string]bool, len(declared))
	for _, d := range declared {
		have[d] = true
	}

	var missing, from []string
	for _, bound := range mutator.BoundPlaceholders() {
		target := bound
		if mapped, ok := op.Mappings[bound]; ok {
			target = mapped
		}
		if have[target] {
			continue
		}
		from = append(from, bound)
		if target == bound {
			missing = append(missing, bound)
		} else {
			missing = append(missing, fmt.Sprintf("%s (mapped to %s)", bound, target))
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(from)
	return mdlerrors.NewValidation(fmt.Sprintf(
		"layout %s does not declare the placeholder %s that this page binds to; it has %s. "+
			"Add the placeholder to the layout, or remap the binding: "+
			"`set Layout = %s map (%s as %s)`",
		newLayoutQN, strings.Join(missing, ", "), strings.Join(declared, ", "),
		newLayoutQN, from[0], declared[0]))
}

// execAlterPagesLayout handles the bulk repoint:
//
//	ALTER PAGES [IN <module>] SET LAYOUT = Module.Layout
//	  [MAP (Old AS New, …)] [WHERE LAYOUT = Module.Old]
//
// An app has one layout and many pages, so moving off Atlas_Default is this
// statement rather than forty of the single-page form.
func execAlterPagesLayout(ctx *ExecContext, s *ast.AlterPagesLayoutStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	newLayout, err := findLayoutByQName(ctx, s.NewLayout)
	if err != nil {
		return err
	}
	newLayoutQN := s.NewLayout.Module + "." + s.NewLayout.Name
	declared, err := ctx.Backend.LayoutPlaceholders(newLayout.ID)
	if err != nil {
		return mdlerrors.NewBackend("read placeholders of "+newLayoutQN, err)
	}

	// Resolving the WHERE layout is a lookup, not a string compare: naming a
	// layout that does not exist would otherwise match nothing and report
	// "0 pages" — success, for a typo.
	whereQN := ""
	if s.WhereLayout != nil {
		if _, err := findLayoutByQName(ctx, *s.WhereLayout); err != nil {
			return err
		}
		whereQN = s.WhereLayout.Module + "." + s.WhereLayout.Name
	}

	allPages, err := ctx.Backend.ListPages()
	if err != nil {
		return mdlerrors.NewBackend("list pages", err)
	}

	// Resolved from the listing rather than by a per-page lookup by name: a
	// name lookup is a second backend call per page, and it silently reports a
	// module as project-owned whenever the lookup fails — which is a guard that
	// stops firing rather than one that errors.
	modules, err := ctx.Backend.ListModules()
	if err != nil {
		return mdlerrors.NewBackend("list modules", err)
	}
	byID := make(map[model.ID]*model.Module, len(modules))
	for _, m := range modules {
		byID[m.ID] = m
	}

	var repointed, skippedMarketplace []string
	for _, p := range allPages {
		modID := getModuleID(ctx, p.ContainerID)
		mod := byID[modID]
		modName := getModuleName(ctx, modID)
		if s.Module != "" && modName != s.Module {
			continue
		}
		if whereQN != "" && pageLayoutQN(ctx, p) != whereQN {
			continue
		}
		// A page in a Marketplace module is not ours to rewrite, for the same
		// reason CREATE LAYOUT refuses to write one there. Skipped rather than
		// refused: a project-wide repoint that stopped dead on Administration's
		// pages would be unusable, and naming them is enough.
		if isMarketplaceModule(ctx, mod) {
			skippedMarketplace = append(skippedMarketplace, modName+"."+p.Name)
			continue
		}

		mutator, err := ctx.Backend.OpenPageForMutation(p.ID)
		if err != nil {
			return mdlerrors.NewBackend("open page "+modName+"."+p.Name+" for mutation", err)
		}
		op := &ast.SetLayoutOp{NewLayout: s.NewLayout, Mappings: s.Mappings}
		if err := checkRepointPlaceholdersAgainst(mutator, op, newLayoutQN, declared); err != nil {
			return mdlerrors.NewValidation(fmt.Sprintf("page %s.%s: %s", modName, p.Name, err.Error()))
		}
		if err := mutator.SetLayout(newLayoutQN, s.Mappings); err != nil {
			return mdlerrors.NewBackend("set layout on "+modName+"."+p.Name, err)
		}
		if err := mutator.Save(); err != nil {
			return mdlerrors.NewBackend("save "+modName+"."+p.Name, err)
		}
		repointed = append(repointed, modName+"."+p.Name)
	}

	sort.Strings(repointed)
	sort.Strings(skippedMarketplace)
	for _, name := range repointed {
		fmt.Fprintf(ctx.Output, "Repointed page %s to %s\n", name, newLayoutQN)
	}
	for _, name := range skippedMarketplace {
		fmt.Fprintf(ctx.Output, "Skipped page %s (marketplace module — not ours to edit)\n", name)
	}
	fmt.Fprintf(ctx.Output, "Repointed %d page(s) to %s\n", len(repointed), newLayoutQN)
	return nil
}

// pageLayoutQN returns the qualified name of the layout a page currently uses.
// An unreadable page reports "", which no WHERE clause matches — so a page the
// backend cannot read is left alone rather than repointed on a guess.
func pageLayoutQN(ctx *ExecContext, p *pages.Page) string {
	name, err := ctx.Backend.PageLayoutName(p.ID)
	if err != nil {
		return ""
	}
	return name
}
