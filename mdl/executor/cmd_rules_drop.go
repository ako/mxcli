// SPDX-License-Identifier: Apache-2.0

// Package executor - DROP RULE command
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// execDropRule handles DROP RULE statements. Mirrors execDropNanoflow without
// the dropped-document memo: that memo exists to carry AllowedModuleRoles across
// a drop-then-recreate, and a rule stores none.
func execDropRule(ctx *ExecContext, s *ast.DropRuleStmt) error {
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	rules, err := ctx.Backend.ListRules()
	if err != nil {
		return mdlerrors.NewBackend("list rules", err)
	}

	for _, rule := range rules {
		modID := h.FindModuleID(rule.ContainerID)
		if h.GetModuleName(modID) != s.Name.Module || rule.Name != s.Name.Name {
			continue
		}
		if err := ctx.Backend.DeleteRule(rule.ID); err != nil {
			return mdlerrors.NewBackend("delete rule", err)
		}
		invalidateHierarchy(ctx)
		fmt.Fprintf(ctx.Output, "Dropped rule: %s.%s\n", s.Name.Module, s.Name.Name)
		return nil
	}

	return mdlerrors.NewNotFound("rule", s.Name.Module+"."+s.Name.Name)
}
