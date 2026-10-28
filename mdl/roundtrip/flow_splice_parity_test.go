// SPDX-License-Identifier: Apache-2.0

//go:build integration

package roundtrip

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/scriptdiff"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#839: the `mdl 1;` header alone made `create or modify microflow`
// refuse a flow it had just built from the same bytes, while `diff` said the
// statement was unchanged.
//
// The version was not what differed. diff-then-patch compares the declared
// statements with the stored flow's description, and two spellings authors
// write never matched what describe prints for them, under either version:
//
//   - a retrieve's `where [ … ]` — the bracketed form is read as XPath and the
//     bare form describe prints as an expression, so one constraint parsed to
//     two different trees (`and` against `AND`, the author's line breaks in
//     one and not the other);
//   - a member value followed by a line break before `)` — the source text
//     keeps the whitespace, and describe drops the whitespace around a stored
//     expression.
//
// Unmatched, the retrieve became a drop and an insert. Under mdl 0 that falls
// back to rebuilding the whole flow — reported "Unchanged" only because the
// rebuild happened to store the same bytes — and under mdl 1, where the
// fallback is a refusal, it was refused. diff never ran the splice at all.
//
// The script is the shape of mxcli-ledger's Ledger.ACT_ApplyInsightsFilter:
// annotated retrieves with bracketed constraints over several lines, and a
// change whose closing parenthesis is on a line of its own.
const spliceParityDomain = `create persistent entity MyFirstModule.SpliceRow (
  MonthKey: Integer,
  CategoryName: String(200),
  AccountName: String(200)
);
create non-persistent entity MyFirstModule.SpliceContext (
  FromKey: Integer,
  ToKey: Integer,
  FilterText: String(unlimited)
);
`

const spliceParityFlow = `create or modify microflow MyFirstModule.SpliceParity (
  $Context: MyFirstModule.SpliceContext
)
returns Boolean as $Done
begin
  declare $Done Boolean = true;
  declare $CatName String = '';
  declare $Total Integer = 0;

  @annotation 'Every filter is in the retrieve.'
  retrieve $Rows from MyFirstModule.SpliceRow
    where [MonthKey >= $Context/FromKey and MonthKey <= $Context/ToKey
      and ($CatName = '' or CategoryName = $CatName)
      and not(CategoryName = 'Savings transfer')]
    sort by CategoryName asc, MonthKey asc;

  @annotation 'A second one, so the unmatched pair is a replace and a drop of an annotated retrieve.'
  retrieve $Accounts from MyFirstModule.SpliceRow
    where [($CatName = '' or AccountName = $CatName)]
    sort by AccountName asc;

  @annotation 'The only unconstrained retrieve.'
  retrieve $All from MyFirstModule.SpliceRow sort by MonthKey asc;

  loop $R in $Rows
  begin
    set $Total = $Total + $R/MonthKey;
  end loop;

  change $Context (
    FilterText = 'from ' + toString($Context/FromKey)
      + ' to ' + toString($Context/ToKey)
  );

  return $Done;
end;
`

const spliceParityTarget = "microflow MyFirstModule.SpliceParity"

