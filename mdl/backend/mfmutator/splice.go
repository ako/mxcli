// SPDX-License-Identifier: Apache-2.0

package mfmutator

// # The graph splice (plan item 4.2b)
//
// An `alter microflow` operation edits the STORED document, never a rebuild of
// it. The rebuild (`UpdateMicroflow`, then re-pairing IDs by type and position)
// is what deleted merges, reset curves and moved $IDs onto other nodes on a
// Studio Pro-authored flow (ADR-0012, Context). So the splice works on the raw
// BSON tree of the unit as read from storage:
//
//   - the fragment's objects are the only elements it builds, and they are
//     appended to the collection that holds the target;
//   - the sequence flows around the target are rewired by changing their
//     pointers, connection sides and the control vector at the rewired end —
//     every other property of a rewired flow (its $ID, case value, error-handler
//     flag, the curve at its other end) is kept;
//   - nodes are moved only to make room, and only by a translation;
//   - nothing else is touched, and no $ID is ever rewritten. A removed element
//     leaves no reference behind: Save refuses a document in which any binary
//     still names an element that was removed (CLAUDE.md rule 1), and one in
//     which two elements share an $ID.
//
// What the splice cannot do safely it refuses, with the reason: an insert
// after a decision (which branch?), before an activity several flows enter
// (which path?), a drop or replace of an activity with an error handler (the
// handler would be orphaned), and — for now — anything inside a loop body
// but an activity replaced by one of the same kind, in place, since a body
// node's coordinates are relative to the loop box (ako/mxcli#1106).

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Deps supplies the engine-specific steps: turning a new object or flow into
// its stored BSON form, and writing the unit back. Everything between — which
// flows to rewire, where to place, what to remove — is decided here, once, for
// every storage engine.
type Deps interface {
	SerializeObject(obj microflows.MicroflowObject) (bson.D, error)
	SerializeSequenceFlow(f *microflows.SequenceFlow) (bson.D, error)
	SerializeAnnotationFlow(f *microflows.AnnotationFlow) (bson.D, error)
	// SerializeDocument encodes a whole declared document — a
	// *microflows.Microflow or *microflows.Nanoflow — exactly as the rebuild
	// would write it, with every element given an $ID. SetHeader copies its
	// header properties and parameters from it.
	SerializeDocument(declared any) (bson.D, error)
	// SaveUnit writes the patched unit. The implementation must go through the
	// storage engine's reconciling write (canon.Reconcile), like every write.
	SaveUnit(unitID string, contents []byte) error
}

// Mutator splices fragments into one microflow or nanoflow unit.
type Mutator struct {
	deps   Deps
	unitID model.ID
	doc    bson.D
	// removed holds the $IDs of every element an operation took out, so Save
	// can prove that nothing still points at one of them.
	removed map[string]bool
	// pinned are the points the script states a node is drawn at (Pin).
	pinned map[point]bool
}

var _ backend.MicroflowMutator = (*Mutator)(nil)

// New returns a Mutator over a decoded Microflows$Microflow or
// Microflows$Nanoflow unit.
func New(doc bson.D, unitID model.ID, deps Deps) (*Mutator, error) {
	switch t := dString(doc, "$Type"); t {
	case "Microflows$Microflow", "Microflows$Nanoflow":
	default:
		return nil, fmt.Errorf("unit %s is a %s, not a microflow or nanoflow", unitID, t)
	}
	if dDoc(doc, "ObjectCollection") == nil {
		return nil, fmt.Errorf("unit %s has no object collection", unitID)
	}
	return &Mutator{deps: deps, unitID: unitID, doc: doc, removed: map[string]bool{}}, nil
}

// Bytes returns the patched unit, after the integrity checks Save applies.
func (m *Mutator) Bytes() ([]byte, error) {
	if err := m.checkIntegrity(); err != nil {
		return nil, err
	}
	return bson.Marshal(m.doc)
}

// Save writes the patched unit through Deps.
func (m *Mutator) Save() error {
	out, err := m.Bytes()
	if err != nil {
		return err
	}
	return m.deps.SaveUnit(string(m.unitID), out)
}

// ---------------------------------------------------------------------------
// The graph view
// ---------------------------------------------------------------------------

// node is one object of the flow as stored.
type node struct {
	id   string // model ID form (types.BlobToUUID of the $ID)
	typ  string
	doc  bson.D
	pos  point
	size point
	// loop is the $ID of the loop whose body holds the node, "" at top level.
	loop string
}

// flowRef is one entry of the unit's Flows list.
type flowRef struct {
	id     string
	typ    string
	doc    bson.D
	origin string
	dest   string
	isErr  bool
}

func (f flowRef) isSequence() bool { return f.typ == "Microflows$SequenceFlow" }

type graph struct {
	nodes map[string]*node
	// order is the nodes in storage order, so a shift visits them the same
	// way every run.
	order []*node
	flows []flowRef
}

func (m *Mutator) graph() *graph {
	g := &graph{nodes: map[string]*node{}}
	var walk func(oc bson.D, loop string)
	walk = func(oc bson.D, loop string) {
		for _, el := range arrayElements(dGet(oc, "Objects")) {
			d, ok := el.(bson.D)
			if !ok {
				continue
			}
			n := &node{
				id:   binaryID(dGet(d, "$ID")),
				typ:  dString(d, "$Type"),
				doc:  d,
				pos:  parsePoint(dString(d, "RelativeMiddlePoint")),
				size: parsePoint(dString(d, "Size")),
				loop: loop,
			}
			if n.id != "" {
				g.nodes[n.id] = n
				g.order = append(g.order, n)
			}
			if n.typ == "Microflows$LoopedActivity" {
				if inner := dDoc(d, "ObjectCollection"); inner != nil {
					walk(inner, n.id)
				}
			}
		}
	}
	walk(dDoc(m.doc, "ObjectCollection"), "")
	for _, el := range arrayElements(dGet(m.doc, "Flows")) {
		d, ok := el.(bson.D)
		if !ok {
			continue
		}
		isErr, _ := dGet(d, "IsErrorHandler").(bool)
		g.flows = append(g.flows, flowRef{
			id:     binaryID(dGet(d, "$ID")),
			typ:    dString(d, "$Type"),
			doc:    d,
			origin: binaryID(dGet(d, "OriginPointer")),
			dest:   binaryID(dGet(d, "DestinationPointer")),
			isErr:  isErr,
		})
	}
	return g
}

