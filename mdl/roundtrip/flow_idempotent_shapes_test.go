// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"fmt"
	"strings"
	"testing"
)

// ako/mxcli#859: under `mdl 1` an identical second run of a `create or modify
// microflow` was refused for flows mxcli had just built from the same bytes.
//
// diff-then-patch compares the declared statements with the stored flow's
// description, and several control-flow shapes are spelled one way by authors
// and printed another way by describe, for the same graph:
//
//   - a guard clause directly before the final return,
//     `if c then return a; end if; return b;`, is built as a split whose false
//     flow goes straight to an end event, and described as
//     `if c then return a; else return b; end if;` (S1);
//   - a guard nested in an `if` without else is built with the outer merge as
//     the inner split's false target, and described with a redundant
//     `join shared1; merge shared1;` after the outer `end if` (S2);
//   - two changes straddling a guard's return were merged into one fragment
//     that returns (S4).
//
// Under mdl 0 the mismatch fell back to rebuilding the whole flow; under mdl 1,
// where that fallback is a refusal, the re-run failed. Two spellings that build
// the same flow must match the same stored nodes.
//
// Every flow here is created by the script itself, which is the subject: the
// defect is that mxcli could not re-apply what it had just written.
var idempotentShapes = []struct{ name, script string }{
	{"guard clause before the final return", `create or modify microflow MyFirstModule.Idem_Guard ($N: Integer) returns Integer
begin
  if $N < 0 then
    return 0;
  end if;
  return $N;
end;
`},
	{"guard clause with an activity before the final return", `create or modify microflow MyFirstModule.Idem_GuardAct ($N: Integer) returns Integer
begin
  if $N < 0 then
    return 0;
  end if;
  declare $M Integer = $N + 1;
  return $M;
end;
`},
	{"guards in sequence", `create or modify microflow MyFirstModule.Idem_Guards ($N: Integer) returns String
begin
  if $N < 0 then
    return 'negative';
  end if;
  if $N = 0 then
    return 'zero';
  end if;
  if $N > 100 then
    return 'large';
  end if;
  return 'small';
end;
`},
	{"guard nested in an if without else", `create or modify microflow MyFirstModule.Idem_NestedGuard ($A: Boolean, $B: Boolean)
returns String as $Problem
begin
  if $A then
    if $B then
      return 'b';
    end if;
  end if;
  return '';
end;
`},
	{"nested guards in sequence", `create or modify microflow MyFirstModule.Idem_NestedGuards ($A: Boolean, $B: Boolean, $C: Boolean, $N: Integer)
returns String as $Problem
begin
  if $N < 1 then
    return 'N';
  end if;
  if not($A) then
    if $B then
      return 'B';
    end if;
    if $C and $N > 3 then
      return 'C';
    end if;
  end if;
  if $A and $B then
    declare $Both String = 'both';
    return $Both;
  end if;
  if $N > 10 then
    if $N > 20 then
      return 'huge';
    end if;
    if $N > 15 then
      return 'big';
    end if;
  end if;
  return '';
end;
`},
	{"void guard clause", `create or modify microflow MyFirstModule.Idem_VoidGuard ($N: Integer)
begin
  if $N < 0 then
    return;
  end if;
  log info node 'Idem' 'positive';
end;
`},
	{"nanoflow guard clause", `create or modify nanoflow MyFirstModule.Idem_NanoGuard ($N: Integer) returns Boolean
begin
  if $N < 0 then
    return false;
  end if;
  return true;
end;
`},
	{"guard inside a branch", `create or modify microflow MyFirstModule.Idem_BranchGuard ($A: Boolean, $N: Integer) returns Integer
begin
  if $A then
    if $N < 0 then
      return 0;
    end if;
    return $N;
  else
    return -1;
  end if;
end;
`},
}

