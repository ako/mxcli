// SPDX-License-Identifier: Apache-2.0

package ast

// AlterSettingsStmt represents ALTER SETTINGS commands.
type AlterSettingsStmt struct {
	Section    string         // "MODEL", "CONFIGURATION", "CONSTANT", "LANGUAGE", "WORKFLOWS"
	ConfigName string         // For CONFIGURATION section: the configuration name (e.g., "Default")
	Properties map[string]any // Key-value pairs to set
	// For CONSTANT section:
	ConstantId   string // Qualified constant name
	Value        string // Constant value
	DropConstant bool   // If true, remove the constant override instead of setting it
	// For LANGUAGE ADD/REMOVE: the ISO code of the language to enable or
	// disable. A language is identified by its code alone — Studio Pro derives
	// the display name ("Arabic, Sudan") from `ar_SD` and does not store it.
	LanguageCode   string
	AddLanguage    bool
	ModifyLanguage bool
	// UpsertLanguage is ADD OR MODIFY: enable the language when it is not there,
	// change the named options when it is. It is what DESCRIBE emits, so a
	// described project re-executes against a project that already has some of
	// its languages.
	UpsertLanguage bool
	RemoveLanguage bool
}

func (s *AlterSettingsStmt) isStatement() {}

// CreateConfigurationStmt represents CREATE CONFIGURATION 'name' [properties...].
type CreateConfigurationStmt struct {
	Name       string
	Properties map[string]any
	// CreateOrModify is CREATE OR MODIFY: update the configuration when it is
	// already there instead of refusing. The grammar has always accepted the
	// prefix — `CREATE (OR (MODIFY|REPLACE))?` is generic — so without this the
	// documented upsert parsed and then behaved as a plain CREATE, answering
	// "configuration already exists" to the statement whose whole point is that
	// it should not.
	CreateOrModify bool
}

func (s *CreateConfigurationStmt) isStatement() {}

// DropConfigurationStmt represents DROP CONFIGURATION 'name'.
type DropConfigurationStmt struct {
	Name string
}

func (s *DropConfigurationStmt) isStatement() {}
