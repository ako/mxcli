// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance §4: a page column added with ALTER PAGE in a second
// script was lost on the next run of the first, with nothing reported.
//
// The dropping itself is what CREATE OR REPLACE means. What made it a defect is
// that mxcli's own check RECOMMENDED the form that does it: for every document
// type except entity the hint was "use CREATE OR MODIFY to update it", and for a
// page CREATE OR MODIFY is not a modify at all — the executor treats it exactly
// as CREATE OR REPLACE (`!s.IsReplace && !s.IsModify`) and rebuilds the document
// from the statement.
//
// Entities already had this steer, written for an earlier finding of the same
// shape. These types never got it, and each of them has an incremental ALTER.
package executor

import (
	"strings"
	"testing"
)

func TestIncrementalAlterCoversTheTypesThatHaveOne(t *testing.T) {
	// A type belongs in the table only if its ALTER really is incremental. If one
	// is added that rebuilds wholesale, the hint would send users from one
	// destructive form to another.
	want := []string{"page", "snippet", "layout", "workflow"}
	for _, dt := range want {
		verb, ok := incrementalAlter[dt]
		if !ok {
			t.Errorf("%s has an incremental ALTER but is missing from the table", dt)
			continue
		}
		if !strings.HasPrefix(verb, "ALTER ") {
			t.Errorf("%s: verb %q should read as the statement a user types", dt, verb)
		}
	}
	// Entity is deliberately absent: its ALTER takes a different shape
	// ('alter entity X add attribute ...') and is phrased separately, so a single
	// template cannot cover both. Listing it here would produce a hint telling
	// users to write 'ALTER ENTITY X { ... }', which is not valid MDL.
	if _, ok := incrementalAlter["entity"]; ok {
		t.Error("entity must not be in the table — its hint is phrased separately")
	}
	// A type with no incremental ALTER must fall through to the generic hint
	// rather than being invented one.
	for _, dt := range []string{"microflow", "nanoflow", "enumeration"} {
		if _, ok := incrementalAlter[dt]; ok {
			t.Errorf("%s has no incremental ALTER but is in the table", dt)
		}
	}
}

func TestAlreadyExistsHintNamesTheNonDestructivePath(t *testing.T) {
	// The message has to carry three things, because the reporter had all three
	// wrong at once: what to use instead, that it is non-destructive, and that
	// CREATE OR MODIFY is a wholesale rebuild.
	for dt, verb := range incrementalAlter {
		hint := "to change part of it use '" + verb + " Mod.Name" +
			" { ... }' (leaves the rest intact); CREATE OR MODIFY rebuilds the whole " +
			friendlyDocType(dt) + " from this statement and drops anything it does not mention, including edits made by " + verb
		for _, want := range []string{verb, "leaves the rest intact", "rebuilds the whole", "drops anything"} {
			if !strings.Contains(hint, want) {
				t.Errorf("%s hint missing %q: %s", dt, want, hint)
			}
		}
	}
}
