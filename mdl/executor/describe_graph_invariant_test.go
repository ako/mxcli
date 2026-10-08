// SPDX-License-Identifier: Apache-2.0

package executor

// The invariant every DESCRIBE MICROFLOW must satisfy, as a test helper:
//
//	executing the description rebuilds the stored graph — every stored
//	activity exactly once, and every stored sequence flow (loop back-edges
//	included) between the same two activities.
//
// It is checked by rebuilding the description with the real builder and
// comparing the two graphs, not by grepping the text. A text check can count
// statements, but the three failure shapes this was written for are about
// EDGES as much as nodes: a shared region printed twice (an activity twice, and
// the second copy's edges invented), a `merge … join …` left after both
// branches returned (an edge out of nothing), and a dropped loop back-edge (an
// activity whose edge goes somewhere else). Only a rebuild sees all three.
//
// Exclusive merges are contracted away on both sides: a merge has no behaviour,
// and the builder places its own wherever an `if` closes, so comparing them
// would flag every faithful description. What is compared is the graph a merge
// stands for — which activity can follow which, under which case value.
//
// The end of a loop iteration has three spellings with one meaning: a path that
// simply stops inside the body, one that stops at a merge with no way out, and
// one that reaches a ContinueEvent. Studio Pro draws all three, the builder
// picks one, so all three compare as "the path ends here". (A BreakEvent is
// NOT among them: it leaves the loop, and is compared like any activity.)
//
// Objects are identified by kind and canvas position, which DESCRIBE spells as
// @position on every object when the layout is kept in full (a test calls
// formatMicroflowActivities directly, which keeps it; the Evora sweep sets
// describeFullLayout). Identity by position is what makes a DUPLICATE visible:
// two rebuilt activities claiming one stored position are one activity printed
// twice.

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// assertDescriptionRebuildsGraph fails t when executing description would not
// rebuild stored. description is a whole `create … begin … end;` statement or
// just the body lines, in which case a header is supplied.
func assertDescriptionRebuildsGraph(t *testing.T, description string, stored *microflows.MicroflowObjectCollection) {
	t.Helper()
	if v := describedGraphViolationsFromText(description, stored); len(v) > 0 {
		t.Errorf("the description does not rebuild the stored graph:\n  %s\ndescription:\n%s",
			strings.Join(v, "\n  "), description)
	}
}

// describedGraphViolationsFromText parses and rebuilds a description and
// compares it with the stored collection. A description that does not parse is
// itself a violation.
func describedGraphViolationsFromText(description string, stored *microflows.MicroflowObjectCollection) []string {
	src := description
	if !strings.Contains(src, "create ") {
		src = "create microflow M.Described () returns Boolean\nbegin\n" + src + "\nend;"
	}
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		return []string{fmt.Sprintf("description does not parse: %v", errs[0])}
	}
	var body []ast.MicroflowStatement
	var returns *ast.MicroflowReturnType
	found := false
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			body, returns, found = s.Body, s.ReturnType, true
		case *ast.CreateNanoflowStmt:
			body, returns, found = s.Body, s.ReturnType, true
		}
		if found {
			break
		}
	}
	if !found {
		return []string{"description holds no create microflow/nanoflow statement"}
	}
	fb := &flowBuilder{
		posX:     100,
		posY:     100,
		spacing:  HorizontalSpacing,
		measurer: &layoutMeasurer{},
		varTypes: map[string]string{},
	}
	rebuilt := fb.buildFlowGraph(body, returns)
	return describedGraphViolations(stored, rebuilt)
}

