// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// execAlterFlow handles `alter microflow|nanoflow Module.Name { … }`: a patch
// of the stored flow (ADR-0012 decision 3), never a rebuild.
//
// Every target is resolved against the flow AS STORED, before any operation
// runs, so an address means what `describe … with handles` showed: an
// ambiguity or a miss refuses the whole statement before anything changes, and
// a later operation cannot address what an earlier one inserted.
func execAlterFlow(ctx *ExecContext, s *ast.AlterFlowStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	a, err := loadAlterFlow(ctx, s)
	if err != nil {
		return err
	}
	mut, err := a.plan(ctx)
	if err != nil {
		return err
	}
	if err := mut.Save(); err != nil {
		return mdlerrors.NewBackend("save altered "+s.Kind(), err)
	}
	fmt.Fprintf(ctx.Output, "Altered %s %s\n", s.Kind(), s.Name)
	return nil
}

// plan resolves every target of the statement against the flow as stored and
// applies its operations in memory, writing nothing: exec saves the result,
// and check reports the error exec would (ako/mxcli#876).
func (a *alterFlowContext) plan(ctx *ExecContext) (backend.MicroflowMutator, error) {
	s := a.stmt
	targets := make([]mfmutator.Candidate, len(s.Operations))
	for i, op := range s.Operations {
		c, err := mfmutator.ResolveText(a.cands, op.Target)
		if err != nil {
			return nil, mdlerrors.NewValidation(fmt.Sprintf("alter %s %s: %s %s: %v", s.Kind(), s.Name, op.Op, op.Target, err))
		}
		targets[i] = c
	}
	return a.apply(ctx, s.Operations, targets)
}

// apply opens the stored flow for splicing and applies ops, each aimed at the
// candidate at the same index of targets (resolved against the flow as stored).
// It returns the mutator unsaved: the caller saves, or discards it on error so
// nothing is written.
func (a *alterFlowContext) apply(ctx *ExecContext, ops []*ast.AlterFlowOperation, targets []mfmutator.Candidate) (backend.MicroflowMutator, error) {
	mut, err := ctx.Backend.OpenMicroflowForMutation(a.mf.ID)
	if err != nil {
		return nil, mdlerrors.NewBackend("open "+a.stmt.Kind()+" for alter", err)
	}
	if err := a.applyTo(ctx, mut, ops, targets); err != nil {
		return nil, err
	}
	return mut, nil
}

// applyTo applies ops to an open mutator, as apply does.
func (a *alterFlowContext) applyTo(ctx *ExecContext, mut backend.MicroflowMutator, ops []*ast.AlterFlowOperation, targets []mfmutator.Candidate) error {
	s := a.stmt
	for i, op := range ops {
		target := targets[i]
		fail := func(err error) error {
			return mdlerrors.NewValidation(fmt.Sprintf("alter %s %s: %s %s: %v", s.Kind(), s.Name, op.Op, op.Target, err))
		}
		// An activity inside a loop body is refused by the splice whatever
		// the operation. Say so first: the checks below see only the flow
		// around the loop, so a fragment reading the iterator would otherwise
		// be refused as reading an undeclared variable, which is not why.
		if loop := a.enclosingLoop(target.ID); loop != nil {
			return fail(fmt.Errorf("%s is inside the body of %s; alter does not splice inside a loop body. %s",
				target.Statement, loop.Statement, replaceLoopAdvice(s.Kind(), s.Name.String(), loop.Statement)))
		}
		if op.ReplaceNotes {
			// The statement states the activity's notes, so the stored ones
			// go with it rather than stay (ako/mxcli#859).
			if err := mut.RemoveNotes(target.ID); err != nil {
				return fail(err)
			}
		}
		if op.Op == ast.AlterFlowDrop {
			if err := a.checkOutputUnused(target, nil); err != nil {
				return fail(err)
			}
			if err := mut.Drop(target.ID); err != nil {
				return fail(err)
			}
			a.noteRemoved(target, nil)
			continue
		}
		if op.SetCondition {
			// An `if` whose condition changes is the stored decision with a
			// new condition, set in place (ako/mxcli#888).
			expr, caption, err := a.splitCondition(ctx, op, target)
			if err == nil {
				err = mut.SetCondition(target.ID, expr, caption)
			}
			if err != nil {
				return fail(err)
			}
			continue
		}
		if ret, ok := returnValueEdit(op, target); ok {
			// A return replacing an end event is a new value for it, set in
			// place (ako/mxcli#805): the end event, its flows and its notes stay.
			value, err := a.returnValue(ctx, ret)
			if err == nil {
				err = mut.SetReturnValue(target.ID, value)
			}
			if err != nil {
				return fail(err)
			}
			continue
		}
		frag, err := a.buildFragment(ctx, op.Body)
		if err != nil {
			return fail(err)
		}
		// A stated @position on the fragment is where it goes (ako/mxcli#818).
		frag.Placed = firstStatementPlaced(op.Body)
		if !frag.Placed && laterStatementPlaced(op.Body) {
			return fail(fmt.Errorf("a later inserted statement states @position but the first does not; the " +
				"inserted statements are placed as a whole, so state @position on the first one too, or on none"))
		}
		if err := a.checkFragmentScope(ctx, op, target, frag); err != nil {
			return fail(err)
		}
		switch op.Op {
		case ast.AlterFlowInsertAfter:
			err = mut.InsertAfter(target.ID, frag)
		case ast.AlterFlowInsertBefore:
			err = mut.InsertBefore(target.ID, frag)
		case ast.AlterFlowReplace:
			if err = a.checkOutputUnused(target, frag); err == nil {
				err = mut.Replace(target.ID, frag)
			}
		default:
			err = fmt.Errorf("unknown operation")
		}
		if err != nil {
			return fail(err)
		}
		if op.Op == ast.AlterFlowReplace {
			a.noteRemoved(target, frag)
		}
		a.noteFragment(ctx, frag)
	}
	return nil
}

