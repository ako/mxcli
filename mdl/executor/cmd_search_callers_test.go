// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// upstream #773, second half. `show callers` filtered on RefKind = 'call' — the
// kind a MICROFLOW call activity produces. A microflow invoked from a page action
// button is recorded as 'action', and that row was already sitting in the refs
// table: the reference existed, the query hid it. A page opened by a button or a
// navigation menu was invisible for the same reason.
//
// A false negative here reads as "nothing uses this", which is the answer
// somebody acts on before deleting a document — so the set errs toward including
// a kind rather than omitting it.
func TestCallerRefKinds(t *testing.T) {
	in := map[string]bool{}
	for _, k := range callerRefKinds {
		in[k] = true
	}

	// Every way one document can INVOKE another.
	for _, k := range []string{
		"call",       // microflow/nanoflow call activity — the only one that used to count
		"action",     // widget action: the reported button case
		"show_page",  // a microflow, or a button, opening a page
		"calculate",  // calculated attribute running a microflow
		"home_page",  // navigation
		"login_page", //
		"menu_item",  //
	} {
		if !in[k] {
			t.Errorf("%q means one document invokes another and must count as a caller — "+
				"omitting it reports the target as unused, which is what #773 was", k)
		}
	}

	// Uses of a TYPE or a LAYOUT are NOT invocations. Folding these in would make
	// `show callers of <entity>` a synonym for `show references to` and destroy
	// the distinction between the two commands.
	for _, k := range []string{
		"datasource", "parameter", "return", "retrieve",
		"create", "change", "delete", "associate", "generalize", "layout",
	} {
		if in[k] {
			t.Errorf("%q is a use of a type or layout, not an invocation — including it "+
				"collapses `show callers` into `show references to`", k)
		}
	}
}

// The kind list reaches SQLite as an IN list, so a malformed render silently
// matches nothing — the same "(no callers found)" the issue reported, from a
// different cause.
func TestCallerRefKindsSQL(t *testing.T) {
	got := callerRefKindsSQL()
	if !strings.HasPrefix(got, "(") || !strings.HasSuffix(got, ")") {
		t.Fatalf("not a SQL IN list: %q", got)
	}
	for _, k := range callerRefKinds {
		if !strings.Contains(got, "'"+k+"'") {
			t.Errorf("%q is not quoted in the IN list %q", k, got)
		}
	}
	if n := strings.Count(got, ","); n != len(callerRefKinds)-1 {
		t.Errorf("%d separators for %d kinds: %q", n, len(callerRefKinds), got)
	}
}
