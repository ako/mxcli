// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The fixture's flows were drawn in Studio Pro, so none of them sits where the
// layout engine would put it — every "it moved" assertion below has that as its
// control, and every "nothing moved" assertion follows a run that did move it.

func layoutExecutor(t *testing.T) *Executor {
	t.Helper()
	exec := New(&bytes.Buffer{})
	exec.SetQuiet(true)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	t.Cleanup(func() { exec.Close() })
	run(t, exec, "CONNECT LOCAL '"+visitor.QuoteString(projectFixture(t))+"'")
	return exec
}

func flowQN(s string) ast.QualifiedName {
	mod, name, _ := strings.Cut(s, ".")
	return ast.QualifiedName{Module: mod, Name: name}
}

// rawFlow reads a flow's stored unit as a generic document.
func rawFlow(t *testing.T, exec *Executor, kind, name string) (bson.M, *layoutGraph) {
	t.Helper()
	g, err := storedFlow(exec.newExecContext(t.Context()), kind, flowQN(name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	raw, err := exec.backend.GetRawUnitBytes(g.id)
	if err != nil {
		t.Fatalf("read raw %s: %v", name, err)
	}
	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return doc, g
}

// changedKeys lists the keys of every leaf value that differs between two
// documents of the same shape, and any structural difference as "shape".
func changedKeys(a, b any, key string, out map[string]int) {
	switch av := a.(type) {
	case bson.M:
		bv, ok := b.(bson.M)
		if !ok || len(av) != len(bv) {
			out["shape@"+key]++
			return
		}
		for k, x := range av {
			y, ok := bv[k]
			if !ok {
				out["shape@"+k]++
				continue
			}
			changedKeys(x, y, k, out)
		}
	case bson.A:
		bv, ok := b.(bson.A)
		if !ok || len(av) != len(bv) {
			out["shape@"+key]++
			return
		}
		for i := range av {
			changedKeys(av[i], bv[i], key, out)
		}
	default:
		if !reflect.DeepEqual(a, b) {
			out[key]++
		}
	}
}

// layoutKeys are the only properties a layout may change.
var layoutKeys = map[string]bool{
	"RelativeMiddlePoint":        true,
	"Size":                       true,
	"OriginConnectionIndex":      true,
	"DestinationConnectionIndex": true,
	"OriginControlVector":        true,
	"DestinationControlVector":   true,
	"OriginBezierVector":         true,
	"DestinationBezierVector":    true,
}

func TestLayoutFlow_ChangesOnlyGeometry(t *testing.T) {
	exec := layoutExecutor(t)
	for _, tc := range []struct{ kind, name string }{
		{"microflow", "Administration.ChangeMyPassword"},
		{"nanoflow", "FeedbackModule.ACT_Feedback_UploadImage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, _ := rawFlow(t, exec, tc.kind, tc.name)
			res, err := exec.LayoutFlow(tc.kind, flowQN(tc.name), false)
			if err != nil {
				t.Fatal(err)
			}
			if res.Refused != "" {
				t.Fatalf("refused: %s", res.Refused)
			}
			if !res.Changed() {
				t.Fatal("a Studio Pro-drawn flow reported nothing to move — the control failed")
			}
			after, _ := rawFlow(t, exec, tc.kind, tc.name)

			diff := map[string]int{}
			changedKeys(before, after, "", diff)
			if diff["RelativeMiddlePoint"] == 0 {
				t.Errorf("no position changed on disk; diff: %v", diff)
			}
			for k, n := range diff {
				if !layoutKeys[k] {
					t.Errorf("layout changed %d %q value(s); only geometry may change", n, k)
				}
			}
		})
	}
}

func TestLayoutFlow_SecondRunChangesNothing(t *testing.T) {
	exec := layoutExecutor(t)
	name := flowQN("Administration.ChangeMyPassword")

	first, err := exec.LayoutFlow("microflow", name, false)
	if err != nil || first.Refused != "" {
		t.Fatalf("first run: %v %s", err, first.Refused)
	}
	if !first.Changed() {
		t.Fatal("control: the first run must move the Studio Pro layout")
	}
	before, _ := rawFlow(t, exec, "microflow", name.String())

	second, err := exec.LayoutFlow("microflow", name, false)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed() {
		t.Errorf("second run moved %d objects and %d flows", second.Moved, second.Flows)
	}
	after, _ := rawFlow(t, exec, "microflow", name.String())
	if diff := map[string]int{}; func() bool { changedKeys(before, after, "", diff); return len(diff) > 0 }() {
		t.Errorf("second run changed the stored flow: %v", diff)
	}
}

func TestLayoutFlow_DryRunWritesNothing(t *testing.T) {
	exec := layoutExecutor(t)
	name := flowQN("Administration.ChangePassword")
	before, _ := rawFlow(t, exec, "microflow", name.String())

	res, err := exec.LayoutFlow("microflow", name, true)
	if err != nil || res.Refused != "" {
		t.Fatalf("%v %s", err, res.Refused)
	}
	if !res.Changed() {
		t.Fatal("control: a dry run must still report what would move")
	}
	after, _ := rawFlow(t, exec, "microflow", name.String())
	diff := map[string]int{}
	changedKeys(before, after, "", diff)
	if len(diff) > 0 {
		t.Errorf("dry run wrote: %v", diff)
	}
}

// layoutAnnotationLine is a DESCRIBE line that only pins geometry.
var layoutAnnotationLine = regexp.MustCompile(`^\s*@(position|anchor|curve|merge|start)\b`)

// notePosition is the placement inside an @annotation line; its size stays.
var notePosition = regexp.MustCompile(`,?\s*position:\s*\(\s*-?\d+\s*,\s*-?\d+\s*\)`)

// TestLayoutFlow_MatchesCreate is the invariant the command exists for: a flow
// laid out in place looks exactly like the same flow created from MDL without
// layout annotations. The copy is made from the DESCRIBE text with those lines
// deleted — independently of stripFlowLayout — and executed as a new microflow.
func TestLayoutFlow_MatchesCreate(t *testing.T) {
	exec := layoutExecutor(t)
	for _, name := range []string{
		"Administration.ChangeMyPassword", // hand-placed start event (#951) must be reset too
		"Administration.SaveNewAccount",
		"FeedbackModule.SUB_Feedback_Sanitize",
	} {
		t.Run(name, func(t *testing.T) {
			ctx := exec.newExecContext(t.Context())
			mdl, _, err := describeMicroflowToString(ctx, flowQN(name))
			if err != nil {
				t.Fatal(err)
			}
			var kept []string
			for _, line := range strings.Split(mdl, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "grant ") {
					continue
				}
				if layoutAnnotationLine.MatchString(line) {
					continue
				}
				if strings.Contains(line, "@annotation(") {
					line = notePosition.ReplaceAllString(line, "")
				}
				kept = append(kept, line)
			}
			copyName := name + "_LayoutCopy"
			script := strings.Replace(strings.Join(kept, "\n"), "microflow "+name+" ", "microflow "+copyName+" ", 1)
			if !strings.Contains(script, copyName) {
				t.Fatalf("could not rename the described microflow:\n%s", script)
			}
			run(t, exec, script)

			res, err := exec.LayoutFlow("microflow", flowQN(name), false)
			if err != nil || res.Refused != "" {
				t.Fatalf("%v %s", err, res.Refused)
			}

			got := flowGeometryFingerprint(t, exec, name)
			want := flowGeometryFingerprint(t, exec, copyName)
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("laid out in place:\n  %s\ncreated without annotations:\n  %s",
					strings.Join(got, "\n  "), strings.Join(want, "\n  "))
			}
		})
	}
}