// alterFlowContext is what the operations of one statement share: the stored
// flow (a nanoflow wrapped as a microflow, the way describe renders one), its
// addressable activities, and the name maps rendering needs.
type alterFlowContext struct {
	stmt           *ast.AlterFlowStmt
	mf             *microflows.Microflow
	cands          []mfmutator.Candidate
	entityNames    map[model.ID]string
	microflowNames map[model.ID]string

	// What the statement's earlier operations did to the variables, so a later
	// one is checked against the flow as it will be written, not as stored:
	// the variables their fragments declare, the ones their fragments read
	// (with who reads them), and the stored outputs they took away.
	declaredByOps map[string]bool
	readByOps     map[string][]string
	removedByOps  map[string]bool
	// removedIDs are the stored activities earlier operations took out; what
	// they read no longer counts as a use.
	removedIDs map[model.ID]bool

	// declaredVarTypes is the entity each variable holds as a full build of
	// the declared flow resolves it ("Module.Entity" or "List of
	// Module.Entity"), when the statement is a `create or modify` whose
	// declared body builds. A spliced fragment is built with it, so it writes
	// the members a full build of the same statement writes (ako/mxcli#885):
	// the stored flow alone does not say what every variable holds.
	declaredVarTypes map[string]string
}

// enclosingLoop returns the top-level loop whose body holds the stored
// object id, at any depth, or nil when id is not inside a loop. The top-level
// loop is the one to replace: the splice addresses nothing inside a loop,
// a nested loop included.
func (a *alterFlowContext) enclosingLoop(id model.ID) *mfmutator.Candidate {
	for _, obj := range a.mf.ObjectCollection.Objects {
		loop, ok := obj.(*microflows.LoopedActivity)
		if !ok || !loopHolds(loop, id) {
			continue
		}
		for i := range a.cands {
			if a.cands[i].ID == loop.ID {
				return &a.cands[i]
			}
		}
	}
	return nil
}

func loopHolds(loop *microflows.LoopedActivity, id model.ID) bool {
	if loop.ObjectCollection == nil {
		return false
	}
	for _, obj := range loop.ObjectCollection.Objects {
		if obj.GetID() == id {
			return true
		}
		if inner, ok := obj.(*microflows.LoopedActivity); ok && loopHolds(inner, id) {
			return true
		}
	}
	return false
}

// noteRemoved records that target's output is gone, unless the fragment that
// replaces it declares it again.
func (a *alterFlowContext) noteRemoved(target mfmutator.Candidate, replacement *backend.MicroflowFragment) {
	a.removedIDs[target.ID] = true
	v := target.OutputVariable
	if v == "" || fragmentDeclares(replacement, v) {
		return
	}
	a.removedByOps[v] = true
}

