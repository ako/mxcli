// SPDX-License-Identifier: Apache-2.0

package ast

// CreateJsonStructureStmt represents:
//
//	CREATE [OR REPLACE] JSON STRUCTURE Module.Name [COMMENT 'doc'] SNIPPET '...json...' [CUSTOM NAME MAP (...)];
type CreateJsonStructureStmt struct {
	Name             QualifiedName
	JsonSnippet      string            // Raw JSON snippet
	Documentation    string            // Optional documentation comment
	DocumentationSet bool              // see mendixlabs/mxcli#1018: absent preserves, empty clears
	Folder           string            // Optional folder path within module
	CreateOrModify   bool              // true for CREATE OR MODIFY (or OR REPLACE, treated identically)
	CustomNameMap    map[string]string // Optional: JSON key → custom ExposedName
	// CustomItemNameMap names the ITEM element of an array, keyed by the array's
	// JSON key ("Root" for a root-level array). An item has no key of its own,
	// so CustomNameMap cannot reach it and its name was derived and unspellable
	// (ako/mxcli#272).
	CustomItemNameMap map[string]string
}

func (s *CreateJsonStructureStmt) isStatement() {}

// DropJsonStructureStmt represents: DROP JSON STRUCTURE Module.Name
type DropJsonStructureStmt struct {
	Name QualifiedName
}

func (s *DropJsonStructureStmt) isStatement() {}
