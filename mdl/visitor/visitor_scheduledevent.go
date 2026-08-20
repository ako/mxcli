// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

// ExitCreateScheduledEventStatement builds a CreateScheduledEventStmt from
// CREATE [OR REPLACE|MODIFY] SCHEDULED EVENT Module.Name ( ... ).
//
// Every property is carried through as written; the executor decides which ones
// the chosen Repeat actually has and rejects the rest. Numeric properties are
// stored as pointers so an explicit 0 (a real hour, minute or month offset) is
// distinguishable from an omitted one.
func (b *Builder) ExitCreateScheduledEventStatement(ctx *parser.CreateScheduledEventStatementContext) {
	stmt := &ast.CreateScheduledEventStmt{
		Name:          buildQualifiedName(ctx.QualifiedName()),
		Documentation: findDocCommentText(ctx),
	}
	if lit := ctx.STRING_LITERAL(); lit != nil {
		stmt.Folder = unquoteString(lit.GetText())
	}
	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
	}

	if body := ctx.ScheduledEventBody(); body != nil {
		bodyCtx := body.(*parser.ScheduledEventBodyContext)
		for _, prop := range bodyCtx.AllScheduledEventProperty() {
			pc, ok := prop.(*parser.ScheduledEventPropertyContext)
			if !ok || pc == nil {
				continue
			}
			iok := pc.IdentifierOrKeyword(0)
			if iok == nil {
				continue
			}
			key := strings.ToLower(identifierOrKeywordText(iok))
			val := scheduledEventPropertyText(pc)
			switch key {
			case "microflow":
				stmt.Microflow = val
			case "repeat":
				stmt.Repeat = val
			case "multiplier":
				stmt.Multiplier = parseIntPtr(val)
			case "minuteoffset":
				stmt.MinuteOffset = parseIntPtr(val)
			case "monthoffset":
				stmt.MonthOffset = parseIntPtr(val)
			case "hourofday":
				stmt.HourOfDay = parseIntPtr(val)
			case "minuteofhour":
				stmt.MinuteOfHour = parseIntPtr(val)
			case "dayofmonth":
				stmt.DayOfMonth = parseIntPtr(val)
			case "month":
				stmt.Month = parseIntPtr(val)
			case "weekdays":
				stmt.Weekdays = val
			case "dayselector":
				stmt.DaySelector = val
			case "weekday":
				stmt.Weekday = val
			case "startdatetime":
				stmt.StartDateTime = val
			case "timezone":
				stmt.TimeZone = val
			case "onoverlap":
				stmt.OnOverlap = val
			case "enabled":
				stmt.Enabled = parseBoolPtr(val)
			case "excluded":
				stmt.Excluded = parseBoolPtr(val)
			case "exportlevel":
				stmt.ExportLevel = val
			case "documentation":
				stmt.Documentation = val
			}
		}
	}

	b.statements = append(b.statements, stmt)
}

// scheduledEventPropertyText returns the value side of a property, unquoted.
//
// The key is identifierOrKeyword(0), so an identifier value is index 1 —
// reading index 0 would echo the key back as the value.
func scheduledEventPropertyText(pc *parser.ScheduledEventPropertyContext) string {
	if qn := pc.QualifiedName(); qn != nil {
		return getQualifiedNameText(qn)
	}
	if n := pc.NUMBER_LITERAL(); n != nil {
		return n.GetText()
	}
	if s := pc.STRING_LITERAL(); s != nil {
		return unquoteString(s.GetText())
	}
	if bl := pc.BooleanLiteral(); bl != nil {
		return bl.GetText()
	}
	if v := pc.IdentifierOrKeyword(1); v != nil {
		return identifierOrKeywordText(v)
	}
	return ""
}

// parseIntPtr returns nil for anything that is not an integer, so a malformed
// value is reported by the executor's validation rather than silently becoming 0.
func parseIntPtr(s string) *int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return &n
}

func parseBoolPtr(s string) *bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true":
		v := true
		return &v
	case "false":
		v := false
		return &v
	}
	return nil
}