// noteFragment records what an inserted or replacing fragment declares and
// reads.
func (a *alterFlowContext) noteFragment(ctx *ExecContext, frag *backend.MicroflowFragment) {
	for _, obj := range frag.Objects {
		if act, ok := obj.(*microflows.ActionActivity); ok {
			if v := mfmutator.OutputVariable(act.Action); v != "" {
				a.declaredByOps[v] = true
			}
		}
		text := formatActivity(ctx, obj, a.entityNames, a.microflowNames)
		for _, m := range variableRef.FindAllStringSubmatch(withoutStringLiterals(text), -1) {
			a.readByOps[m[1]] = append(a.readByOps[m[1]], text)
		}
	}
}

func fragmentDeclares(frag *backend.MicroflowFragment, v string) bool {
	if frag == nil {
		return false
	}
	for _, obj := range frag.Objects {
		if act, ok := obj.(*microflows.ActionActivity); ok && mfmutator.OutputVariable(act.Action) == v {
			return true
		}
	}
	return false
}

func loadAlterFlow(ctx *ExecContext, s *ast.AlterFlowStmt) (*alterFlowContext, error) {
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, mdlerrors.NewBackend("build hierarchy", err)
	}
	a := &alterFlowContext{stmt: s, entityNames: getEntityNames(ctx, h),
		declaredByOps: map[string]bool{}, readByOps: map[string][]string{}, removedByOps: map[string]bool{},
		removedIDs: map[model.ID]bool{}}
	// A copy: nanoflow names are added below, and the cached map is shared.
	a.microflowNames = map[model.ID]string{}
	for id, n := range getMicroflowNames(ctx, h) {
		a.microflowNames[id] = n
	}
	inModule := func(container model.ID, name string) bool {
		return h.GetModuleName(h.FindModuleID(container)) == s.Name.Module && name == s.Name.Name
	}
	if s.Nanoflow {
		nfs, err := ctx.Backend.ListNanoflows()
		if err != nil {
			return nil, mdlerrors.NewBackend("list nanoflows", err)
		}
		for _, nf := range nfs {
			a.microflowNames[nf.ID] = h.GetQualifiedName(nf.ContainerID, nf.Name)
		}
		nf, ok := pickLive(nfs,
			func(nf *microflows.Nanoflow) bool { return inModule(nf.ContainerID, nf.Name) },
			func(nf *microflows.Nanoflow) bool { return nf.Excluded })
		if !ok {
			return nil, mdlerrors.NewNotFound("nanoflow", s.Name.String())
		}
		a.mf = &microflows.Microflow{
			BaseElement:        nf.BaseElement,
			ContainerID:        nf.ContainerID,
			Name:               nf.Name,
			Parameters:         nf.Parameters,
			ReturnType:         nf.ReturnType,
			ReturnVariableName: nf.ReturnVariableName,
			ObjectCollection:   nf.ObjectCollection,
		}
	} else {
		mfs, err := microflowsNamed(ctx, s.Name.Name)
		if err != nil {
			return nil, mdlerrors.NewBackend("list microflows", err)
		}
		mf, ok := pickLive(mfs,
			func(mf *microflows.Microflow) bool { return inModule(mf.ContainerID, mf.Name) },
			func(mf *microflows.Microflow) bool { return mf.Excluded })
		if !ok {
			return nil, mdlerrors.NewNotFound("microflow", s.Name.String())
		}
		a.mf = mf
	}
	if a.mf.ObjectCollection == nil {
		return nil, mdlerrors.NewValidation(fmt.Sprintf("%s %s has no flow to alter", s.Kind(), s.Name))
	}
	a.cands, _, _, _ = microflowTargets(ctx, a.mf, a.entityNames, a.microflowNames)
	return a, nil
}

// returnValueEdit reports whether op replaces an end event with a single
// return, which is an edit of the value the end event returns rather than a
// fragment: an end event ends its path, so nothing could lead on from a
// fragment put in its place.
func returnValueEdit(op *ast.AlterFlowOperation, target mfmutator.Candidate) (*ast.ReturnStmt, bool) {
	if op.Op != ast.AlterFlowReplace || len(op.Body) != 1 {
		return nil, false
	}
	if _, ok := target.Object.(*microflows.EndEvent); !ok {
		return nil, false
	}
	ret, ok := op.Body[0].(*ast.ReturnStmt)
	return ret, ok
}

