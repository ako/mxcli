// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

// storedAuthService is a service as Studio Pro stores it, with the given
// authentication. The methods are ako/TestApp's (mendixlabs/mxcli#1331).
func storedAuthService(types []string, mf string) *model.PublishedRestService {
	s := &model.PublishedRestService{
		Name: "Orders", Path: "rest/orders/v1", Version: "1.0.0",
		AuthenticationTypes: types, AuthenticationMicroflow: mf,
		Resources: []*model.PublishedRestResource{{
			Name:       "orders",
			Operations: []*model.PublishedRestOperation{{HTTPMethod: "GET", Path: "status", Microflow: "RestQ.GetStatus"}},
		}},
	}
	s.ID = nextID("prs")
	return s
}

func authOf(s *model.PublishedRestService) string {
	if s == nil {
		return "<not written>"
	}
	return strings.Join(s.AuthenticationTypes, ",") + " / " + s.AuthenticationMicroflow
}

func TestCreatePublishedRestService_WritesAuthentication(t *testing.T) {
	f, ctx, _ := newPublishedRestFixture(t)
	assertNoError(t, execPublishedRest(t, ctx, `create published rest service RestQ.Orders (
  Path: 'rest/orders/v1',
  Authentication: (microflow RestQ.Authenticate, session)
) {
  resource 'orders' { get 'status' microflow RestQ.GetStatus; }
};`))
	if got, want := authOf(f.created), "Microflow,Session / RestQ.Authenticate"; got != want {
		t.Errorf("authentication = %q, want %q", got, want)
	}
}

// A statement that does not state authentication keeps the stored setting.
// Before #1331 the writer carried it; now the executor must, or every
// `create or modify` and `alter` of a Studio Pro service would switch its
// authentication off (the ako/mxcli#571 regression).
func TestPublishedRestService_UnstatedAuthenticationIsKept(t *testing.T) {
	for name, src := range map[string]string{
		"create or modify": `create or modify published rest service RestQ.Orders (Path: 'rest/orders/v2') {
  resource 'orders' { get 'status' microflow RestQ.GetStatus; }
};`,
		"alter set list":   `alter published rest service RestQ.Orders set (Version: '2.0.0');`,
		"alter set assign": `alter published rest service RestQ.Orders set Version = '2.0.0';`,
	} {
		t.Run(name, func(t *testing.T) {
			stored := storedAuthService([]string{"Basic", "Session", "Microflow"}, "RestQ.Authenticate")
			f, ctx, _ := newPublishedRestFixture(t, stored)
			assertNoError(t, execPublishedRest(t, ctx, src))
			if got, want := authOf(f.updated), "Basic,Session,Microflow / RestQ.Authenticate"; got != want {
				t.Errorf("authentication = %q, want %q (the stored setting)", got, want)
			}
		})
	}
}

// Stating it replaces methods and microflow together: a method list without
// `microflow` clears the microflow, as Studio Pro does when Custom is unticked.
func TestPublishedRestService_StatedAuthenticationReplaces(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"create or modify none", `create or modify published rest service RestQ.Orders (Path: 'rest/orders/v1', Authentication: none) {
  resource 'orders' { get 'status' microflow RestQ.GetStatus; }
};`, " / "},
		{"alter custom off", `alter published rest service RestQ.Orders set (Authentication: (basic, session));`, "Basic,Session / "},
		{"alter none", `alter published rest service RestQ.Orders set (Authentication: none);`, " / "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stored := storedAuthService([]string{"Basic", "Session", "Microflow"}, "RestQ.Authenticate")
			f, ctx, _ := newPublishedRestFixture(t, stored)
			assertNoError(t, execPublishedRest(t, ctx, c.src))
			if got := authOf(f.updated); got != c.want {
				t.Errorf("authentication = %q, want %q", got, c.want)
			}
		})
	}
}

// describe prints the stored methods in the stored order, and executing its
// output writes the same setting back — for every state measured on TestApp.
// The control is the order: TestApp's MicroflowSession service stores
// Microflow before Session, so a describe that sorted would fail here.
func TestDescribePublishedRestService_AuthenticationRoundTrips(t *testing.T) {
	cases := []struct {
		name  string
		types []string
		mf    string
		line  string
	}{
		{"all three", []string{"Basic", "Session", "Microflow"}, "RestQ.Authenticate", "Authentication: (basic, session, microflow RestQ.Authenticate)"},
		{"microflow first", []string{"Microflow", "Session"}, "RestQ.Authenticate", "Authentication: (microflow RestQ.Authenticate, session)"},
		{"custom off", []string{"Basic", "Session"}, "", "Authentication: (basic, session)"},
		{"none", nil, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stored := storedAuthService(c.types, c.mf)
			f, ctx, out := newPublishedRestFixture(t, stored)
			assertNoError(t, describePublishedRestService(ctx, ast.QualifiedName{Module: "RestQ", Name: "Orders"}))
			text := out.String()
			if c.line != "" && !strings.Contains(text, c.line) {
				t.Fatalf("describe lacks %q:\n%s", c.line, text)
			}
			if c.line == "" && strings.Contains(text, "Authentication") {
				t.Fatalf("describe prints the default (none):\n%s", text)
			}
			// Replay onto a service whose stored setting differs, so a dropped
			// or reordered property cannot pass by carrying the old value.
			other := storedAuthService([]string{"Session"}, "")
			other.ContainerID = f.mod.ID
			f.stored = []*model.PublishedRestService{other}
			stmt := text[:strings.Index(text, "};")+2]
			assertNoError(t, execPublishedRest(t, ctx, stmt))
			if c.line == "" {
				// none is the creation default and is not printed, so replaying
				// it keeps whatever is stored — the documented meaning of an
				// unstated property.
				return
			}
			if !reflect.DeepEqual(f.updated.AuthenticationTypes, c.types) || f.updated.AuthenticationMicroflow != c.mf {
				t.Errorf("replayed as %q, want %v / %q", authOf(f.updated), c.types, c.mf)
			}
		})
	}
}

