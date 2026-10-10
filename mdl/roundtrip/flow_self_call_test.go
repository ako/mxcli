// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#843, found by the beta dress rehearsal (M5, M2). Both made an
// mdl 1 script un-authorable or un-re-runnable:
//
//   - a flow that calls itself failed exec with "microflow not found" (check
//     passed), and the stub-then-real workaround is refused by the splice;
//   - a nanoflow stored without ReturnVariableName refused the `as $Var` a
//     later statement states ("set it in Studio Pro").
//
// Each statement is executed twice: the first writes, the second writes
// nothing (the re-run rule). One harness for all of it — this suite is near
// its time limit (#870).
func TestFlowCreate_SelfCallAndNanoflowReturnVariable(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	twice := func(t *testing.T, name, script string) []byte {
		t.Helper()
		if err := h.exec(script); err != nil {
			t.Fatalf("exec 1: %v", err)
		}
		first := h.flowUnit(t, name)
		if err := h.exec(script); err != nil {
			t.Fatalf("exec 2: %v", err)
		}
		if again := h.flowUnit(t, name); !bytes.Equal(again, first) {
			t.Fatalf("exec 2 rewrote %s: %v", name, strictDiff(t, first, again))
		}
		return first
	}

	t.Run("recursive microflow", func(t *testing.T) {
		twice(t, "Rt843_Countdown", `mdl 1;
create or modify microflow MyFirstModule.Rt843_Countdown ($N: Integer)
returns Boolean as $Done
begin
  if $N <= 0 then
    return true;
  end if;
  $Below = call microflow MyFirstModule.Rt843_Countdown(N = $N - 1);
  return $Below;
end;`)
	})

	t.Run("recursive nanoflow", func(t *testing.T) {
		twice(t, "Rt843_CountdownNf", `mdl 1;
create or modify nanoflow MyFirstModule.Rt843_CountdownNf ($N: Integer)
returns Boolean as $Done
begin
  if $N <= 0 then
    return true;
  end if;
  $Below = call nanoflow MyFirstModule.Rt843_CountdownNf(N = $N - 1);
  return $Below;
end;`)
	})

	t.Run("nanoflow return variable added", func(t *testing.T) {
		if err := h.exec(`create or modify nanoflow MyFirstModule.Rt843_ReturnName ($Flag: Boolean)
returns Boolean
begin
  return true;
end;`); err != nil {
			t.Fatalf("setup: %v", err)
		}
		stored := h.flowUnit(t, "Rt843_ReturnName")
		// Since mendixlabs/mxcli#1373 the writer stores the key Studio Pro
		// always has, empty; what matters is that no return variable is named.
		if name, err := bson.Raw(stored).LookupErr("ReturnVariableName"); err == nil && name.StringValue() != "" {
			t.Fatalf("precondition: the setup nanoflow already names return variable %q", name.StringValue())
		}
		// The control is exec 1 itself: it must change the unit, so an
		// unchanged exec 2 is the re-run rule and not a check that sees nothing.
		after := twice(t, "Rt843_ReturnName", `mdl 1;
create or modify nanoflow MyFirstModule.Rt843_ReturnName ($Flag: Boolean)
returns Boolean as $Done
begin
  return true;
end;`)
		diff := strictDiff(t, stored, after)
		if len(diff) != 1 {
			t.Fatalf("want exactly the added ReturnVariableName, got %v", diff)
		}
		if out := h.mustDescribeMdl0(t, "nanoflow MyFirstModule.Rt843_ReturnName"); !bytes.Contains([]byte(out), []byte("returns Boolean as $Done")) {
			t.Fatalf("describe does not state the return variable:\n%s", out)
		}
	})
}