func TestPedAppSpliceParity_AuthoredSpellings(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	if err := h.exec(spliceParityDomain + spliceParityFlow); err != nil {
		t.Fatalf("create the flow: %v\n%s", err, h.out.String())
	}
	created := h.snapshot()

	for _, header := range []string{"", "mdl 1;\n"} {
		name := map[string]string{"": "mdl 0", "mdl 1;\n": "mdl 1"}[header]
		t.Run(name, func(t *testing.T) {
			script := header + spliceParityFlow
			// diff first: it must reach exec's verdict, and it writes nothing.
			if out := h.diff(script); !strings.Contains(out, "exec would write nothing") ||
				strings.Contains(out, "Refused") {
				t.Errorf("diff of the unchanged source under %s:\n%s", name, out)
			}
			if err := h.exec(script); err != nil {
				t.Fatalf("exec the unchanged source under %s: %v", name, err)
			}
			out := h.out.String()
			if strings.Contains(out, "MDL-V1-REBUILD") {
				t.Errorf("under %s the splice did not match the stored flow and fell back to a rebuild:\n%s", name, out)
			}
			if !strings.Contains(out, "Unchanged microflow: MyFirstModule.SpliceParity") {
				t.Errorf("under %s the unchanged source did not report Unchanged:\n%s", name, out)
			}
			if changed := created.diff(h.snapshot()); len(changed) != 0 {
				t.Errorf("under %s the unchanged source wrote %d unit(s):\n  %s", name, len(changed), strings.Join(changed, "\n  "))
			}
		})
	}

	// Control: a constraint that stores something else is a change, and is
	// spliced in — the comparison is not blind to the where clause.
	t.Run("an edited constraint writes", func(t *testing.T) {
		edited := strings.Replace(spliceParityFlow, "MonthKey >= $Context/FromKey", "MonthKey > $Context/FromKey", 1)
		if out := h.diff("mdl 1;\n" + edited); !strings.Contains(out, "0 new, 1 modified, 0 removed") {
			t.Errorf("diff of an edited constraint:\n%s", out)
		}
		if err := h.exec("mdl 1;\n" + edited); err != nil {
			t.Fatalf("exec an edited constraint: %v\n%s", err, h.out.String())
		}
		if !strings.Contains(h.out.String(), "Modified microflow: MyFirstModule.SpliceParity (spliced: 1 edited in place)") {
			t.Errorf("an edited constraint must be spliced in as one retrieve edited in place:\n%s", h.out.String())
		}
		if len(created.diff(h.snapshot())) == 0 {
			t.Error("an edited constraint wrote nothing")
		}
		if got := h.describeUnder("mdl 1;", spliceParityTarget); !strings.Contains(got, "MonthKey > $Context/FromKey") {
			t.Errorf("the edit is not in the description:\n%s", got)
		}
	})

	// Control: a change exec refuses under mdl 1 is a refusal in diff too.
	t.Run("diff reports the refusal exec makes", func(t *testing.T) {
		// A decision added inside the loop body: a change of its shape (a
		// value edit there is spliced in place, ako/mxcli#1106, and so is a
		// plain activity added, ako/mxcli#1693).
		inLoop := strings.Replace(spliceParityFlow, "set $Total = $Total + $R/MonthKey;",
			"set $Total = $Total + $R/MonthKey;\n    if $R/MonthKey > 0 then\n      set $Total = $Total + 1;\n    end if;", 1)
		if inLoop == spliceParityFlow {
			t.Fatal("the flow has no loop-body set")
		}
		before := h.snapshot()
		out := h.diff("mdl 1;\n" + inLoop)
		if !strings.Contains(out, "Refused: exec would stop at this error, having written nothing") || !strings.Contains(out, "cannot be spliced") {
			t.Errorf("diff of a change exec refuses:\n%s", out)
		}
		err := h.exec("mdl 1;\n" + inLoop)
		if err == nil || !strings.Contains(err.Error(), "cannot be spliced") {
			t.Fatalf("exec of a change inside a loop under mdl 1: got %v, want the splice refusal", err)
		}
		if changed := before.diff(h.snapshot()); len(changed) != 0 {
			t.Errorf("a refused statement wrote %d unit(s)", len(changed))
		}
		// Under mdl 0 the same change is rebuilt, and diff shows the change.
		if out := h.diff(inLoop); !strings.Contains(out, "0 new, 1 modified, 0 removed") {
			t.Errorf("diff of the same change under mdl 0:\n%s", out)
		}
	})
}

// The other direction of #839's diff/exec agreement: a statement whose MDL
// renders the same as the stored flow's but which exec still writes. A
// `create or modify` stating another folder is one: exec moves the flow
// ("Moved microflow"), and diff, comparing renderings that leave the folder
// out, said unchanged.
func TestPedAppSpliceParity_DiffReportsTheWriteExecMakes(t *testing.T) {
	h := newHarness(t)
	defer h.close()
	if err := h.exec(spliceParityDomain + spliceParityFlow); err != nil {
		t.Fatalf("create the flow: %v\n%s", err, h.out.String())
	}
	moved := strings.Replace(spliceParityFlow, "returns Boolean as $Done\nbegin", "returns Boolean as $Done\nfolder 'Moved'\nbegin", 1)
	if moved == spliceParityFlow {
		t.Fatal("the folder clause was not added")
	}
	for _, header := range []string{"", "mdl 1;\n"} {
		name := map[string]string{"": "mdl 0", "mdl 1;\n": "mdl 1"}[header]
		// Control: the same statement in the stored folder is unchanged.
		if out := h.diff(header + spliceParityFlow); !strings.Contains(out, "exec would write nothing") {
			t.Errorf("diff of the unchanged source under %s:\n%s", name, out)
		}
		// The folder is new, and the flow is moved into it.
		if out := h.diff(header + moved); !strings.Contains(out, "1 new, 1 modified, 0 removed") ||
			!strings.Contains(out, "moved to 'MyFirstModule/Moved'") {
			t.Errorf("diff of a statement exec moves, under %s:\n%s", name, out)
		}
	}
	before := h.snapshot()
	if err := h.exec("mdl 1;\n" + moved); err != nil {
		t.Fatalf("exec the moved statement: %v\n%s", err, h.out.String())
	}
	if !strings.Contains(h.out.String(), "Moved microflow: MyFirstModule.SpliceParity") {
		t.Errorf("exec of the moved statement:\n%s", h.out.String())
	}
	if len(before.diff(h.snapshot())) == 0 {
		t.Error("exec of the moved statement wrote nothing, so diff's modified is the wrong verdict")
	}
}

