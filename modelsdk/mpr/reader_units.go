// SPDX-License-Identifier: Apache-2.0

// Package mpr - Unit listing infrastructure for Reader.
package mpr

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ResolveModuleName walks the container hierarchy upward until it finds a module.
// This is necessary because in MPR v2 projects, documents live inside folders,
// so a document's direct ContainerID is a folder, not the module.
func ResolveModuleName(containerID string, moduleMap map[string]string, containerParent map[string]string) string {
	current := containerID
	for range 20 {
		if name, ok := moduleMap[current]; ok {
			return name
		}
		parent, ok := containerParent[current]
		if !ok || parent == current {
			break
		}
		current = parent
	}
	return ""
}

// BuildContainerParent builds a map of unit ID → parent container ID for hierarchy walking.
func (r *Reader) BuildContainerParent() (map[string]string, error) {
	units, err := r.ListUnits()
	if err != nil {
		return nil, err
	}
	containerParent := make(map[string]string, len(units))
	for _, u := range units {
		containerParent[string(u.ID)] = string(u.ContainerID)
	}
	return containerParent, nil
}

// rawUnit holds raw unit data from the database.
type rawUnit struct {
	ID              string
	ContainerID     string
	ContainmentName string
	Type            string
	Contents        []byte
}

// UnitRef holds unit metadata returned by ListUnitsByType.
type UnitRef struct {
	ID          string
	ContainerID string
	Type        string // BSON $Type (e.g. "Microflows$Microflow")
	Contents    []byte
}

// ListUnitsByType returns all units matching the given BSON $Type prefix.
// This is the exported version for use by TreeWriter and other packages.
func (r *Reader) ListUnitsByType(typePrefix string) ([]UnitRef, error) {
	units, err := r.listUnitsByType(typePrefix)
	if err != nil {
		return nil, err
	}
	result := make([]UnitRef, len(units))
	for i, u := range units {
		result[i] = UnitRef{ID: u.ID, ContainerID: u.ContainerID, Type: u.Type, Contents: u.Contents}
	}
	return result, nil
}

// listUnitsByType returns all units of exactly the given storage type. An empty
// typeName returns every unit.
//
// Exact, not prefix: Mendix storage names nest (`Forms$Page` is a prefix of
// `Forms$PageTemplate`), so a prefix match silently folds one document type into
// another. See the note on the same function in sdk/mpr.
func (r *Reader) listUnitsByType(typeName string) ([]rawUnit, error) {
	if r.version == MPRVersionV2 {
		return r.listUnitsByTypeV2(typeName)
	}
	return r.listUnitsByTypeV1(typeName)
}

// listUnitTypes returns every unit's metadata and $Type without reading the
// contents of any unit whose type is already indexed (Contents is nil). For
// callers that need the project's shape, not its documents.
func (r *Reader) listUnitTypes() ([]rawUnit, error) {
	if r.version != MPRVersionV2 {
		return r.listUnitTypesV1()
	}
	if !r.unitCacheValid {
		if err := r.buildUnitCache(); err != nil {
			return nil, err
		}
	}
	units := make([]rawUnit, 0, len(r.unitCache))
	for _, cu := range r.unitCache {
		units = append(units, rawUnit{ID: cu.ID, ContainerID: cu.ContainerID, ContainmentName: cu.ContainmentName, Type: cu.Type})
	}
	return units, nil
}

// listUnitsByTypeV2 handles MPR v2 format (contents in mprcontents folder).
// Uses caching to avoid reading every file for each query.
func (r *Reader) listUnitsByTypeV2(typeName string) ([]rawUnit, error) {
	if !r.unitCacheValid {
		if err := r.buildUnitCache(); err != nil {
			return nil, err
		}
	}

	// Filter by type using cache, only read contents for matching units.
	var units []rawUnit
	for _, cu := range r.unitCache {
		if typeName == "" || cu.Type == typeName {
			contents, err := r.readMprContents(cu.ID)
			if err != nil {
				continue
			}
			units = append(units, rawUnit{
				ID:              cu.ID,
				ContainerID:     cu.ContainerID,
				ContainmentName: cu.ContainmentName,
				Type:            cu.Type,
				Contents:        contents,
			})
		}
	}
	return units, nil
}

// buildUnitCache reads all unit metadata once and caches it.
func (r *Reader) buildUnitCache() error {
	rows, err := r.db.Query(`
		SELECT UnitID, ContainerID, ContainmentName
		FROM Unit
	`)
	if err != nil {
		return fmt.Errorf("failed to query units: %w", err)
	}
	defer rows.Close()

	r.unitCache = nil
	for rows.Next() {
		var unitID, containerID []byte
		var containmentName string

		if err := rows.Scan(&unitID, &containerID, &containmentName); err != nil {
			return fmt.Errorf("failed to scan unit row: %w", err)
		}

		unitUUID := blobToUUID(unitID)
		contents, err := r.readMprContents(unitUUID)
		if err != nil {
			continue
		}

		typeName := getTypeFromContents(contents)
		r.unitCache = append(r.unitCache, cachedUnit{
			ID:              blobToUUID(unitID),
			ContainerID:     blobToUUID(containerID),
			ContainmentName: containmentName,
			Type:            typeName,
		})
	}

	r.unitCacheValid = true
	return nil
}