// returnValue renders a return's value as the expression the end event
// stores, the way the builder writes it for `create microflow`.
func (a *alterFlowContext) returnValue(ctx *ExecContext, ret *ast.ReturnStmt) (string, error) {
	if ret.Value == nil {
		return "", nil
	}
	fb := a.fragmentBuilder(ctx)
	value := fb.exprToString(ret.Value)
	if errs := fb.GetErrors(); len(errs) > 0 {
		return "", fmt.Errorf("the return value has errors:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return value, nil
}

// splitCondition renders the condition of a SetCondition operation as the
// expression a decision stores, and the caption the decision gets with it: a
// caption that read as the old condition (the builder's default, which
// describe leaves out) reads as the new one, and any other is kept — the
// declared `if` states the same @caption as the stored one, or the operation
// would not be a condition edit.
func (a *alterFlowContext) splitCondition(ctx *ExecContext, op *ast.AlterFlowOperation, target mfmutator.Candidate) (expr, caption string, err error) {
	split, ok := target.Object.(*microflows.ExclusiveSplit)
	if !ok || len(op.Body) != 1 {
		return "", "", fmt.Errorf("a condition can be set on a decision only")
	}
	ifs, ok := op.Body[0].(*ast.IfStmt)
	if !ok {
		return "", "", fmt.Errorf("a condition can be set from an if only")
	}
	old, ok := split.SplitCondition.(*microflows.ExpressionSplitCondition)
	if !ok {
		return "", "", fmt.Errorf("the decision calls a rule; the splice sets an expression's text only")
	}
	fb := a.fragmentBuilder(ctx)
	if fb.tryBuildRuleSplitCondition(ifs.Condition) != nil {
		return "", "", fmt.Errorf("the new condition calls a rule, which would replace the decision's expression; " +
			"the splice sets an expression's text only")
	}
	expr = fb.exprToString(ifs.Condition)
	fb.buildSplitCondition(ifs.Condition, expr) // reports a call that is not a rule
	if errs := fb.GetErrors(); len(errs) > 0 {
		return "", "", fmt.Errorf("the condition has errors:\n  - %s", strings.Join(errs, "\n  - "))
	}
	if missing := a.unscopedRefs(expr, target.ID); len(missing) > 0 {
		return "", "", fmt.Errorf("the condition uses %s, which is not declared on the path to the decision", strings.Join(missing, ", "))
	}
	caption = split.Caption
	if caption == old.Expression {
		caption = expr
	}
	return expr, caption, nil
}

// fragmentBuilder is the builder `create microflow` uses, seeded with the
// variables the stored flow declares.
func (a *alterFlowContext) fragmentBuilder(ctx *ExecContext) *flowBuilder {
	varTypes, declared := a.storedVariables(ctx)
	for name, t := range a.declaredVarTypes {
		varTypes[name] = t
		delete(declared, name)
	}
	hierarchy, _ := getHierarchy(ctx)
	restServices, _ := loadRestServices(ctx)
	return &flowBuilder{
		textLang:     authoringLanguage(ctx),
		posX:         200,
		posY:         200,
		baseY:        200,
		spacing:      HorizontalSpacing,
		varTypes:     varTypes,
		declaredVars: declared,
		measurer:     &layoutMeasurer{varTypes: varTypes},
		backend:      ctx.Backend,
		hierarchy:    hierarchy,
		restServices: restServices,
		isNanoflow:   a.stmt.Nanoflow,

		// A fragment is spliced into a stored flow: a member it cannot
		// qualify is refused, never written bare (ako/mxcli#885).
		qualifiedMembersOnly: true,
	}
}

// buildFragment builds a fragment's statements with the builder `create
// microflow` uses, seeded with the variables the stored flow declares, and
// cuts it out of the start and end events the builder wraps it in.
func (a *alterFlowContext) buildFragment(ctx *ExecContext, body []ast.MicroflowStatement) (*backend.MicroflowFragment, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("the fragment is empty; use drop to remove an activity")
	}
	fb := a.fragmentBuilder(ctx)
	oc := fb.buildFlowGraph(body, nil)
	if errs := fb.GetErrors(); len(errs) > 0 {
		return nil, fmt.Errorf("the fragment has errors:\n  - %s", strings.Join(errs, "\n  - "))
	}
	if fb.endsWithReturn {
		return nil, fmt.Errorf("every path through the fragment ends the flow with a return, so nothing would lead on " +
			"to the rest of it; a return can be inserted only on a path that branches off the rest (a guard clause)")
	}
	return cutFragment(oc, fb.fallThroughEndID, fb.returnEndIDs)
}

// cutFragment removes the builder's start event and the end event the body
// falls through to (fallThrough), and says where the fragment is entered and
// left. Several paths reaching that end (an if without a merge before it, an
// error handler that rejoins at the end) are joined by a merge, which becomes
// the exit.
//
// An end event a `return` in the fragment drew (returns) is part of the
// fragment: a guard clause's return ends its own path, which branches off the
// rest of the flow, and is written as a new end event of the flow where the
// builder drew it relative to the fragment (ako/mxcli#888). So is the return
// that ends an error handler of the fragment (ako/mxcli#905). Any other end
// event is one the builder adds to end a handler that states no return, with
// the flow's default value, and is refused.
func cutFragment(oc *microflows.MicroflowObjectCollection, fallThrough model.ID, returns map[model.ID]bool) (*backend.MicroflowFragment, error) {
	var start, end microflows.MicroflowObject
	for _, obj := range oc.Objects {
		switch obj.(type) {
		case *microflows.StartEvent:
			start = obj
		case *microflows.EndEvent:
			switch {
			case obj.GetID() == fallThrough:
				end = obj
			case !returns[obj.GetID()]:
				return nil, fmt.Errorf("an error handler in the fragment ends at an end event the script does not state; " +
					"an inserted error handler has to rejoin the rest of the flow or end in a return of its own")
			}
		}
	}
	if start == nil || end == nil {
		return nil, fmt.Errorf("the fragment does not continue: its last statement ends the flow, so nothing would lead on to the rest of it")
	}
	frag := &backend.MicroflowFragment{}
	var intoEnd []*microflows.SequenceFlow
	for _, f := range oc.Flows {
		switch {
		case f.OriginID == start.GetID():
			if frag.Entry != "" {
				return nil, fmt.Errorf("the fragment starts with more than one flow")
			}
			frag.Entry = f.DestinationID
		case f.DestinationID == end.GetID():
			intoEnd = append(intoEnd, f)
		default:
			frag.Flows = append(frag.Flows, f)
		}
	}
	for _, obj := range oc.Objects {
		if obj != start && obj != end {
			frag.Objects = append(frag.Objects, obj)
		}
	}
	frag.AnnotationFlows = oc.AnnotationFlows
	switch {
	case frag.Entry == "" || frag.Entry == end.GetID() || len(frag.Objects) == 0:
		return nil, fmt.Errorf("the fragment builds no activity")
	case len(intoEnd) == 0:
		return nil, fmt.Errorf("no path through the fragment leads on to the rest of the flow")
	case len(intoEnd) == 1:
		frag.Exit = intoEnd[0].OriginID
		// The flow leaving a decision that ends the fragment is one of its
		// paths, and carries its case on (ako/mxcli#888).
		switch cv := intoEnd[0].CaseValue; cv.(type) {
		case nil, microflows.NoCase, *microflows.NoCase:
		default:
			frag.ExitCase = cv
		}
	default:
		p := end.GetPosition()
		merge := &microflows.ExclusiveMerge{BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			Position:    p,
			Size:        model.Size{Width: MergeSize, Height: MergeSize},
		}}
		for _, f := range intoEnd {
			f.DestinationID = merge.ID
			frag.Flows = append(frag.Flows, f)
		}
		frag.Objects = append(frag.Objects, merge)
		frag.Exit = merge.ID
	}
	return frag, nil
}

