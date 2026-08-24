// SPDX-License-Identifier: Apache-2.0

// Translation statements: CREATE [OR MODIFY|REPLACE] TRANSLATIONS and
// DESCRIBE TRANSLATIONS. See docs/11-proposals/PROPOSAL_translations.md.
package visitor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateTranslationsStatement builds CREATE [OR MODIFY|REPLACE] TRANSLATIONS
// [IN Module] FOR <lang> ( 'src' AS 'target', … ).
//
// The OR MODIFY / OR REPLACE prefix belongs to the shared createStatement rule,
// so it is read from the parent rather than from this one.
func (b *Builder) ExitCreateTranslationsStatement(ctx *parser.CreateTranslationsStatementContext) {
	ids := ctx.AllIdentifierOrKeyword()
	if len(ids) == 0 {
		return
	}
	stmt := &ast.CreateTranslationsStmt{Mode: ast.TranslationsCreate}

	// `IN Module FOR lang` gives two identifiers, `FOR lang` only one — and the
	// language is always the last, because FOR comes last in the rule.
	stmt.Language = identifierOrKeywordText(ids[len(ids)-1])
	if len(ids) > 1 {
		stmt.Module = identifierOrKeywordText(ids[0])
	}

	if p, ok := ctx.GetParent().(*parser.CreateStatementContext); ok {
		switch {
		case p.REPLACE() != nil:
			stmt.Mode = ast.TranslationsReplace
		case p.MODIFY() != nil:
			stmt.Mode = ast.TranslationsModify
		}
	}

	for _, e := range ctx.AllTranslationEntry() {
		ec, ok := e.(*parser.TranslationEntryContext)
		if !ok {
			continue
		}
		lits := ec.AllSTRING_LITERAL()
		if len(lits) != 2 {
			continue
		}
		stmt.Entries = append(stmt.Entries, ast.TranslationEntry{
			Source: unquoteString(lits[0].GetText()),
			Target: unquoteString(lits[1].GetText()),
		})
	}

	b.statements = append(b.statements, stmt)
}
