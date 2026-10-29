// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ako/mxcli#1771: what an older user task cannot store is refused by
// `check -p` and by exec's pre-flight with MDL-VERSION01, before anything is
// written. The write-time refusal in canon.LowerToProjectShape stays as the
// last line; these name the statement and say what to do instead.
//   - Group targeting is 11.2. Before it, a user task targets users only.
//   - A multi-user task is 9.22.
//   - Its rules majority and threshold are 10.0, microflow is 10.3, and
//     `await all users` is 10.2 (mendixmodelsdk 4.117.0).
func TestCheckVersionGates_UserTaskFeatures(t *testing.T) {
	task := func(f func(*ast.WorkflowUserTaskNode)) *ast.WorkflowUserTaskNode {
		n := &ast.WorkflowUserTaskNode{Name: "Review", Caption: "Review"}
		f(n)
		return n
	}
	groups := task(func(n *ast.WorkflowUserTaskNode) {
		n.Targeting = ast.WorkflowTargetingNode{Kind: "group_xpath", XPath: "[Name = 'Reviewers']"}
	})
	groupsMicroflow := task(func(n *ast.WorkflowUserTaskNode) {
		n.Targeting = ast.WorkflowTargetingNode{Kind: "group_microflow", Microflow: ast.QualifiedName{Module: "M", Name: "Groups"}}
	})
	multi := task(func(n *ast.WorkflowUserTaskNode) { n.IsMultiUser = true })
	rule := func(r string) *ast.WorkflowUserTaskNode {
		return task(func(n *ast.WorkflowUserTaskNode) {
			n.IsMultiUser = true
			n.Completion = &ast.WorkflowCompletionRuleNode{Rule: r}
		})
	}
	await := task(func(n *ast.WorkflowUserTaskNode) { n.IsMultiUser, n.AwaitAllUsers = true, true })

	for _, c := range []struct {
		name  string
		act   ast.WorkflowActivityNode
		below [3]int
		want  string
	}{
		{"groups xpath", groups, [3]int{11, 1, 0}, "targeting groups"},
		{"groups microflow", groupsMicroflow, [3]int{10, 24, 24}, "targeting groups"},
		{"multi user task", multi, [3]int{9, 21, 0}, "multi user task"},
		{"majority", rule("majority"), [3]int{9, 24, 30}, "decide by majority"},
		{"threshold", rule("threshold"), [3]int{9, 24, 30}, "decide by threshold"},
		{"microflow rule", rule("microflow"), [3]int{10, 2, 0}, "decide by microflow"},
		{"await all users", await, [3]int{10, 1, 0}, "await all users"},
	} {
		for _, stmt := range []ast.Statement{
			&ast.CreateWorkflowStmt{Activities: []ast.WorkflowActivityNode{c.act}},
			&ast.AlterWorkflowStmt{Operations: []ast.AlterWorkflowOp{
				&ast.InsertAfterOp{NewActivities: []ast.WorkflowActivityNode{c.act}}}},
		} {
			prog := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"}, stmt}}
			vs := versionGateViolations(versionCtx(t, c.below[0], c.below[1], c.below[2]), prog)
			if len(vs) != 1 || vs[0].RuleID != "MDL-VERSION01" || !strings.Contains(vs[0].Message, c.want) || vs[0].Suggestion == "" {
				t.Errorf("%s (%T) below its floor: want one MDL-VERSION01 naming %q with a hint, got %+v", c.name, stmt, c.want, vs)
			}
			// Control: a version with every feature passes.
			if vs := versionGateViolations(versionCtx(t, 11, 99, 0), prog); len(vs) != 0 {
				t.Errorf("%s (%T): reported at 11.99: %+v", c.name, stmt, vs)
			}
		}
	}
}

// The floors are each feature's own: a user task targeting users, and a
// multi-user task deciding by consensus or veto, are written from 9.22.
func TestCheckVersionGates_UserTaskFeaturesAtTheirFloors(t *testing.T) {
	prog := &ast.Program{Statements: []ast.Statement{
		&ast.CreateModuleStmt{Name: "M"},
		&ast.CreateWorkflowStmt{Activities: []ast.WorkflowActivityNode{
			&ast.WorkflowUserTaskNode{Name: "A", Targeting: ast.WorkflowTargetingNode{Kind: "xpath", XPath: "[Name = 'a']"}},
			&ast.WorkflowUserTaskNode{Name: "B", Targeting: ast.WorkflowTargetingNode{Kind: "microflow", Microflow: ast.QualifiedName{Module: "M", Name: "U"}}},
			&ast.WorkflowUserTaskNode{Name: "C", IsMultiUser: true, Completion: &ast.WorkflowCompletionRuleNode{Rule: "veto", Veto: "No"}},
			&ast.WorkflowUserTaskNode{Name: "D", IsMultiUser: true, Completion: &ast.WorkflowCompletionRuleNode{Rule: "consensus"}},
		}},
	}}
	if vs := versionGateViolations(versionCtx(t, 9, 22, 0), prog); len(vs) != 0 {
		t.Errorf("user targeting and consensus/veto multi-user tasks reported on 9.22: %+v", vs)
	}
	for _, v := range [][3]int{{10, 0, 0}, {10, 2, 0}, {10, 3, 0}, {11, 2, 0}} {
		for _, act := range []*ast.WorkflowUserTaskNode{
			{Name: "M", IsMultiUser: true, Completion: &ast.WorkflowCompletionRuleNode{Rule: "majority"}},
			{Name: "G", Targeting: ast.WorkflowTargetingNode{Kind: "group_xpath", XPath: "[Name = 'g']"}},
		} {
			floor := [3]int{10, 0, 0}
			if act.Name == "G" {
				floor = [3]int{11, 2, 0}
			}
			if v[0] < floor[0] || (v[0] == floor[0] && v[1] < floor[1]) {
				continue
			}
			p := &ast.Program{Statements: []ast.Statement{&ast.CreateModuleStmt{Name: "M"},
				&ast.CreateWorkflowStmt{Activities: []ast.WorkflowActivityNode{act}}}}
			if vs := versionGateViolations(versionCtx(t, v[0], v[1], v[2]), p); len(vs) != 0 {
				t.Errorf("%s reported on %v, at or past its floor: %+v", act.Name, v, vs)
			}
		}
	}
}