// outgoing returns the sequence flows leaving id, split into normal and
// error-handler flows.
func (g *graph) outgoing(id string) (normal, errs []flowRef) {
	for _, f := range g.flows {
		if f.isSequence() && f.origin == id {
			if f.isErr {
				errs = append(errs, f)
			} else {
				normal = append(normal, f)
			}
		}
	}
	return normal, errs
}

// incoming returns the sequence flows entering id.
func (g *graph) incoming(id string) []flowRef {
	var out []flowRef
	for _, f := range g.flows {
		if f.isSequence() && f.dest == id {
			out = append(out, f)
		}
	}
	return out
}

// annotationFlows returns the annotation flows attached to id.
func (g *graph) annotationFlows(id string) []flowRef {
	var out []flowRef
	for _, f := range g.flows {
		if !f.isSequence() && (f.origin == id || f.dest == id) {
			out = append(out, f)
		}
	}
	return out
}

func (g *graph) node(id model.ID) (*node, error) {
	n, ok := g.nodes[string(id)]
	if !ok {
		return nil, fmt.Errorf("activity %s is not in the stored flow (was it dropped by an earlier operation?)", id)
	}
	if n.loop != "" {
		// create or modify does not edit inside a loop either, so the
		// advice is the one operation that makes the change.
		return nil, fmt.Errorf("%s is inside a loop body; alter does not splice inside a loop yet — "+
			"replace the whole loop instead: `replace loop … with begin loop … begin … end loop; end;`, "+
			"addressing the loop by its handle from `describe … with handles`", describeNode(n))
	}
	return n, nil
}

// ---------------------------------------------------------------------------
// Operations
// ---------------------------------------------------------------------------

// InsertAfter splices frag onto the flow leaving target.
func (m *Mutator) InsertAfter(target model.ID, frag *backend.MicroflowFragment) error {
	g := m.graph()
	x, err := g.node(target)
	if err != nil {
		return err
	}
	normal, _ := g.outgoing(x.id)
	switch {
	case len(normal) == 0:
		return fmt.Errorf("nothing follows %s: it ends the flow; insert before it instead", describeNode(x))
	case len(normal) > 1:
		return fmt.Errorf("%s has %d outgoing flows (it is a decision): insert after it would not say which branch; "+
			"insert before the first activity of the branch instead", describeNode(x), len(normal))
	}
	return m.spliceOnFlow(g, normal[0], frag)
}

// InsertBefore splices frag onto the flow entering target.
func (m *Mutator) InsertBefore(target model.ID, frag *backend.MicroflowFragment) error {
	g := m.graph()
	y, err := g.node(target)
	if err != nil {
		return err
	}
	in := g.incoming(y.id)
	switch {
	case len(in) == 0:
		return fmt.Errorf("no flow enters %s; there is no path to insert on", describeNode(y))
	case len(in) > 1:
		return fmt.Errorf("%d flows enter %s: insert before it would not say on which path; "+
			"insert after one of its predecessors instead", len(in), describeNode(y))
	}
	return m.spliceOnFlow(g, in[0], frag)
}

// spliceOnFlow puts frag on flow f (origin X, destination Y): f keeps its
// origin end and now enters the fragment's entry; a new flow leaves the
// fragment's exit and enters Y where f used to.
func (m *Mutator) spliceOnFlow(g *graph, f flowRef, frag *backend.MicroflowFragment) error {
	x, y := g.nodes[f.origin], g.nodes[f.dest]
	if x == nil || y == nil {
		return fmt.Errorf("flow %s points at an object that is not in the flow", f.id)
	}
	if x.loop != "" || y.loop != "" {
		return fmt.Errorf("the flow runs inside a loop body; alter does not splice inside a loop yet")
	}
	fb, err := fragmentGeometry(frag)
	if err != nil {
		return err
	}
	var entrySide, exitSide int
	if frag.Placed {
		// The script stated where the fragment goes: it stays where the
		// builder put it, nothing is moved to make room, and each new end
		// faces the node it connects to.
		if entrySide, exitSide, err = placedSides(frag, x, y); err != nil {
			return err
		}
	} else if entrySide, exitSide, err = m.makeRoom(g, fb, f, x, y); err != nil {
		return err
	}
	destIdx, destVec := flowEnd(f.doc, "Destination")
	if err := m.addFragment(frag); err != nil {
		return err
	}
	// f now enters the fragment.
	setPointer(f.doc, "DestinationPointer", frag.Entry)
	setInt(f.doc, "DestinationConnectionIndex", entrySide)
	setVector(f.doc, "DestinationControlVector", sideVector(entrySide))
	// And a new flow carries on from the fragment to Y.
	return m.addFlow(&microflows.SequenceFlow{
		BaseElement:                model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:                   frag.Exit,
		DestinationID:              model.ID(y.id),
		OriginConnectionIndex:      exitSide,
		DestinationConnectionIndex: destIdx,
		OriginControlVector:        sideVector(exitSide),
		DestinationControlVector:   destVec,
		CaseValue:                  frag.ExitCase,
	})
}

// makeRoom places the fragment in the gap between x and y, moving everything
// past the gap along when it does not fit, and returns the sides its new ends
// connect at.
func (m *Mutator) makeRoom(g *graph, fb *fragmentBox, f flowRef, x, y *node) (entrySide, exitSide int, err error) {
	ax, s, err := flowAxis(x, y)
	if err != nil {
		return 0, 0, err
	}

	// Make room: the fragment plus a gap on either side has to fit between
	// X's and Y's facing edges; what is missing is added by moving everything
	// past the midpoint of that gap further along the axis.
	a := x.pos.get(ax) + s*x.size.get(ax)/2
	b := y.pos.get(ax) - s*y.size.get(ax)/2
	need := fb.length(ax) + 2*minGap
	if deficit := need - s*(b-a); deficit > 0 {
		cut := float64(a+b) / 2
		if p := m.pinnedPast(g, ax, s, cut, ""); p != nil {
			blocked := ", and making it would move " + describeNode(p) + ","
			if p == y {
				blocked = ","
			}
			return 0, 0, fmt.Errorf("the inserted statements need %dpx more room than there is between %s and %s%s "+
				"which the script draws where it is; %s", deficit, describeNode(x), describeNode(y), blocked, roomAdvice)
		}
		m.shift(g, ax, s, cut, s*deficit, "")
		b += s * deficit
	}

	// Place the fragment centred in the gap, its entry level with the flow.
	cross := ax.other()
	mid := point{}
	mid = mid.set(ax, (a+b)/2)
	mid = mid.set(cross, (x.pos.get(cross)+y.pos.get(cross))/2)
	fb.placeCentred(ax, mid)
	if err := fb.checkRoom(g, "", f.id); err != nil {
		return 0, 0, err
	}
	return side(ax, -s), side(ax, s), nil
}

