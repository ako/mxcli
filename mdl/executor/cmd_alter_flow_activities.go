// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// flowTarget is one flow document a statement will act on.
type flowTarget struct {
	ID       model.ID
	Module   string
	Name     string
	Excluded bool
}

func (t flowTarget) qname() string { return t.Module + "." + t.Name }

// execAlterFlowActivities handles
//
//	ALTER MICROFLOW Mod.Flow  DISABLE ACTIVITIES WHERE …
//	ALTER MICROFLOWS [IN Mod] ENABLE  ACTIVITIES WHERE …
func execAlterFlowActivities(ctx *ExecContext, s *ast.AlterFlowActivitiesStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	// Toggling the flag needs the property to exist, exactly as authoring it
	// does — the same gate, reached from ALTER instead of CREATE.
	if err := checkFeature(ctx, "microflows", "disabled_activity", "disable/enable activities",
		"Microflows$ActionActivity has no `disabled` property before Mendix 9.12"); err != nil {
		return err
	}
	if problems := types.CheckActivityFilter(s.Filter, nil); len(problems) > 0 {
		return mdlerrors.NewValidationf("%s", strings.Join(problems, "\n  - "))
	}

	targets, err := resolveFlowTargets(ctx, s)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return flowScopeEmptyError(s)
	}

	var (
		changedFlows []string
		totalChanged int
		totalMatched int
		noField      []string
	)
	for _, t := range targets {
		res, err := ctx.Backend.SetActivitiesDisabled(t.ID, s.Filter, s.Disable)
		if err != nil {
			return mdlerrors.NewBackend("set activity state on "+t.qname(), err)
		}
		totalMatched += res.Matched
		totalChanged += res.Changed
		if res.Changed > 0 {
			changedFlows = append(changedFlows, fmt.Sprintf("%s (%d)", t.qname(), res.Changed))
		}
		if res.WithoutField > 0 {
			noField = append(noField, t.qname())
		}
	}
	sort.Strings(changedFlows)
	sort.Strings(noField)

	verb := "Disabled"
	if !s.Disable {
		verb = "Enabled"
	}

	// Three outcomes, reported apart. "Nothing matched" and "everything matched
	// was already in that state" both write nothing, and collapsing them into
	// one message is how a typo'd filter reads as a successful no-op — the same
	// failure ALTER PAGES' resolved WHERE layout exists to prevent.
	switch {
	case totalMatched == 0:
		fmt.Fprintf(ctx.Output, "No activity matched in %s — nothing to %s.\n",
			flowScopeDescription(s, len(targets)), strings.ToLower(strings.TrimSuffix(verb, "d")))
	case totalChanged == 0:
		fmt.Fprintf(ctx.Output, "Unchanged: all %d matching %s already %s.\n",
			totalMatched, pluralActivities(totalMatched), strings.ToLower(verb))
	default:
		fmt.Fprintf(ctx.Output, "%s %d %s in %s: %s\n",
			verb, totalChanged, pluralActivities(totalChanged),
			pluralFlows(len(changedFlows), string(s.Flavour)), strings.Join(changedFlows, ", "))
	}

	// A matching activity whose document has no Disabled key at all predates
	// Mendix 9.12. Named rather than patched: adding a property the type does
	// not have is what makes a project Studio Pro cannot open.
	if len(noField) > 0 {
		fmt.Fprintf(ctx.Output,
			"  note: %s %s no `Disabled` property on the matching activities "+
				"(written before Mendix 9.12) and %s left unchanged.\n",
			pluralFlows(len(noField), string(s.Flavour)), hasHave(len(noField)), wasWere(len(noField)))
	}
	return nil
}

