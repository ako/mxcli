// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestCreateQueue(t *testing.T) {
	input := `CREATE QUEUE Ops.OrderProcessing (
		Parallelism: 3,
		ClusterWide: true
	);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateQueueStmt)
	if !ok {
		t.Fatalf("expected CreateQueueStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.Module != "Ops" || stmt.Name.Name != "OrderProcessing" {
		t.Errorf("Name = %+v", stmt.Name)
	}
	if stmt.Parallelism != "3" {
		t.Errorf("Parallelism = %q, want 3", stmt.Parallelism)
	}
	if !stmt.ClusterWide {
		t.Error("ClusterWide = false, want true")
	}
	if stmt.CreateOrModify {
		t.Error("CreateOrModify set without OR MODIFY")
	}
}

// TestCreateQueue_ExpressionParallelism covers the reason Parallelism is a
// string all the way down: Mendix stores an expression, not a number.
func TestCreateQueue_ExpressionParallelism(t *testing.T) {
	prog, errs := Build(`CREATE OR MODIFY QUEUE Ops.Q ( Parallelism: '$Config/Workers' );`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateQueueStmt)
	if stmt.Parallelism != "$Config/Workers" {
		t.Errorf("Parallelism = %q, want the unquoted expression", stmt.Parallelism)
	}
	if !stmt.CreateOrModify {
		t.Error("CreateOrModify not set for CREATE OR MODIFY")
	}
}

func TestCreateQueue_Defaults(t *testing.T) {
	prog, errs := Build(`CREATE QUEUE Ops.Q ();`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateQueueStmt)
	if stmt.Parallelism != "" {
		t.Errorf("Parallelism = %q, want empty (the backend supplies the default)", stmt.Parallelism)
	}
	if stmt.ClusterWide {
		t.Error("ClusterWide defaulted to true")
	}
}

func TestDropQueue(t *testing.T) {
	prog, errs := Build(`DROP QUEUE Ops.OrderProcessing;`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.DropQueueStmt)
	if !ok {
		t.Fatalf("expected DropQueueStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.String() != "Ops.OrderProcessing" {
		t.Errorf("Name = %s", stmt.Name.String())
	}
}

func TestShowAndDescribeQueues(t *testing.T) {
	prog, errs := Build(`SHOW QUEUES; LIST QUEUES IN Ops; DESCRIBE QUEUE Ops.OrderProcessing;`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if len(prog.Statements) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(prog.Statements))
	}
	if _, ok := prog.Statements[0].(*ast.ShowQueuesStmt); !ok {
		t.Errorf("statement 0 = %T, want ShowQueuesStmt", prog.Statements[0])
	}
	s1, ok := prog.Statements[1].(*ast.ShowQueuesStmt)
	if !ok {
		t.Fatalf("statement 1 = %T, want ShowQueuesStmt", prog.Statements[1])
	}
	if s1.Module != "Ops" {
		t.Errorf("Module = %q, want Ops", s1.Module)
	}
	s2, ok := prog.Statements[2].(*ast.DescribeQueueStmt)
	if !ok {
		t.Fatalf("statement 2 = %T, want DescribeQueueStmt", prog.Statements[2])
	}
	if s2.Name.String() != "Ops.OrderProcessing" {
		t.Errorf("Name = %s", s2.Name.String())
	}
}

// TestQueueKeywordStillUsableAsIdentifier guards the cost of adding QUEUE to the
// lexer: a new keyword stops being usable as an ordinary name unless it is also
// listed in the `keyword` rule. "queue" is a plausible attribute name.
func TestQueueKeywordStillUsableAsIdentifier(t *testing.T) {
	prog, errs := Build(`CREATE ENTITY Ops.Job ( queue: String(100), queues: String(100) );`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateEntityStmt)
	if !ok {
		t.Fatalf("expected CreateEntityStmt, got %T", prog.Statements[0])
	}
	if len(stmt.Attributes) != 2 {
		t.Fatalf("expected 2 attributes, got %d", len(stmt.Attributes))
	}
	if stmt.Attributes[0].Name != "queue" || stmt.Attributes[1].Name != "queues" {
		t.Errorf("attribute names = %q, %q", stmt.Attributes[0].Name, stmt.Attributes[1].Name)
	}
}
