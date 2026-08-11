// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// checkNoQueuedCalls refuses to rewrite a microflow that has a call bound to a
// task queue, because the rewrite would silently drop that binding.
//
// A CREATE OR REPLACE/MODIFY rebuilds the microflow from the statement, and both
// engines hardcode QueueSettings to null on a call — correct for a newly
// authored call, wrong for one that was already queued:
//
//	codec.RegisterTypeDefaults("Microflows$MicroflowCall", codec.TypeDefaults{
//	    NullFields: []string{"QueueSettings"}, ...
//
// Nothing signals the loss afterwards. Measured on Mendix 11.13: with the
// binding present `mx check` reports CE1613 ("The selected task queue … no
// longer exists") on the call activity; after the rewrite it reports 0 errors.
// So mxcli "fixes" the build by deleting the user's configuration, and the
// project then looks healthy.
//
// MDL cannot yet author a queued call, so the binding cannot be restated in the
// script either — refusing is the only option that does not lose data
// (guard-don't-drop, ADR-0005). Remove this guard when `in queue` exists and the
// rebuild carries the binding through.
func checkNoQueuedCalls(ctx *ExecContext, microflowID model.ID, qualifiedName string) error {
	raw, err := ctx.Backend.GetRawUnit(microflowID)
	if err != nil {
		// Unreadable stored unit is not this guard's business; the rewrite path
		// reports its own errors.
		return nil
	}
	queues := queuedCallTargets(raw)
	if len(queues) == 0 {
		return nil
	}
	sort.Strings(queues)
	return mdlerrors.NewUnsupported(fmt.Sprintf(
		"microflow %s has %d call(s) bound to a task queue (%s), and rewriting it would silently "+
			"drop that binding — MDL cannot express a queued call yet, so the queue cannot be restated "+
			"in this script.\n"+
			"  Change the microflow in Studio Pro, or remove the task queue from the call first "+
			"(the binding lives on the call activity, not the microflow).",
		qualifiedName, len(queues), strings.Join(queues, ", ")))
}

// queuedCallTargets walks a stored microflow document and returns the queue
// bound to each call that has one.
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
