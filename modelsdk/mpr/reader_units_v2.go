// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
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
// Beside the $Type the index holds the unit's top-level Name and Excluded,
// which are as much a function of the bytes as the type is. They are what a
// name listing needs: the container hierarchy names every folder, and a
// describe names every microflow, so without them each of those opened every
// folder or microflow file on every call — 2,904 of a 4,753-unit app's files
// for one `describe microflow` (ako/mxcli#1791).
//
// The index is persisted to <project>/.mxcli/unit-index-2.idx so the next
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

// The file name carries the format: an mxcli that wrote the previous format
// (unit-types.idx, type only) and one that writes this one each read their
// own, rather than discarding each other's as corrupt on every call when two
// versions work on one project — the VS Code extension's and the CLI's.
const unitTypeIndexName = "unit-index-2.idx"

// unitTypeIndexHeader is the first line of the persisted index; a file
// without it is ignored, so a format change only costs one cold start.
const unitTypeIndexHeader = "mxcli-unit-index 2"

// ColdUnitIndexNotice, when set, is told in one line that a reader is about to
// build the unit index from nothing: no persisted index, or one in an older
// format. That build opens every unit file once — on the first command after an
// upgrade or on a fresh clone — and on Windows, where an open costs about a
// millisecond, it is a visible stall with no explanation (ako/mxcli#1806). nil,
// the default, says nothing: a library caller did not ask for stderr output.
var ColdUnitIndexNotice io.Writer

// unitIndexEntry is what the index knows of the unit whose bytes hash to its
// key: everything derived from the top level of the document.
type unitIndexEntry struct {
	Type     string
	Name     string
	Excluded bool
}

// unitIndexEntryOf derives an entry from a unit's bytes.
func unitIndexEntryOf(contents []byte) unitIndexEntry {
	e := unitIndexEntry{Type: getTypeFromContents(contents)}
	raw := bson.Raw(contents)
	if raw.Validate() == nil {
		e.Name, _ = raw.Lookup("Name").StringValueOK()
		e.Excluded, _ = raw.Lookup("Excluded").BooleanOK()
	}
	return e
}

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

// knownUnit returns the indexed entry for a ContentsHash, loading the
// persisted index on first use. Caller holds v2TypesMu.
func (r *Reader) knownUnit(hash string) (unitIndexEntry, bool) {
	if !r.v2TypesLoaded {
		r.v2TypesLoaded = true
		r.v2Types = loadUnitTypeIndex(r.unitTypeIndexPath())
	}
	if hash == "" {
		return unitIndexEntry{}, false
	}
	e, ok := r.v2Types[hash]
	return e, ok
}

// announceColdUnitIndex tells ColdUnitIndexNotice that this build starts from
// an empty index and so opens every unit file. Only where the index is
// persisted: without a path every build is cold, and a notice on every command
// would promise a "once" that never comes. Called before buildUnitCache opens
// its row query, so the count runs on a free connection.
func (r *Reader) announceColdUnitIndex() {
	w := ColdUnitIndexNotice
	if w == nil || r.unitTypeIndexPath() == "" {
		return
	}
	r.v2TypesMu.Lock()
	r.knownUnit("") // loads the persisted index on first use
	cold := len(r.v2Types) == 0
	r.v2TypesMu.Unlock()
	if !cold {
		return
	}
	var units int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM Unit`).Scan(&units); err != nil || units == 0 {
		return
	}
	fmt.Fprintf(w, "Building the unit index for this project's %d units (once; later commands reuse .mxcli/%s)…\n",
		units, unitTypeIndexName)
}

// learnUnit records a hash's entry. Caller holds v2TypesMu.
func (r *Reader) learnUnit(hash string, e unitIndexEntry) {
	if hash == "" {
		return
	}
	if r.v2Types == nil {
		r.v2Types = make(map[string]unitIndexEntry)
	}
	if old, ok := r.v2Types[hash]; !ok || old != e {
		r.v2Types[hash] = e
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
		e, ok := r.v2Types[hash]
		if !ok {
			continue
		}
		// hash, type, excluded, name: the name last and quoted, so no
		// character a name can hold splits the line.
		excluded := "0"
		if e.Excluded {
			excluded = "1"
		}
		buf.WriteString(hash)
		buf.WriteByte('\t')
		buf.WriteString(e.Type)
		buf.WriteByte('\t')
		buf.WriteString(excluded)
		buf.WriteByte('\t')
		buf.WriteString(strconv.Quote(e.Name))
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
func loadUnitTypeIndex(path string) map[string]unitIndexEntry {
	out := make(map[string]unitIndexEntry)
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
		f := strings.SplitN(sc.Text(), "\t", 4)
		if len(f) != 4 || f[0] == "" || (f[2] != "0" && f[2] != "1") {
			return make(map[string]unitIndexEntry) // corrupt: start cold rather than half-trust it
		}
		name, err := strconv.Unquote(f[3])
		if err != nil {
			return make(map[string]unitIndexEntry)
		}
		out[f[0]] = unitIndexEntry{Type: f[1], Excluded: f[2] == "1", Name: name}
	}
	if sc.Err() != nil {
		return make(map[string]unitIndexEntry)
	}
	return out
}
