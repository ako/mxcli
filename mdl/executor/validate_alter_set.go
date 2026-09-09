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

// `ALTER PAGE … SET <prop> ON <widget>` was the one widget-property position
// reference checking never looked at.
//
// ValidateWidgetProperties resolves the properties of widgets a statement
// CARRIES — the trees under CREATE PAGE, and under ALTER's INSERT and REPLACE —
// against the widget definitions. A `SET` carries no widget: it names one that
// is already stored, so its property has to be resolved against the DOCUMENT,
// which that pass never opens. Both of these passed `check -p --references` and
// were then refused by `exec`, on a real 11.13.0 project:
//
//	set NoSuchProperty = 10 on dgProducts;
//	  exec -> pluggable property "NoSuchProperty" not found
//	set PageSize = 12 on noSuchWidget;
//	  exec -> widget "noSuchWidget" not found
//
// Same inversion validate_alter_target.go describes for the document itself:
// check is meant to be the strict gate and exec the thing that runs, and here
// exec was stricter — so a script passed every pre-flight and then stopped
// halfway, having applied the statements before the typo and none after.
//
// What makes this checkable at all is that the answer is not re-derived. The
// vocabulary a `SET` accepts is partly a switch in the setter and partly the
// stored widget's own template keys, which belong to whatever widget package the
// project has installed; anything that restated it here would drift, and drift
// silently in the direction that hurts. Instead the setter is RUN, against a
// throwaway copy of the document (pagemutator.Probe), and only its error is
// kept. Check and exec therefore disagree only if the copy differs from the
// original, which is the one thing that is cheap to guarantee.

// pageProbe is the optional capability the shared BSON page mutator offers: a
// discardable copy to dry-run an operation against, and the property names the
// stored widget declares.
//
// It is an assertion rather than an addition to backend.PageMutator on purpose.
// The MCP mutator has neither — and no pluggable path at all — so asserting is
// also what keeps this pass off a backend whose SET support is different, rather
// than reporting that difference as the author's mistake.
type pageProbe interface {
	Probe() (backend.PageMutator, error)
	ResolvesTarget(widgetRef, columnRef string) bool
	WidgetPropertyKeys(widgetRef, columnRef string) []string
}

// validateAlterSetProperties dry-runs every ALTER … SET against the stored
// document and reports what exec would refuse.
func validateAlterSetProperties(ctx *ExecContext, prog *ast.Program, sc *scriptContext) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return nil
	}
	grows := documentsTheScriptAddsWidgetsTo(prog)
	opened := map[model.ID]pageProbe{}

	var errs []error
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.AlterPageStmt)
		if !ok || !hasSetPropertyOp(s) || alterTargetComesFromScript(sc, s) {
			continue
		}
		unitID, containerID, containerType, err := resolveAlterPageUnit(ctx, s, h)
		if err != nil {
			// A target that does not resolve is validateAlterTarget's finding,
			// not this one's. Reporting it twice, in two wordings, reads as two
			// problems.
			continue
		}
		probe, ok := opened[unitID]
		if !ok {
			probe = openPageProbe(ctx, unitID)
			opened[unitID] = probe
		}
		if probe == nil {
			continue
		}
		label := fmt.Sprintf("alter %s %s", containerType, s.PageName.String())
		modName := h.GetModuleName(containerID)
		for _, op := range s.Operations {
			set, ok := op.(*ast.SetPropertyOp)
			if !ok {
				continue
			}
			// A widget an INSERT or REPLACE in this script puts on the page is
			// not in the stored document, so a dry run would report it missing.
			// Only its EXISTENCE is affected, though — a target that does
			// resolve is checked as normal — and matching by name would not
			// work anyway: a DataGrid 2 column is addressed by a derived name,
			// so `column colTitle` is inserted under one name and set under
			// another. Its properties are checked where it is written, by
			// ValidateWidgetProperties.
			if grows[s.PageName.String()] && !probe.ResolvesTarget(set.Target.Widget, set.Target.Column) {
				continue
			}
			errs = append(errs, checkSetOp(ctx, probe, label, set, modName, containerID)...)
		}
	}
	return errs
}

// openPageProbe opens the stored document and returns it as a prober, or nil
// when it cannot be established — an engine without the capability, a unit that
// will not load. Silence, never a finding: a false "no such property" blocks a
// script that would have worked, which is worse than the gap it replaces.
func openPageProbe(ctx *ExecContext, unitID model.ID) pageProbe {
	mutator, err := ctx.Backend.OpenPageForMutation(unitID)
	if err != nil {
		return nil
	}
	probe, ok := mutator.(pageProbe)
	if !ok {
		return nil
	}
	return probe
}

