// SPDX-License-Identifier: Apache-2.0

package evalrunner

import "testing"

// `SHOW ENTITIES` as the renderer actually emits it — a pipe-delimited markdown
// table (mdl/executor/format.go, writeResultTable). The fixture is in that
// shape on purpose: a space-delimited one passes these tests while telling us
// nothing about the output the check is given in a real run.
//
// The Type column is what distinguishes a view entity from a persistent one,
// and a brief that asks for a view entity is unscored without it — the agent
// can satisfy entity_exists with an ordinary entity of the same name.
const showEntities = `| Entity                     | Type           | Attrs | Assocs | Validations | Indexes | Events | AccessRules |
|----------------------------|----------------|-------|--------|-------------|---------|--------|-------------|
| Service.Fab                | Persistent     | 3     | 0      | 0           | 0       | 0      | 1           |
| Service.OpenRequestsPerFab | View           | 3     | 1      | 0           | 0       | 0      | 1           |
| Service.RequestView        | Persistent     | 2     | 0      | 0           | 0       | 0      | 0           |
| Service.Scratch            | Non-Persistent | 1     | 0      | 0           | 0       | 0      | 0           |

(4 entities)`

func TestCheckEntityHasType(t *testing.T) {
	cases := []struct {
		name string
		args string
		want bool
		why  string
	}{
		{"a view entity is a View", "Service.OpenRequestsPerFab View", true, ""},
		{"a persistent entity is Persistent", "Service.Fab Persistent", true, ""},
		{"a hyphenated type is one field", "Service.Scratch Non-Persistent", true, ""},
		{"case does not matter", "Service.Fab persistent", true, ""},
		{"wildcards resolve like every other check", "*.OpenRequestsPerFab View", true, ""},

		// The control the check exists for: without the Type column these two
		// would pass, which is precisely the hole in scoring a view entity.
		{"a persistent entity is not a View", "Service.Fab View", false,
			"a brief asking for a view entity must fail when an ordinary one was built"},
		{"a name ending in View is not a View", "Service.RequestView View", false,
			"the Type column decides, not the entity's name"},
		{"a view entity is not Persistent", "Service.OpenRequestsPerFab Persistent", false, ""},

		{"an absent entity fails", "Service.Nope View", false, ""},
		{"a missing type is rejected", "Service.Fab", false, ""},
		{"a third field is rejected", "Service.Fab View extra", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runCheck(Check{Type: "entity_has_type", Args: c.args},
				CheckOptions{}, showEntities, "", "", "", map[string]string{})
			if got.Passed != c.want {
				t.Errorf("entity_has_type %q = %v (%s), want %v. %s",
					c.args, got.Passed, got.Detail, c.want, c.why)
			}
		})
	}
}

// A failing entity_has_type must say which of the two things went wrong: the
// entity is missing, or it is there and is the wrong kind. The second is the
// interesting one and reads as a pass in every other check kind.
func TestCheckEntityHasTypeDistinguishesAbsentFromMistyped(t *testing.T) {
	absent := runCheck(Check{Type: "entity_has_type", Args: "Service.Nope View"},
		CheckOptions{}, showEntities, "", "", "", map[string]string{})
	mistyped := runCheck(Check{Type: "entity_has_type", Args: "Service.Fab View"},
		CheckOptions{}, showEntities, "", "", "", map[string]string{})
	if absent.Detail == mistyped.Detail {
		t.Fatalf("both failures read %q; the mistyped case must be distinguishable", absent.Detail)
	}
	if !contains(mistyped.Detail, "Service.Fab") {
		t.Errorf("the mistyped detail %q should name the entity it did find", mistyped.Detail)
	}
}

// workflow_exists reads `list workflows`. The cache is the seam: a seeded entry
// means the check is graded without running the binary, so this asserts the
// wiring (right listing, right matcher) rather than the backend.
func TestCheckWorkflowExists(t *testing.T) {
	cache := map[string]string{
		"list:list workflows": `| Qualified Name            | Activities | User Tasks | Decisions | Parameter Entity       |
|---------------------------|------------|------------|-----------|------------------------|
| Service.WF_ServiceRequest | 4          | 1          | 1         | Service.ServiceRequest |
| Service.WF_Onboarding     | 2          | 1          | 0         | Service.Engineer       |

(2 workflows)`,
	}
	for _, c := range []struct {
		args string
		want bool
	}{
		{"Service.WF_ServiceRequest", true},
		{"*.WF_Onboarding", true},
		{"Service.WF_Missing", false},
	} {
		got := runCheck(Check{Type: "workflow_exists", Args: c.args},
			CheckOptions{}, "", "", "", "", cache)
		if got.Passed != c.want {
			t.Errorf("workflow_exists %q = %v (%s), want %v", c.args, got.Passed, got.Detail, c.want)
		}
	}
}

// A workflow is not a microflow: before workflow_exists existed, a brief whose
// centrepiece was a workflow could only be scored by mx_check_passes, and
// microflow_exists over `SHOW MICROFLOWS` does not see one.
func TestWorkflowIsNotFoundByMicroflowExists(t *testing.T) {
	microflows := `| Qualified Name                  | Parameters | Activities |
|---------------------------------|------------|------------|
| Service.ACT_Request_Assign      | 1          | 4          |
| Service.SUB_WorkOrder_TotalCost | 1          | 3          |

(2 microflows)`
	got := runCheck(Check{Type: "microflow_exists", Args: "Service.WF_ServiceRequest"},
		CheckOptions{}, "", "", microflows, "", map[string]string{})
	if got.Passed {
		t.Fatal("microflow_exists matched a workflow; the two listings must stay distinct")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
