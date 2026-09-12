// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"strings"
	"testing"
	"time"
)

// A requirement's anchors point FORWARD — not resolving means "not built yet",
// which is what makes `brain plan` a progress report derived from the model
// rather than a status column somebody has to remember to update. A module
// anchor breaks that: a module resolves the instant it exists, so the
// requirement reports BUILT before any of its work is done.
//
// Measured on ako/ChipCoV1: two requirements anchored at @Maintenance (the
// theme, and the device profiles) both read as built after slice 01 with none
// of their work started.
func TestRequirementRefusesAModuleAnchor(t *testing.T) {
	_, err := NewRequirement("Technicians can close a work order",
		[]string{"@Maintenance"}, "05-workflow", time.Now())
	if err == nil {
		t.Fatal("a requirement anchored at a bare module must be refused")
	}
	// The message has to say what to do instead, or the author just deletes the
	// anchor — which loses the progress signal entirely rather than fixing it.
	for _, want := range []string{"@Maintenance", "resolves as soon as the module exists", "Anchor at a document"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message missing %q: %v", want, err)
		}
	}
}

// The control, in both directions: a document anchor is accepted on a
// requirement, and a module anchor stays legal on a DECISION and on a QUESTION,
// whose anchors point backward — "this module exists" is exactly what a
// cross-cutting decision about it asserts. Without this the fix could be
// "refuse every module anchor" and the test above would still pass.
func TestModuleAnchorStaysLegalOnADecisionAndAQuestion(t *testing.T) {
	now := time.Now()
	if _, err := NewRequirement("Coordinator can assign work",
		[]string{"@Maintenance.ACT_AssignWork"}, "05-workflow", now); err != nil {
		t.Errorf("a document anchor must be accepted on a requirement: %v", err)
	}
	if _, err := NewEntry("Maintenance owns all scheduling", []string{"@Maintenance"}, now); err != nil {
		t.Errorf("a module anchor must stay legal on a decision: %v", err)
	}
	if _, err := NewQuestion("Should Maintenance own scheduling?", []string{"@Maintenance"}, "", now); err != nil {
		t.Errorf("a module anchor must stay legal on a question: %v", err)
	}
}

// A requirement with a mix is refused on the module one, and must name THAT
// anchor — blaming the first in the list sends the author to edit a good one.
func TestRequirementNamesTheOffendingAnchor(t *testing.T) {
	_, err := NewRequirement("Roles are scoped",
		[]string{"@Maintenance.Home_Web", "@Sales"}, "01-accounts", time.Now())
	if err == nil {
		t.Fatal("expected refusal")
	}
	if !strings.Contains(err.Error(), "@Sales") {
		t.Errorf("must name the module anchor, got: %v", err)
	}
	if strings.Contains(err.Error(), "Home_Web") {
		t.Errorf("must not blame the document anchor, got: %v", err)
	}
}