// storedVariables returns the variables the stored flow declares, in the two
// maps the builder keeps: entity-typed ones with their entity (a change or a
// member access resolves attributes through it), and the rest as declared.
func (a *alterFlowContext) storedVariables(ctx *ExecContext) (varTypes, declared map[string]string) {
	varTypes, declared = map[string]string{}, map[string]string{}
	add := func(name string, dt microflows.DataType) {
		if name == "" {
			return
		}
		t := "Unknown"
		if dt != nil {
			t = formatMicroflowDataType(ctx, dt, a.entityNames)
		}
		switch dt.(type) {
		case *microflows.ObjectType, *microflows.ListType:
			varTypes[name] = t
		default:
			declared[name] = t
		}
	}
	for _, p := range a.mf.Parameters {
		add(p.Name, p.Type)
	}
	for _, c := range a.cands {
		act, ok := c.Object.(*microflows.ActionActivity)
		if !ok || c.OutputVariable == "" {
			continue
		}
		switch x := act.Action.(type) {
		case *microflows.CreateVariableAction:
			add(c.OutputVariable, x.DataType)
		case *microflows.CreateObjectAction:
			if x.EntityQualifiedName != "" {
				varTypes[c.OutputVariable] = x.EntityQualifiedName
			} else if n, ok := a.entityNames[x.EntityID]; ok {
				varTypes[c.OutputVariable] = n
			} else {
				declared[c.OutputVariable] = "Object"
			}
		case *microflows.RetrieveAction:
			// A database retrieve names its entity; one over an association
			// depends on which side it starts from and stays untyped here.
			src, ok := x.Source.(*microflows.DatabaseRetrieveSource)
			entity := ""
			if ok {
				entity = src.EntityQualifiedName
				if entity == "" {
					entity = a.entityNames[src.EntityID]
				}
			}
			switch {
			case entity == "":
				declared[c.OutputVariable] = "Unknown"
			case src.Range != nil && src.Range.RangeType == microflows.RangeTypeFirst:
				varTypes[c.OutputVariable] = entity
			default:
				varTypes[c.OutputVariable] = "List of " + entity
			}
		case *microflows.MicroflowCallAction:
			// A call's result has the called microflow's return type.
			var rt microflows.DataType
			if x.MicroflowCall != nil {
				rt = (&flowBuilder{backend: ctx.Backend}).lookupMicroflowReturnType(x.MicroflowCall.Microflow)
			}
			switch rt.(type) {
			case *microflows.ObjectType, *microflows.ListType:
				add(c.OutputVariable, rt)
			default:
				declared[c.OutputVariable] = "Unknown"
			}
		default:
			declared[c.OutputVariable] = "Unknown"
		}
	}
	return varTypes, declared
}