// A stored value MDL cannot spell is not printed as the property, so replaying
// the output keeps it rather than writing something else.
func TestDescribePublishedRestService_UnspellableAuthenticationIsAComment(t *testing.T) {
	for name, st := range map[string]*model.PublishedRestService{
		"guest":                storedAuthService([]string{"Guest"}, ""),
		"microflow, no method": storedAuthService([]string{"Basic"}, "RestQ.Authenticate"),
		"method, no microflow": storedAuthService([]string{"Microflow"}, ""),
	} {
		t.Run(name, func(t *testing.T) {
			_, ctx, out := newPublishedRestFixture(t, st)
			assertNoError(t, describePublishedRestService(ctx, ast.QualifiedName{Module: "RestQ", Name: "Orders"}))
			text := out.String()
			if strings.Contains(text, "  Authentication:") {
				t.Errorf("printed an unspellable setting as the property:\n%s", text)
			}
			if !strings.Contains(text, "-- Authentication") {
				t.Errorf("no comment for the stored setting:\n%s", text)
			}
		})
	}
}

func TestAlterPublishedRestService_SetListRefusesFolder(t *testing.T) {
	_, ctx, _ := newPublishedRestFixture(t, storedAuthService(nil, ""))
	err := execPublishedRest(t, ctx, `alter published rest service RestQ.Orders set (Folder: 'Apis');`)
	if err == nil || !strings.Contains(err.Error(), "Folder") {
		t.Fatalf("err = %v, want a refusal naming Folder", err)
	}
}

// The custom-authentication microflow must be one Mendix accepts, measured with
// mx check on 11.14.0: it returns System.User (CE0334), and every parameter is
// one Mendix supplies, System.HttpRequest or System.HttpResponse, matched by
// type and not by name (CE0336). No parameters at all is accepted.
func TestPublishedRestService_AuthenticationMicroflowSignature(t *testing.T) {
	cases := []struct{ mf, want string }{
		{"RestQ.Authenticate", ""},
		{"RestQ.AuthenticateBoth", ""},
		{"RestQ.AuthenticateNoParams", ""},
		{"RestQ.AuthenticateBool", "CE0334"},
		{"RestQ.AuthenticateToken", "CE0336"},
		{"RestQ.NoSuchMicroflow", "not found"},
	}
	for _, c := range cases {
		for name, src := range map[string]string{
			"create": `create published rest service RestQ.Orders (Path: 'rest/orders/v1', Authentication: (microflow ` + c.mf + `)) {
  resource 'orders' { get 'status' microflow RestQ.GetStatus; }
};`,
			"alter": `alter published rest service RestQ.Orders set (Authentication: (session, microflow ` + c.mf + `));`,
		} {
			t.Run(c.mf+"/"+name, func(t *testing.T) {
				var stored []*model.PublishedRestService
				if name == "alter" {
					stored = append(stored, storedAuthService(nil, ""))
				}
				f, ctx, _ := newPublishedRestFixture(t, stored...)
				err := execPublishedRest(t, ctx, src)
				if c.want == "" {
					assertNoError(t, err)
					return
				}
				if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), c.mf) {
					t.Fatalf("err = %v, want a refusal naming %s and %s", err, c.mf, c.want)
				}
				if f.created != nil || f.updated != nil {
					t.Errorf("wrote the service despite the refusal")
				}
			})
		}
	}
}

// check --references predicts exec's MDL-REST04: a missing microflow and a
// project microflow Mendix would reject are reported before anything runs. A
// microflow the script itself creates is known by name only, so its signature
// is left to exec.
func TestReferences_PublishedRestAuthenticationMicroflow(t *testing.T) {
	cases := []struct{ src, want string }{
		{`create published rest service RestQ.Orders (Path: 'p', Authentication: (microflow RestQ.Authenticate)) { resource 'r' { get '' microflow RestQ.GetStatus; } };`, ""},
		{`create published rest service RestQ.Orders (Path: 'p', Authentication: (microflow RestQ.Nope)) { resource 'r' { get '' microflow RestQ.GetStatus; } };`, "not found"},
		{`create published rest service RestQ.Orders (Path: 'p', Authentication: (microflow RestQ.AuthenticateBool)) { resource 'r' { get '' microflow RestQ.GetStatus; } };`, "CE0334"},
		{`alter published rest service RestQ.Orders set (Authentication: (microflow RestQ.AuthenticateToken));`, "CE0336"},
		{`create microflow RestQ.Later () returns System.User begin return empty; end;
create published rest service RestQ.Orders (Path: 'p', Authentication: (microflow RestQ.Later)) { resource 'r' { get '' microflow RestQ.GetStatus; } };`, ""},
	}
	for _, c := range cases {
		t.Run(c.want+"/"+c.src[:30], func(t *testing.T) {
			_, ctx, _ := newPublishedRestFixture(t, storedAuthService(nil, ""))
			errs := enumErrors(validateProgram(ctx, parseMDL(t, c.src)))
			if c.want == "" {
				if strings.Contains(errs, "MDL-REST04") {
					t.Fatalf("unexpected: %s", errs)
				}
				return
			}
			if !strings.Contains(errs, "MDL-REST04") || !strings.Contains(errs, c.want) {
				t.Fatalf("errors %q, want MDL-REST04 with %q", errs, c.want)
			}
		})
	}
}
