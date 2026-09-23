// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// Workflow groups are the entries under App Settings ▸ Workflows ▸ Groups: named
// buckets a user task's group targeting selects from. They live on the
// Settings$WorkflowsProjectSettingsPart as a Groups part-list, which is why they
// are ALTER SETTINGS WORKFLOWS rather than a document of their own.
//
// A Settings$WorkflowGroup stores exactly Name and Description and declares no
// identifier property — agreed by modelsdk/gen, generated/metamodel and the
// Mendix Model SDK's own gen (mendixmodelsdk 4.115.0), and confirmed by a blank
// 11.13.0 project, whose empty list is stored as `Groups: [2]`. So the NAME is
// the group's identity, the same way a language's code is, and that is what
// these statements address a group by.
//
// Nothing in the metamodel points at a group: a user task targets groups through
// a microflow or an XPath returning System.WorkflowGroup objects, never by a
// reference to the settings entry. There is therefore no dangling reference to
// check on REMOVE — the coupling is at runtime, where Mendix materialises one
// System.WorkflowGroup per entry, keyed by name.

// workflowGroupOptionKeys names every option the ADD/MODIFY forms accept, for the
// error that lists what would have worked.
const workflowGroupOptionKeys = "Description"

// alterSettingsWorkflowGroup dispatches the ADD / ADD OR MODIFY / MODIFY /
// REMOVE GROUP forms of ALTER SETTINGS WORKFLOWS.
func alterSettingsWorkflowGroup(ctx *ExecContext, ps *model.ProjectSettings, stmt *ast.AlterSettingsStmt) error {
	if ps.Workflows == nil {
		return mdlerrors.NewNotFound("settings section", "workflows")
	}
	if err := checkFeature(ctx, "workflows", "groups",
		"alter settings workflows add group '<name>'",
		"Workflow groups (Settings$WorkflowGroup) were introduced in the 11.2 metamodel. "+
			"On an earlier version, model the buckets as your own entity and target user tasks with a microflow."); err != nil {
		return err
	}
	if strings.TrimSpace(stmt.GroupName) == "" {
		return mdlerrors.NewValidation(
			"a workflow group needs a name — write e.g. `alter settings workflows add group 'Approvers';`")
	}
	switch {
	case stmt.UpsertGroup:
		return alterSettingsWorkflowGroupUpsert(ctx, ps, stmt)
	case stmt.AddGroup:
		return alterSettingsWorkflowGroupAdd(ctx, ps, stmt)
	case stmt.ModifyGroup:
		return alterSettingsWorkflowGroupModify(ctx, ps, stmt)
	}
	return alterSettingsWorkflowGroupRemove(ctx, ps, stmt)
}

// alterSettingsWorkflowGroupAdd appends a group. Studio Pro appends rather than
// sorting, so the stored order is the order groups were added; the overlay
// rebuilds the list from this slice in the same order.
func alterSettingsWorkflowGroupAdd(ctx *ExecContext, ps *model.ProjectSettings, stmt *ast.AlterSettingsStmt) error {
	ws := ps.Workflows
	if i := indexOfWorkflowGroup(ws.Groups, stmt.GroupName); i >= 0 {
		return mdlerrors.NewValidationf(
			"workflow group %q already exists — change it with "+
				"`alter settings workflows modify group '%s' (Description: '…')`, or use `add or modify` to do either",
			ws.Groups[i].Name, ws.Groups[i].Name)
	}
	g := model.WorkflowGroup{Name: stmt.GroupName}
	if err := applyWorkflowGroupOptions(&g, stmt.Properties); err != nil {
		return err
	}
	ws.Groups = append(ws.Groups, g)
	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update workflow settings", err)
	}
	fmt.Fprintf(ctx.Output, "Added workflow group: %s (%d group(s))\n", g.Name, len(ws.Groups))
	return nil
}

// alterSettingsWorkflowGroupUpsert is ADD OR MODIFY: add the group when it is not
// there, change the named options when it is. It exists for the same reason the
// language form does — DESCRIBE has to emit something that re-executes against a
// project that already has some of its groups.
func alterSettingsWorkflowGroupUpsert(ctx *ExecContext, ps *model.ProjectSettings, stmt *ast.AlterSettingsStmt) error {
	if i := indexOfWorkflowGroup(ps.Workflows.Groups, stmt.GroupName); i >= 0 {
		if len(stmt.Properties) == 0 {
			// Nothing to change and nothing to add: report the state rather than
			// an error, so a replay of a described project is quiet.
			fmt.Fprintf(ctx.Output, "Unchanged workflow group: %s\n", ps.Workflows.Groups[i].Name)
			return nil
		}
		return alterSettingsWorkflowGroupModify(ctx, ps, stmt)
	}
	return alterSettingsWorkflowGroupAdd(ctx, ps, stmt)
}

