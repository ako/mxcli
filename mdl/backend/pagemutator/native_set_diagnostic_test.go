// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1135, route 1, as amended by ako/mxcli#515.
//
// `alter page … set 'Remove empty text' = off on lvThings` dead-ended with
//
//	property "Remove empty text" not found (widget has no pluggable Object)
//
// "pluggable Object" is not a thing the author wrote and cannot be made true by
// editing the script. #1135 replaced it with a message naming ALTER STYLING as
// the route that did work.
//
// #515 then taught `set` to write design properties itself, so naming a second
// statement became stale advice. Reaching this error now means the key is
// NEITHER a first-class property NOR a design property the theme declares for
// this widget's type — a genuinely unknown key — so the message says that and
// points at the command that lists the ones it does declare.
//
// This test was asserting the ALTER STYLING wording. Inverted rather than
// deleted: the half that still holds is that the message must not describe
// mxcli's internals, and must leave the reader somewhere to go.
func TestSetWidgetProperty_NativeWidgetErrorIsActionable(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("lvThings"))
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	err := m.SetWidgetProperty("lvThings", "Remove empty text", false)
	if err == nil {
		t.Fatal("setting an unknown property on a built-in widget must still fail")
	}
	msg := err.Error()
	if strings.Contains(msg, "pluggable Object") {
		t.Errorf("the message still describes mxcli's internals: %q", msg)
	}
	// Sending the reader to ALTER STYLING is now stale: `set` writes design
	// properties, so the two no longer differ.
	if strings.Contains(msg, "alter styling") {
		t.Errorf("the message still names a second statement for a route `set` now has: %q", msg)
	}
	if !strings.Contains(msg, "show design properties") {
		t.Errorf("the message leaves the reader nowhere to go: %q", msg)
	}
	if !strings.Contains(msg, "Remove empty text") {
		t.Errorf("the message does not name the property: %q", msg)
	}
}

// The control: a widget that IS pluggable must keep the error that names its own
// vocabulary. Redirecting a mistyped pluggable key to ALTER STYLING would send
// the author to a command that cannot write it either.
func TestSetWidgetProperty_PluggableWidgetKeepsItsOwnError(t *testing.T) {
	rawData := makeRawPage(makePluggableWidget("dg1", "pageSize", "10"))
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	err := m.SetWidgetProperty("dg1", "NoSuchProperty", 1)
	if err == nil {
		t.Fatal("an unknown pluggable property must fail")
	}
	if strings.Contains(err.Error(), "alter styling") {
		t.Errorf("a pluggable property was redirected to ALTER STYLING: %q", err.Error())
	}
}
