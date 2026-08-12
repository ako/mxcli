// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ----------------------------------------------------------------------------
// SQL Statements (external database connectivity)
// ----------------------------------------------------------------------------

// sqlWords returns the identifierOrKeyword texts of a SQL statement context.
//
// ANTLR still walks the tree after a syntax error, so a listener can be handed a
// context whose children are missing. Reading them through this helper (and
// checking the count) keeps a malformed statement an error rather than a crash:
// `SQL DISCONNECT source` used to segfault `mxcli check` on ctx.IDENTIFIER()
// returning nil, because `source` lexes as SOURCE_KW and the rule wanted a bare
// IDENTIFIER.
func sqlWords(all []parser.IIdentifierOrKeywordContext) []string {
	out := make([]string, 0, len(all))
	for _, c := range all {
		iok, ok := c.(*parser.IdentifierOrKeywordContext)
		if !ok || iok == nil {
			continue
		}
		out = append(out, identifierOrKeywordText(iok))
	}
	return out
}

// ExitSqlConnect handles SQL CONNECT <driver> '<dsn>' AS <alias>
func (b *Builder) ExitSqlConnect(ctx *parser.SqlConnectContext) {
	words := sqlWords(ctx.AllIdentifierOrKeyword())
	if len(words) < 2 || ctx.STRING_LITERAL() == nil {
		return
	}
	b.statements = append(b.statements, &ast.SQLConnectStmt{
		Driver: words[0],
		DSN:    unquoteString(ctx.STRING_LITERAL().GetText()),
		Alias:  words[1],
	})
}

// ExitSqlDisconnect handles SQL DISCONNECT <alias>
func (b *Builder) ExitSqlDisconnect(ctx *parser.SqlDisconnectContext) {
	words := sqlWords([]parser.IIdentifierOrKeywordContext{ctx.IdentifierOrKeyword()})
	if len(words) < 1 {
		return
	}
	b.statements = append(b.statements, &ast.SQLDisconnectStmt{
		Alias: words[0],
	})
}

// ExitSqlConnections handles SQL CONNECTIONS
func (b *Builder) ExitSqlConnections(ctx *parser.SqlConnectionsContext) {
	b.statements = append(b.statements, &ast.SQLConnectionsStmt{})
}

// ExitSqlShowTables handles SQL <alias> SHOW TABLES|VIEWS|FUNCTIONS
func (b *Builder) ExitSqlShowTables(ctx *parser.SqlShowTablesContext) {
	words := sqlWords(ctx.AllIdentifierOrKeyword())
	if len(words) < 2 {
		return
	}
	alias := words[0]
	target := strings.ToUpper(words[1])

	switch target {
	case "VIEWS":
		b.statements = append(b.statements, &ast.SQLShowViewsStmt{Alias: alias})
	case "FUNCTIONS", "PROCEDURES":
		b.statements = append(b.statements, &ast.SQLShowFunctionsStmt{Alias: alias})
	default:
		// Default: TABLES (or any other target)
		b.statements = append(b.statements, &ast.SQLShowTablesStmt{Alias: alias})
	}
}

// ExitSqlDescribeTable handles SQL <alias> DESCRIBE <table>
func (b *Builder) ExitSqlDescribeTable(ctx *parser.SqlDescribeTableContext) {
	words := sqlWords(ctx.AllIdentifierOrKeyword())
	if len(words) < 2 {
		return
	}
	alias := words[0]
	table := words[1]
	b.statements = append(b.statements, &ast.SQLDescribeTableStmt{
		Alias: alias,
		Table: table,
	})
}