func TestFlowModify_IdenticalReRunIsUnchanged(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	for _, c := range idempotentShapes {
		for _, header := range []string{"", "mdl 1;\n"} {
			name := map[string]string{"": "mdl 0", "mdl 1;\n": "mdl 1"}[header]
			t.Run(c.name+"/"+name, func(t *testing.T) {
				h.restore()
				script := header + c.script
				if err := h.exec(script); err != nil {
					t.Fatalf("first run: %v\n%s", err, h.out.String())
				}
				created := h.snapshot()
				err := h.exec(script)
				out := h.out.String()
				if err != nil {
					t.Fatalf("the identical second run failed: %v\n--- described ---\n%s", err, h.describeFlow(c.script))
				}
				if strings.Contains(out, "MDL-V1-REBUILD") {
					t.Errorf("the identical second run fell back to a rebuild:\n%s\n--- described ---\n%s", out, h.describeFlow(c.script))
				}
				if !strings.Contains(out, "Unchanged ") {
					t.Errorf("the identical second run did not report Unchanged:\n%s", out)
				}
				if changed := created.diff(h.snapshot()); len(changed) != 0 {
					t.Errorf("the identical second run wrote %d unit(s):\n  %s", len(changed), strings.Join(changed, "\n  "))
				}
			})
		}
	}
}

// Controls: the matching that makes a guard clause match its describe-normal
// form still sees a change on either side of the guard, and splices it in.
func TestFlowModify_GuardClauseEditsAreSpliced(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const guard = `mdl 1;
create or modify microflow MyFirstModule.Idem_GuardEdit ($N: Integer) returns Integer
begin
  declare $Base Integer = 1;
  if $N < 0 then
    declare $Neg Integer = 0 - $N;
    return $Neg;
  end if;
  return $N;
end;
`
	type edit struct{ from, to string }
	for _, c := range []struct {
		name  string
		edits []edit
		want  string
	}{
		// The final return: the stored else of the described form.
		{"a changed final return", []edit{{"  return $N;\nend;", "  return $N + $Base;\nend;"}}, "spliced: 1 edited in place"},
		// An activity inserted between the guard and the final return.
		{"an activity after the guard", []edit{{"  return $N;\nend;", "  declare $M Integer = $N * 2;\n  return $M;\nend;"}}, "spliced: 1 inserted"},
		// Two changes straddling the guard's return: one each, never one
		// fragment holding the return (S4).
		{"changes on both sides of the guard's return", []edit{
			{"declare $Base Integer = 1;", "declare $Base Integer = 2;"},
			{"declare $Neg Integer = 0 - $N;", "declare $Neg Integer = 1 - $N;"},
		}, "spliced: 2 edited in place"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h.restore()
			if err := h.exec(guard); err != nil {
				t.Fatalf("create: %v\n%s", err, h.out.String())
			}
			edited := guard
			for _, e := range c.edits {
				edited = strings.Replace(edited, e.from, e.to, 1)
			}
			if edited == guard {
				t.Fatal("the edit did not apply")
			}
			before := h.snapshot()
			if err := h.exec(edited); err != nil {
				t.Fatalf("the edit was refused: %v", err)
			}
			if !strings.Contains(h.out.String(), c.want) {
				t.Errorf("want %q, got:\n%s", c.want, h.out.String())
			}
			if len(before.diff(h.snapshot())) == 0 {
				t.Error("the edit wrote nothing")
			}
			// And the edited flow is now what an identical run matches.
			after := h.snapshot()
			if err := h.exec(edited); err != nil {
				t.Fatalf("re-run of the edit: %v", err)
			}
			if changed := after.diff(h.snapshot()); len(changed) != 0 {
				t.Errorf("re-run of the edit wrote:\n  %s", strings.Join(changed, "\n  "))
			}
		})
	}
}

// describeFlow describes the first flow a script creates, for a failure
// message.
func (h *harness) describeFlow(script string) string {
	for _, kw := range []string{"microflow", "nanoflow"} {
		i := strings.Index(script, "create or modify "+kw+" ")
		if i < 0 {
			continue
		}
		rest := script[i+len("create or modify "+kw+" "):]
		name := rest[:strings.IndexAny(rest, " (\n")]
		out, _ := h.describe(kw + " " + name)
		return out
	}
	return ""
}

