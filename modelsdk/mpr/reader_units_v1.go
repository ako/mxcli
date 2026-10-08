// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
)

// Typed listings on MPR v1.
//
// A v1 project keeps every unit's contents in the Unit.Contents blob, and the
// table has no type column: a unit's $Type is only known from its BSON. The
// listing used to read every blob in the file and filter afterwards, so each
// ListMicroflows, ListModules, ListConsumedRestServices… cost a full read of
// the project — 140 MB on a large app, fourteen times in one `describe
// microflow`. MPR v2 never paid this, because buildUnitCache indexes the types
// once and listUnitsByTypeV2 then reads only the matching files.
//
// This is the v1 counterpart of that index, with one difference: v1's
// updateUnit writes the Unit table without calling InvalidateCache, and
// another process can change the file under a long-lived reader, so the index
// cannot rely on being invalidated. Instead every listing re-reads the cheap
// columns (ID, container, containment name and length(Contents) — the length
// comes from the record header, no overflow page is touched) and re-types any
// unit that is new or whose length moved. A unit's $Type is fixed for its
// lifetime in Mendix, so the length is a belt-and-braces change detector, not
// the thing correctness rests on.
//
// The type itself is peeked from the blob's first v1TypePeekBytes bytes:
// Studio Pro writes `$ID` then `$Type` at the head of every unit. A document
// that does not carry `$Type` within the prefix is read in full.

// v1TypePeekBytes is how much of a unit's BSON is read to find its $Type.
const v1TypePeekBytes = 256

// v1StaleRescan is the number of untyped units above which the index is
// refreshed with one table scan instead of one point query per unit.
const v1StaleRescan = 64

type v1TypeEntry struct {
	typ  string
	size int64
}

// v1UnitMeta is one Unit row without its contents. IDs stay as blobs until a
// unit is returned: formatting a UUID costs more than the rest of the row, and
// a typed listing returns a small fraction of the rows it inspects.
type v1UnitMeta struct {
	blob            []byte
	key             string // string(blob): the type index key
	containerBlob   []byte
	containmentName string
	size            int64 // -1 for NULL contents
	held            []byte
	overlaid        bool
}

// listUnitsByTypeV1 handles MPR v1 format (contents in database).
func (r *Reader) listUnitsByTypeV1(typeName string) ([]rawUnit, error) {
	if typeName == "" {
		return r.listAllUnitsV1()
	}

	metas, err := r.scanUnitMetaV1()
	if err != nil {
		return nil, err
	}

	r.v1TypesMu.Lock()
	defer r.v1TypesMu.Unlock()
	if err := r.refreshTypeIndexV1(metas); err != nil {
		return nil, err
	}

	var stmt *sql.Stmt
	defer func() {
		if stmt != nil {
			_ = stmt.Close()
		}
	}()
	var units []rawUnit
	for _, m := range metas {
		var contents []byte
		var unitType string
		if m.overlaid {
			contents = m.held
			unitType = getTypeFromContents(contents)
			if unitType != typeName {
				continue
			}
		} else {
			unitType = r.v1Types[m.key].typ
			if unitType != typeName {
				continue
			}
			if stmt == nil {
				if stmt, err = r.db.Prepare(`SELECT Contents FROM Unit WHERE UnitID = ?`); err != nil {
					return nil, fmt.Errorf("failed to prepare unit read: %w", err)
				}
			}
			if err := stmt.QueryRow(m.blob).Scan(&contents); err != nil {
				if err == sql.ErrNoRows {
					continue // deleted since the metadata scan
				}
				return nil, fmt.Errorf("failed to read unit %s: %w", blobToUUID(m.blob), err)
			}
			r.blobReads.Add(1)
			// The row may have changed between the metadata scan and this read;
			// type what was actually read, as the full scan did.
			if int64(len(contents)) != m.size {
				unitType = getTypeFromContents(contents)
				r.v1Types[m.key] = v1TypeEntry{typ: unitType, size: int64(len(contents))}
				if unitType != typeName {
					continue
				}
			}
		}
		units = append(units, rawUnit{
			ID:              blobToUUID(m.blob),
			ContainerID:     blobToUUID(m.containerBlob),
			ContainmentName: m.containmentName,
			Type:            unitType,
			Contents:        contents,
		})
	}
	return units, nil
}