// flowGeometryFingerprint is every object's kind, position and size, sorted, so
// two flows with the same layout compare equal whatever their IDs.
func flowGeometryFingerprint(t *testing.T, exec *Executor, name string) []string {
	t.Helper()
	_, g := rawFlow(t, exec, "microflow", name)
	var out []string
	idx := indexFlowGraph(g.objects)
	for _, id := range idx.order {
		o := idx.object[id]
		out = append(out, fmt.Sprintf("%s @%v %v", layoutKind(o), o.GetPosition(), objectSize(o)))
	}
	sort.Strings(out)
	return out
}

// A merge with one flow in and one out joins nothing, so DESCRIBE leaves it out
// and the rebuild has no node for it. It must not make the flow unlayoutable.
func TestLayoutFlow_PassThroughMerge(t *testing.T) {
	exec := layoutExecutor(t)
	name := flowQN("FeedbackModule.ACT_SubmitFeedback")
	_, g := rawFlow(t, exec, "nanoflow", name.String())
	idx := indexFlowGraph(g.objects)
	var merge microflows.MicroflowObject
	for _, o := range idx.object {
		if _, ok := o.(*microflows.ExclusiveMerge); ok && idx.incoming[o.GetID()] == 1 {
			merge = o
		}
	}
	if merge == nil {
		t.Fatal("fixture no longer has a pass-through merge")
	}

	res, err := exec.LayoutFlow("nanoflow", name, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Refused != "" {
		t.Fatalf("refused: %s", res.Refused)
	}
	_, after := rawFlow(t, exec, "nanoflow", name.String())
	moved := indexFlowGraph(after.objects).object[merge.GetID()]
	if moved.GetPosition() == merge.GetPosition() {
		t.Errorf("the pass-through merge stayed at %v while everything around it moved", merge.GetPosition())
	}
}

// A flow whose description rebuilds into a different graph is left alone.
func TestLayoutFlow_RefusesWhatDoesNotRoundTrip(t *testing.T) {
	exec := layoutExecutor(t)
	name := flowQN("FeedbackModule.VAL_Feedback") // branches share merges MDL rebuilds differently
	before, _ := rawFlow(t, exec, "microflow", name.String())

	res, err := exec.LayoutFlow("microflow", name, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Refused == "" {
		t.Fatal("laid out a flow that does not round-trip through MDL")
	}
	if !strings.Contains(res.Refused, "round-trip") {
		t.Errorf("refusal does not say why: %s", res.Refused)
	}
	after, _ := rawFlow(t, exec, "microflow", name.String())
	diff := map[string]int{}
	changedKeys(before, after, "", diff)
	if len(diff) > 0 {
		t.Errorf("a refused flow was written: %v", diff)
	}
}

// stripFlowLayout must reach every nested body — a missed one would leave that
// body pinned where it was.
func TestStripFlowLayout_ReachesNestedBodies(t *testing.T) {
	src := `create microflow M.F (
  @position(1, 2)
  $P: String
)
begin
  @start(0, 0)
  @position(10, 10)
  @anchor(from: bottom, to: top)
  @curve(from: (1, 1), to: (2, 2))
  @annotation(text: 'note', position: (5, 5), size: (100, 40))
  declare $L List of M.E = empty;
  @position(20, 20)
  loop $I in $L begin
    @position(30, 30)
    if $P = 'x' then
      @position(40, 40)
      log info node 'n' 'a';
    else
      @position(50, 50)
      log info node 'n' 'b';
    end if;
  end loop;
  @position(60, 60)
  @merge(65, 65)
  if $P = 'y' then
    @position(70, 70)
    $R = call microflow M.G() on error {
      @position(80, 80)
      log info node 'n' 'c';
    };
  end if;
end;`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	annotations := 0
	var check func(v reflect.Value, strip bool) int
	check = func(v reflect.Value, _ bool) int {
		n := 0
		switch v.Kind() {
		case reflect.Ptr, reflect.Interface:
			if v.IsNil() {
				return 0
			}
			if ann, ok := v.Interface().(*ast.ActivityAnnotations); ok {
				annotations++
				for _, f := range []any{ann.Position, ann.Anchor, ann.Curve, ann.Merge, ann.Start} {
					if !reflect.ValueOf(f).IsNil() {
						n++
					}
				}
				for _, note := range append(ann.Notes, ann.FreeNotes...) {
					if note.Position != nil {
						n++
					}
				}
			}
			return n + check(v.Elem(), false)
		case reflect.Struct:
			if p, ok := v.Interface().(ast.MicroflowParam); ok && p.Position != nil {
				n++
			}
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).IsExported() {
					n += check(v.Field(i), false)
				}
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				n += check(v.Index(i), false)
			}
		}
		return n
	}

	if before := check(reflect.ValueOf(stmt), false); before < 12 {
		t.Fatalf("control: expected the fixture to carry layout annotations throughout, found %d", before)
	}
	annotations = 0
	stripFlowLayout(stmt)
	if left := check(reflect.ValueOf(stmt), false); left != 0 {
		t.Errorf("%d layout annotations survived the strip", left)
	}
	if annotations == 0 {
		t.Fatal("walked no annotations at all")
	}

	// Content survives: the note keeps its text and size.
	var note *ast.MicroflowAnnotation
	for _, s := range stmt.Body {
		if ann := ast.StatementAnnotations(s); ann != nil && len(ann.Notes)+len(ann.FreeNotes) > 0 {
			all := append(ann.Notes, ann.FreeNotes...)
			note = &all[0]
		}
	}
	if note == nil || note.Text != "note" || note.Size == nil {
		t.Errorf("the strip damaged the note: %+v", note)
	}
}
