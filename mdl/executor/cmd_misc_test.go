// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// The REPL's `help <topic>` and `mxcli syntax <topic>` answer the same question
// and must answer it the same way (mendixlabs/mxcli#1025). The resolution table
// lives with the resolver in cmd/mxcli/syntax; this pins the REPL's end of it.
func TestExecHelpResolvesTopics(t *testing.T) {
	tests := []struct {
		name  string
		topic []string
		want  string
	}{
		{"single word exact", []string{"workflow"}, "workflow"},
		{"multi-level path", []string{"workflow", "user", "task"}, "workflow.user-task"},
		{"three-level path", []string{"workflow", "user", "task", "targeting"}, "workflow.user-task.targeting"},
		{"hyphenated as typed", []string{"workflow", "user-task", "targeting"}, "workflow.user-task.targeting"},
		{"dotted", []string{"workflow.user-task.targeting"}, "workflow.user-task.targeting"},
		{"security prefix", []string{"security", "entity", "access"}, "security.entity-access"},
		{"legacy alias", []string{"entity"}, "domain-model.entity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			ctx := &ExecContext{Output: &out}
			if err := execHelp(ctx, &ast.HelpStmt{Topic: tt.topic}); err != nil {
				t.Fatalf("execHelp: %v", err)
			}
			got := out.String()
			if strings.Contains(got, "No syntax help found") {
				t.Errorf("help %v: %s", tt.topic, strings.TrimSpace(got))
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("help %v did not reach %q:\n%s", tt.topic, tt.want, got)
			}
		})
	}
}