// ExitImportFromQuery handles IMPORT FROM <alias> QUERY '<sql>' INTO Module.Entity MAP (...) [BATCH n] [LIMIT n]
func (b *Builder) ExitImportFromQuery(ctx *parser.ImportFromQueryContext) {
	alias := identifierOrKeywordText(ctx.IdentifierOrKeyword())
	if ctx.QualifiedName() == nil {
		return
	}
	var query string
	if sl := ctx.STRING_LITERAL(); sl != nil {
		query = unquoteString(sl.GetText())
	} else if ds := ctx.DOLLAR_STRING(); ds != nil {
		s := ds.GetText()
		if len(s) >= 4 && strings.HasPrefix(s, "$$") && strings.HasSuffix(s, "$$") {
			query = s[2 : len(s)-2]
		}
	} else {
		return
	}
	entity := buildQualifiedName(ctx.QualifiedName()).String()

	var mappings []ast.ImportMapping
	for _, m := range ctx.AllImportMapping() {
		mc := m.(*parser.ImportMappingContext)
		ioks := mc.AllIdentifierOrKeyword()
		if len(ioks) < 2 {
			continue
		}
		mappings = append(mappings, ast.ImportMapping{
			SourceColumn: identifierOrKeywordText(ioks[0]),
			TargetAttr:   identifierOrKeywordText(ioks[1]),
		})
	}

	// Parse optional LINK clause
	var links []ast.LinkMapping
	for _, lm := range ctx.AllLinkMapping() {
		switch lc := lm.(type) {
		case *parser.LinkLookupContext:
			ioks := lc.AllIdentifierOrKeyword()
			if len(ioks) < 3 {
				continue
			}
			links = append(links, ast.LinkMapping{
				SourceColumn:    identifierOrKeywordText(ioks[0]),
				AssociationName: identifierOrKeywordText(ioks[1]),
				LookupAttr:      identifierOrKeywordText(ioks[2]),
			})
		case *parser.LinkDirectContext:
			ioks := lc.AllIdentifierOrKeyword()
			if len(ioks) < 2 {
				continue
			}
			links = append(links, ast.LinkMapping{
				SourceColumn:    identifierOrKeywordText(ioks[0]),
				AssociationName: identifierOrKeywordText(ioks[1]),
			})
		}
	}

	stmt := &ast.ImportStmt{
		SourceAlias:  alias,
		Query:        query,
		TargetEntity: entity,
		Mappings:     mappings,
		Links:        links,
	}

	// Parse optional BATCH
	nums := ctx.AllNUMBER_LITERAL()
	numIdx := 0
	if ctx.BATCH() != nil && numIdx < len(nums) {
		stmt.BatchSize, _ = strconv.Atoi(nums[numIdx].GetText())
		numIdx++
	}
	// Parse optional LIMIT
	if ctx.LIMIT() != nil && numIdx < len(nums) {
		stmt.Limit, _ = strconv.Atoi(nums[numIdx].GetText())
	}

	b.statements = append(b.statements, stmt)
}

// ExitSqlGenerateConnector handles SQL <alias> GENERATE CONNECTOR INTO <module> [TABLES (...)] [VIEWS (...)] [EXEC]
func (b *Builder) ExitSqlGenerateConnector(ctx *parser.SqlGenerateConnectorContext) {
	// The alias is now the first identifierOrKeyword of the rule (it used to be a
	// separate IDENTIFIER token), so the module is [1] and the table/view lists
	// start at [2].
	allIOK := ctx.AllIdentifierOrKeyword()
	if len(allIOK) < 2 {
		return
	}
	alias := identifierOrKeywordText(allIOK[0])
	module := identifierOrKeywordText(allIOK[1])
	rest := allIOK[2:]

	hasTables := ctx.TABLES() != nil
	hasViews := ctx.VIEWS() != nil

	var tables []string
	var views []string

	if hasTables && hasViews {
		// Use token position of VIEWS keyword to split
		viewsPos := ctx.VIEWS().GetSymbol().GetTokenIndex()
		for _, iok := range rest {
			iokPos := iok.(*parser.IdentifierOrKeywordContext).GetStart().GetTokenIndex()
			if iokPos < viewsPos {
				tables = append(tables, identifierOrKeywordText(iok))
			} else {
				views = append(views, identifierOrKeywordText(iok))
			}
		}
	} else if hasTables {
		for _, iok := range rest {
			tables = append(tables, identifierOrKeywordText(iok))
		}
	} else if hasViews {
		for _, iok := range rest {
			views = append(views, identifierOrKeywordText(iok))
		}
	}

	stmt := &ast.SQLGenerateConnectorStmt{
		Alias:  alias,
		Module: module,
		Exec:   ctx.EXEC() != nil,
	}
	if hasTables {
		stmt.Tables = tables
	}
	if hasViews {
		stmt.Views = views
	}

	b.statements = append(b.statements, stmt)
}

// ExitSqlQuery handles SQL <alias> <raw-sql>
func (b *Builder) ExitSqlQuery(ctx *parser.SqlQueryContext) {
	words := sqlWords([]parser.IIdentifierOrKeywordContext{ctx.IdentifierOrKeyword()})
	passthrough := ctx.SqlPassthrough()
	if len(words) < 1 || passthrough == nil {
		return
	}
	alias := words[0]
	query := getSpacedText(passthrough)
	b.statements = append(b.statements, &ast.SQLQueryStmt{
		Alias: alias,
		Query: query,
	})
}
