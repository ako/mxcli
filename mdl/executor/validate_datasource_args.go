// SPDX-License-Identifier: Apache-2.0

// CE1571: "No argument has been selected for parameter 'X' and no default is
// available. Please select an argument manually."
//
// A microflow used as a widget's data source must be given an argument for every
// parameter it declares. mxcli accepted a bare `datasource: microflow M.F` and
// wrote a data source with no parameter mappings, so the page built to CE1571 at
// the far end of an `mx check` — while `mxcli check --references` passed, because
// the microflow itself resolves fine (ako/mxcli-maintenance-2).
//
// This is a pure model-consistency question: the microflow's signature and the
// arguments are both in hand, so it needs no runtime and no build.
//
// Measured on Mendix 11.13 — and the second case is the one that decides the
// severity, because it is the plausible reason to make this a warning:
//
//	page with NO parameters, datasource without arguments   → CE1571
//	page WITH a $Task parameter of the parameter's exact
//	  entity type, datasource without arguments             → CE1571
//
// So Studio Pro does NOT auto-map a matching object in scope, and a missing
// argument is always an error rather than something that might be filled in.
// That is what makes this an error rather than a warning; "no default is
// available" in Mendix's own message reads as though a default sometimes is, and
// for a page data source it is not.
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// validateDataSourceArguments reports microflow/nanoflow data sources whose
// argument list does not match the flow's parameters.
//
// Two distinct faults, both provable:
//
//   - a parameter with no argument — CE1571, the reported case
//   - an argument naming no parameter — a typo that silently binds nothing
//
// A flow that cannot be resolved yields nothing: "not found" is already
// validateWidgetReferences' job, and reporting it twice would be noise. A flow
// created earlier in the SAME script is likewise skipped rather than guessed at,
// since its signature is not in the project yet.
func validateDataSourceArguments(ctx *ExecContext, widgets []*ast.WidgetV3, sc *scriptContext) []string {
	if !ctx.Connected() || len(widgets) == 0 {
		return nil
	}
	params := buildFlowParameterNames(ctx)
	// A flow the SAME script creates is not in the project yet, and that is the
	// common shape — one script writing a data source microflow and the page that
	// binds it. Script definitions win on a name clash: they are what this run
	// will leave behind.
	if sc != nil {
		for name, ps := range sc.flowParams {
			params[name] = ps
		}
	}
	if len(params) == 0 {
		return nil
	}

	var errs []string
	var walk func(ws []*ast.WidgetV3)
	walk = func(ws []*ast.WidgetV3) {
		for _, w := range ws {
			if w == nil {
				continue
			}
			if ds := w.GetDataSource(); ds != nil {
				errs = append(errs, dataSourceArgErrors(w.Name, ds, params)...)
			}
			walk(w.Children)
		}
	}
	walk(widgets)
	return errs
}

// dataSourceArgErrors compares one data source against the flow's signature.
func dataSourceArgErrors(widgetName string, ds *ast.DataSourceV3, params map[string][]string) []string {
	if ds.Type != "microflow" && ds.Type != "nanoflow" {
		return nil
	}
	want, known := params[strings.ToLower(ds.Reference)]
	if !known {
		return nil // resolution is validateWidgetReferences' job
	}

	given := make(map[string]bool, len(ds.Args))
	for _, a := range ds.Args {
		given[strings.ToLower(a.Name)] = true
	}
	wanted := make(map[string]bool, len(want))
	for _, p := range want {
		wanted[strings.ToLower(p)] = true
	}

	var missing []string
	for _, p := range want {
		if !given[strings.ToLower(p)] {
			missing = append(missing, p)
		}
	}
	var unknown []string
	for _, a := range ds.Args {
		if !wanted[strings.ToLower(a.Name)] {
			unknown = append(unknown, a.Name)
		}
	}
	sort.Strings(missing)
	sort.Strings(unknown)

	var out []string
	if len(missing) > 0 {
		out = append(out, fmt.Sprintf(
			"widget '%s': %s data source %s has no argument for %s %s — Mendix rejects this with "+
				"CE1571 and does not fill it in, even when an object of the right type is in scope. "+
				"Write it as `%s(%s: $Value)`",
			widgetName, ds.Type, ds.Reference,
			plural(len(missing), "parameter", "parameters"),
			quoteJoin(missing), ds.Reference, missing[0]))
	}
	if len(unknown) > 0 {
		out = append(out, fmt.Sprintf(
			"widget '%s': %s data source %s has no %s %s (it declares %s)",
			widgetName, ds.Type, ds.Reference,
			plural(len(unknown), "parameter", "parameters"),
			quoteJoin(unknown), quoteJoin(want)))
	}
	return out
}

func quoteJoin(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = "'" + n + "'"
	}
	return strings.Join(q, ", ")
}

// buildFlowParameterNames maps each microflow and nanoflow qualified name
// (lower-cased, because Mendix name resolution is case-insensitive) to its
// parameter names in declaration order.
//
// A flow with no parameters is present with an empty list, which is the point:
// "known, and takes nothing" has to be distinguishable from "not found", or a
// typo in the flow name would be reported as a missing argument.
func buildFlowParameterNames(ctx *ExecContext) map[string][]string {
	result := make(map[string][]string)
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return result
	}
	if mfs, err := ctx.Backend.ListMicroflows(); err == nil {
		for _, mf := range mfs {
			if mf == nil {
				continue
			}
			names := make([]string, 0, len(mf.Parameters))
			for _, p := range mf.Parameters {
				if p != nil {
					names = append(names, p.Name)
				}
			}
			result[strings.ToLower(h.GetQualifiedName(mf.ContainerID, mf.Name))] = names
		}
	}
	if nfs, err := ctx.Backend.ListNanoflows(); err == nil {
		for _, nf := range nfs {
			if nf == nil {
				continue
			}
			names := make([]string, 0, len(nf.Parameters))
			for _, p := range nf.Parameters {
				if p != nil {
					names = append(names, p.Name)
				}
			}
			result[strings.ToLower(h.GetQualifiedName(nf.ContainerID, nf.Name))] = names
		}
	}
	return result
}
