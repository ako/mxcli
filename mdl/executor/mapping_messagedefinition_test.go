// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
)

// Issue #263: `with message definition` did not exist, so 74 of the 327 mapping
// documents in the demo apps were unauthorable — the second-largest gap after
// the array root. DESCRIBE dropped the source clause entirely AND its output
// parsed, so re-executing one rebuilt the mapping bound to nothing.
//
// The fixture carries three real message-definition collections and the mappings
// over them (see testdata/mapping-fixtures/README.md). These tests use them
// rather than synthesising a definition, because a definition is not authorable
// from MDL and a hand-built one would be a guess at the shape.

func newMessageDefEnv(t *testing.T) *testEnv {
	t.Helper()
	// modelsdk only: the legacy engine has no reader for the document and
	// refuses by design.
	env := setupTestEnvWithBackend(t, func() backend.FullBackend { return modelsdkbackend.New() })
	loadMappingFixture(t, env)
	return env
}

// TestMessageDefinitionMappingReproducesStudioPro authors a mapping over a real
// definition and checks it against the Studio Pro document for the same
// definition, which the fixture also carries.
func TestMessageDefinitionMappingReproducesStudioPro(t *testing.T) {
	env := newMessageDefEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create import mapping AgentCore.ZZ_Email_Import
  with message definition AgentCore.MesDef_Email.Email
{
  create AgentCore.Email {
    From = From,
    Content = Content,
    Subject = Subject
  }
};`); err != nil {
		t.Fatalf("authoring over a message definition failed (#263): %v", err)
	}

	// AgentCore.Email_Import is Studio Pro's own mapping over the same
	// definition. Same body, so the two documents must agree on everything the
	// definition determines.
	got := rootElement(t, env, "ImportMappings$ImportMapping", "ZZ_Email_Import")
	want := rootElement(t, env, "ImportMappings$ImportMapping", "Email_Import")

	for _, key := range []string{"Entity", "ExposedName", "JsonPath", "XmlPath",
		"ObjectHandling", "ObjectHandlingBackup", "ElementType"} {
		if g, w := lookupString(got, key), lookupString(want, key); g != w {
			t.Errorf("root %s = %q, Studio Pro has %q", key, g, w)
		}
	}

	gotKids, wantKids := childElements(got), childElements(want)
	if len(gotKids) != len(wantKids) {
		t.Fatalf("root has %d children, Studio Pro has %d", len(gotKids), len(wantKids))
	}
	for i := range gotKids {
		for _, key := range []string{"Attribute", "ExposedName", "JsonPath", "XmlPath"} {
			if g, w := lookupString(gotKids[i], key), lookupString(wantKids[i], key); g != w {
				t.Errorf("child %d %s = %q, Studio Pro has %q", i, key, g, w)
			}
		}
	}
}

// TestMessageDefinitionMappingBothPathFamilies pins the rule that makes this its
// own builder: a message-definition mapping stores an XmlPath built from the
// definition's exposed names alongside the JSON projection.
func TestMessageDefinitionMappingBothPathFamilies(t *testing.T) {
	env := newMessageDefEnv(t)
	defer env.teardown()

	if err := env.executeMDL(`create import mapping AgentCore.ZZ_Paths
  with message definition AgentCore.MesDef_Email.Email
{
  create AgentCore.Email { From = From }
};`); err != nil {
		t.Fatalf("create failed: %v", err)
	}

	root := rootElement(t, env, "ImportMappings$ImportMapping", "ZZ_Paths")
	// The root is unbounded, so it projects as an array on both families — and
	// its own name appears in the XML path but NOT in the JSON one.
	if got := lookupString(root, "JsonPath"); got != "(Array)|(Object)" {
		t.Errorf("root JsonPath = %q, want (Array)|(Object)", got)
	}
	if got := lookupString(root, "XmlPath"); got != "Emails|Email" {
		t.Errorf("root XmlPath = %q, want Emails|Email", got)
	}
	kids := childElements(root)
	if len(kids) != 1 {
		t.Fatalf("want 1 child, got %d", len(kids))
	}
	if got := lookupString(kids[0], "JsonPath"); got != "(Array)|(Object)|From" {
		t.Errorf("value JsonPath = %q", got)
	}
	if got := lookupString(kids[0], "XmlPath"); got != "Emails|Email|From" {
		t.Errorf("value XmlPath = %q", got)
	}
}

// TestMessageDefinitionRefusals: an unresolvable reference or an unexposed
// member is refused and says what would have worked, rather than being written
// through to fail later in mxbuild (#259).
func TestMessageDefinitionRefusals(t *testing.T) {
	env := newMessageDefEnv(t)
	defer env.teardown()

	cases := []struct {
		name, mdl, want string
	}{
		{
			name: "unknown definition",
			mdl: `create import mapping AgentCore.ZZ_Bad1
  with message definition AgentCore.MesDef_Email.NoSuchDefinition
{ create AgentCore.Email { From = From } };`,
			want: "not found",
		},
		{
			name: "two-part reference",
			mdl: `create import mapping AgentCore.ZZ_Bad2
  with message definition AgentCore.MesDef_Email
{ create AgentCore.Email { From = From } };`,
			want: "three parts",
		},
		{
			name: "member the definition does not expose",
			mdl: `create import mapping AgentCore.ZZ_Bad3
  with message definition AgentCore.MesDef_Email.Email
{ create AgentCore.Email { From = NoSuchMember } };`,
			want: "not exposed by the message definition",
		},
		{
			name: "entity disagrees with the definition",
			mdl: `create import mapping AgentCore.ZZ_Bad4
  with message definition AgentCore.MesDef_Email.Email
{ create MendixSSO.AppRole { From = From } };`,
			want: "exposes AgentCore.Email in the message definition",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := env.executeMDL(tc.mdl)
			if err == nil {
				t.Fatalf("accepted, want a refusal mentioning %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}
