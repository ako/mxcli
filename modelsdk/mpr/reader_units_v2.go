// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// The MPR v2 unit-type index.
//
// A v2 project keeps each unit in its own file under mprcontents/, and the
// Unit table has no type column, so a unit's $Type is only known from its
// BSON. buildUnitCache used to open every file to learn it and throw the bytes
// away — 393 of a blank app's 394 files for one `describe microflow`, and all
// of them again after every write (mendixlabs/mxcli#1603).
//
// What the table does carry is ContentsHash, base64(SHA-256) of the file,
// which every v2 writer — Studio Pro's and ours (writer_core.go) — keeps in
// step with the bytes. The index maps that hash to the $Type, so a unit is
// opened only when its hash is one the index has not seen. Keying by content
// rather than by unit means an entry can never be stale: a unit whose bytes
// change — including a re-type in place (#503) — has a new hash and is read
// again; unchanged bytes have the type they had.
//
// The index is persisted to <project>/.mxcli/unit-types.idx so the next
// process starts warm. The file is a pure cache: absent, unreadable, corrupt
// or unwritable all mean "type from the file", never a wrong answer. A unit
// with no ContentsHash cannot be keyed and is read every time, as before.
//
// An entry is learnt under the hash of the bytes actually read, not the hash
// the row claims, so a file caught between a writer's rename and its hash
// update — or one edited behind the table's back — cannot teach the index a
// wrong pair. What the index does trust is that a row's ContentsHash describes
// its file: where it does not, the project is already broken (`mxcli mpr
// verify-hashes` finds it, ako/mxcli#972), and a lookup by that stale hash
// returns the type of the bytes it was last true of.

const unitTypeIndexName = "unit-types.idx"

// unitTypeIndexHeader is the first line of the persisted index; a file
// without it is ignored, so a format change only costs one cold start.
const unitTypeIndexHeader = "mxcli-unit-types 1"

// unitTypeIndexPersistAfter is how many files a later rebuild must have
// opened before it rewrites the persisted index straight away. A reader's
// first rebuild always persists what it learnt; a rebuild after a write opens
// one or two files, and those are saved by Close instead of rewriting the
// whole index after every statement of a script.
const unitTypeIndexPersistAfter = 32

// unitTypeIndexPath is where the persisted index lives, or "" when the reader
// has no project path to put it next to.
func (r *Reader) unitTypeIndexPath() string {
	if r.path == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(r.path), ".mxcli", unitTypeIndexName)
}

// knownUnitType returns the indexed $Type for a ContentsHash, loading the
// persisted index on first use. Caller holds v2TypesMu.
func (r *Reader) knownUnitType(hash string) (string, bool) {
	if !r.v2TypesLoaded {
		r.v2TypesLoaded = true
		r.v2Types = loadUnitTypeIndex(r.unitTypeIndexPath())
	}
	if hash == "" {
		return "", false
	}
	typ, ok := r.v2Types[hash]
	return typ, ok
}

// learnUnitType records a hash's $Type. Caller holds v2TypesMu.
func (r *Reader) learnUnitType(hash, typ string) {
	if hash == "" {
		return
	}
	if r.v2Types == nil {
		r.v2Types = make(map[string]string)
	}
	if old, ok := r.v2Types[hash]; !ok || old != typ {
		r.v2Types[hash] = typ
		r.v2TypesDirty = true
	}
}

// persistUnitTypeIndex writes the hashes the project holds now, and drops
// those of units that have since changed or gone, so the file stays the size
// of the project however many writes a session made. Best effort: a project
// directory the user cannot write to only means the next process types its
// units from their files. Caller holds v2TypesMu.
func (r *Reader) persistUnitTypeIndex() {
	path := r.unitTypeIndexPath()
	if !r.v2TypesDirty || path == "" || r.v2Hashes == nil {
		return
	}
	var buf bytes.Buffer
	buf.WriteString(unitTypeIndexHeader)
	buf.WriteByte('\n')
	for hash := range r.v2Hashes {
		typ, ok := r.v2Types[hash]
		if !ok {
			continue
		}
		buf.WriteString(hash)
		buf.WriteByte('\t')
		buf.WriteString(typ)
		buf.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), unitTypeIndexName+".*.tmp")
	if err != nil {
		return
	}
	_, werr := tmp.Write(buf.Bytes())
	cerr := tmp.Close()
	_ = os.Chmod(tmp.Name(), 0o644) // CreateTemp's 0600 would differ from every other .mxcli/ file
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), path) != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	r.v2TypesDirty = false
}

// loadUnitTypeIndex reads a persisted index; any problem yields an empty one.
func loadUnitTypeIndex(path string) map[string]string {
	out := make(map[string]string)
	if path == "" {
		return out
	}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() || sc.Text() != unitTypeIndexHeader {
		return out
	}
	for sc.Scan() {
		hash, typ, ok := strings.Cut(sc.Text(), "\t")
		if !ok || hash == "" || strings.Contains(typ, "\t") {
			return make(map[string]string) // corrupt: start cold rather than half-trust it
		}
		out[hash] = typ
	}
	if sc.Err() != nil {
		return make(map[string]string)
	}
	return out
}