// placedSides returns the sides a placed fragment's new flow ends connect at:
// its entry's side facing x, which the flow into it comes from, and its exit's
// side facing y, which the flow out of it goes to.
func placedSides(frag *backend.MicroflowFragment, x, y *node) (entrySide, exitSide int, err error) {
	entry, exit := fragmentNode(frag, frag.Entry), fragmentNode(frag, frag.Exit)
	if entry == nil || exit == nil {
		return 0, 0, fmt.Errorf("the fragment's entry or exit is not one of its objects")
	}
	ax, s, err := flowAxis(x, entry)
	if err != nil {
		return 0, 0, err
	}
	ax2, s2, err := flowAxis(exit, y)
	if err != nil {
		return 0, 0, err
	}
	return side(ax, -s), side(ax2, s2), nil
}

// fragmentNode is a fragment object seen as a node of the graph, for geometry.
func fragmentNode(frag *backend.MicroflowFragment, id model.ID) *node {
	for _, obj := range frag.Objects {
		if obj.GetID() == id {
			p := obj.GetPosition()
			return &node{id: string(id), typ: strings.TrimPrefix(fmt.Sprintf("%T", obj), "*microflows."),
				pos: point{p.X, p.Y}, size: objectSize(obj)}
		}
	}
	return nil
}

// Replace puts frag where target is: every flow that entered target enters
// the fragment, the flow that left it leaves the fragment's exit, and
// annotations attached to target are attached to the fragment's entry.
func (m *Mutator) Replace(target model.ID, frag *backend.MicroflowFragment) error {
	g := m.graph()
	if n, ok := g.nodes[string(target)]; ok && n.loop != "" {
		return m.replaceInLoop(g, n, frag)
	}
	x, err := g.node(target)
	if err != nil {
		return err
	}
	out, err := removable(g, x, "replace")
	if err != nil {
		return err
	}
	y := g.nodes[out.dest]
	if y == nil {
		return fmt.Errorf("flow %s points at an object that is not in the flow", out.id)
	}
	fb, err := fragmentGeometry(frag)
	if err != nil {
		return err
	}
	// A placed fragment stays where the script stated (ako/mxcli#818).
	if !frag.Placed {
		ax, s, err := flowAxis(x, y)
		if err != nil {
			return err
		}
		// The fragment starts where target started; if it is longer, everything
		// past target moves along by the difference.
		near := x.pos.get(ax) - s*x.size.get(ax)/2
		if extra := fb.length(ax) - x.size.get(ax); extra > 0 {
			cut := float64(x.pos.get(ax))
			if p := m.pinnedPast(g, ax, s, cut, x.id); p != nil {
				return fmt.Errorf("the replacing statements are %dpx longer than %s, and making room for them would move %s, "+
					"which the script draws where it is; %s", extra, describeNode(x), describeNode(p), roomAdvice)
			}
			m.shift(g, ax, s, cut, s*extra, x.id)
		}
		centre := point{}
		centre = centre.set(ax, near+s*fb.length(ax)/2)
		centre = centre.set(ax.other(), x.pos.get(ax.other()))
		fb.placeCentred(ax, centre)
		if err := fb.checkRoom(g, x.id, ""); err != nil {
			return err
		}
	}

	if err := m.addFragment(frag); err != nil {
		return err
	}
	for _, in := range g.incoming(x.id) {
		setPointer(in.doc, "DestinationPointer", frag.Entry)
	}
	setPointer(out.doc, "OriginPointer", frag.Exit)
	if frag.ExitCase != nil {
		if err := m.setCase(out.doc, frag.ExitCase); err != nil {
			return err
		}
	}
	for _, af := range g.annotationFlows(x.id) {
		key := "DestinationPointer"
		if af.origin == x.id {
			key = "OriginPointer"
		}
		setPointer(af.doc, key, frag.Entry)
	}
	m.removeFlows(g.bodyFlows(x.id))
	return m.removeObject(x)
}

