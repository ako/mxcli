// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"fmt"
	"sort"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// An element's GUID is the DATABASE's identity for it, and it is not
// interchangeable with its $ID. The runtime keys mendixsystem$entity.id and
// mendixsystem$attribute.id on the GUID verbatim, so changing only an
// attribute's GUID — same name, same type, same entity — makes the synchroniser
// treat it as an attribute deleted and a new one added, and DROP its column on
// the next deploy (CLAUDE.md, "A GUID Is the Database's Identity").
//
// That is a data-loss bug with no diagnostic behind it. The model stays
// perfectly valid, so mx check is clean and the build passes; mxcli's own reader
// never surfaces the GUID, so DESCRIBE is byte-identical before and after. It
// only becomes visible when the package meets a database that already holds
// data — which is to say, in production. Issue #1119 lost 28 attributes of 607
// rows that way, from one ALTER of one entity.
//
// Worse, the damage does not repeat: the codec writes GUID = $ID, and the $ID is
// held stable by TransplantIDs, so the SECOND identical write produces
// byte-identical bytes and is elided. The corruption happens exactly once, on
// the first write, and every check afterwards — including a re-run of the same
// script — reports "Unchanged".
//
// So the three guards that already stand at this choke point cannot see it:
//
//   - No-op elision compares a document with itself and finds it stable.
//   - TestFreshGUIDFieldsHaveAnIdentityDecision sees only codec FreshGUIDFields.
//     A GUID derived from the $ID is registered as EmitGUID, a different
//     mechanism, so it was never in that guard's view — the same blind spot that
//     let Workflows$*.PersistentId through in #949.
//   - identityFields/CarryIdentity reach only top-level properties of the
//     document root, and these GUIDs sit on elements nested inside it.
//
// Hence a guard here, in the same spirit as DuplicateElementIDError: cheap, at
// the moment the bytes would land, and phrased as the message the user would
// otherwise never get. It converts silent data loss into one refusal naming the
// element.
//
// It is deliberately NOT a repair. Carrying the stored GUID here would mean
// trusting the structural pairing TransplantIDs is built on, whose correctness
// bar is explicitly low — a wrong $ID match only makes a diff bigger, but a
// wrong GUID match makes the runtime adopt another column's data under a new
// name and type. The fix belongs where the write knows which element is which:
// see carryChildIdentity in mdl/backend/modelsdk, which pairs on the $ID the
// executor tracked through the statement.
//
// The check runs AFTER TransplantIDs, and that ordering is what makes it exact.
// Before the transplant a rebuilt element carries a freshly minted $ID that
// appears in no stored document, so there would be nothing to compare it
// against. After it, an element sharing an $ID with a stored one is an element
// the transplant judged to be the same element — so a differing GUID on it is
// unambiguously a rewrite that dropped the stored identity.

// GUIDChange is one element that kept its $ID across a write while its GUID
// changed — the shape of the #1119 defect.
type GUIDChange struct {
	ElementID string
	Type      string
	Stored    string
	Written   string
}

// StorageGUIDChanges reports every element that appears in both documents under
// the same $ID but with a different GUID, in a stable order.
//
// Only an element is considered: a sub-document carrying both $ID and $Type.
// A pointer to another element is a primitive property holding the same 16-byte
// shape under a different key, and a containment walk meets plenty of those.
//
// An element present on only one side is not a change: a genuinely new element
// has an $ID no stored element holds, and a deleted one is simply absent. Nor is
// a GUID that only one side carries — an optional property Mendix fills in on
// load must not be invented, and one it stopped writing must not be preserved.
//
// A document that cannot be unmarshalled yields no changes rather than an error.
// This runs on the write path, where failing a write because the guard could not
// read the bytes would be worse than the defect it prevents.
func StorageGUIDChanges(contents, stored []byte) []GUIDChange {
	newGUIDs, newTypes := elementGUIDs(contents)
	if len(newGUIDs) == 0 {
		return nil
	}
	oldGUIDs, _ := elementGUIDs(stored)
	if len(oldGUIDs) == 0 {
		return nil
	}

	ids := make([]string, 0, len(newGUIDs))
	for id := range newGUIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var out []GUIDChange
	for _, id := range ids {
		was, ok := oldGUIDs[id]
		if !ok {
			continue
		}
		if now := newGUIDs[id]; now != was {
			out = append(out, GUIDChange{
				ElementID: id,
				Type:      newTypes[id],
				Stored:    was,
				Written:   now,
			})
		}
	}
	return out
}

// elementGUIDs maps element $ID -> GUID for every element in raw that carries
// both, plus each one's $Type for the error message.
func elementGUIDs(raw []byte) (map[string]string, map[string]string) {
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return nil, nil
	}
	guids := map[string]string{}
	types := map[string]string{}
	var walk func(any)
	walk = func(v any) {
		if doc, ok := asDoc(v); ok {
			if id, ok := elementID(doc); ok && hasType(doc) {
				if g, ok := binary16(doc, "GUID"); ok {
					guids[id] = blobToUUID(g)
					types[id] = typeOf(doc)
				}
			}
			for _, k := range sortedKeys(doc) {
				walk(doc[k])
			}
			return
		}
		if s, ok := asSlice(v); ok {
			for _, e := range s {
				walk(e)
			}
		}
	}
	walk(d)
	return guids, types
}

// StorageGUIDError returns the error a write should fail with, or nil.
// unitLabel is whatever the caller can name the unit by — an id is enough, a
// qualified name is better.
func StorageGUIDError(unitLabel string, contents, stored []byte) error {
	changes := StorageGUIDChanges(contents, stored)
	if len(changes) == 0 {
		return nil
	}
	const maxReported = 3
	msg := fmt.Sprintf("refusing to write unit %s: %d element(s) kept their $ID but would be "+
		"written with a different GUID. The runtime keys the database on that GUID "+
		"(mendixsystem$entity.id / mendixsystem$attribute.id), so deploying this would make "+
		"the synchroniser drop and recreate the affected columns and lose their data. "+
		"The model would still be valid and `mx check` would still pass",
		unitLabel, len(changes))
	for i, c := range changes {
		if i == maxReported {
			msg += fmt.Sprintf("\n  ... and %d more", len(changes)-maxReported)
			break
		}
		msg += fmt.Sprintf("\n  %s (%s): stored %s, would write %s", c.ElementID, c.Type, c.Stored, c.Written)
	}
	return fmt.Errorf("%s", msg)
}
