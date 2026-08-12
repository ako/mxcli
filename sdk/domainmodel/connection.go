// SPDX-License-Identifier: Apache-2.0

package domainmodel

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/model"
)

// An association's line anchors — where the connector attaches to the FROM
// (ParentConnection) and TO (ChildConnection) entity boxes in the domain model
// editor. Mendix stores each as the string "x;y", a PERCENTAGE of the entity box
// in 0..100 (measured across 88 pairs in four Studio-Pro-authored sources; and
// it cannot be pixels, because DomainModels$EntityImpl stores no size — the box
// is sized by the editor from the name and attribute list).
//
// These are the values mxcli writes for a NEW association. They are not
// Mendix's: a blank 11.13 app's own `Administration.AccountPasswordData_Account`
// stores 0;54 / 100;54. Both engines hardcoded these two strings on every write,
// so any association write — including a documentation-only
// `alter association … set comment` — silently discarded whatever the developer
// had dragged the line to in Studio Pro. Read the stored value and write it back
// (guard-don't-drop, ADR-0005); these apply only when there is nothing stored.
//
// DomainModels$CrossAssociation has no connection properties at all, and writing
// them there crashes Studio Pro (issue #50) — so a cross-module association has
// no anchors to preserve. (issue #872)
const (
	DefaultParentConnection = "0;50"
	DefaultChildConnection  = "100;50"
)

// ParseConnectionPoint reads a stored "x;y" anchor. Returns nil when the value
// is absent or not two integers, so an unreadable anchor falls back to the
// default rather than being written back as 0;0 (a legitimate anchor, which is
// why the field is a pointer: the zero Point is a real position, not "unset").
//
// Both components are integers by necessity, not by convention: Mendix's loader
// rejects a non-integer component outright — a hand-patched "0.5;50" fails with
// StorageLoadException "One or more invalid values were detected while loading
// the project", verified on 11.13.0. It does NOT range-check, so negatives and
// values past 100 load fine and must round-trip untouched.
func ParseConnectionPoint(s string) *model.Point {
	x, y, ok := strings.Cut(s, ";")
	if !ok {
		return nil
	}
	xi, err := strconv.Atoi(strings.TrimSpace(x))
	if err != nil {
		return nil
	}
	yi, err := strconv.Atoi(strings.TrimSpace(y))
	if err != nil {
		return nil
	}
	return &model.Point{X: xi, Y: yi}
}

// FormatConnectionPoint renders an anchor back into Mendix's "x;y" form,
// falling back to the given default when nothing was read.
func FormatConnectionPoint(p *model.Point, fallback string) string {
	if p == nil {
		return fallback
	}
	return fmt.Sprintf("%d;%d", p.X, p.Y)
}
