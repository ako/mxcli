// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// checkNoQueuedCalls refuses a microflow rewrite that would silently drop a
// call's task-queue binding.
//
// A CREATE OR REPLACE/MODIFY rebuilds the microflow from the statement, so any
// binding the script does not restate is gone. Nothing signals the loss
// afterwards: measured on Mendix 11.13, with the binding present `mx check`
// reports CE1613 ("The selected task queue … no longer exists") on the call
// activity; after a dropping rewrite it reports 0 errors. So mxcli "fixes" the
// build by deleting the user's configuration, and the project then looks
// healthy (guard-don't-drop, ADR-0005).
//
// `IN QUEUE` makes the binding authorable, so a script that restates every
// stored queue is allowed through — that is the normal way to edit a microflow
// with a queued call. Two things still refuse:
//
//   - A stored queue the new script does not name. Restating some and dropping
//     others is far more likely a mistake than an intent.
//   - A stored QueueSettings carrying a Retry (Queues$QueueFixedRetry /
//     Queues$QueueExponentialRetry). MDL has no syntax for a retry policy, so
//     the rewrite cannot preserve one, and re-running an "unchanged" script
//     would quietly reset it.
func checkNoQueuedCalls(ctx *ExecContext, microflowID model.ID, qualifiedName string, stmt *ast.CreateMicroflowStmt) error {
	raw, err := ctx.Backend.GetRawUnit(microflowID)
	if err != nil {
		// Unreadable stored unit is not this guard's business; the rewrite path
		// reports its own errors.
		return nil
	}
	stored := queuedCallTargets(raw)
	if len(stored) == 0 {
		return nil
	}

	if retries := storedQueueRetries(raw); len(retries) > 0 {
		sort.Strings(retries)
		return mdlerrors.NewUnsupported(fmt.Sprintf(
			"microflow %s has %d queued call(s) with a retry policy (%s), and MDL cannot express one — "+
				"rewriting the microflow would reset it.\n"+
				"  Change the microflow in Studio Pro, or remove the retry from the call first.",
			qualifiedName, len(retries), strings.Join(retries, ", ")))
	}

	restated := authoredQueueTargets(stmt)
	var lost []string
	for _, q := range stored {
		if !restated[strings.ToLower(q)] {
			lost = append(lost, q)
		}
	}
	if len(lost) == 0 {
		return nil
	}
	sort.Strings(lost)
	return mdlerrors.NewUnsupported(fmt.Sprintf(
		"microflow %s has %d call(s) bound to a task queue that this script does not restate (%s), "+
			"and rewriting it would silently drop the binding.\n"+
			"  Add the queue to the call — `CALL MICROFLOW Mod.Target(...) IN QUEUE %s` (same clause on "+
			"CALL JAVA ACTION) — or change the microflow in Studio Pro.",
		qualifiedName, len(lost), strings.Join(lost, ", "), lost[0]))
}

// authoredQueueTargets returns the lower-cased queue names the incoming
// statement binds calls to, found by walking the whole statement tree — call
// statements nest inside IF / LOOP / error handlers, and a hand-written switch
// over statement types silently misses whichever nesting was added last.
func authoredQueueTargets(stmt *ast.CreateMicroflowStmt) map[string]bool {
	out := map[string]bool{}
	if stmt == nil {
		return out
	}
	collectAuthoredQueues(reflect.ValueOf(stmt), out, map[uintptr]bool{})
	return out
}

func collectAuthoredQueues(v reflect.Value, out map[string]bool, seen map[uintptr]bool) {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return
		}
		if v.Kind() == reflect.Ptr {
			if seen[v.Pointer()] {
				return
			}
			seen[v.Pointer()] = true
			switch s := v.Interface().(type) {
			case *ast.CallMicroflowStmt:
				addQueueName(s.Queue, out)
			case *ast.CallJavaActionStmt:
				addQueueName(s.Queue, out)
			}
		}
		collectAuthoredQueues(v.Elem(), out, seen)
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			collectAuthoredQueues(v.Index(i), out, seen)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath != "" {
				continue // unexported
			}
			collectAuthoredQueues(v.Field(i), out, seen)
		}
	}
}

func addQueueName(q *ast.QualifiedName, out map[string]bool) {
	if q == nil {
		return
	}
	out[strings.ToLower(q.Module+"."+q.Name)] = true
}

// queuedCallTargets walks a stored microflow document and returns the queue
// bound to each call that has one.
//
// Arrays are matched as BOTH []any and bson.A. The two engines' readers return
// different shapes for the same document — modelsdk yields []interface{},
// the legacy mpr reader yields bson.A — and bson.A is a NAMED slice type, so
// `case []any` silently does not match it. Missing that case made this guard a
// no-op on `--engine legacy`: the walk never descended into
// ObjectCollection.Objects, found no queued calls, and let the rewrite drop the
// binding (see TestQueuedCallTargets_HandlesBsonArrays).
//
// The binding that matters is QueueSettings — a Queues$QueueSettings node whose
// own Queue property names the queue. The call's top-level Queue property is
// also read, because it is in the metamodel, but on its own it is inert:
// measured on 11.13, a call carrying only Queue (with QueueSettings null) draws
// no complaint from mx check at all, while one carrying QueueSettings does.
func queuedCallTargets(v any) []string {
	var out []string
	switch t := v.(type) {
	case map[string]any:
		if qs, ok := t["QueueSettings"].(map[string]any); ok && qs != nil {
			name, _ := qs["Queue"].(string)
			if name == "" {
				name = "(unnamed queue)"
			}
			out = append(out, name)
		} else if name, ok := t["Queue"].(string); ok && name != "" {
			// Present without QueueSettings: not something Mendix acts on, but
			// still authored state that the rewrite would drop.
			out = append(out, name)
		}
		for _, val := range t {
			out = append(out, queuedCallTargets(val)...)
		}
	case []any:
		for _, el := range t {
			out = append(out, queuedCallTargets(el)...)
		}
	case bson.A:
		for _, el := range t {
			out = append(out, queuedCallTargets(el)...)
		}
	}
	return dedupeStrings(out)
}

// storedQueueRetries returns the queue names whose QueueSettings carries a retry
// policy. `IN QUEUE` writes Retry as null, so a non-null one can only have come
// from Studio Pro and has no MDL spelling.
func storedQueueRetries(v any) []string {
	var out []string
	switch t := v.(type) {
	case map[string]any:
		if qs, ok := t["QueueSettings"].(map[string]any); ok && qs != nil {
			if retry, ok := qs["Retry"]; ok && retry != nil {
				name, _ := qs["Queue"].(string)
				if name == "" {
					name = "(unnamed queue)"
				}
				out = append(out, name)
			}
		}
		for _, val := range t {
			out = append(out, storedQueueRetries(val)...)
		}
	case []any:
		for _, el := range t {
			out = append(out, storedQueueRetries(el)...)
		}
	case bson.A:
		for _, el := range t {
			out = append(out, storedQueueRetries(el)...)
		}
	}
	return dedupeStrings(out)
}

func dedupeStrings(in []string) []string {
	if len(in) < 2 {
		return in
	}
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