// alterSettingsWorkflowGroupModify changes an existing group's description.
//
// Only the options the statement NAMES are touched, which is what distinguishes
// MODIFY from re-adding: a statement that mentions no option cannot silently
// blank a description somebody wrote in Studio Pro. The group's stored document
// is preserved either way, so its $ID survives.
func alterSettingsWorkflowGroupModify(ctx *ExecContext, ps *model.ProjectSettings, stmt *ast.AlterSettingsStmt) error {
	ws := ps.Workflows
	idx := indexOfWorkflowGroup(ws.Groups, stmt.GroupName)
	if idx < 0 {
		return mdlerrors.NewValidationf(
			"workflow group %q does not exist in this project (%s) — add it first with "+
				"`alter settings workflows add group '%s';`",
			stmt.GroupName, workflowGroupsSummary(ws.Groups), stmt.GroupName)
	}
	if len(stmt.Properties) == 0 {
		return mdlerrors.NewValidationf(
			"no properties given — write e.g. `alter settings workflows modify group '%s' (Description: '…');`",
			stmt.GroupName)
	}

	g := ws.Groups[idx]
	if err := applyWorkflowGroupOptions(&g, stmt.Properties); err != nil {
		return err
	}
	changed := sortedSettingsOptionKeys(stmt.Properties)
	unchanged := g == ws.Groups[idx]
	ws.Groups[idx] = g

	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update workflow settings", err)
	}
	// Say which of the two happened: a replayed DESCRIBE names every option of
	// every group, so reporting "Modified" for all of them would describe a
	// rewrite that ADR-0008 elides.
	if unchanged {
		fmt.Fprintf(ctx.Output, "Unchanged workflow group: %s\n", g.Name)
	} else {
		fmt.Fprintf(ctx.Output, "Modified workflow group: %s (%s)\n", g.Name, strings.Join(changed, ", "))
	}
	return nil
}

// alterSettingsWorkflowGroupRemove drops a group from the settings list.
//
// This is not refused when a workflow is in flight, because the model cannot
// tell: nothing in it references a settings group. What removal does mean is
// said instead — the runtime materialises System.WorkflowGroup rows from this
// list, so a removed group's row stops being maintained while the user tasks
// already assigned to it keep their association.
func alterSettingsWorkflowGroupRemove(ctx *ExecContext, ps *model.ProjectSettings, stmt *ast.AlterSettingsStmt) error {
	ws := ps.Workflows
	idx := indexOfWorkflowGroup(ws.Groups, stmt.GroupName)
	if idx < 0 {
		return mdlerrors.NewValidationf(
			"workflow group %q does not exist in this project (%s)",
			stmt.GroupName, workflowGroupsSummary(ws.Groups))
	}
	name := ws.Groups[idx].Name
	ws.Groups = append(ws.Groups[:idx], ws.Groups[idx+1:]...)
	if err := ctx.Backend.UpdateProjectSettings(ps); err != nil {
		return mdlerrors.NewBackend("update workflow settings", err)
	}
	fmt.Fprintf(ctx.Output, "Removed workflow group: %s (%d group(s))\n", name, len(ws.Groups))
	return nil
}

// applyWorkflowGroupOptions writes the named ( key: value ) options onto a group.
// An unknown key is refused with the list of what would have worked, rather than
// stored under a property Settings$WorkflowGroup does not declare.
func applyWorkflowGroupOptions(g *model.WorkflowGroup, props map[string]any) error {
	for _, key := range sortedSettingsOptionKeys(props) {
		valStr := settingsValueToString(props[key])
		switch key {
		case "Description":
			g.Description = valStr
		default:
			return mdlerrors.NewUnsupported(fmt.Sprintf(
				"unknown workflow group option: %s\n  valid keys: %s", key, workflowGroupOptionKeys))
		}
	}
	return nil
}

// indexOfWorkflowGroup finds a group by name, case-insensitively — Studio Pro
// will not let two groups differ only in case, so treating 'Approvers' and
// 'approvers' as the same group is what keeps ADD from creating a second one a
// script can no longer address unambiguously.
func indexOfWorkflowGroup(groups []model.WorkflowGroup, name string) int {
	for i, g := range groups {
		if strings.EqualFold(g.Name, name) {
			return i
		}
	}
	return -1
}

// workflowGroupsSummary renders the existing group names for an error message.
func workflowGroupsSummary(groups []model.WorkflowGroup) string {
	if len(groups) == 0 {
		return "this project has no workflow groups"
	}
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.Name)
	}
	sort.Strings(names)
	return "existing: " + strings.Join(names, ", ")
}

// sortedSettingsOptionKeys returns a settings-option map's keys in a stable order: iterating
// the map directly would make the reported option list — and any error naming the
// first bad key — differ between runs.
func sortedSettingsOptionKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// listWorkflowGroups backs SHOW WORKFLOW GROUPS.
func listWorkflowGroups(ctx *ExecContext) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	ps, err := ctx.Backend.GetProjectSettings()
	if err != nil {
		return mdlerrors.NewBackend("read project settings", err)
	}
	var groups []model.WorkflowGroup
	if ps.Workflows != nil {
		groups = ps.Workflows.Groups
	}
	tr := &TableResult{
		Columns: []string{"Name", "Description"},
		Summary: fmt.Sprintf("(%d workflow group(s))", len(groups)),
	}
	for _, g := range groups {
		tr.Rows = append(tr.Rows, []any{g.Name, g.Description})
	}
	return writeResult(ctx, tr)
}