// checkSetOp dry-runs one SET, one property at a time.
//
// Per property rather than per op, on its own copy each time, so a statement
// setting four properties reports all four mistakes instead of stopping at the
// first — the difference between one round of correction and four.
func checkSetOp(ctx *ExecContext, p pageProbe, label string, op *ast.SetPropertyOp, modName string, modID model.ID) []error {
	names := make([]string, 0, len(op.Properties))
	for name := range op.Properties {
		names = append(names, name)
	}
	sort.Strings(names)

	var errs []error
	for _, name := range names {
		probe, err := p.Probe()
		if err != nil {
			return errs
		}
		one := &ast.SetPropertyOp{
			Target:     op.Target,
			Properties: map[string]any{name: op.Properties[name]},
		}
		if err := applySetPropertyMutator(ctx, probe, one, modName, modID); err != nil {
			errs = append(errs, mdlerrors.NewValidation(fmt.Sprintf(
				"%s: %v%s", label, err, declaredPropertyHint(p, op.Target, name))))
		}
	}
	return errs
}

// declaredPropertyHint names what the widget does have, when the widget resolves
// and declares its own property keys. A near-miss is called out first: the
// spelling of a pluggable key is the thing authors get wrong (it is lowerCamel
// in the template, capitalised in DESCRIBE output), so `pagesize` against
// `pageSize` should read as a typo and not as a list to search.
func declaredPropertyHint(p pageProbe, target ast.WidgetRef, prop string) string {
	if target.Widget == "" {
		return "" // page-level SET: the setter's own error lists what it takes
	}
	keys := p.WidgetPropertyKeys(target.Widget, target.Column)
	if len(keys) == 0 {
		return ""
	}
	for _, k := range keys {
		if strings.EqualFold(k, prop) {
			return "" // it does have it — the failure is about the value
		}
	}
	hint := ""
	if near := nearestKey(prop, keys); near != "" {
		hint = fmt.Sprintf(" — did you mean `%s`?", near)
	}
	const max = 8
	if len(keys) > max {
		return fmt.Sprintf("%s (%s declares %s and %d more)",
			hint, target.Name(), strings.Join(keys[:max], ", "), len(keys)-max)
	}
	return fmt.Sprintf("%s (%s declares %s)", hint, target.Name(), strings.Join(keys, ", "))
}

// hasSetPropertyOp reports whether a statement carries anything this pass would
// look at, so a script of pure INSERTs never opens a document.
func hasSetPropertyOp(s *ast.AlterPageStmt) bool {
	for _, op := range s.Operations {
		if _, ok := op.(*ast.SetPropertyOp); ok {
			return true
		}
	}
	return false
}

// alterTargetComesFromScript reports whether the document this ALTER edits is
// one the script itself creates — in which case there is nothing stored to dry
// run against, and the widgets it names are checked where they are written.
func alterTargetComesFromScript(sc *scriptContext, s *ast.AlterPageStmt) bool {
	if sc == nil {
		return false
	}
	qn := s.PageName.String()
	if s.PageName.Module == "" || sc.modules[s.PageName.Module] {
		return true
	}
	switch strings.ToUpper(s.ContainerType) {
	case "SNIPPET":
		return sc.snippets[qn]
	case "LAYOUT":
		return sc.layouts[qn]
	default:
		return sc.pages[qn]
	}
}

// documentsTheScriptAddsWidgetsTo names the documents an INSERT or REPLACE
// somewhere in the script puts new widgets on — the ones whose stored widget
// tree is not the tree a later SET will run against.
//
// Deliberately not ordered: a SET before the INSERT that adds its target fails
// at exec too, so suppressing it here costs one unreported error, while getting
// the order wrong the other way costs a false one on a script that works. Those
// are not comparable — the second blocks the run.
func documentsTheScriptAddsWidgetsTo(prog *ast.Program) map[string]bool {
	grows := map[string]bool{}
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.AlterPageStmt)
		if !ok {
			continue
		}
		for _, op := range s.Operations {
			switch op.(type) {
			case *ast.InsertWidgetOp, *ast.ReplaceWidgetOp:
				grows[s.PageName.String()] = true
			}
		}
	}
	return grows
}