// Controls for the built comparison (builtAsStored): a change to a property
// the reader does not read back is still a change, and is written. The built
// comparison reads both sides back through the codec, so whatever the reader
// drops would compare equal on both sides; a flow whose build does not
// survive its own read back must not be matched that way. A changed page
// title override was reported Unchanged and silently not written.
func TestFlowModify_PropertyEditsAreWritten(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	for _, header := range []string{"mdl 1;\n", ""} {
		for _, c := range []struct {
			name     string
			from, to string
		}{
			{"a changed page title override",
				"show page Administration.Account_Overview with title = 'First';",
				"show page Administration.Account_Overview with title = 'Second';"},
			{"a page title override added",
				"show page Administration.Account_Overview;",
				"show page Administration.Account_Overview with title = 'First';"},
			// Taken out, the override is seen only once describe prints it:
			// the statement diff compares against the description
			// (ako/mxcli#869).
			{"a page title override taken out",
				"show page Administration.Account_Overview with title = 'First';",
				"show page Administration.Account_Overview;"},
		} {
			name := "mdl 0"
			if header != "" {
				name = "mdl 1"
			}
			t.Run(name+"/"+c.name, func(t *testing.T) {
				h.restore()
				const flow = "create or modify microflow MyFirstModule.Idem_PropEdit ()\nbegin\n  %s\nend;\n"
				if err := h.exec(header + fmt.Sprintf(flow, c.from)); err != nil {
					t.Fatalf("create: %v\n%s", err, h.out.String())
				}
				before := h.snapshot()
				if err := h.exec(header + fmt.Sprintf(flow, c.to)); err != nil {
					t.Fatalf("the edit was refused: %v\n%s", err, h.out.String())
				}
				if strings.Contains(h.out.String(), "Unchanged ") || len(before.diff(h.snapshot())) == 0 {
					t.Errorf("the edit wrote nothing:\n%s", h.out.String())
				}
				// And an identical run of the edited flow still writes nothing.
				after := h.snapshot()
				if err := h.exec(header + fmt.Sprintf(flow, c.to)); err != nil {
					t.Fatalf("re-run of the edit: %v", err)
				}
				if changed := after.diff(h.snapshot()); len(changed) != 0 {
					t.Errorf("re-run of the edit wrote:\n  %s\n%s", strings.Join(changed, "\n  "), h.out.String())
				}
			})
		}
	}
}

// A call web service activity the structured form does not reproduce keeps its
// raw document, which carries the element IDs of its own build: re-running the
// statement unchanged re-spliced it on every run (ako/mxcli#861). A dangling
// receive mapping is such a call — its result type stays Void. The control: a
// changed timeout is still written.
func TestFlowModify_WebServiceCallRerunsClean(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	const setup = "create module SampleSOAP;\ncreate entity SampleSOAP.OrderResponse (Status : string(50));\n"
	const flow = `create or modify microflow SampleSOAP.Idem_Soap ()
returns SampleSOAP.OrderResponse as $Root
begin
  $Root = call web service SampleSOAP.OrderService
  operation FetchSampleItems
  receive mapping SampleSOAP.OrderResponse
  timeout %s;
  return $Root;
end;
`
	for _, header := range []string{"mdl 1;\n", ""} {
		name := "mdl 0"
		if header != "" {
			name = "mdl 1"
		}
		t.Run(name, func(t *testing.T) {
			h.restore()
			if err := h.exec(header + setup + fmt.Sprintf(flow, "30")); err != nil {
				t.Fatalf("create: %v\n%s", err, h.out.String())
			}
			before := h.snapshot()
			if err := h.exec(header + fmt.Sprintf(flow, "30")); err != nil {
				t.Fatalf("re-run: %v\n%s", err, h.out.String())
			}
			if changed := before.diff(h.snapshot()); len(changed) != 0 {
				t.Errorf("an identical re-run wrote:\n  %s\n%s", strings.Join(changed, "\n  "), h.out.String())
			}
			if err := h.exec(header + fmt.Sprintf(flow, "60")); err != nil {
				t.Fatalf("the edit was refused: %v\n%s", err, h.out.String())
			}
			if strings.Contains(h.out.String(), "Unchanged ") || len(before.diff(h.snapshot())) == 0 {
				t.Errorf("a changed timeout wrote nothing:\n%s", h.out.String())
			}
		})
	}
}
