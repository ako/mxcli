// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func buildNF(t *testing.T, src string) *ast.CreateNanoflowStmt {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	return prog.Statements[0].(*ast.CreateNanoflowStmt)
}

// TestValidateNanoflow_UnknownFunction guards mendixlabs/mxcli#1033: MDL044
// (#828) ran only over CREATE MICROFLOW, so `currentDeviceType()` in a
// nanoflow passed `check` and `exec` and then failed the build with
//
//	[error] [CE0117] "Error(s) in expression." at ... Test.NF_Dev ...
func TestValidateNanoflow_UnknownFunction(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantMDL bool
	}{
		{
			// The reported repro, verbatim.
			name: "issue repro: unknown call in a log message",
			src: `create nanoflow Test.NF_Dev ()
begin
  log info 'device: ' + currentDeviceType();
end;`,
			wantMDL: true,
		},
		{
			name: "unknown call in a declare",
			src: `create nanoflow Test.NF_Dev () returns Boolean
begin
  declare $b Boolean = currentDeviceType() = 'Phone';
  return $b;
end;`,
			wantMDL: true,
		},
		{
			name: "unknown call nested in an if branch",
			src: `create nanoflow Test.NF_Dev ($x: String)
begin
  if $x != empty then
    set $x = trunc(1.5);
  end if;
end;`,
			wantMDL: true,
		},
		{
			// A token is not a function call, so MDL044 stays out of it. Measured
			// on mxbuild 11.13.0: this nanoflow builds at 0 errors. (The report's
			// suggested workaround, [%CurrentDeviceType%], does NOT — it is CE0117
			// in a nanoflow and a microflow alike, so it is not pinned here.)
			name: "a real token is accepted",
			src: `create nanoflow Test.NF_Dev ()
begin
  log info 'now: ' + toString([%CurrentDateTime%]);
end;`,
		},
		{
			name: "known functions are accepted",
			src: `create nanoflow Test.NF_Dev ($x: String) returns String
begin
  declare $s String = toUpperCase(trim($x));
  return $s;
end;`,
		},
		{
			// Only MDL044 runs over a nanoflow. MDL057 (`synchronize` is
			// nanoflow-only) would be a false positive here, which is why the
			// microflow rule set is not run wholesale.
			name: "synchronize is not flagged in a nanoflow",
			src: `create nanoflow Test.NF_Sync ()
begin
  synchronize all;
end;`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nf := buildNF(t, tc.src)
			vs := ValidateNanoflow(nf)
			got := false
			for _, v := range vs {
				if v.RuleID == "MDL044" {
					got = true
					if v.Location.DocumentType != "nanoflow" {
						t.Errorf("violation should locate a nanoflow, got %q", v.Location.DocumentType)
					}
				} else {
					t.Errorf("only MDL044 runs over a nanoflow, got %s: %s", v.RuleID, v.Message)
				}
			}
			if got != tc.wantMDL {
				t.Fatalf("MDL044 fired=%v, want %v (violations %v)", got, tc.wantMDL, vs)
			}
			// The exec barrier must agree with check.
			err := validateNanoflowRules(nf)
			if tc.wantMDL {
				if err == nil || !strings.Contains(err.Error(), "MDL044") {
					t.Errorf("exec validation should refuse with MDL044, got: %v", err)
				}
			} else if err != nil {
				t.Errorf("valid nanoflow rejected by exec validation: %v", err)
			}
		})
	}
}
