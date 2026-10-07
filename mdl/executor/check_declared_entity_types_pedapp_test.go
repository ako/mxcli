// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// A declared type — a flow's parameter or return type, a page's or snippet's
// parameter — that names an entity the project does not have passed
// `check --references`, for System and user modules alike:
//
//	create microflow M.F ($p: System.Nope) begin end;         -> Check passed
//	create microflow M.F ($p: M.Nope) begin end;              -> Check passed
//	create page M.P (params: ($x: System.Nope), …) { … };     -> Check passed
//
// Measured on Evora Factory Management (10.24.15). exec refuses the user-module
// shapes and every page parameter ("entity … not found") after the statements
// before it are written; a System one in a flow signature it writes by name,
// leaving a reference nothing resolves. The body of a flow (retrieve, create),
// an association's endpoints and an EXTENDS target were already resolved — the
// signature was the one place a type name went unchecked.
//
// Run on the real path: the parsed script through ValidateProgram and the
// executor's reference check, against the Studio Pro-authored PedApp fixture,
// whose System module is the virtual one the modelsdk backend builds.
func TestCheckDeclaredEntityTypes_UnknownEntityIsRefused(t *testing.T) {
	cases := []struct {
		name, src, missing string
	}{
		{"association FROM System.Nope (the reported shape)",
			"create association ModT.Nope_Thing from System.Nope to ModT.Thing;", "System.Nope"},
		{"microflow parameter, System",
			"create microflow ModT.F1 ($p: System.Nope) begin end;", "System.Nope"},
		{"microflow parameter, user module",
			"create microflow ModT.F1 ($p: ModT.Nope) begin end;", "ModT.Nope"},
		{"microflow list parameter",
			"create microflow ModT.F1 ($p: list of System.Nope) begin end;", "System.Nope"},
		{"microflow return type",
			"create microflow ModT.F1 () returns System.Nope begin return empty; end;", "System.Nope"},
		{"nanoflow parameter",
			"create nanoflow ModT.N1 ($p: System.Nope) begin end;", "System.Nope"},
		{"page parameter",
			"create page ModT.P1 (params: ($x: System.Nope), title: 'x', layout: Atlas_Core.Atlas_Default) { };",
			"System.Nope"},
		{"snippet parameter",
			"create snippet ModT.S1 (params: ($x: ModT.Nope)) { };", "ModT.Nope"},
	}

	exec, _, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, "mdl 1;\ncreate module ModT;\ncreate persistent entity ModT.Thing (Name: String(100));"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := strings.Join(agreeCheck(t, exec, dir, "mdl 1;\n"+c.src), "\n")
			if !strings.Contains(got, c.missing) {
				t.Fatalf("check --references must name the unknown entity %s, reported:\n%s", c.missing, got)
			}
		})
	}
}

// The controls: real System entities and enumerations, and an entity the
// script itself creates, must keep passing. A reference check louder than the
// model is a blocker, not a safety net.
func TestCheckDeclaredEntityTypes_ResolvableTypesPass(t *testing.T) {
	cases := []struct{ name, src string }{
		{"association to System.User",
			"create association ModT.Thing_User from ModT.Thing to System.User;"},
		{"microflow parameter System.User",
			"create microflow ModT.F1 ($u: System.User) begin end;"},
		{"microflow list parameter of System.FileDocument",
			"create microflow ModT.F1 ($l: list of System.FileDocument) begin end;"},
		{"microflow returning System.Image",
			"create microflow ModT.F1 () returns System.Image begin return empty; end;"},
		{"microflow parameter of a System enumeration",
			"create microflow ModT.F1 ($d: System.DeviceType) begin end;"},
		{"microflow parameter of a stored user entity",
			"create microflow ModT.F1 ($t: ModT.Thing) begin end;"},
		{"nanoflow parameter System.Session",
			"create nanoflow ModT.N1 ($s: System.Session) begin end;"},
		{"page parameter System.User",
			"create page ModT.P1 (params: ($x: System.User), title: 'x', layout: Atlas_Core.Atlas_Default) { };"},
		{"parameter of an entity the script creates",
			"create persistent entity ModT.Later (Name: String(10));\ncreate microflow ModT.F1 ($l: ModT.Later) begin end;"},
	}

	exec, _, dir := openPedAppCopy(t)
	if err := agreeExec(t, exec, "mdl 1;\ncreate module ModT;\ncreate persistent entity ModT.Thing (Name: String(100));"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := agreeCheck(t, exec, dir, "mdl 1;\n"+c.src); len(got) != 0 {
				t.Fatalf("a resolvable type was refused:\n%s", strings.Join(got, "\n"))
			}
		})
	}
}
