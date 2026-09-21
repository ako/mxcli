// SPDX-License-Identifier: Apache-2.0

package flowmutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// activity builds one Microflows$ActionActivity document. Key order matches
// what the codec writes, so a test that accidentally depends on order fails the
// same way a real document would.
func activity(caption string, disabled bool, action bson.D) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Microflows$ActionActivity"},
		{Key: "$ID", Value: caption + "-id"},
		{Key: "Action", Value: action},
		{Key: "Disabled", Value: disabled},
		{Key: "Caption", Value: caption},
	}
}

func logAction(level string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Microflows$LogMessageAction"},
		{Key: "Level", Value: level},
	}
}

func commitAction() bson.D {
	return bson.D{{Key: "$Type", Value: "Microflows$CommitAction"}}
}

// unit wraps activities the way a stored flow does: an ObjectCollection whose
// Objects array holds them, with a LOOP nesting a second collection. The nesting
// is the point — an activity inside a loop body is in the same document and must
// be reachable.
func unit(objects ...any) bson.D {
	return bson.D{
		{Key: "$Type", Value: "Microflows$Microflow"},
		{Key: "ObjectCollection", Value: bson.D{
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: bson.A(objects)},
		}},
	}
}

func knownTypes() map[string]bool {
	return map[string]bool{
		"Microflows$LogMessageAction": true,
		"Microflows$CommitAction":     true,
	}
}

func filter(conds ...types.ActivityCondition) types.ActivityFilter {
	return types.ActivityFilter{Conditions: conds}
}

func eq(col string, vals ...string) types.ActivityCondition {
	return types.ActivityCondition{Column: col, Values: vals}
}

// The reported headline: "disable all debug log statements".
func TestApplyDisablesDebugLogsOnly(t *testing.T) {
	doc := unit(
		activity("debug one", false, logAction("Debug")),
		activity("info one", false, logAction("Info")),
		activity("commit it", false, commitAction()),
		activity("debug two", false, logAction("Debug")),
	)
	m := NewMatch(filter(eq(types.ActivityColAction, "log"), eq(types.ActivityColLevel, "debug")), knownTypes())

	if got := Count(doc, m); got != 2 {
		t.Fatalf("Count = %d, want 2", got)
	}
	if got := Apply(doc, m, true); got != 2 {
		t.Fatalf("Apply changed %d, want 2", got)
	}

	want := map[string]bool{"debug one": true, "info one": false, "commit it": false, "debug two": true}
	EachActivity(doc, func(a bson.D) {
		cap := stringField(a, "Caption")
		if got := boolField(a, "Disabled"); got != want[cap] {
			t.Errorf("%q: Disabled = %v, want %v", cap, got, want[cap])
		}
	})

	// Applying the same thing again changes NOTHING. That is what makes the
	// executor able to say "Unchanged" rather than reporting a second success,
	// and what keeps a re-run from dirtying the .mpr.
	if got := Apply(doc, m, true); got != 0 {
		t.Errorf("second Apply changed %d, want 0 — Apply counts changes, not matches", got)
	}
	// Control: the count is not stuck at zero. Enabling them is a real change.
	if got := Apply(doc, m, false); got != 2 {
		t.Errorf("Apply(false) changed %d, want 2", got)
	}
}

// An activity nested in a loop body is in the same document and must be
// reachable. A walk over a maintained list of container keys would miss it, and
// the miss is silent — the statement reports success having skipped the body.
func TestApplyReachesNestedActivities(t *testing.T) {
	inner := bson.D{
		{Key: "$Type", Value: "Microflows$LoopedActivity"},
		{Key: "ObjectCollection", Value: bson.D{
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: bson.A{activity("inside the loop", false, logAction("Debug"))}},
		}},
	}
	doc := unit(activity("outside", false, logAction("Debug")), inner)
	m := NewMatch(filter(eq(types.ActivityColAction, "log")), knownTypes())

	if got := Count(doc, m); got != 2 {
		t.Fatalf("Count = %d, want 2 — the loop body was not walked", got)
	}
	if got := Apply(doc, m, true); got != 2 {
		t.Errorf("Apply changed %d, want 2", got)
	}
}

