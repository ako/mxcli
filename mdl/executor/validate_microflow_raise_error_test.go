// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1030: `raise error;` in a microflow's MAIN flow passed
// `mxcli check`, was written by `exec`, and was then rejected by the build with
//
//	[error] [CE0710] "The main flow cannot join an error flow or end in an error event"
//
// The reported diagnosis — "mxcli's flow-graph builder unconditionally appends a
// trailing EndEvent unless an internal 'ends with return' flag is set" — is not
// the cause here: `isTerminalStmt` already treats RaiseErrorStmt as a
// terminator, and the graph mxcli builds is the one the reporter asked for (see
// TestMainFlowErrorEventIsTheShapeMxbuildRejects below). The bug is that the
// graph is illegal Mendix no matter how it is wired: an error event may only
// close an error-handling flow.

// findViolation returns the first violation with the given rule ID.
func findViolation(vs []linter.Violation, ruleID string) *linter.Violation {
	for i := range vs {
		if vs[i].RuleID == ruleID {
			return &vs[i]
		}
	}
	return nil
}

func violationsForMicroflowSource(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog := parseMDL(t, src)
	for _, stmt := range prog.Statements {
		if c, ok := stmt.(*ast.CreateMicroflowStmt); ok {
			return ValidateMicroflow(c)
		}
	}
	t.Fatal("no CREATE MICROFLOW in the parsed program")
	return nil
}

// The four reproductions from the issue, verbatim.
func TestMDL084_MainFlowRaiseErrorRefused(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"RaiseOnly", `create microflow Test.MF_RaiseOnly ()
begin
  raise error;
end;`},
		{"RaiseTyped", `create microflow Test.MF_RaiseTyped () returns boolean as $Result
begin
  raise error;
end;`},
		{"RaiseIf", `create microflow Test.MF_RaiseIf ($X: boolean) returns boolean as $Result
begin
  if $X then
    return true;
  else
    raise error;
  end if;
end;`},
		{"RaiseGuard", `create microflow Test.MF_RaiseGuard ($X: boolean) returns boolean as $Result
begin
  if $X = false then
    raise error;
  end if;
  return true;
end;`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := findViolation(violationsForMicroflowSource(t, tc.src), "MDL084")
			if v == nil {
				t.Fatal("main-flow `raise error;` was accepted — mxbuild rejects it with CE0710")
			}
			if v.Severity != linter.SeverityError {
				t.Errorf("severity = %v, want error (exec must refuse rather than write it)", v.Severity)
			}
			if !strings.Contains(v.Message, "CE0710") {
				t.Errorf("message does not name the build error the author will see:\n%s", v.Message)
			}
		})
	}
}

// A raise inside a loop, and inside a branch inside a loop, is still the main
// flow: no amount of nesting turns a sequence flow into an error flow.
func TestMDL084_MainFlowRaiseErrorInLoopRefused(t *testing.T) {
	src := `create microflow Test.MF_RaiseInLoop ($Items: list of System.FileDocument)
begin
  loop $Item in $Items
  begin
    if $Item != empty then
      raise error;
    end if;
  end loop;
end;`
	if findViolation(violationsForMicroflowSource(t, src), "MDL084") == nil {
		t.Fatal("`raise error;` nested in a loop body was accepted")
	}
}

// The control that bounds the rule: an error event closing an ERROR-handling
// flow is exactly what Mendix's error event is for, and must not be flagged.
// Without this case the rule could be "refuse RAISE ERROR everywhere" and still
// pass every assertion above.
func TestMDL084_HandlerRaiseErrorAccepted(t *testing.T) {
	src := `create microflow Test.MF_Handler ()
begin
  call microflow Test.MF_Other ()
  on error {
    log error node 'Test' 'failed';
    raise error;
  };
end;`
	if v := findViolation(violationsForMicroflowSource(t, src), "MDL084"); v != nil {
		t.Fatalf("`raise error;` inside an ON ERROR handler was refused: %s", v.Message)
	}
}

// Nested one branch deeper inside the handler — still an error flow.
func TestMDL084_HandlerRaiseErrorInBranchAccepted(t *testing.T) {
	src := `create microflow Test.MF_HandlerIf ($Retry: boolean)
begin
  call microflow Test.MF_Other ()
  on error {
    if $Retry then
      log warning node 'Test' 'retrying';
    else
      raise error;
    end if;
  };
end;`
	if v := findViolation(violationsForMicroflowSource(t, src), "MDL084"); v != nil {
		t.Fatalf("`raise error;` inside a branch of an ON ERROR handler was refused: %s", v.Message)
	}
}

// The shape behind CE0710, pinned so the claim in the rule's message is checkable
// against the document mxcli actually builds: the error event is reached from the
// start event by an ordinary SequenceFlow, not by an error-handler flow.
//
// This is NOT the regression test — the builder is unchanged by the fix, and
// mxcli now refuses the script before it reaches the builder. It is the evidence
// that the refusal is about a real illegal document rather than a guess.
func TestMainFlowErrorEventIsTheShapeMxbuildRejects(t *testing.T) {
	col := buildMicroflowFromMDL(t, `create microflow Test.MF_RaiseOnly ()
begin
  raise error;
end;`)

	var errEvent *microflows.ErrorEvent
	for _, o := range col.Objects {
		if ee, ok := o.(*microflows.ErrorEvent); ok {
			errEvent = ee
		}
	}
	if errEvent == nil {
		t.Fatal("no error event in the graph; the assertion would be vacuous")
	}
	inbound := 0
	for _, f := range col.Flows {
		if f.DestinationID != errEvent.ID {
			continue
		}
		inbound++
		if f.IsErrorHandler {
			t.Error("inbound flow is an error-handler flow, so CE0710 would not apply — " +
				"re-measure before trusting MDL084's message")
		}
	}
	if inbound != 1 {
		t.Fatalf("error event has %d inbound flows, want 1", inbound)
	}
}

// A rule is built by the same flow builder, so a main-flow `raise error;` in one
// is the same CE0710. `validateRule` is what both `check` and `exec` call.
func TestMDL084_RuleMainFlowRaiseErrorRefused(t *testing.T) {
	boolRet := &ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeBoolean}}

	msg := validateRule("Mod.R", []ast.MicroflowStatement{&ast.RaiseErrorStmt{}}, boolRet)
	if !strings.Contains(msg, "CE0710") {
		t.Fatalf("a rule whose body is `raise error;` was accepted; got:\n%s", msg)
	}

	// Control: inside an ON ERROR handler it stays legal in a rule too.
	handled := []ast.MicroflowStatement{
		&ast.CallMicroflowStmt{
			MicroflowName: ast.QualifiedName{Module: "Mod", Name: "MF"},
			ErrorHandling: &ast.ErrorHandlingClause{
				Type: ast.ErrorHandlingCustom,
				Body: []ast.MicroflowStatement{&ast.RaiseErrorStmt{}},
			},
		},
		&ast.ReturnStmt{},
	}
	if msg := validateRule("Mod.R", handled, boolRet); msg != "" {
		t.Fatalf("`raise error;` inside a rule's ON ERROR handler was refused: %s", msg)
	}
}
