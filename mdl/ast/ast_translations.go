// SPDX-License-Identifier: Apache-2.0

package ast

// TranslationEntry is one line of a translation file: a source string and what
// it says in the target language. An empty Target means "present but not
// translated yet" — which is what DESCRIBE emits for an untranslated string, and
// what makes its output usable as an LLM prompt.
type TranslationEntry struct {
	Source string
	Target string
}

// TranslationMode is what a CREATE TRANSLATIONS statement does to the
// translations already in the project. The language is the thing that exists, so
// the three map onto MDL's CREATE verbs directly.
type TranslationMode int

const (
	// TranslationsCreate — bare CREATE. Refuses when the language already has
	// translations, so it is the "add a new language" statement.
	TranslationsCreate TranslationMode = iota
	// TranslationsModify — CREATE OR MODIFY. Merge; sources not named are left
	// alone.
	TranslationsModify
	// TranslationsReplace — CREATE OR REPLACE. The file is authoritative: a
	// translation whose source it does not name is removed.
	TranslationsReplace
)

func (m TranslationMode) String() string {
	switch m {
	case TranslationsModify:
		return "create or modify"
	case TranslationsReplace:
		return "create or replace"
	}
	return "create"
}

// CreateTranslationsStmt is CREATE [OR MODIFY|REPLACE] TRANSLATIONS [IN Module]
// FOR <lang> ( 'src' AS 'target', … ).
type CreateTranslationsStmt struct {
	Language string
	Module   string // optional scope; empty means the whole project
	Mode     TranslationMode
	Entries  []TranslationEntry
}

func (s *CreateTranslationsStmt) isStatement() {}

// DescribeTranslationsStmt is DESCRIBE TRANSLATIONS [IN Module] FOR <lang>.
type DescribeTranslationsStmt struct {
	Language string
	Module   string
}

func (s *DescribeTranslationsStmt) isStatement() {}
