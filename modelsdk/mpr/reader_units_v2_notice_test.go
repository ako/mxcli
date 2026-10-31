// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"strings"
	"testing"
)

// ako/mxcli#1806: the first command after an upgrade (or on a fresh clone)
// builds the unit index by opening every unit file — 641 opens where the
// previous build opened 383, before the steady-state win. On Windows, with an
// open costing about a millisecond, that is an unexplained stall at the moment
// a user is checking whether the new version works. Say it is happening.
func TestBuildingTheUnitIndexFromNothingIsAnnounced(t *testing.T) {
	const n = 40
	p, _ := seedProject(t, n)

	var notice bytes.Buffer
	prev := ColdUnitIndexNotice
	ColdUnitIndexNotice = &notice
	t.Cleanup(func() { ColdUnitIndexNotice = prev })

	listModules(t, p.reader())
	got := notice.String()
	if !strings.Contains(got, "unit index") || !strings.Contains(got, "41 units") {
		t.Errorf("cold build: notice = %q, want one line naming the unit index and the 41 units", got)
	}
	if strings.Count(got, "\n") != 1 {
		t.Errorf("cold build: notice printed %d lines, want 1: %q", strings.Count(got, "\n"), got)
	}

	// Control: the next process finds the index and says nothing.
	notice.Reset()
	listModules(t, p.reader())
	if notice.Len() != 0 {
		t.Errorf("warm build announced an index build: %q", notice.String())
	}
}

// A library caller that did not ask for the notice gets none.
func TestUnitIndexNoticeIsSilentByDefault(t *testing.T) {
	if ColdUnitIndexNotice != nil {
		t.Fatalf("ColdUnitIndexNotice defaults to %v, want nil", ColdUnitIndexNotice)
	}
}