// replaceInLoop replaces an activity inside a loop body by one activity of
// the same kind, in place (ako/mxcli#1106): the new activity takes the stored
// one's place in the loop's collection, is drawn where it was at the size it
// was, and the flows that entered and left it now enter and leave the new one.
// Nothing else in the loop moves, so no layout is needed — which is why this
// is the one edit inside a loop body the splice makes. A body node's
// coordinates are relative to the loop box, so anything that adds, removes or
// redraws a node there would have to re-lay the body out; that is refused.
func (m *Mutator) replaceInLoop(g *graph, x *node, frag *backend.MicroflowFragment) error {
	if frag == nil || len(frag.Objects) != 1 || len(frag.Flows) > 0 || len(frag.AnnotationFlows) > 0 ||
		frag.ExitCase != nil || frag.Entry != frag.Objects[0].GetID() || frag.Exit != frag.Entry {
		return fmt.Errorf("%s is inside a loop body, where the splice replaces an activity by one activity "+
			"only (nothing in the body is added, removed or redrawn); replace the whole loop instead", describeNode(x))
	}
	if x.typ != "Microflows$ActionActivity" {
		return fmt.Errorf("cannot replace %s in place: only an activity can be", describeNode(x))
	}
	// The body's last activity ends it, with no flow out.
	normal, errs := g.outgoing(x.id)
	if len(errs) > 0 {
		return fmt.Errorf("cannot replace %s: it has an error handler, which would be left with no activity; "+
			"replace the whole loop instead", describeNode(x))
	}
	if len(normal) > 1 {
		return fmt.Errorf("cannot replace %s: it has %d outgoing flows, not one", describeNode(x), len(normal))
	}
	loop := g.nodes[x.loop]
	if loop == nil {
		return fmt.Errorf("%s is in a loop that is not in the flow", describeNode(x))
	}
	oc := dDoc(loop.doc, "ObjectCollection")
	obj := frag.Objects[0]
	obj.SetPosition(model.Point{X: x.pos.X, Y: x.pos.Y})
	d, err := m.deps.SerializeObject(obj)
	if err != nil {
		return fmt.Errorf("serialize %T: %w", obj, err)
	}
	if d == nil {
		return fmt.Errorf("cannot write a %T into a flow", obj)
	}
	if t := dString(d, "$Type"); t != x.typ {
		return fmt.Errorf("%s is inside a loop body, where the splice replaces an activity by one of the same "+
			"kind only, not by a %s; replace the whole loop instead", describeNode(x), t)
	}
	// Drawn as the stored one is: a Studio Pro activity may have been resized.
	dSet(d, "RelativeMiddlePoint", x.pos.String())
	dSet(d, "Size", x.size.String())
	objs := arrayElements(dGet(oc, "Objects"))
	found := false
	for i, el := range objs {
		if od, ok := el.(bson.D); ok && binaryID(dGet(od, "$ID")) == x.id {
			objs[i], found = d, true
			break
		}
	}
	if !found || !setArray(oc, "Objects", objs) {
		return fmt.Errorf("%s is not in its loop's collection", describeNode(x))
	}
	m.markRemoved(x.doc)
	for _, in := range g.incoming(x.id) {
		setPointer(in.doc, "DestinationPointer", frag.Entry)
	}
	for _, out := range normal {
		setPointer(out.doc, "OriginPointer", frag.Exit)
	}
	for _, af := range g.annotationFlows(x.id) {
		key := "DestinationPointer"
		if af.origin == x.id {
			key = "OriginPointer"
		}
		setPointer(af.doc, key, frag.Entry)
	}
	return nil
}

// setCase gives a stored flow the case value cv, in the form the engine
// writes it: the case properties of a flow serialized with cv replace the
// stored ones, and the flow keeps its $ID and everything else. The stored
// case element (a NoCase) is taken out with them.
func (m *Mutator) setCase(d bson.D, cv microflows.CaseValue) error {
	tmp, err := m.deps.SerializeSequenceFlow(&microflows.SequenceFlow{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		CaseValue:   cv,
	})
	if err != nil {
		return fmt.Errorf("serialize case value: %w", err)
	}
	set := false
	for _, key := range []string{"CaseValues", "NewCaseValue"} {
		v := dGet(tmp, key)
		if v == nil || dGet(d, key) == nil {
			continue
		}
		m.markRemoved(dGet(d, key))
		dSet(d, key, v)
		set = true
	}
	if !set {
		return fmt.Errorf("the flow leaving the fragment needs a case value, and the stored flow %s has nowhere to keep one",
			binaryID(dGet(d, "$ID")))
	}
	return nil
}

// RemoveNotes takes out the annotations attached to target, with their
// lines. A note also attached to another object is refused: it would be taken
// from that object too.
func (m *Mutator) RemoveNotes(target model.ID) error {
	g := m.graph()
	x, err := g.node(target)
	if err != nil {
		return err
	}
	drop, seen := map[string]bool{}, map[string]bool{}
	var notes []*node
	for _, af := range g.annotationFlows(x.id) {
		other := af.origin
		if other == x.id {
			other = af.dest
		}
		n := g.nodes[other]
		if n == nil || n.typ != "Microflows$Annotation" {
			return fmt.Errorf("the annotation line %s of %s does not lead to an annotation", af.id, describeNode(x))
		}
		for _, f := range g.annotationFlows(n.id) {
			if f.origin != x.id && f.dest != x.id {
				return fmt.Errorf("a note on %s is also attached to another object; changing it here would change it there too — "+
					"edit that note in Studio Pro", describeNode(x))
			}
			drop[f.id] = true
		}
		if !seen[n.id] {
			seen[n.id] = true
			notes = append(notes, n)
		}
	}
	m.removeFlows(drop)
	for _, n := range notes {
		if err := m.removeObject(n); err != nil {
			return err
		}
	}
	return nil
}

// Drop removes target and joins the flows that entered it to the object it
// led to. Annotation lines attached to it go with it; the annotations stay.
func (m *Mutator) Drop(target model.ID) error {
	g := m.graph()
	x, err := g.node(target)
	if err != nil {
		return err
	}
	out, err := removable(g, x, "drop")
	if err != nil {
		return err
	}
	destIdx, destVec := flowEnd(out.doc, "Destination")
	destPtr := dGet(out.doc, "DestinationPointer")
	for _, in := range g.incoming(x.id) {
		if in.origin == out.dest {
			return fmt.Errorf("dropping %s would leave a flow from an object to itself", describeNode(x))
		}
	}
	for _, in := range g.incoming(x.id) {
		setBinary(in.doc, "DestinationPointer", destPtr)
		setInt(in.doc, "DestinationConnectionIndex", destIdx)
		setVector(in.doc, "DestinationControlVector", destVec)
	}
	drop := g.bodyFlows(x.id)
	drop[out.id] = true
	for _, af := range g.annotationFlows(x.id) {
		drop[af.id] = true
	}
	m.removeFlows(drop)
	return m.removeObject(x)
}

// SetReturnValue sets the value an end event returns, in place: the end event
// keeps its $ID, its position, the flows into it and the notes attached to
// it, and no other element changes. It is how a changed `return` is written
// (ako/mxcli#805): an end event cannot be dropped or replaced like an activity,
// because it ends a path, but its value is a property like an activity's
// caption.
func (m *Mutator) SetReturnValue(target model.ID, value string) error {
	g := m.graph()
	x, err := g.node(target)
	if err != nil {
		return err
	}
	if x.typ != "Microflows$EndEvent" {
		return fmt.Errorf("cannot set the return value of %s: only an end event returns one", describeNode(x))
	}
	if !dSet(x.doc, "ReturnValue", value) {
		return fmt.Errorf("%s stores no ReturnValue", describeNode(x))
	}
	return nil
}