// resolveFlowTargets turns the statement's scope into the units to act on.
//
// The singular form RESOLVES the name rather than filtering a listing by it: a
// misspelled flow would otherwise match nothing and report a clean success,
// which is the failure mode this whole statement's reporting is built around.
func resolveFlowTargets(ctx *ExecContext, s *ast.AlterFlowActivitiesStmt) ([]flowTarget, error) {
	all, err := listFlowsOfFlavour(ctx, s.Flavour)
	if err != nil {
		return nil, err
	}
	if !s.Bulk {
		want := s.Name.Module + "." + s.Name.Name
		for _, t := range all {
			if strings.EqualFold(t.qname(), want) {
				return []flowTarget{t}, nil
			}
		}
		return nil, mdlerrors.NewNotFound(string(s.Flavour), want)
	}
	if s.Module == "" {
		return all, nil
	}
	// Same reason: a module that does not exist must not read as "0 flows".
	if !moduleExists(ctx, s.Module) {
		return nil, mdlerrors.NewNotFound("module", s.Module)
	}
	var out []flowTarget
	for _, t := range all {
		if strings.EqualFold(t.Module, s.Module) {
			out = append(out, t)
		}
	}
	return out, nil
}

func listFlowsOfFlavour(ctx *ExecContext, flavour ast.FlowFlavour) ([]flowTarget, error) {
	var out []flowTarget
	switch flavour {
	case ast.FlowNanoflow:
		nfs, err := ctx.Backend.ListNanoflows()
		if err != nil {
			return nil, mdlerrors.NewBackend("list nanoflows", err)
		}
		for _, nf := range nfs {
			out = append(out, flowTarget{
				ID: nf.ID, Name: nf.Name, Excluded: nf.Excluded,
				Module: getModuleName(ctx, getModuleID(ctx, nf.ContainerID)),
			})
		}
	case ast.FlowRule:
		rules, err := ctx.Backend.ListRules()
		if err != nil {
			return nil, mdlerrors.NewBackend("list rules", err)
		}
		for _, r := range rules {
			out = append(out, flowTarget{
				ID: r.ID, Name: r.Name, Excluded: r.Excluded,
				Module: getModuleName(ctx, getModuleID(ctx, r.ContainerID)),
			})
		}
	default:
		mfs, err := ctx.Backend.ListMicroflows()
		if err != nil {
			return nil, mdlerrors.NewBackend("list microflows", err)
		}
		for _, mf := range mfs {
			out = append(out, flowTarget{
				ID: mf.ID, Name: mf.Name, Excluded: mf.Excluded,
				Module: getModuleName(ctx, getModuleID(ctx, mf.ContainerID)),
			})
		}
	}
	return out, nil
}

func moduleExists(ctx *ExecContext, name string) bool {
	mods, err := ctx.Backend.ListModules()
	if err != nil {
		return true // cannot tell; do not invent a failure
	}
	for _, m := range mods {
		if strings.EqualFold(m.Name, name) {
			return true
		}
	}
	return false
}

func flowScopeEmptyError(s *ast.AlterFlowActivitiesStmt) error {
	if s.Module != "" {
		return mdlerrors.NewValidationf("module %s holds no %ss", s.Module, s.Flavour)
	}
	return mdlerrors.NewValidationf("the project holds no %ss", s.Flavour)
}

func flowScopeDescription(s *ast.AlterFlowActivitiesStmt, n int) string {
	if !s.Bulk {
		return string(s.Flavour) + " " + s.Name.Module + "." + s.Name.Name
	}
	if s.Module != "" {
		return fmt.Sprintf("%s (%d %s)", s.Module, n, pluralWord(string(s.Flavour), n))
	}
	return fmt.Sprintf("%d %s", n, pluralWord(string(s.Flavour), n))
}

func pluralActivities(n int) string { return pluralWord("activity", n) }

func pluralFlows(n int, flavour string) string {
	return fmt.Sprintf("%d %s", n, pluralWord(flavour, n))
}

func pluralWord(word string, n int) string {
	if n == 1 {
		return word
	}
	if strings.HasSuffix(word, "y") {
		return strings.TrimSuffix(word, "y") + "ies"
	}
	return word + "s"
}

func hasHave(n int) string {
	if n == 1 {
		return "has"
	}
	return "have"
}

func wasWere(n int) string {
	if n == 1 {
		return "was"
	}
	return "were"
}

var _ = backend.FlowActivityChange{}