// TestPedAppFlowSpliceParity / TestTestAppFlowSpliceParity: the property
// #839 broke, over every Studio Pro-authored microflow and nanoflow. A flow's
// description, executed back, is matched statement for statement by the
// splice — no rebuild, no refusal — and writes nothing, under mdl 0 and under
// mdl 1 (the flow described in that language, executed under its header).
//
// The main round trip (TestPedAppRoundTrip) holds mdl 0 to GetPut, but a
// fallback rebuild that happens to store the same bytes passes it: the rebuild
// warning is what shows the splice did not match, so it is a failure here.
func TestPedAppFlowSpliceParity(t *testing.T)  { runFlowSpliceParity(t, pedApp) }
func TestTestAppFlowSpliceParity(t *testing.T) { runFlowSpliceParity(t, testApp) }

func runFlowSpliceParity(t *testing.T, fx fixture) {
	h := newFixtureHarness(t, fx)
	defer h.close()

	flows := 0
	for _, d := range h.documents() {
		if d.keyword != "microflow" && d.keyword != "nanoflow" {
			continue
		}
		flows++
		t.Run(d.key(), func(t *testing.T) {
			// A description that does not parse is the round trip's failure,
			// allowlisted there, and says nothing about the splice.
			for _, l := range fx.knownFailures[d.key()].laws {
				if l == lawDescribe || l == lawParse {
					t.Skipf("its description does not %s (%s: %s)", l, fx.knownFailures[d.key()].issue, fx.knownFailures[d.key()].why)
				}
			}
			for _, header := range []string{"", "mdl 1;"} {
				name := map[string]string{"": "mdl 0", "mdl 1;": "mdl 1"}[header]
				var described string
				if header == "" {
					var err error
					if described, err = h.describeAs(langver.V0, d.target()); err != nil {
						t.Fatalf("describe %s: %v", d.target(), err)
					}
				} else {
					described = header + "\n" + h.describeUnder(header, d.target())
				}
				err := h.exec(described)
				out := h.out.String()
				changed := h.orig.diff(h.snapshot())
				switch {
				case err != nil && strings.Contains(err.Error(), "description does not parse"):
					// The splice compares against the stored flow's own
					// description; one its parser rejects makes every re-run
					// of the flow a refusal (ako/mxcli#859).
					t.Errorf("%s: the stored flow's description does not re-parse: %v", name, err)
				case err != nil && len(changed) == 0 && isRefusal(err):
					t.Logf("%s: refused, nothing written: %v", name, err)
				case err != nil:
					t.Errorf("%s: exec of the description failed: %v\n--- script ---\n%s", name, err, described)
				case strings.Contains(out, "MDL-V1-REBUILD"):
					t.Errorf("%s: the description did not splice into the flow it describes; it was rebuilt:\n%s", name, out)
				}
				if len(changed) != 0 {
					t.Errorf("%s: executing the description wrote %d unit(s):\n  %s", name, len(changed), strings.Join(changed, "\n  "))
				}
				if err != nil || len(changed) != 0 {
					h.restore()
				}
			}
		})
	}
	if flows == 0 {
		t.Fatalf("%s has no microflow or nanoflow — the enumeration is broken", fx.name)
	}
	t.Logf("%d flows checked under mdl 0 and mdl 1", flows)
}

// diff runs `mxcli diff` on a script and returns what it printed: the script
// executed on a scratch copy of the working copy, compared unit by unit.
func (h *harness) diff(script string) string {
	h.t.Helper()
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		h.t.Fatalf("parse: %v", errs[0])
	}
	rep, err := scriptdiff.Run(h.mpr, prog, scriptdiff.Options{
		NewBackend: func() backend.FullBackend { return modelsdkbackend.New() },
	})
	if err != nil {
		h.t.Fatalf("diff: %v", err)
	}
	var out bytes.Buffer
	rep.Write(&out, executor.DiffOptions{}, false)
	return out.String()
}
