// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ako/mxcli#1437 (mendixlabs/mxcli#1539): permissions_for() returned one row
// per module role and right with no id of the access rule it came from, so a
// rule could not count an entity's access rules per setting or list one rule's
// module roles exactly — it had to guess by grouping on XPath, default and
// rights, which merges two rules that happen to agree on those.
func TestPermissionRowsNameTheirAccessRule(t *testing.T) {
	cat := emptyCatalog(t)
	if _, err := cat.CatalogDB().Exec(`INSERT INTO permissions
		(ModuleRoleName, ElementType, ElementName, AccessType, XPathConstraint, DefaultMemberAccessRights, ModuleName, AccessRuleId, AccessRuleIndex) VALUES
		-- Two rules identical in every grouping key a rule could use before.
		('Shop.User',  'ENTITY', 'Shop.Order', 'READ', '', 'ReadWrite', 'Shop', 'rule-a', 0),
		('Shop.Admin', 'ENTITY', 'Shop.Order', 'READ', '', 'ReadWrite', 'Shop', 'rule-b', 1),
		('Shop.Admin', 'ENTITY', 'Shop.Order', 'WRITE', '', 'ReadWrite', 'Shop', 'rule-b', 1),
		-- A page permission comes from no access rule.
		('Shop.User',  'PAGE', 'Shop.Order_Overview', 'VIEW', NULL, NULL, 'Shop', NULL, NULL)`); err != nil {
		t.Fatal(err)
	}
	ctx := linter.NewLintContext(cat, nil)

	got := runDump1437(t, ctx, `    rules = {}
    for p in permissions_for("Shop.Order"):
        rules.setdefault("%s@%d" % (p.access_rule_id, p.access_rule_index), []).append(p.module_role_name)
    return [violation(message = "%s=%s" % (k, ",".join(sorted({r: True for r in v}.keys())))) for k, v in sorted(rules.items())]`)
	if want := "rule-a@0=Shop.User|rule-b@1=Shop.Admin"; strings.Join(got, "|") != want {
		t.Errorf("permissions_for grouped by access rule = %v, want %s", got, want)
	}

	got = runDump1437(t, ctx, `    return [violation(message = "%s|%s|%d" % (p.element_name, p.access_rule_id, p.access_rule_index)) for p in permissions() if p.element_type == "PAGE"]`)
	if want := "Shop.Order_Overview||-1"; strings.Join(got, ",") != want {
		t.Errorf("a page permission = %v, want %s (no access rule: empty id, index -1)", got, want)
	}
}

func runDump1437(t *testing.T, ctx *linter.LintContext, body string) []string {
	t.Helper()
	vs, _ := runSrc(t, ctx, "def check():\n"+body)
	var got []string
	for _, v := range vs {
		got = append(got, v.Message)
	}
	return got
}
