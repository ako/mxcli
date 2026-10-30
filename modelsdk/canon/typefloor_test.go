// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/version"
)

// The embedded data parses, and a broken file is not silently empty: a parse
// error would disable the type floor for every write (typeFloors falls back to
// no data rather than failing writes).
func TestTypeVersionsParse(t *testing.T) {
	d, err := parseTypeVersions(typeVersionsJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.introduced) < 500 {
		t.Errorf("only %d type introductions; the SDK 4.117.0 extraction has over 600", len(d.introduced))
	}
	for _, bad := range []string{
		`{"source":"mendixmodelsdk@1","types":[{"type":"NoDollar","introduced":"1.0.0"}]}`,
		`{"source":"mendixmodelsdk@1","types":[{"type":"A$B","introduced":"x"}]}`,
		`{"source":"mendixmodelsdk@1","types":[{"type":"A$B"},{"type":"A$B"}]}`,
		`{"source":"elsewhere","types":[{"type":"A$B","introduced":"1.0.0"}]}`,
	} {
		if _, err := parseTypeVersions([]byte(bad)); err == nil {
			t.Errorf("parsed %s", bad)
		}
	}
}

// The versions the lowerings hold as constants are the data's: each was read
// from the same SDK by hand (ako/mxcli#1163, #1771) before the data existed.
func TestTypeFloor_AgreesWithTheLowerings(t *testing.T) {
	for typ, want := range map[string]version.Version{
		"Workflows$EndOfParallelSplitPathActivity": endOfParallelSplitPathSince,
		"Workflows$SingleUserTaskActivity":         userTaskActivitySince,
		"Workflows$MultiUserTaskActivity":          userTaskActivitySince,
		"Workflows$EmptyUserSource":                emptyUserSourceSince,
		"Workflows$MajorityCompletionCriteria":     completionCriteriaSince["Workflows$MajorityCompletionCriteria"],
		"Workflows$ThresholdCompletionCriteria":    completionCriteriaSince["Workflows$ThresholdCompletionCriteria"],
		"Workflows$MicroflowCompletionCriteria":    completionCriteriaSince["Workflows$MicroflowCompletionCriteria"],
	} {
		got, ok := TypeIntroduced(typ)
		if !ok || got.Compare(want) != 0 {
			t.Errorf("%s: data says %v (%v), the lowering %v", typ, got, ok, want)
		}
	}
}

// The rows #1774 names, as the SDK has them.
func TestTypeFloor_IssueRows(t *testing.T) {
	for typ, want := range map[string]string{
		"Workflows$WaitForTimerActivity":                             "10.6.0",
		"Workflows$WaitForNotificationActivity":                      "10.0.0",
		"Workflows$WorkflowDefinitionNameSelection":                  "10.0.0",
		"Workflows$WorkflowDefinitionObjectSelection":                "10.0.0",
		"Workflows$TimerBoundaryEvent":                               "10.14.0",
		"Workflows$InterruptingTimerBoundaryEvent":                   "10.20.0",
		"Workflows$NonInterruptingTimerBoundaryEvent":                "10.20.0",
		"Workflows$NotificationActivity":                             "11.11.0",
		"Workflows$StartWorkflowActivity":                            "11.1.0",
		"Forms$FormCallArgument":                                     "", // Studio Pro 9.1.1 and 9.4.0 store it; only an alias of the 9.7 PageParameterMapping reaches it
		"Forms$ToggleDesignPropertyValue":                            "10.2.0",
		"Microflows$CreateChangeAction":                              "", // CreateObjectAction's storage name: always there
		"Workflows$CallMicroflowTask":                                "9.0.2",
		"Workflows$NonInterruptingTimerEventSubProcessStartActivity": "11.13.0",
	} {
		got, ok := TypeIntroduced(typ)
		switch {
		case want == "" && ok:
			t.Errorf("%s: introduced %v, want no floor", typ, got)
		case want != "" && (!ok || got.String() != want):
			t.Errorf("%s: introduced %v (%v), want %s", typ, got, ok, want)
		}
	}
}

// A type the SDK renamed keeps its storage name: the SDK deletes
// CallMicroflowTask in 11.9 and introduces CallMicroflowActivity, which the
// generated codec still stores as Workflows$CallMicroflowTask. The extraction
// keeps the stored name valid on both sides, and deletions are not enforced.
func TestTypeFloor_RenamedTypeKeepsItsStorageName(t *testing.T) {
	for _, v := range []string{"9.24.30", "10.24.24", "11.9.0", "11.15.0"} {
		if !TypeDeclared("Workflows$CallMicroflowTask", version.Parse(v)) {
			t.Errorf("%s: Workflows$CallMicroflowTask not declared", v)
		}
	}
	if d, ok := TypeDeleted("Workflows$CallMicroflowTask"); ok {
		t.Errorf("Workflows$CallMicroflowTask carries a deletion (%v) though CallMicroflowActivity is stored under it", d)
	}
	doc, _ := bson.Marshal(bson.D{{Key: "$Type", Value: "Workflows$UserTask"}})
	if err := UndeclaredTypeError(doc, ver("11.13.0")); err != nil {
		t.Errorf("a deletion was enforced: %v", err)
	}
}

func ver(s string) *version.Version { v := version.Parse(s); return &v }

// The walk sees elements in documents and in lists, at any depth, and names
// each refused type once.
func TestUndeclaredTypeError_WalksTheWholeUnit(t *testing.T) {
	wait := bson.D{{Key: "$Type", Value: "Workflows$WaitForTimerActivity"}}
	doc, _ := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Workflows$Workflow"},
		{Key: "Flow", Value: bson.D{{Key: "$Type", Value: "Workflows$Flow"},
			{Key: "Activities", Value: bson.A{int32(3), wait, bson.D{
				{Key: "$Type", Value: "Workflows$ParallelSplitActivity"},
				{Key: "Outcomes", Value: bson.A{int32(3), bson.D{{Key: "$Type", Value: "Workflows$ParallelSplitOutcome"},
					{Key: "Flow", Value: bson.D{{Key: "$Type", Value: "Workflows$Flow"},
						{Key: "Activities", Value: bson.A{int32(3), wait,
							bson.D{{Key: "$Type", Value: "Workflows$WaitForNotificationActivity"}}}}}}}}}}}}}},
	})
	err := UndeclaredTypeError(doc, ver("9.24.30"))
	if err == nil {
		t.Fatal("not refused")
	}
	msg := err.Error()
	for _, want := range []string{"this Mendix 9.24.30 project cannot store",
		"Workflows$WaitForNotificationActivity (Mendix 10.0.0+); Workflows$WaitForTimerActivity (Mendix 10.6.0+)",
		"the types did not exist yet"} {
		if !strings.Contains(msg, want) {
			t.Errorf("%q does not say %q", msg, want)
		}
	}
	if strings.Count(msg, "WaitForTimerActivity") != 1 {
		t.Errorf("a type held twice is named twice: %s", msg)
	}
	if err := UndeclaredTypeError(doc, ver("10.6.0")); err != nil {
		t.Errorf("control 10.6.0: %v", err)
	}
	if err := UndeclaredTypeError(doc, nil); err != nil {
		t.Errorf("no version: %v", err)
	}
}