// SetCondition sets the expression a decision branches on, and its caption,
// in place (ako/mxcli#888): the decision keeps its $ID, its flows and the case
// values on them, so both of its paths stay as drawn. Only an expression
// decision has an expression to set; one that calls a rule is refused, since
// turning it into an expression (or back) replaces its condition element.
func (m *Mutator) SetCondition(target model.ID, expression, caption string) error {
	g := m.graph()
	x, err := g.node(target)
	if err != nil {
		return err
	}
	if x.typ != "Microflows$ExclusiveSplit" {
		return fmt.Errorf("cannot set the condition of %s: only a decision has one", describeNode(x))
	}
	cond := dDoc(x.doc, "SplitCondition")
	if cond == nil || dString(cond, "$Type") != "Microflows$ExpressionSplitCondition" {
		return fmt.Errorf("cannot set the condition of %s: it calls a rule, and the splice sets an expression's text only", describeNode(x))
	}
	if !dSet(cond, "Expression", expression) {
		return fmt.Errorf("%s stores no Expression", describeNode(x))
	}
	if !dSet(x.doc, "Caption", caption) {
		return fmt.Errorf("%s stores no Caption", describeNode(x))
	}
	return nil
}

// Move sets where target is drawn (ako/mxcli#818): a stated @position or
// @start that differs from the stored one. Only the node's RelativeMiddlePoint
// changes. Its flows keep their ends, sides and control vectors — a control
// vector is relative to its end, so a curve keeps its shape — and no other
// node moves. A node inside a loop body is refused: its coordinates are
// relative to the loop box, and alter does not edit inside a loop yet.
func (m *Mutator) Move(target model.ID, to model.Point) error {
	g := m.graph()
	x, err := g.node(target)
	if err != nil {
		return err
	}
	if !dSet(x.doc, "RelativeMiddlePoint", point{to.X, to.Y}.String()) {
		return fmt.Errorf("%s stores no position", describeNode(x))
	}
	return nil
}

// Pin records that the script states a node is drawn at at
// (mendixlabs/mxcli#1598, case 6). A point rather than a node: the node there
// may be one a later fragment places, at the position its statement states.
func (m *Mutator) Pin(at model.Point) error {
	if m.pinned == nil {
		m.pinned = map[point]bool{}
	}
	m.pinned[point{at.X, at.Y}] = true
	return nil
}

// bodyFlows returns the flows that run inside loop's body, at any depth. They
// are stored in the unit's Flows list, not in the loop, so taking the loop out
// has to take them too; left behind they would point at removed objects.
func (g *graph) bodyFlows(loop string) map[string]bool {
	inside := func(id string) bool {
		for n := g.nodes[id]; n != nil && n.loop != ""; n = g.nodes[n.loop] {
			if n.loop == loop {
				return true
			}
		}
		return false
	}
	out := map[string]bool{}
	for _, f := range g.flows {
		if inside(f.origin) || inside(f.dest) {
			out[f.id] = true
		}
	}
	return out
}

// removable checks that x can be taken out of the flow and returns the one
// flow that leaves it.
func removable(g *graph, x *node, verb string) (flowRef, error) {
	switch x.typ {
	case "Microflows$ActionActivity", "Microflows$LoopedActivity":
	default:
		return flowRef{}, fmt.Errorf("cannot %s %s: only an activity or a loop can be, because a decision or an end event "+
			"changes the shape of the flow", verb, describeNode(x))
	}
	normal, errs := g.outgoing(x.id)
	if len(errs) > 0 {
		return flowRef{}, fmt.Errorf("cannot %s %s: it has an error handler, which would be left with no activity; "+
			"rewrite the flow with create or modify instead", verb, describeNode(x))
	}
	if len(normal) != 1 {
		return flowRef{}, fmt.Errorf("cannot %s %s: it has %d outgoing flows, not one", verb, describeNode(x), len(normal))
	}
	return normal[0], nil
}

// ---------------------------------------------------------------------------
// Tree edits
// ---------------------------------------------------------------------------

// addFragment serializes the fragment and appends it: its objects to the top
// level collection (appended, so no stored object changes list position), its
// flows to the Flows list.
func (m *Mutator) addFragment(frag *backend.MicroflowFragment) error {
	oc := dDoc(m.doc, "ObjectCollection")
	objs := arrayElements(dGet(oc, "Objects"))
	for _, obj := range frag.Objects {
		d, err := m.deps.SerializeObject(obj)
		if err != nil {
			return fmt.Errorf("serialize %T: %w", obj, err)
		}
		if d == nil {
			return fmt.Errorf("cannot write a %T into a flow", obj)
		}
		objs = append(objs, d)
	}
	if !setArray(oc, "Objects", objs) {
		return fmt.Errorf("the object collection has no Objects list")
	}
	for _, f := range frag.Flows {
		if err := m.addFlow(f); err != nil {
			return err
		}
	}
	for _, af := range frag.AnnotationFlows {
		d, err := m.deps.SerializeAnnotationFlow(af)
		if err != nil {
			return fmt.Errorf("serialize annotation flow: %w", err)
		}
		if err := m.appendFlowDoc(d); err != nil {
			return err
		}
	}
	return nil
}

func (m *Mutator) addFlow(f *microflows.SequenceFlow) error {
	d, err := m.deps.SerializeSequenceFlow(f)
	if err != nil {
		return fmt.Errorf("serialize sequence flow: %w", err)
	}
	return m.appendFlowDoc(d)
}

func (m *Mutator) appendFlowDoc(d bson.D) error {
	if d == nil {
		return fmt.Errorf("a flow serialized to nothing")
	}
	flows := arrayElements(dGet(m.doc, "Flows"))
	flows = append(flows, d)
	if !setArray(m.doc, "Flows", flows) {
		return fmt.Errorf("the unit has no Flows list")
	}
	return nil
}

func (m *Mutator) removeFlows(ids map[string]bool) {
	var keep []any
	for _, el := range arrayElements(dGet(m.doc, "Flows")) {
		if d, ok := el.(bson.D); ok && ids[binaryID(dGet(d, "$ID"))] {
			m.markRemoved(d)
			continue
		}
		keep = append(keep, el)
	}
	setArray(m.doc, "Flows", keep)
}