// listUnitTypesV1 returns every unit's metadata and type without its
// contents (Contents is nil), in table order.
func (r *Reader) listUnitTypesV1() ([]rawUnit, error) {
	metas, err := r.scanUnitMetaV1()
	if err != nil {
		return nil, err
	}
	r.v1TypesMu.Lock()
	defer r.v1TypesMu.Unlock()
	if err := r.refreshTypeIndexV1(metas); err != nil {
		return nil, err
	}
	units := make([]rawUnit, 0, len(metas))
	for _, m := range metas {
		typ := r.v1Types[m.key].typ
		if m.overlaid {
			typ = getTypeFromContents(m.held)
		}
		units = append(units, rawUnit{ID: blobToUUID(m.blob), ContainerID: blobToUUID(m.containerBlob), ContainmentName: m.containmentName, Type: typ})
	}
	return units, nil
}

// listAllUnitsV1 returns every unit with its contents: an unfiltered listing
// needs every blob anyway, so it reads them in one scan.
func (r *Reader) listAllUnitsV1() ([]rawUnit, error) {
	rows, err := r.db.Query(`
		SELECT UnitID, ContainerID, ContainmentName, Contents
		FROM Unit
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query units: %w", err)
	}
	defer rows.Close()

	var units []rawUnit
	for rows.Next() {
		var unitID, containerID []byte
		var containmentName sql.NullString
		var contents []byte

		if err := rows.Scan(&unitID, &containerID, &containmentName, &contents); err != nil {
			return nil, fmt.Errorf("failed to scan unit row: %w", err)
		}
		r.blobReads.Add(1)

		if held, ok := r.overlaid(blobToUUID(unitID)); ok {
			contents = held
		}
		units = append(units, rawUnit{
			ID:              blobToUUID(unitID),
			ContainerID:     blobToUUID(containerID),
			ContainmentName: containmentName.String,
			Type:            getTypeFromContents(contents),
			Contents:        contents,
		})
	}
	return units, rows.Err()
}

// scanUnitMetaV1 reads every unit's metadata, without its contents, in table
// order (the order the full scan returned units in).
func (r *Reader) scanUnitMetaV1() ([]v1UnitMeta, error) {
	rows, err := r.db.Query(`
		SELECT UnitID, ContainerID, ContainmentName, length(Contents)
		FROM Unit
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query units: %w", err)
	}
	defer rows.Close()

	var metas []v1UnitMeta
	for rows.Next() {
		var unitID, containerID []byte
		var containmentName sql.NullString
		var size sql.NullInt64
		if err := rows.Scan(&unitID, &containerID, &containmentName, &size); err != nil {
			return nil, fmt.Errorf("failed to scan unit row: %w", err)
		}
		m := v1UnitMeta{
			blob:            unitID,
			key:             string(unitID),
			containerBlob:   containerID,
			containmentName: containmentName.String,
			size:            -1,
		}
		if size.Valid {
			m.size = size.Int64
		}
		if len(r.overlay) > 0 {
			m.held, m.overlaid = r.overlaid(blobToUUID(unitID))
		}
		metas = append(metas, m)
	}
	return metas, rows.Err()
}

