// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// The mapping from a rename to the qualified name an anchor holds. An anchor is
// @Module[.Element[.Member]], so anything the anchor grammar cannot express is
// skipped rather than guessed at — writing a name no anchor could have held
// would corrupt entries instead of repairing them, and there is no signal when
// it happens.
func TestBrainRenameTargetOnlyClaimsWhatAnAnchorCanName(t *testing.T) {
	for _, tc := range []struct {
		objectType, qualified string
		want                  string
		ok                    bool
	}{
		{"ENTITY", "Sales.Order", "Sales.Order", true},
		{"MICROFLOW", "Sales.ACT_Post", "Sales.ACT_Post", true},
		{"NANOFLOW", "Sales.NF_Refresh", "Sales.NF_Refresh", true},
		{"PAGE", "Sales.Order_Overview", "Sales.Order_Overview", true},
		{"ENUMERATION", "Sales.ENUM_Status", "Sales.ENUM_Status", true},
		{"ASSOCIATION", "Sales.Order_Customer", "Sales.Order_Customer", true},
		{"CONSTANT", "Sales.ApiRoot", "Sales.ApiRoot", true},
		{"MODULE", "Sales", "Sales", true},

		// An element rename with no module cannot be turned into an anchor:
		// there is nothing to qualify it with, and rewriting on the bare name
		// would match every module's element of that name.
		{"ENTITY", "Order", "", false},
		// A type no anchor names. Skipped, not guessed.
		{"FOLDER", "Sales.Things", "", false},
	} {
		got, ok := brainRenameTarget(tc.objectType, tc.qualified)
		if ok != tc.ok || got != tc.want {
			t.Errorf("brainRenameTarget(%q, %q) = (%q, %v), want (%q, %v)",
				tc.objectType, tc.qualified, got, ok, tc.want, tc.ok)
		}
	}
}