// removeObject takes x out of the top-level collection.
func (m *Mutator) removeObject(x *node) error {
	oc := dDoc(m.doc, "ObjectCollection")
	var keep []any
	found := false
	for _, el := range arrayElements(dGet(oc, "Objects")) {
		if d, ok := el.(bson.D); ok && binaryID(dGet(d, "$ID")) == x.id {
			m.markRemoved(d)
			found = true
			continue
		}
		keep = append(keep, el)
	}
	if !found {
		return fmt.Errorf("%s is not in the top-level collection", describeNode(x))
	}
	setArray(oc, "Objects", keep)
	return nil
}

// markRemoved records the $ID of d and of every element nested in it.
func (m *Mutator) markRemoved(v any) {
	switch t := v.(type) {
	case bson.D:
		for _, e := range t {
			if e.Key == "$ID" {
				if b, ok := e.Value.(bson.Binary); ok {
					m.removed[string(b.Data)] = true
				}
				continue
			}
			m.markRemoved(e.Value)
		}
	case bson.A:
		for _, el := range t {
			m.markRemoved(el)
		}
	}
}

// shift moves every top-level node past the cut (along axis ax, in the
// direction of s) by delta, but the parameters (shifted). The sweep keeps
// everything on either side of the cut exactly as it was relative to its
// neighbours, so only the flows that cross the cut get longer; a flow's
// control vectors are relative to its ends, so no curve changes. skip is a
// node that is about to be removed.
func (m *Mutator) shift(g *graph, ax axis, s int, cut float64, delta int, skip string) {
	for _, n := range g.order {
		if !shifted(n, ax, s, cut, skip) {
			continue
		}
		n.pos = n.pos.set(ax, n.pos.get(ax)+delta)
		dSet(n.doc, "RelativeMiddlePoint", n.pos.String())
	}
}

// shifted reports whether shift moves n: a top-level node past the cut that
// is not a parameter. A parameter is not on the line the room is made on: it
// belongs to the flow as a whole, and Studio Pro authors draw it above the
// start event (63 of 71 in PedApp and JTSBootLogboek at the start's x, none
// within 60px of the line). Moved, it drifted right with every insert and
// describe printed a @position the script never stated (ako/mxcli#1547).
func shifted(n *node, ax axis, s int, cut float64, skip string) bool {
	return n.loop == "" && n.id != skip && n.typ != "Microflows$MicroflowParameter" &&
		float64(s)*(float64(n.pos.get(ax))-cut) > 0
}

// pinnedPast returns the first node shift would move that is drawn at a point
// the script states (Pin), or nil. Moving it would make the stored flow say
// something other than the script — a later fragment placed at its own stated
// position would be left behind on the wrong side of a node that moved, and
// re-running the script would move the node back (mendixlabs/mxcli#1598,
// case 6) — so the caller refuses instead.
func (m *Mutator) pinnedPast(g *graph, ax axis, s int, cut float64, skip string) *node {
	for _, n := range g.order {
		if m.pinned[n.pos] && shifted(n, ax, s, cut, skip) {
			return n
		}
	}
	return nil
}

// roomAdvice is the way out of a refused shift.
const roomAdvice = "state @position on the new statements, where there is room for them, or leave unstated the positions " +
	"of what comes after them (@position, @merge, a note's position:) so that it can move along"

