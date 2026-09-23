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
)

// ALTER PAGES [IN <module>] SET '<design property>' = <value>, … WHERE WIDGETTYPE = <kw> [DRY RUN]
//
// The bulk form of ALTER PAGE's design-property SET (ako/mxcli#515). A house
// style is "every data grid is compact and striped", which should be one
// statement rather than one per page.
//
// It reuses three things deliberately, rather than growing a fourth of each:
// findMatchingWidgets (the catalog query UPDATE WIDGETS uses), the per-widget
// routing decision from ALTER PAGE SET, and updateOutcome's three-way reporting
// from ako/mxcli#520 — so a sweep that matches widgets and writes none of them
// says so instead of claiming success.

// execAlterPagesStyling applies one design-property sweep.
func execAlterPagesStyling(ctx *ExecContext, s *ast.AlterPagesStylingStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !s.DryRun && !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	if len(s.Assignments) == 0 {
		return mdlerrors.NewValidation("ALTER PAGES … SET needs at least one design property")
	}

	// The MDL keyword is resolved to the widget id it writes, so `datagrid`
	// selects Data grid 2 and not the data grid's FILTER widgets — which is what
	// a LIKE over the stored id does instead (measured: 20 widgets in 6
	// containers on a blank project, across five different widget types).
	// The catalog answers "every widget of this type, across every page", and
	// this statement builds it rather than requiring the reader to have run
	// `refresh catalog full` first — the same thing UPDATE WIDGETS does, and for
	// the same reason: a sweep that silently matched nothing because the catalog
	// was cold would read as "no such widgets".
	if err := ensureCatalog(ctx, true); err != nil {
		return mdlerrors.NewBackend("build catalog", err)
	}

	widgetType := resolveWidgetTypeSelector(s.WidgetType)
	widgets, err := findMatchingWidgets(ctx, []ast.WidgetFilter{
		{Field: "WidgetType", Operator: "=", Value: widgetType},
	}, s.Module)
	if err != nil {
		return mdlerrors.NewBackend("find widgets", err)
	}
	if len(widgets) == 0 {
		fmt.Fprintf(ctx.Output, "No %s widgets found%s\n", s.WidgetType, inModuleSuffix(s.Module))
		return nil
	}

	containers := groupWidgetsByContainer(widgets)
	fmt.Fprintf(ctx.Output, "\nFound %d %s widget(s) in %d container(s)\n",
		len(widgets), s.WidgetType, len(containers))
	if s.DryRun {
		fmt.Fprintln(ctx.Output, "\n[dry run] The following changes would be made:")
	}

	var total updateOutcome
	for _, containerID := range sortedContainerIDs(containers) {
		outcome, err := styleWidgetsInContainer(ctx, containerID, containers[containerID], s)
		if err != nil {
			fmt.Fprintf(ctx.Output, "Warning: Failed to style widgets in %s: %v\n", containerID, err)
			continue
		}
		total.add(outcome)
	}

	verb := "Styled"
	if s.DryRun {
		verb = "[dry run] Would style"
	}
	fmt.Fprintf(ctx.Output, "\n%s %d widget(s)\n", verb, total.WidgetsChanged)
	if total.WidgetsUnchanged > 0 {
		fmt.Fprintf(ctx.Output, "%d widget(s) matched but had no design property that could be set\n",
			total.WidgetsUnchanged)
	}
	if total.WidgetsMissing > 0 {
		fmt.Fprintf(ctx.Output, "%d widget(s) are in the catalog but not in the document — "+
			"run 'refresh catalog full force' and try again\n", total.WidgetsMissing)
	}
	if s.DryRun {
		fmt.Fprintln(ctx.Output, "\nRun without dry run to apply changes.")
		return nil
	}
	if total.WidgetsChanged > 0 {
		fmt.Fprintln(ctx.Output, "\nNote: Run 'refresh catalog full force' to update the catalog with changes.")
	}
	if total.changedNothing() {
		return mdlerrors.NewValidation(fmt.Sprintf(
			"no widget was styled: %d assignment(s) could not be applied. "+
				"Run `mxcli show design properties for %s` to see what its theme declares",
			len(total.Failures), s.WidgetType))
	}
	return nil
}

