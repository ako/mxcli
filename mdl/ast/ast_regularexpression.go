// SPDX-License-Identifier: Apache-2.0

package ast

// CreateRegularExpressionStmt represents:
//
//	CREATE [OR REPLACE|MODIFY] REGULAR EXPRESSION Module.Name (
//	  Expression: '^[a-z]+$'
//	);
type CreateRegularExpressionStmt struct {
	Name          QualifiedName
	Documentation string
	// Expression is the pattern, as written (unquoted).
	Expression  string
	ExportLevel string
	// CreateOrModify is set by the shared CREATE OR REPLACE/MODIFY prefix.
	CreateOrModify bool
}

func (s *CreateRegularExpressionStmt) isStatement() {}

// DropRegularExpressionStmt represents: DROP REGULAR EXPRESSION Module.Name;
type DropRegularExpressionStmt struct {
	Name QualifiedName
}

func (s *DropRegularExpressionStmt) isStatement() {}

// ShowRegularExpressionsStmt represents: SHOW|LIST REGULAR EXPRESSIONS [IN Module];
type ShowRegularExpressionsStmt struct {
	Module string
}

func (s *ShowRegularExpressionsStmt) isStatement() {}

// DescribeRegularExpressionStmt represents: DESCRIBE REGULAR EXPRESSION Module.Name;
type DescribeRegularExpressionStmt struct {
	Name QualifiedName
}

func (s *DescribeRegularExpressionStmt) isStatement() {}