// checkIntegrity is the rule-1 guard: no binary anywhere in the unit may
// still name an element that was removed, and no two elements may share an
// $ID. Either would make the document unopenable, and neither is caught by
// anything cheaper than Studio Pro.
func (m *Mutator) checkIntegrity() error {
	seen := map[string]bool{}
	var dup, dangling string
	var walk func(v any, key string)
	walk = func(v any, key string) {
		switch t := v.(type) {
		case bson.D:
			for _, e := range t {
				walk(e.Value, e.Key)
			}
		case bson.A:
			for _, el := range t {
				walk(el, key)
			}
		case bson.Binary:
			if len(t.Data) != 16 {
				return
			}
			k := string(t.Data)
			if key == "$ID" {
				if seen[k] && dup == "" {
					dup = types.BlobToUUID(t.Data)
				}
				seen[k] = true
				return
			}
			if m.removed[k] && dangling == "" {
				dangling = key + " -> " + types.BlobToUUID(t.Data)
			}
		}
	}
	walk(m.doc, "")
	if dup != "" {
		return fmt.Errorf("refusing to write: two elements would share $ID %s", dup)
	}
	if dangling != "" {
		return fmt.Errorf("refusing to write: a reference still points at a removed element (%s)", dangling)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Geometry
// ---------------------------------------------------------------------------

// minGap is the free space kept on each side of an inserted fragment, the
// edge-to-edge distance mxcli's own layout leaves between activities.
const minGap = 40

type axis int

const (
	axisX axis = iota
	axisY
)

func (a axis) other() axis { return 1 - a }

type point struct{ X, Y int }

func (p point) get(a axis) int {
	if a == axisX {
		return p.X
	}
	return p.Y
}

func (p point) set(a axis, v int) point {
	if a == axisX {
		p.X = v
	} else {
		p.Y = v
	}
	return p
}

func (p point) String() string { return strconv.Itoa(p.X) + ";" + strconv.Itoa(p.Y) }

func parsePoint(s string) point {
	x, y, ok := strings.Cut(s, ";")
	if !ok {
		return point{}
	}
	px, _ := strconv.Atoi(strings.TrimSpace(x))
	py, _ := strconv.Atoi(strings.TrimSpace(y))
	return point{px, py}
}

// flowAxis is the direction a flow from x to y runs: the axis along which the
// centres are further apart, and the sign along it.
func flowAxis(x, y *node) (axis, int, error) {
	dx, dy := y.pos.X-x.pos.X, y.pos.Y-x.pos.Y
	switch {
	case dx == 0 && dy == 0:
		return 0, 0, fmt.Errorf("%s and %s are drawn on the same spot; cannot tell which way the flow runs", describeNode(x), describeNode(y))
	case abs(dx) >= abs(dy):
		return axisX, sign(dx), nil
	default:
		return axisY, sign(dy), nil
	}
}

// Connection indexes, as Mendix numbers the sides of a box.
const (
	sideTop    = 0
	sideRight  = 1
	sideBottom = 2
	sideLeft   = 3
)

// side is the side of a box that faces direction s along ax.
func side(ax axis, s int) int {
	switch {
	case ax == axisX && s > 0:
		return sideRight
	case ax == axisX:
		return sideLeft
	case s > 0:
		return sideBottom
	default:
		return sideTop
	}
}

// sideVector is the control vector Studio Pro draws a straight flow end with:
// perpendicular to the side, pointing out of the box.
func sideVector(sd int) string {
	switch sd {
	case sideTop:
		return "0;-15"
	case sideRight:
		return "15;0"
	case sideBottom:
		return "0;15"
	default:
		return "-15;0"
	}
}

// fragmentBox is the fragment's layout as the builder produced it, to be
// translated into place.
type fragmentBox struct {
	frag     *backend.MicroflowFragment
	min, max point
}

func fragmentGeometry(frag *backend.MicroflowFragment) (*fragmentBox, error) {
	if frag == nil || len(frag.Objects) == 0 || frag.Entry == "" || frag.Exit == "" {
		return nil, fmt.Errorf("the fragment is empty")
	}
	fb := &fragmentBox{frag: frag}
	first := true
	for _, obj := range frag.Objects {
		// A note sits off the line it documents, so it takes no room on it.
		// Counted, a note at a stated position 1300px away made the box that
		// wide and shifted the rest of the row by 1180px; a note wider than its
		// activity shifted it by the difference (mendixlabs/mxcli#1598).
		if _, isNote := obj.(*microflows.Annotation); isNote {
			continue
		}
		p, sz := obj.GetPosition(), objectSize(obj)
		lo := point{p.X - sz.X/2, p.Y - sz.Y/2}
		hi := point{p.X + sz.X/2, p.Y + sz.Y/2}
		if first {
			fb.min, fb.max, first = lo, hi, false
			continue
		}
		fb.min = point{min(fb.min.X, lo.X), min(fb.min.Y, lo.Y)}
		fb.max = point{max(fb.max.X, hi.X), max(fb.max.Y, hi.Y)}
	}
	return fb, nil
}

func (fb *fragmentBox) length(ax axis) int { return fb.max.get(ax) - fb.min.get(ax) }

// placeCentred translates the fragment so that its box is centred on c along
// ax, and its entry object sits on c across it.
func (fb *fragmentBox) placeCentred(ax axis, c point) {
	var entry point
	for _, obj := range fb.frag.Objects {
		if obj.GetID() == fb.frag.Entry {
			entry = point{obj.GetPosition().X, obj.GetPosition().Y}
		}
	}
	d := point{}
	d = d.set(ax, c.get(ax)-(fb.min.get(ax)+fb.max.get(ax))/2)
	d = d.set(ax.other(), c.get(ax.other())-entry.get(ax.other()))
	for _, obj := range fb.frag.Objects {
		// A stated canvas position is where the script wants the object, not a
		// place relative to the fragment.
		if fb.frag.Pinned[obj.GetID()] {
			continue
		}
		p := obj.GetPosition()
		obj.SetPosition(model.Point{X: p.X + d.X, Y: p.Y + d.Y})
	}
	fb.min = point{fb.min.X + d.X, fb.min.Y + d.Y}
	fb.max = point{fb.max.X + d.X, fb.max.Y + d.Y}
}

// checkRoom refuses a placement that would draw the fragment over an object
// that stays where it is. The shift clears the space past the cut; an object
// that already sat inside the gap (a branch drawn below the main line, say)
// is not moved, and drawing over it would hide it in Studio Pro. skip is the
// object a replace removes; onFlow the flow an insert goes on, which runs to
// the fragment's entry afterwards rather than across it.
func (fb *fragmentBox) checkRoom(g *graph, skip, onFlow string) error {
	for _, n := range g.order {
		if n.loop != "" || n.id == skip {
			continue
		}
		lo := point{n.pos.X - n.size.X/2, n.pos.Y - n.size.Y/2}
		hi := point{n.pos.X + n.size.X/2, n.pos.Y + n.size.Y/2}
		if lo.X < fb.max.X && fb.min.X < hi.X && lo.Y < fb.max.Y && fb.min.Y < hi.Y {
			return fmt.Errorf("there is no free room for the fragment: at (%d, %d)-(%d, %d) it would be drawn over %s; "+
				"move that object aside in Studio Pro first", fb.min.X, fb.min.Y, fb.max.X, fb.max.Y, describeNode(n))
		}
	}
	return fb.checkBranches(g, skip, onFlow)
}

// checkBranches refuses a placement whose return branches would be drawn
// across a stored flow (ako/mxcli#888). A return in the fragment ends its own
// path, off the line the fragment is spliced into: the builder draws its end
// event beside that line, and the flow to it leaves its decision sideways. The
// end event is checked with the rest of the fragment by checkRoom; what
// checkRoom cannot see is a stored flow running through that space — a branch
// below the line that rejoins past the gap, which the shift has just
// stretched across it. Such a flow is approximated by the straight line
// between the centres of its ends, which is how a flow between two aligned
// nodes is drawn.
func (fb *fragmentBox) checkBranches(g *graph, skip, onFlow string) error {
	objs := map[model.ID]microflows.MicroflowObject{}
	for _, obj := range fb.frag.Objects {
		objs[obj.GetID()] = obj
	}
	for _, f := range fb.frag.Flows {
		end, ok := objs[f.DestinationID].(*microflows.EndEvent)
		if !ok || objs[f.OriginID] == nil {
			continue
		}
		r := branchArea(objs[f.OriginID], end)
		for _, sf := range g.flows {
			if !sf.isSequence() || sf.id == onFlow || sf.origin == skip || sf.dest == skip {
				continue
			}
			a, b := g.nodes[sf.origin], g.nodes[sf.dest]
			if a == nil || b == nil || a.loop != "" || b.loop != "" {
				continue
			}
			if segmentCrosses(a.pos, b.pos, r) {
				p := end.GetPosition()
				return fmt.Errorf("there is no free room for the return at (%d, %d): its branch would be drawn across the "+
					"flow from %s to %s; move them aside in Studio Pro first", p.X, p.Y, describeNode(a), describeNode(b))
			}
		}
	}
	return nil
}

// branchArea is the space the path from a decision to one of its end events
// takes: from the side of the decision that faces the end event to the far
// side of the end event, across the full width between them.
func branchArea(from microflows.MicroflowObject, end *microflows.EndEvent) [2]point {
	o, e := from.GetPosition(), end.GetPosition()
	os, es := objectSize(from), objectSize(end)
	lo := point{min(o.X, e.X-es.X/2), min(o.Y, e.Y-es.Y/2)}
	hi := point{max(o.X, e.X+es.X/2), max(o.Y, e.Y+es.Y/2)}
	// Leave the decision's own box out: the flow that enters it comes in
	// along the line the fragment is spliced into.
	switch dx, dy := e.X-o.X, e.Y-o.Y; {
	case abs(dy) >= abs(dx) && dy > 0:
		lo.Y = o.Y + os.Y/2
	case abs(dy) >= abs(dx):
		hi.Y = o.Y - os.Y/2
	case dx > 0:
		lo.X = o.X + os.X/2
	default:
		hi.X = o.X - os.X/2
	}
	return [2]point{lo, hi}
}

// segmentCrosses reports whether the segment from a to b passes through the
// inside of the rectangle r (Liang-Barsky clipping).
func segmentCrosses(a, b point, r [2]point) bool {
	t0, t1 := 0.0, 1.0
	dx, dy := float64(b.X-a.X), float64(b.Y-a.Y)
	clip := func(p, q float64) bool {
		switch {
		case p == 0:
			return q > 0
		case p < 0:
			t := q / p
			if t > t1 {
				return false
			}
			t0 = max(t0, t)
		default:
			t := q / p
			if t < t0 {
				return false
			}
			t1 = min(t1, t)
		}
		return true
	}
	ax, ay := float64(a.X), float64(a.Y)
	return clip(-dx, ax-float64(r[0].X)) && clip(dx, float64(r[1].X)-ax) &&
		clip(-dy, ay-float64(r[0].Y)) && clip(dy, float64(r[1].Y)-ay) && t0 < t1
}

func objectSize(obj microflows.MicroflowObject) point {
	if s, ok := obj.(interface{ GetSize() model.Size }); ok {
		sz := s.GetSize()
		return point{sz.Width, sz.Height}
	}
	return point{}
}

// flowEnd reads a flow's connection index and control vector at one end
// ("Origin" or "Destination").
func flowEnd(d bson.D, end string) (int, string) {
	idx := 0
	switch v := dGet(d, end+"ConnectionIndex").(type) {
	case int32:
		idx = int(v)
	case int64:
		idx = int(v)
	case int:
		idx = v
	}
	vec := ""
	if line := dDoc(d, "Line"); line != nil {
		vec = dString(line, end+"ControlVector")
	}
	return idx, vec
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sign(v int) int {
	if v < 0 {
		return -1
	}
	return 1
}

func describeNode(n *node) string {
	name := strings.TrimPrefix(n.typ, "Microflows$")
	if c := dString(n.doc, "Caption"); c != "" && name != "ActionActivity" {
		name += " '" + c + "'"
	}
	return fmt.Sprintf("the %s at (%d, %d)", name, n.pos.X, n.pos.Y)
}

// ---------------------------------------------------------------------------
// bson.D helpers. The unit is decoded with bson v1 into bson.D, whose nested
// documents are bson.D and whose arrays are bson.A with Mendix's leading
// int32 list marker.
// ---------------------------------------------------------------------------

func dGet(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

func dDoc(d bson.D, key string) bson.D {
	v, _ := dGet(d, key).(bson.D)
	return v
}

func dString(d bson.D, key string) string {
	v, _ := dGet(d, key).(string)
	return v
}

func dSet(d bson.D, key string, v any) bool {
	for i := range d {
		if d[i].Key == key {
			d[i].Value = v
			return true
		}
	}
	return false
}

// arrayElements returns the elements of a Mendix list, without its marker.
func arrayElements(v any) []any {
	a, ok := v.(bson.A)
	if !ok || len(a) == 0 {
		return nil
	}
	if _, isMarker := a[0].(int32); isMarker {
		return append([]any(nil), a[1:]...)
	}
	return append([]any(nil), a...)
}

// setArray replaces a list's elements, keeping its stored marker.
func setArray(d bson.D, key string, elements []any) bool {
	a, ok := dGet(d, key).(bson.A)
	if !ok {
		return false
	}
	out := bson.A{}
	if len(a) > 0 {
		if marker, isMarker := a[0].(int32); isMarker {
			out = append(out, marker)
		}
	}
	return dSet(d, key, append(out, elements...))
}

func binaryID(v any) string {
	b, ok := v.(bson.Binary)
	if !ok || len(b.Data) != 16 {
		return ""
	}
	return types.BlobToUUID(b.Data)
}

// setPointer points key at the element id, keeping the binary subtype the
// stored pointer uses.
func setPointer(d bson.D, key string, id model.ID) {
	sub := byte(0)
	if b, ok := dGet(d, key).(bson.Binary); ok {
		sub = b.Subtype
	}
	dSet(d, key, bson.Binary{Subtype: sub, Data: types.UUIDToBlob(string(id))})
}

func setBinary(d bson.D, key string, v any) {
	if b, ok := v.(bson.Binary); ok {
		dSet(d, key, bson.Binary{Subtype: b.Subtype, Data: bytes.Clone(b.Data)})
	}
}

// setInt writes an integer property with the width it is stored in.
func setInt(d bson.D, key string, v int) {
	switch dGet(d, key).(type) {
	case int64:
		dSet(d, key, int64(v))
	default:
		dSet(d, key, int32(v))
	}
}

// setVector sets a control vector on the flow's line. A flow without a
// Bezier line (a pre-10 document) has nothing to set.
func setVector(d bson.D, key, v string) {
	if v == "" {
		return
	}
	if line := dDoc(d, "Line"); line != nil {
		dSet(line, key, v)
	}
}