// InvalidateCache marks the unit cache as invalid and clears content cache entries.
// Should be called after any write operation.
func (r *Reader) InvalidateCache() {
	r.unitCacheValid = false
	// Clear content cache entries but keep the map non-nil so caching stays active.
	// If contentCache is nil (per-request mode), remain disabled.
	r.contentMu.Lock()
	if r.contentCache != nil {
		clear(r.contentCache)
	}
	r.contentMu.Unlock()
}

// EnableContentCache activates the in-memory content cache for this reader.
// Call once after Connect in persistent daemon mode. The cache survives across
// requests; InvalidateCache empties it (but keeps caching active) on writes.
func (r *Reader) EnableContentCache() {
	r.contentMu.Lock()
	defer r.contentMu.Unlock()
	if r.contentCache == nil {
		r.contentCache = make(map[string][]byte)
	}
	r.cacheScopes = -1
}

// CacheUnitReads holds every unit read from mprcontents/ in memory until
// release is called, so a pass that looks documents up again and again reads
// each file once (mendixlabs/mxcli#1272: check -p re-read every microflow of
// the project several times per `create or modify` it predicted). Scopes
// nest; the cache is dropped when the outermost one is released, so it never
// outlives the pass — a long session must not keep serving bytes Studio Pro
// has since rewritten. A write through this reader empties it as before.
//
// MPR v1 reads its contents from SQLite, which caches its own pages; there
// the scope changes nothing.
func (r *Reader) CacheUnitReads() (release func()) {
	r.contentMu.Lock()
	defer r.contentMu.Unlock()
	if r.cacheScopes < 0 {
		return func() {} // cached for the reader's lifetime
	}
	if r.contentCache == nil {
		r.contentCache = make(map[string][]byte)
	}
	r.cacheScopes++
	var once sync.Once
	return func() {
		once.Do(func() {
			r.contentMu.Lock()
			defer r.contentMu.Unlock()
			if r.cacheScopes <= 0 {
				return
			}
			if r.cacheScopes--; r.cacheScopes == 0 {
				r.contentCache = nil
			}
		})
	}
}

// UnitFileReads is how many unit files this reader has read from
// mprcontents/. For tests.
func (r *Reader) UnitFileReads() int64 { return r.fileReads.Load() }

// readMprContents reads content from the mprcontents folder for v2 format.
// The path is: mprcontents/XX/YY/UUID.mxunit where XX and YY are first two chars of UUID.
//
// While the content cache is on (EnableContentCache, CacheUnitReads) a unit is
// read from disk once and served from memory after that, as a copy: every
// caller gets bytes of its own, as os.ReadFile gave it, so one that edits them
// cannot change what the next caller reads.
func (r *Reader) readMprContents(unitUUID string) ([]byte, error) {
	if len(unitUUID) < 4 {
		return nil, fmt.Errorf("invalid unit UUID: %s", unitUUID)
	}

	// Bytes held in memory (an import buffer, or a deferred run of writes) are
	// what the unit currently is; the listings read through here too, so a
	// held write is seen by every read, not only by GetRawUnitBytes.
	if data, ok := r.overlaid(unitUUID); ok {
		return data, nil
	}

	r.contentMu.Lock()
	cached, ok := r.contentCache[unitUUID]
	caching := r.contentCache != nil
	r.contentMu.Unlock()
	if ok {
		return bytes.Clone(cached), nil
	}

	// Build path: mprcontents/XX/YY/UUID.mxunit
	path := filepath.Join(
		r.contentsDir,
		unitUUID[0:2],
		unitUUID[2:4],
		unitUUID+".mxunit",
	)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r.fileReads.Add(1)

	if caching {
		r.contentMu.Lock()
		if r.contentCache != nil {
			r.contentCache[unitUUID] = bytes.Clone(data)
		}
		r.contentMu.Unlock()
	}
	return data, nil
}

// getTypeFromContents extracts the $Type field from BSON contents.
// Uses bson.Raw.LookupErr for O(1) field extraction instead of unmarshalling
// the entire document into map[string]any.
func getTypeFromContents(contents []byte) string {
	if len(contents) == 0 {
		return ""
	}
	val, err := bson.Raw(contents).LookupErr("$Type")
	if err != nil {
		return ""
	}
	s, ok := val.StringValueOK()
	if !ok {
		return ""
	}
	return s
}

// RawUnitInfo contains information about a raw unit for BSON debugging.
// Aliased to mdl/types.RawUnitInfo so reader_raw.go methods and modelsdk/codec
// consumers share a single concrete struct.
type RawUnitInfo = types.RawUnitInfo