// An activity whose document has no Disabled key predates Mendix 9.12. It is
// left ALONE, never patched: adding a property the type does not have at that
// version is what makes a project Studio Pro cannot open.
func TestApplyNeverAddsTheDisabledKey(t *testing.T) {
	old := bson.D{
		{Key: "$Type", Value: "Microflows$ActionActivity"},
		{Key: "Action", Value: logAction("Debug")},
		{Key: "Caption", Value: "pre-9.12"},
	}
	doc := unit(old)
	m := NewMatch(filter(eq(types.ActivityColAction, "log")), knownTypes())

	if got := Count(doc, m); got != 1 {
		t.Fatalf("Count = %d, want 1 — the activity must still MATCH so it can be reported", got)
	}
	if got := Apply(doc, m, true); got != 0 {
		t.Errorf("Apply changed %d, want 0", got)
	}
	EachActivity(doc, func(a bson.D) {
		if HasDisabledKey(a) {
			t.Error("Apply added a Disabled key to a document that had none")
		}
	})
}

func TestMatchColumns(t *testing.T) {
	doc := unit(
		activity("Refresh entity", true, logAction("Error")),
		activity("Save the order", false, commitAction()),
	)
	kt := knownTypes()

	for _, tc := range []struct {
		name string
		f    types.ActivityFilter
		want int
	}{
		{"caption =", filter(eq(types.ActivityColCaption, "Save the order")), 1},
		{"caption like", filter(types.ActivityCondition{
			Column: types.ActivityColCaption, Like: true, Values: []string{"refresh%"}}), 1},
		{"caption like matching neither", filter(types.ActivityCondition{
			Column: types.ActivityColCaption, Like: true, Values: []string{"%nothing%"}}), 0},
		{"disabled = true", filter(eq(types.ActivityColDisabled, "true")), 1},
		{"disabled = false", filter(eq(types.ActivityColDisabled, "false")), 1},
		{"action != log", filter(types.ActivityCondition{
			Column: types.ActivityColAction, Negate: true, Values: []string{"log"}}), 1},
		{"level IN", filter(eq(types.ActivityColLevel, "error", "debug")), 1},
		{"action by storage label", filter(eq(types.ActivityColAction, "CommitAction")), 1},
		{"two conditions ANDed", filter(
			eq(types.ActivityColAction, "log"), eq(types.ActivityColDisabled, "true")), 1},
		{"two conditions that cannot both hold", filter(
			eq(types.ActivityColAction, "log"), eq(types.ActivityColDisabled, "false")), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Count(doc, NewMatch(tc.f, kt)); got != tc.want {
				t.Errorf("Count = %d, want %d", got, tc.want)
			}
		})
	}
}

// LIKE is a glob over a caption, which is prose. The iterative matcher must
// agree with SQL on the cases people write.
func TestLikeOne(t *testing.T) {
	for _, tc := range []struct {
		s, pattern string
		want       bool
	}{
		{"refresh entity", "refresh%", true},
		{"refresh entity", "%entity", true},
		{"refresh entity", "%fresh%", true},
		{"refresh entity", "refresh entity", true},
		{"refresh entity", "refresh_entity", true},
		{"refresh entity", "refresh", false},
		{"refresh entity", "%missing%", false},
		{"", "%", true},
		{"anything", "%%%%", true},
	} {
		if got := likeOne(tc.s, tc.pattern); got != tc.want {
			t.Errorf("likeOne(%q, %q) = %v, want %v", tc.s, tc.pattern, got, tc.want)
		}
	}
}

// A non-ActionActivity must never be touched, whatever the filter says. Mendix
// stores Disabled on that type and no other, so writing it onto a split or an
// end event would be inventing a property.
func TestApplyIgnoresNonActionActivities(t *testing.T) {
	split := bson.D{
		{Key: "$Type", Value: "Microflows$ExclusiveSplit"},
		{Key: "Caption", Value: "Is it?"},
		{Key: "Disabled", Value: false}, // not a real property; here as bait
	}
	doc := unit(split, activity("real one", false, logAction("Debug")))
	m := NewMatch(filter(eq(types.ActivityColCaption, "Is it?")), knownTypes())

	if got := Count(doc, m); got != 0 {
		t.Errorf("Count = %d, want 0 — a split is not an action activity", got)
	}
	if got := Apply(doc, m, true); got != 0 {
		t.Errorf("Apply changed %d on a split, want 0", got)
	}
	// Control: the same walk DOES reach the action activity next to it, so the
	// zero above is about the type and not about the walk being broken.
	m2 := NewMatch(filter(eq(types.ActivityColCaption, "real one")), knownTypes())
	if got := Apply(doc, m2, true); got != 1 {
		t.Errorf("Apply on the sibling action activity changed %d, want 1", got)
	}
}