// systemVariables are in scope everywhere they exist at all; the platform
// reports a misuse (a $latestError outside an error handler) itself.
var systemVariables = map[string]bool{
	"currentUser": true, "currentSession": true, "currentObject": true, "currentDeviceType": true,
	"latestError": true, "latestHttpResponse": true, "latestSoapFault": true,
}

var variableRef = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// withoutStringLiterals blanks the contents of every '…' string literal in an
// activity's MDL text, so a `$` inside one — `'/odata?$filter='` — is not read
// as a variable reference by the scope checks below (ako/mxcli#859, rehearsal
// M4). describe doubles a quote inside a string in both language versions.
func withoutStringLiterals(text string) string {
	b := []byte(text)
	in := false
	for i := 0; i < len(b); i++ {
		switch {
		case b[i] == '\'' && in && i+1 < len(b) && b[i+1] == '\'':
			b[i], b[i+1] = ' ', ' '
			i++
		case b[i] == '\'':
			in = !in
		case in:
			b[i] = ' '
		}
	}
	return string(b)
}

// checkFragmentScope is plan item 4.2d: the fragment is checked in the scope
// of its insertion point. A variable it declares that the flow already has is
// an error (it would shadow or clash with the stored one); a variable it uses
// that is not declared upstream of where it goes, nor by the fragment itself,
// is an error too, since the fragment would read something that does not exist
// yet on that path.
func (a *alterFlowContext) checkFragmentScope(ctx *ExecContext, op *ast.AlterFlowOperation, target mfmutator.Candidate, frag *backend.MicroflowFragment) error {
	existing := map[string]bool{}
	for _, p := range a.mf.Parameters {
		existing[p.Name] = true
	}
	for _, c := range a.cands {
		if c.OutputVariable != "" {
			existing[c.OutputVariable] = true
		}
	}
	own := map[string]bool{}
	for _, obj := range frag.Objects {
		// A loop's iterator exists only inside the loop, which the fragment
		// brings along; it reads it there, so it is the fragment's own.
		if loop, ok := obj.(*microflows.LoopedActivity); ok {
			if src, ok := loop.LoopSource.(*microflows.IterableList); ok && src.VariableName != "" {
				own[src.VariableName] = true
			}
			continue
		}
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		v := mfmutator.OutputVariable(act.Action)
		if v == "" {
			continue
		}
		replacingSame := op.Op == ast.AlterFlowReplace && v == target.OutputVariable
		if existing[v] && !replacingSame {
			return fmt.Errorf("the fragment declares $%s, which the %s already has; choose another name", v, a.stmt.Kind())
		}
		if a.declaredByOps[v] {
			return fmt.Errorf("the fragment declares $%s, which an earlier operation of this alter already declares; choose another name", v)
		}
		own[v] = true
	}

	inScope := map[string]bool{}
	for _, p := range a.mf.Parameters {
		inScope[p.Name] = true
	}
	for id := range a.upstreamOf(target.ID, op.Op == ast.AlterFlowInsertAfter) {
		for _, c := range a.cands {
			if c.ID == id && c.OutputVariable != "" {
				inScope[c.OutputVariable] = true
			}
		}
	}
	var missing []string
	seen := map[string]bool{}
	for _, obj := range frag.Objects {
		for _, m := range variableRef.FindAllStringSubmatch(withoutStringLiterals(formatActivity(ctx, obj, a.entityNames, a.microflowNames)), -1) {
			v := m[1]
			if seen[v] || systemVariables[v] || own[v] || (inScope[v] && !a.removedByOps[v]) {
				continue
			}
			seen[v] = true
			missing = append(missing, "$"+v)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		where := "before " + op.Target
		if op.Op == ast.AlterFlowInsertAfter {
			where = "after " + op.Target
		}
		return fmt.Errorf("the fragment uses %s, which is not declared on the path %s", strings.Join(missing, ", "), where)
	}
	return nil
}

// unscopedRefs returns the variables expr reads that do not exist where the
// stored node id runs: not a parameter, not the output of an activity before
// it (or one an earlier operation took away), not declared by an earlier
// operation, and not a system variable.
func (a *alterFlowContext) unscopedRefs(expr string, id model.ID) []string {
	inScope := map[string]bool{}
	for _, p := range a.mf.Parameters {
		inScope[p.Name] = true
	}
	for up := range a.upstreamOf(id, false) {
		for _, c := range a.cands {
			if c.ID == up && c.OutputVariable != "" {
				inScope[c.OutputVariable] = true
			}
		}
	}
	var missing []string
	seen := map[string]bool{}
	for _, m := range variableRef.FindAllStringSubmatch(withoutStringLiterals(expr), -1) {
		v := m[1]
		if seen[v] || systemVariables[v] || a.declaredByOps[v] || (inScope[v] && !a.removedByOps[v]) {
			continue
		}
		seen[v] = true
		missing = append(missing, "$"+v)
	}
	sort.Strings(missing)
	return missing
}

// upstreamOf returns every object from which id can be reached along the
// stored flows — the activities whose outputs exist when the flow gets there.
// id itself is included only when including says so (an insert after it runs
// once it has).
func (a *alterFlowContext) upstreamOf(id model.ID, including bool) map[model.ID]bool {
	preds := map[model.ID][]model.ID{}
	for _, f := range a.mf.ObjectCollection.Flows {
		preds[f.DestinationID] = append(preds[f.DestinationID], f.OriginID)
	}
	out := map[model.ID]bool{}
	queue := append([]model.ID(nil), preds[id]...)
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if out[n] {
			continue
		}
		out[n] = true
		queue = append(queue, preds[n]...)
	}
	if including {
		out[id] = true
	}
	return out
}

// checkOutputUnused refuses to take away an activity whose output variable
// another activity still reads — unless the replacement declares it again.
func (a *alterFlowContext) checkOutputUnused(target mfmutator.Candidate, replacement *backend.MicroflowFragment) error {
	v := target.OutputVariable
	if v == "" || fragmentDeclares(replacement, v) {
		return nil
	}
	if readers := a.readByOps[v]; len(readers) > 0 {
		return fmt.Errorf("$%s is read by what an earlier operation of this alter adds: %s", v, strings.Join(readers, "; "))
	}
	ref := regexp.MustCompile(`\$` + regexp.QuoteMeta(v) + `\b`)
	var users []string
	for _, c := range a.cands {
		if c.ID == target.ID || a.removedIDs[c.ID] {
			continue
		}
		for _, text := range append([]string{c.Statement}, c.Alternates...) {
			if ref.MatchString(withoutStringLiterals(text)) {
				users = append(users, c.Statement)
				break
			}
		}
	}
	if len(users) > 0 {
		return fmt.Errorf("$%s is still used by: %s", v, strings.Join(users, "; "))
	}
	return nil
}