// describedGraphViolations compares two collections modulo exclusive merges.
// It returns one line per discrepancy, empty when the graphs agree.
func describedGraphViolations(stored, rebuilt *microflows.MicroflowObjectCollection) []string {
	s := contractedGraphOf(stored)
	r := contractedGraphOf(rebuilt)
	var out []string

	for _, k := range unionKeys(s.nodes, r.nodes) {
		switch sn, rn := s.nodes[k], r.nodes[k]; {
		case rn == 0:
			out = append(out, "missing activity "+k)
		case rn > sn:
			out = append(out, fmt.Sprintf("activity %s described %d times, stored %d", k, rn, sn))
		case rn < sn:
			out = append(out, fmt.Sprintf("activity %s described %d times, stored %d", k, rn, sn))
		}
	}

	origins := map[string]bool{}
	for k := range s.out {
		origins[k] = true
	}
	for k := range r.out {
		origins[k] = true
	}
	keys := make([]string, 0, len(origins))
	for k := range origins {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, origin := range keys {
		se, re := s.out[origin], r.out[origin]
		if sameEdges(se, re) {
			continue
		}
		// The describer negates a split whose THEN arm is empty and swaps its
		// arms (`if not(c) then …`). That is the same graph; accept it when the
		// condition text changed, which is the only way the swap shows.
		if s.expr[origin] != r.expr[origin] && sameEdges(se, swapBooleanCases(re)) {
			continue
		}
		missing, extra := edgeDiff(se, re)
		for _, e := range missing {
			out = append(out, "missing flow "+origin+" "+e)
		}
		for _, e := range extra {
			out = append(out, "invented flow "+origin+" "+e)
		}
	}
	return out
}

type contractedGraph struct {
	nodes map[string]int      // node key → count
	out   map[string][]string // origin key → "-[case]-> dest" (sorted)
	expr  map[string]string   // split key → condition text
}

func contractedGraphOf(col *microflows.MicroflowObjectCollection) contractedGraph {
	g := contractedGraph{nodes: map[string]int{}, out: map[string][]string{}, expr: map[string]string{}}
	objects := map[model.ID]microflows.MicroflowObject{}
	var flows []*microflows.SequenceFlow
	var collect func(c *microflows.MicroflowObjectCollection)
	collect = func(c *microflows.MicroflowObjectCollection) {
		if c == nil {
			return
		}
		flows = append(flows, c.Flows...)
		for _, o := range c.Objects {
			if o == nil {
				continue
			}
			objects[o.GetID()] = o
			if loop, ok := o.(*microflows.LoopedActivity); ok {
				collect(loop.ObjectCollection)
			}
		}
	}
	collect(col)

	succ := map[model.ID][]*microflows.SequenceFlow{}
	for _, f := range flows {
		if f != nil {
			succ[f.OriginID] = append(succ[f.OriginID], f)
		}
	}
	key := func(id model.ID) string {
		o := objects[id]
		if o == nil {
			return "<dangling " + string(id) + ">"
		}
		p := o.GetPosition()
		return fmt.Sprintf("%s(%d,%d)", objectKind(o), p.X, p.Y)
	}
	// resolve follows merges to the activities they stand for. A merge with
	// no way out and a ContinueEvent both end the iteration, which is what a
	// path that simply stops does, so they resolve to nothing.
	var resolve func(id model.ID, seen map[model.ID]bool) []string
	resolve = func(id model.ID, seen map[model.ID]bool) []string {
		switch objects[id].(type) {
		case *microflows.ContinueEvent:
			return nil
		case *microflows.ExclusiveMerge:
		default:
			return []string{key(id)}
		}
		if seen[id] {
			return []string{"<merge cycle>"}
		}
		seen[id] = true
		var out []string
		for _, f := range succ[id] {
			out = append(out, resolve(f.DestinationID, seen)...)
		}
		return out
	}

	for id, o := range objects {
		switch o.(type) {
		case *microflows.ExclusiveMerge, *microflows.Annotation, *microflows.ContinueEvent:
			continue
		}
		k := key(id)
		g.nodes[k]++
		if s, ok := o.(*microflows.ExclusiveSplit); ok {
			if c, ok := s.SplitCondition.(*microflows.ExpressionSplitCondition); ok {
				g.expr[k] = c.Expression
			}
		}
		_, isSplit := o.(*microflows.ExclusiveSplit)
		_, isTypeSplit := o.(*microflows.InheritanceSplit)
		for _, f := range succ[id] {
			// A case value means something only on a decision's outgoing flow;
			// Studio Pro leaves stale ones on ordinary flows, which no
			// description can (or needs to) spell.
			label := ""
			if isSplit || isTypeSplit {
				label = caseLabel(f.CaseValue)
			}
			if f.IsErrorHandler {
				label = "error"
			}
			for _, d := range resolve(f.DestinationID, map[model.ID]bool{}) {
				g.out[k] = append(g.out[k], "-["+label+"]-> "+d)
			}
		}
		sort.Strings(g.out[k])
	}
	return g
}

func objectKind(o microflows.MicroflowObject) string {
	switch a := o.(type) {
	case *microflows.ActionActivity:
		if a.Action == nil {
			return "activity"
		}
		return strings.TrimPrefix(fmt.Sprintf("%T", a.Action), "*microflows.")
	default:
		return strings.TrimPrefix(fmt.Sprintf("%T", o), "*microflows.")
	}
}

func caseLabel(c microflows.CaseValue) string {
	switch v := c.(type) {
	case nil, *microflows.NoCase, microflows.NoCase:
		return ""
	case *microflows.ExpressionCase:
		return v.Expression
	case microflows.ExpressionCase:
		return v.Expression
	case *microflows.BooleanCase:
		return fmt.Sprint(v.Value)
	case microflows.BooleanCase:
		return fmt.Sprint(v.Value)
	case *microflows.EnumerationCase:
		return v.Value
	case microflows.EnumerationCase:
		return v.Value
	case *microflows.InheritanceCase, microflows.InheritanceCase:
		// Stored cases carry the entity by ID, rebuilt ones by name; the
		// destination already tells the arms apart.
		return "type"
	}
	return fmt.Sprintf("%T", c)
}

func swapBooleanCases(edges []string) []string {
	out := make([]string, len(edges))
	for i, e := range edges {
		switch {
		case strings.HasPrefix(e, "-[true]->"):
			out[i] = "-[false]->" + strings.TrimPrefix(e, "-[true]->")
		case strings.HasPrefix(e, "-[false]->"):
			out[i] = "-[true]->" + strings.TrimPrefix(e, "-[false]->")
		default:
			out[i] = e
		}
	}
	sort.Strings(out)
	return out
}

func sameEdges(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// edgeDiff is the multiset difference both ways.
func edgeDiff(stored, rebuilt []string) (missing, extra []string) {
	count := map[string]int{}
	for _, e := range stored {
		count[e]++
	}
	for _, e := range rebuilt {
		count[e]--
	}
	for _, e := range stored {
		if count[e] > 0 {
			missing = append(missing, e)
			count[e]--
		}
	}
	for e, n := range count {
		for ; n < 0; n++ {
			extra = append(extra, e)
		}
	}
	sort.Strings(extra)
	return missing, extra
}

func unionKeys(a, b map[string]int) []string {
	set := map[string]bool{}
	for k := range a {
		set[k] = true
	}
	for k := range b {
		set[k] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