// styleWidgetsInContainer applies the sweep to one page or snippet.
func styleWidgetsInContainer(ctx *ExecContext, containerID string, refs []widgetRef, s *ast.AlterPagesStylingStmt) (updateOutcome, error) {
	var out updateOutcome
	if len(refs) == 0 {
		return out, nil
	}
	containerName := refs[0].ContainerName

	mutator, err := ctx.Backend.OpenPageForMutation(model.ID(containerID))
	if err != nil {
		return out, mdlerrors.NewBackend(fmt.Sprintf("open %s for mutation", containerName), err)
	}
	if mutator == nil {
		return out, mdlerrors.NewBackend(fmt.Sprintf("open %s for mutation", containerName),
			fmt.Errorf("backend returned nil mutator for %s", containerID))
	}

	// A dry run works against a discardable copy, so the preview reports what
	// would actually happen — the same seam `mxcli check` uses for ALTER PAGE
	// SET, and the fix ako/mxcli#520 applied to UPDATE WIDGETS' preview.
	target := mutator
	if s.DryRun {
		if p, ok := mutator.(interface {
			Probe() (backend.PageMutator, error)
		}); ok {
			if probe, perr := p.Probe(); perr == nil && probe != nil {
				target = probe
			}
		}
	}

	theme := ctx.GetThemeRegistry()
	for _, ref := range refs {
		landed := 0
		for _, a := range s.Assignments {
			p := designPropertyForStoredWidget(theme, target, ref.Name, a.Property)
			if p == nil {
				out.Failures = append(out.Failures, fmt.Sprintf("'%s' on %s in %s",
					a.Property, ref.Name, containerName))
				fmt.Fprintf(ctx.Output, "  Warning: %s '%s' on %s: not a design property this "+
					"widget's theme declares\n", cannotVerb(s.DryRun), a.Property, ref.Name)
				continue
			}
			if err := applyDesignPropertySet(target, ast.WidgetRef{Widget: ref.Name}, p, stylingAssignmentValue(a)); err != nil {
				out.Failures = append(out.Failures, fmt.Sprintf("'%s' on %s in %s: %v",
					a.Property, ref.Name, containerName, err))
				fmt.Fprintf(ctx.Output, "  Warning: %s '%s' on %s: %v\n",
					cannotVerb(s.DryRun), a.Property, ref.Name, err)
				continue
			}
			landed++
			if s.DryRun {
				fmt.Fprintf(ctx.Output, "  Would set '%s' on %s in %s\n",
					a.Property, ref.Name, containerName)
			}
		}
		if landed > 0 {
			out.WidgetsChanged++
		} else {
			out.WidgetsUnchanged++
		}
	}

	if !s.DryRun && out.WidgetsChanged > 0 {
		if err := mutator.Save(); err != nil {
			return out, mdlerrors.NewBackend(fmt.Sprintf("save %s", containerName), err)
		}
	}
	return out, nil
}

// stylingAssignmentValue converts one assignment back to the scalar the shared
// design-property conversion takes, so both the singular and bulk forms hand it
// the same shapes.
func stylingAssignmentValue(a ast.StylingAssignment) any {
	if a.IsToggle {
		return a.ToggleOn
	}
	return a.Value
}

func cannotVerb(dryRun bool) string {
	if dryRun {
		return "Cannot set"
	}
	return "Failed to set"
}

// resolveWidgetTypeSelector turns the MDL keyword in WHERE WIDGETTYPE into the
// widget id the catalog stores, leaving a full id untouched.
//
// This is what makes `datagrid` mean Data grid 2 and nothing else. The
// alternative a user reaches for — `WidgetType LIKE '%datagrid%'` — also matches
// DatagridTextFilter, DatagridDateFilter and DatagridDropdownFilter, which are
// different widgets that do not carry its design properties.
func resolveWidgetTypeSelector(selector string) string {
	if strings.Contains(selector, ".") {
		return selector // already a full widget id
	}
	if id, ok := pluggableKeywordIDs()[strings.ToLower(selector)]; ok {
		return id
	}
	return selector
}

func inModuleSuffix(module string) string {
	if module == "" {
		return ""
	}
	return " in " + module
}

// sortedContainerIDs keeps the output order stable — a map walk would reorder
// the report between identical runs, which makes a diff of two runs unreadable.
func sortedContainerIDs(containers map[string][]widgetRef) []string {
	ids := make([]string, 0, len(containers))
	for id := range containers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
