// SPDX-License-Identifier: Apache-2.0

// Package executor — regular expression commands (CREATE/DROP/SHOW/DESCRIBE
// REGULAR EXPRESSION).
//
// A Mendix regular expression is a named, shared pattern. Attribute validation
// rules reference it by qualified name rather than carrying a pattern of their
// own, which is why it is a document.
package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// findRegularExpression returns the regex with the given module-qualified name.
func findRegularExpression(ctx *ExecContext, moduleName, name string) *model.RegularExpression {
	all, err := ctx.Backend.ListRegularExpressions()
	if err != nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	for _, re := range all {
		if !strings.EqualFold(re.Name, name) {
			continue
		}
		mod := h.GetModuleName(h.FindModuleID(re.ContainerID))
		if strings.EqualFold(mod, moduleName) {
			return re
		}
	}
	return nil
}

// execCreateRegularExpression handles CREATE [OR REPLACE|MODIFY] REGULAR EXPRESSION.
func execCreateRegularExpression(ctx *ExecContext, s *ast.CreateRegularExpressionStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	if err := validateRegularExpressionStmt(s); err != nil {
		return err
	}

	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	existing := findRegularExpression(ctx, s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("regular expression", s.Name.String())
	}

	var existingContainer model.ID
	if existing != nil {
		existingContainer = existing.ContainerID
	}
	containerID, err := containerForDocument(ctx, module.ID, s.Folder, existingContainer)
	if err != nil {
		return err
	}

	re := &model.RegularExpression{
		ContainerID:   containerID,
		Name:          s.Name.Name,
		Documentation: s.Documentation,
		Expression:    s.Expression,
		ExportLevel:   s.ExportLevel,
	}
	if existing != nil {
		re.ID = existing.ID
		re.Excluded = existing.Excluded
		if err := ctx.Backend.UpdateRegularExpression(re); err != nil {
			return mdlerrors.NewBackend("update regular expression", err)
		}
		if _, err := applyDocumentFolder(ctx, re.ID, existingContainer, containerID); err != nil {
			return err
		}
		ctx.ReportMutation("Modified", "regular expression: %s", s.Name.String())
		return nil
	}
	if err := ctx.Backend.CreateRegularExpression(re); err != nil {
		return mdlerrors.NewBackend("create regular expression", err)
	}
	fmt.Fprintf(ctx.Output, "Created regular expression: %s\n", s.Name.String())
	return nil
}

// validateRegularExpressionStmt rejects a statement that could not produce a
// working document.
//
// The pattern is compiled with Go's regexp so an obviously broken one is caught
// before it is written. This is a WARNING-grade check dressed as an error only
// for syntax Go rejects outright: Mendix validates with .NET's engine, which
// accepts constructs Go's RE2 does not (backreferences, lookaround). A real
// Mendix regex — `.*(?<!/)$` from the Email Connector — is one of those, so a
// pattern Go cannot compile is reported as unverifiable rather than invalid.
func validateRegularExpressionStmt(s *ast.CreateRegularExpressionStmt) error {
	if strings.TrimSpace(s.Expression) == "" {
		return mdlerrors.NewValidation(
			"regular expression " + s.Name.String() + " has no Expression — add `Expression: '<pattern>'`")
	}
	if err := validateEnumProperty("ExportLevel", s.ExportLevel, []string{"Hidden", "Public"}); err != nil {
		return err
	}
	return nil
}

// regexCompilesInGo reports whether Go's RE2 accepts the pattern. Used only to
// annotate output: .NET accepts more than RE2 does.
func regexCompilesInGo(pattern string) bool {
	_, err := regexp.Compile(pattern)
	return err == nil
}

// execDropRegularExpression handles DROP REGULAR EXPRESSION Module.Name.
func execDropRegularExpression(ctx *ExecContext, s *ast.DropRegularExpressionStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	existing := findRegularExpression(ctx, s.Name.Module, s.Name.Name)
	if existing == nil {
		return mdlerrors.NewNotFound("regular expression", s.Name.String())
	}
	if err := ctx.Backend.DeleteRegularExpression(string(existing.ID)); err != nil {
		return mdlerrors.NewBackend("drop regular expression", err)
	}
	fmt.Fprintf(ctx.Output, "Dropped regular expression: %s\n", s.Name.String())
	return nil
}

// execShowRegularExpressions handles SHOW|LIST REGULAR EXPRESSIONS [IN Module].
func execShowRegularExpressions(ctx *ExecContext, s *ast.ShowRegularExpressionsStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	all, err := ctx.Backend.ListRegularExpressions()
	if err != nil {
		return mdlerrors.NewBackend("list regular expressions", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	type row struct{ qualified, pattern, doc string }
	var rows []row
	for _, re := range all {
		mod := h.GetModuleName(h.FindModuleID(re.ContainerID))
		if s.Module != "" && !strings.EqualFold(mod, s.Module) {
			continue
		}
		rows = append(rows, row{mod + "." + re.Name, re.Expression, re.Documentation})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].qualified < rows[j].qualified })

	result := &TableResult{
		Columns: []string{"Regular Expression", "Pattern", "Documentation"},
		Summary: fmt.Sprintf("(%d regular expression(s))", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualified, r.pattern, r.doc})
	}
	return writeResult(ctx, result)
}

// execDescribeRegularExpression handles DESCRIBE REGULAR EXPRESSION Module.Name,
// emitting re-executable MDL so describe → exec round-trips.
func execDescribeRegularExpression(ctx *ExecContext, s *ast.DescribeRegularExpressionStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	re := findRegularExpression(ctx, s.Name.Module, s.Name.Name)
	if re == nil {
		return mdlerrors.NewNotFound("regular expression", s.Name.String())
	}

	if re.Documentation != "" {
		fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", re.Documentation)
	}
	fmt.Fprintf(ctx.Output, "create or modify regular expression %s%s (\n", s.Name.String(), describeFolderClause(ctx, re.ContainerID))
	fmt.Fprintf(ctx.Output, "  Expression: '%s',\n", strings.ReplaceAll(re.Expression, "'", "''"))
	if re.ExportLevel != "" && re.ExportLevel != "Hidden" {
		fmt.Fprintf(ctx.Output, "  ExportLevel: %s,\n", re.ExportLevel)
	}
	fmt.Fprint(ctx.Output, ");\n")
	// Mendix validates with .NET's regex engine, which accepts constructs Go's
	// RE2 does not (lookaround, backreferences) — a real Mendix pattern,
	// `.*(?<!/)$`, is one. Note it rather than implying the pattern is broken.
	if !regexCompilesInGo(re.Expression) {
		fmt.Fprintln(ctx.Output, "-- note: uses .NET regex syntax that Go cannot compile (e.g. lookaround); not verifiable here")
	}
	return nil
}
