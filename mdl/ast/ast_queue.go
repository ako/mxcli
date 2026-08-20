// SPDX-License-Identifier: Apache-2.0

package ast

// CreateQueueStmt represents:
//
//	CREATE [OR REPLACE|MODIFY] QUEUE Module.Name ( Parallelism: 3, ClusterWide: true );
type CreateQueueStmt struct {
	Folder        string // Folder path within module (empty = leave placement alone)
	Name          QualifiedName
	Documentation string
	// Parallelism is kept as written. Mendix stores it as an expression string
	// (Queues$BasicQueueConfig.ParallelismExpression), so `3` and `'3'` are the
	// same thing and an arbitrary expression is legal.
	Parallelism string
	ClusterWide bool
	ExportLevel string
	// CreateOrModify is set by the shared CREATE OR REPLACE/MODIFY prefix.
	CreateOrModify bool
}

func (s *CreateQueueStmt) isStatement() {}

// DropQueueStmt represents: DROP QUEUE Module.Name;
type DropQueueStmt struct {
	Name QualifiedName
}

func (s *DropQueueStmt) isStatement() {}

// ShowQueuesStmt represents: SHOW|LIST QUEUES [IN Module];
type ShowQueuesStmt struct {
	Module string
}

func (s *ShowQueuesStmt) isStatement() {}

// DescribeQueueStmt represents: DESCRIBE QUEUE Module.Name;
type DescribeQueueStmt struct {
	Name QualifiedName
}

func (s *DescribeQueueStmt) isStatement() {}
