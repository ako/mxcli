// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// widget29 parses a script and returns its MDL-WIDGET29 violations.
//
// Through visitor.Build rather than hand-built widgets, because half of what is
// being asserted is that a region's property list reaches the rule at all: MDL
// accepts any `key: value` on a region, which is exactly how a mistyped
// ToggleMode used to pass check, pass exec and vanish.
func widget29(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parsing %q: %v", src, errs)
	}
	var out []linter.Violation
	for _, v := range ValidateRegionToggleMode(prog) {
		if v.RuleID == "MDL-WIDGET29" {
			out = append(out, v)
		}
	}
	return out
}

func layoutSrc(regionProps string) string {
	return fmt.Sprintf(`create layout M.App_Default (layouttype: 'Responsive') {
  scrollcontainer layoutContainer {
    region left (%s) {
      navigationtree navMenu (Profile: 'Responsive')
    }
    region center (Class: 'region-content') {
      placeholder Main
    }
  }
};`, regionProps)
}

// Studio Pro's dropdown shows captions, so this is the value a person reading
// the editor would type. Before the rule it was accepted, dropped on load, and
// the layout rendered with no toggle behaviour at 0 build errors.
func TestRegionToggleMode_StudioProCaptionIsReported(t *testing.T) {
	got := widget29(t, layoutSrc(`Size: 232, SizeMode: 'Pixels', ToggleMode: 'Shrink content (initially closed)'`))
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
	if got[0].Severity != linter.SeverityError {
		t.Errorf("severity = %v, want error", got[0].Severity)
	}
	for _, want := range []string{"layout M.App_Default", "region", "left", "Shrink content (initially closed)"} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("message %q does not mention %q", got[0].Message, want)
		}
	}
	if !strings.Contains(got[0].Suggestion, "ShrinkContentInitiallyClosed") {
		t.Errorf("suggestion %q does not name the member to use", got[0].Suggestion)
	}
}

// The control for the whole rule: every member Mendix stores must pass. A rule
// that also flagged a valid mode would push authors back to writing nothing,
// which is the state this change exists to leave.
func TestRegionToggleMode_EveryMemberIsClean(t *testing.T) {
	for _, mode := range pages.ScrollContainerToggleModes {
		t.Run(mode, func(t *testing.T) {
			if got := widget29(t, layoutSrc(fmt.Sprintf("ToggleMode: '%s'", mode))); len(got) != 0 {
				t.Errorf("member %q reported: %+v", mode, got)
			}
		})
	}
}

func TestRegionToggleMode_CaseIsNotTheFault(t *testing.T) {
	if got := widget29(t, layoutSrc(`ToggleMode: 'shrinkcontentinitiallyclosed'`)); len(got) != 0 {
		t.Errorf("a lowercase member was reported: %+v", got)
	}
}

// A region that says nothing about toggling is the ordinary case and must stay
// silent — the rule is about a value that cannot mean anything, not about the
// property being absent.
func TestRegionToggleMode_AbsentPropertyIsSilent(t *testing.T) {
	if got := widget29(t, layoutSrc(`Size: 232, SizeMode: 'Pixels', Class: 'region-sidebar'`)); len(got) != 0 {
		t.Errorf("a region without a toggle mode was reported: %+v", got)
	}
}

// Pages carry scroll containers too, and the rule rides forEachWidget rather
// than the layout statement, so this is the case that would break if the walk
// were narrowed to CREATE LAYOUT.
func TestRegionToggleMode_ReportedOnAPageToo(t *testing.T) {
	got := widget29(t, `create page M.Home (title: 'Home') {
  scrollcontainer sc {
    region left (ToggleMode: 'SlideOver') {
      text lbl (Content: 'x')
    }
    region center (Class: 'region-content') {
      text lbl2 (Content: 'y')
    }
  }
};`)
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "page M.Home") {
		t.Errorf("message %q does not say which document", got[0].Message)
	}
}