// refreshTypeIndexV1 types every unit in metas that the index does not hold at
// its current length, and drops units that no longer exist. Caller holds
// v1TypesMu.
func (r *Reader) refreshTypeIndexV1(metas []v1UnitMeta) error {
	if r.v1Types == nil {
		r.v1Types = make(map[string]v1TypeEntry, len(metas))
	}
	var stale []v1UnitMeta
	present := make(map[string]struct{}, len(metas))
	for _, m := range metas {
		present[m.key] = struct{}{}
		if m.overlaid {
			continue
		}
		if e, ok := r.v1Types[m.key]; !ok || e.size != m.size {
			stale = append(stale, m)
		}
	}
	for id := range r.v1Types {
		if _, ok := present[id]; !ok {
			delete(r.v1Types, id)
		}
	}
	if len(stale) == 0 {
		return nil
	}
	if len(stale) > v1StaleRescan {
		return r.peekAllTypesV1()
	}

	stmt, err := r.db.Prepare(`SELECT length(Contents), substr(Contents, 1, ?) FROM Unit WHERE UnitID = ?`)
	if err != nil {
		return fmt.Errorf("failed to prepare unit type peek: %w", err)
	}
	defer stmt.Close()
	for _, m := range stale {
		var size sql.NullInt64
		var prefix []byte
		if err := stmt.QueryRow(v1TypePeekBytes, m.blob).Scan(&size, &prefix); err != nil {
			if err == sql.ErrNoRows {
				delete(r.v1Types, m.key) // deleted since the metadata scan
				continue
			}
			return fmt.Errorf("failed to peek unit %s: %w", blobToUUID(m.blob), err)
		}
		if err := r.recordTypeV1(m.blob, size, prefix); err != nil {
			return err
		}
	}
	return nil
}

// peekAllTypesV1 (re)builds the whole index in one scan of the blob prefixes.
func (r *Reader) peekAllTypesV1() error {
	rows, err := r.db.Query(`SELECT UnitID, length(Contents), substr(Contents, 1, ?) FROM Unit`, v1TypePeekBytes)
	if err != nil {
		return fmt.Errorf("failed to peek unit types: %w", err)
	}
	type peeked struct {
		blob   []byte
		size   sql.NullInt64
		prefix []byte
	}
	var all []peeked
	for rows.Next() {
		var p peeked
		if err := rows.Scan(&p.blob, &p.size, &p.prefix); err != nil {
			rows.Close()
			return fmt.Errorf("failed to scan unit type: %w", err)
		}
		all = append(all, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close() // before recordTypeV1 may query again
	for _, p := range all {
		if err := r.recordTypeV1(p.blob, p.size, p.prefix); err != nil {
			return err
		}
	}
	return nil
}

// recordTypeV1 stores a unit's type from its peeked prefix, reading the whole
// blob when the prefix does not settle it.
func (r *Reader) recordTypeV1(blob []byte, size sql.NullInt64, prefix []byte) error {
	if !size.Valid {
		r.v1Types[string(blob)] = v1TypeEntry{typ: "", size: -1}
		return nil
	}
	typ, ok := peekBSONType(prefix, size.Int64)
	if !ok {
		var contents []byte
		if err := r.db.QueryRow(`SELECT Contents FROM Unit WHERE UnitID = ?`, blob).Scan(&contents); err != nil {
			if err == sql.ErrNoRows {
				delete(r.v1Types, string(blob))
				return nil
			}
			return fmt.Errorf("failed to read unit %s: %w", blobToUUID(blob), err)
		}
		r.blobReads.Add(1)
		typ = getTypeFromContents(contents)
		size.Int64 = int64(len(contents))
	}
	r.v1Types[string(blob)] = v1TypeEntry{typ: typ, size: size.Int64}
	return nil
}

// peekBSONType returns the top-level $Type string of a BSON document of total
// length size, given only its first bytes. ok is false when the prefix does not
// settle it — the caller then reads the whole document. It answers exactly what
// getTypeFromContents would on the full bytes: "" for a document whose length
// header disagrees with its size, or that has no string $Type.
func peekBSONType(prefix []byte, size int64) (typ string, ok bool) {
	if size == 0 {
		return "", true
	}
	if len(prefix) < 4 {
		return "", false
	}
	if int64(int32(binary.LittleEndian.Uint32(prefix))) != size {
		return "", false // malformed header: let the full decode decide
	}
	rest := prefix[4:]
	for len(rest) > 0 {
		if rest[0] == 0x00 {
			// End of the document: settled only if the whole of it was read.
			return "", int64(len(prefix)) == size
		}
		elem, remaining, ok := bsoncore.ReadElement(rest)
		if !ok {
			return "", false // element runs past the prefix
		}
		if bytes.Equal(elem.KeyBytes(), []byte("$Type")) {
			s, isString := elem.Value().StringValueOK()
			if !isString {
				return "", true
			}
			return s, true
		}
		rest = remaining
	}
	return "", false
}
