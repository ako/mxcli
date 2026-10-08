// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/modelsdk/canon"
	mpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// gitnote.go builds the mx_metadata git note that Studio Pro attaches to every
// commit it makes (mendixlabs/mxcli#1337, ako/mxcli#972 item 2).
//
// Studio Pro stores, per commit, a JSON note under refs/notes/mx_metadata that
// names the Mendix version and lists the model units the commit changed. It
// reads it to check version compatibility and to show what a revision changed;
// a commit made with plain git has none. The format below is measured, not
// documented — from the 8 notes Studio Pro wrote on MPR v2 commits of a
// Team Server repository with a two-year history (Studio Pro 10.21 – 11.13):
//
//   - Key order is fixed: BranchName, ModelerVersion, ModelChanges,
//     RelatedStories, SolutionVersion, MPRFormatVersion, HasModelerVersion.
//   - BranchName is "" on main and the branch name on any other branch.
//   - ModelerVersion is the .mpr's _MetaData._ProductVersion, verbatim.
//   - Every change has exactly Status, UnitID, UnitType, UnitName, Module.
//     Status is one of Added | Modified | Deleted | Moved.
//   - A deleted module lists only its Projects$ModuleImpl; the units inside it
//     are not listed. Added modules list every unit.
//   - Units whose bytes changed but whose content did not are not listed:
//     the 11.13 upgrade commit rewrote 140 unit files and listed 2.
//   - Moved means the unit's container changed (another folder), which in
//     MPR v2 is a change to the .mpr's Unit table, not to the unit file.
//
// Studio Pro 11.13 also writes a *placeholder* note for a local commit that has
// none, after each background fetch: BranchName and ModelerVersion "(unknown)",
// no changes, HasModelerVersion false. A placeholder is replaced by a real note;
// a real note is never overwritten without --force.

// mxMetadataRef is the notes ref Studio Pro reads and writes.
const mxMetadataRef = "refs/notes/mx_metadata"

// mxNote is the note's JSON. Field order is Studio Pro's.
type mxNote struct {
	BranchName        string          `json:"BranchName"`
	ModelerVersion    string          `json:"ModelerVersion"`
	ModelChanges      []mxModelChange `json:"ModelChanges"`
	RelatedStories    []string        `json:"RelatedStories"`
	SolutionVersion   string          `json:"SolutionVersion"`
	MPRFormatVersion  string          `json:"MPRFormatVersion"`
	HasModelerVersion bool            `json:"HasModelerVersion"`
}

type mxModelChange struct {
	Status   string `json:"Status"`
	UnitID   string `json:"UnitID"`
	UnitType string `json:"UnitType"`
	UnitName string `json:"UnitName"`
	Module   string `json:"Module"`
}

// marshal renders the note compactly, as Studio Pro does.
func (n mxNote) marshal() string {
	if n.ModelChanges == nil {
		n.ModelChanges = []mxModelChange{}
	}
	if n.RelatedStories == nil {
		n.RelatedStories = []string{}
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(n)
	return strings.TrimSuffix(b.String(), "\n")
}

// isPlaceholderNote reports whether text is absent or a note Studio Pro
// back-filled for a commit it did not make — safe to replace.
func isPlaceholderNote(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return true
	}
	var n struct {
		HasModelerVersion *bool `json:"HasModelerVersion"`
	}
	if err := json.Unmarshal([]byte(text), &n); err != nil {
		return false // unreadable: not ours to judge, leave it
	}
	return n.HasModelerVersion != nil && !*n.HasModelerVersion
}

// noteUnit is one row of a revision's .mpr Unit table.
type noteUnit struct {
	Container string
	Hash      string
}

// noteSnapshot is the model at one revision: its unit index and a reader for
// unit contents. A nil snapshot is the empty model (a root commit's parent).
type noteSnapshot struct {
	Version string
	Units   map[string]noteUnit
	Read    func(id string) ([]byte, error)

	heads map[string]unitHead
}

// unitHead is the part of a unit the note needs.
type unitHead struct {
	Type, Name string
}

func (s *noteSnapshot) head(id string) unitHead {
	if s.heads == nil {
		s.heads = map[string]unitHead{}
	}
	if h, ok := s.heads[id]; ok {
		return h
	}
	var h unitHead
	if raw, err := s.Read(id); err == nil {
		var doc struct {
			Type string `bson:"$Type"`
			Name string `bson:"Name"`
		}
		if bson.Unmarshal(raw, &doc) == nil {
			h = unitHead{Type: doc.Type, Name: doc.Name}
		}
	}
	s.heads[id] = h
	return h
}

// isModuleType matches Projects$ModuleImpl (and the older Projects$Module).
func isModuleType(t string) bool {
	return strings.HasPrefix(t, "Projects$Module") && !strings.HasPrefix(t, "Projects$ModuleSettings") && !strings.HasPrefix(t, "Projects$ModuleGuidMapping")
}

// moduleOf returns the id and name of the module containing id ("" for a
// project-level unit). A module unit is its own module.
func (s *noteSnapshot) moduleOf(id string) (string, string) {
	seen := map[string]bool{}
	for cur := id; cur != "" && !seen[cur]; {
		seen[cur] = true
		u, ok := s.Units[cur]
		if !ok {
			return "", ""
		}
		if h := s.head(cur); isModuleType(h.Type) {
			return cur, h.Name
		}
		cur = u.Container
	}
	return "", ""
}

// computeModelChanges lists what changed between parent and child, the way
// Studio Pro lists it. parent may be nil. touched holds the units whose file
// changed in the commit: the index alone misses a unit whose file was missing
// from the parent while its hash was already indexed.
func computeModelChanges(parent, child *noteSnapshot, touched map[string]bool) []mxModelChange {
	if parent == nil {
		parent = &noteSnapshot{Units: map[string]noteUnit{}, Read: func(string) ([]byte, error) { return nil, os.ErrNotExist }}
	}
	var out []mxModelChange
	add := func(s *noteSnapshot, status, id string) {
		h := s.head(id)
		_, module := s.moduleOf(id)
		out = append(out, mxModelChange{Status: status, UnitID: id, UnitType: h.Type, UnitName: h.Name, Module: module})
	}
	upgraded := len(parent.Units) > 0 && parent.Version != child.Version
	for id, cu := range child.Units {
		pu, existed := parent.Units[id]
		switch {
		case !existed:
			add(child, "Added", id)
		case (pu.Hash != cu.Hash || touched[id]) && contentChanged(parent, child, id):
			add(child, "Modified", id)
		case pu.Container != cu.Container:
			add(child, "Moved", id)
		case upgraded && cu.Container == id:
			// The project root: Studio Pro lists it on every version change,
			// although the version lives in the .mpr, not in its unit.
			add(child, "Modified", id)
		}
	}
	for id := range parent.Units {
		if _, kept := child.Units[id]; kept {
			continue
		}
		// A deleted module is listed once, as the module.
		if modID, _ := parent.moduleOf(id); modID != "" && modID != id {
			if _, modKept := child.Units[modID]; !modKept {
				continue
			}
		}
		add(parent, "Deleted", id)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Module != b.Module {
			return a.Module < b.Module
		}
		if a.UnitType != b.UnitType {
			return a.UnitType < b.UnitType
		}
		if a.UnitName != b.UnitName {
			return a.UnitName < b.UnitName
		}
		return a.UnitID < b.UnitID
	})
	return out
}

// contentChanged reports whether a unit whose bytes differ also differs in
// content. canon.Equal ignores which element $IDs a serialisation minted; any
// read or canonicalisation error counts as changed, so a doubt lists the unit.
func contentChanged(parent, child *noteSnapshot, id string) bool {
	a, err := parent.Read(id)
	if err != nil {
		return true
	}
	b, err := child.Read(id)
	if err != nil {
		return true
	}
	if bytes.Equal(a, b) {
		return false
	}
	if parent.Version != child.Version {
		// An upgrade re-serialises units with the properties the new version
		// added. Studio Pro does not count those; within one version every unit
		// of a type has the same properties, so this only applies across one.
		if a, b, err = dropOneSidedKeys(a, b); err != nil {
			return true
		}
	}
	eq, err := canon.Equal(a, b)
	return err != nil || !eq
}

// dropOneSidedKeys removes, from both documents, every property name that
// occurs anywhere in one of them but nowhere in the other.
func dropOneSidedKeys(a, b []byte) ([]byte, []byte, error) {
	var da, db bson.D
	if err := bson.Unmarshal(a, &da); err != nil {
		return nil, nil, err
	}
	if err := bson.Unmarshal(b, &db); err != nil {
		return nil, nil, err
	}
	ka, kb := map[string]bool{}, map[string]bool{}
	collectKeys(da, ka)
	collectKeys(db, kb)
	drop := map[string]bool{}
	for k := range ka {
		if !kb[k] {
			drop[k] = true
		}
	}
	for k := range kb {
		if !ka[k] {
			drop[k] = true
		}
	}
	if len(drop) == 0 {
		return a, b, nil
	}
	ra, err := bson.Marshal(stripKeys(da, drop))
	if err != nil {
		return nil, nil, err
	}
	rb, err := bson.Marshal(stripKeys(db, drop))
	if err != nil {
		return nil, nil, err
	}
	return ra, rb, nil
}

func collectKeys(v any, into map[string]bool) {
	switch t := v.(type) {
	case bson.D:
		for _, e := range t {
			into[e.Key] = true
			collectKeys(e.Value, into)
		}
	case bson.A:
		for _, e := range t {
			collectKeys(e, into)
		}
	}
}

func stripKeys(v any, drop map[string]bool) any {
	switch t := v.(type) {
	case bson.D:
		out := make(bson.D, 0, len(t))
		for _, e := range t {
			if !drop[e.Key] {
				out = append(out, bson.E{Key: e.Key, Value: stripKeys(e.Value, drop)})
			}
		}
		return out
	case bson.A:
		out := make(bson.A, len(t))
		for i, e := range t {
			out[i] = stripKeys(e, drop)
		}
		return out
	}
	return v
}

// --- reading a revision from git -------------------------------------------

// gitNoteRepo is a project inside a git work tree.
type gitNoteRepo struct {
	Root     string // repository top level
	MPRPath  string // .mpr path relative to Root, slash-separated
	Contents string // mprcontents path relative to Root, slash-separated
	tmpDir   string
}

func openGitNoteRepo(mprPath string) (*gitNoteRepo, error) {
	abs, err := filepath.Abs(mprPath)
	if err != nil {
		return nil, err
	}
	root, err := gitOut(filepath.Dir(abs), "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%s is not inside a git repository: %w", mprPath, err)
	}
	root = strings.TrimSpace(root)
	// Resolve symlinks on both sides so the relative path is right on macOS (/private/var).
	rRoot, _ := filepath.EvalSymlinks(root)
	rAbs, _ := filepath.EvalSymlinks(abs)
	rel, err := filepath.Rel(rRoot, rAbs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("%s is outside the repository at %s", mprPath, root)
	}
	rel = filepath.ToSlash(rel)
	tmp, err := os.MkdirTemp("", "mxcli-gitnote-")
	if err != nil {
		return nil, err
	}
	return &gitNoteRepo{Root: root, MPRPath: rel, Contents: path.Join(path.Dir(rel), "mprcontents"), tmpDir: tmp}, nil
}

func (r *gitNoteRepo) Close() { _ = os.RemoveAll(r.tmpDir) }

// gitOut runs git in dir and returns stdout; stderr goes into the error.
func gitOut(dir string, args ...string) (string, error) {
	return gitOutIn(dir, nil, args...)
}

func gitOutIn(dir string, stdin []byte, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat")
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), errors.New(msg)
	}
	return out.String(), nil
}

func (r *gitNoteRepo) blob(rev, p string) ([]byte, error) {
	out, err := gitOut(r.Root, "cat-file", "blob", rev+":"+p)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

// snapshot reads the model at rev. A revision without the .mpr yields an empty
// model (the project was added later); MPR v1 is refused.
func (r *gitNoteRepo) snapshot(rev string) (*noteSnapshot, error) {
	raw, err := r.blob(rev, r.MPRPath)
	if err != nil {
		return &noteSnapshot{Units: map[string]noteUnit{}, Read: func(string) ([]byte, error) { return nil, os.ErrNotExist }}, nil
	}
	f, err := os.CreateTemp(r.tmpDir, "rev-*.mpr")
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()
	db, err := sql.Open("sqlite", "file:"+f.Name()+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	s := &noteSnapshot{Units: map[string]noteUnit{}}
	_ = db.QueryRow("SELECT _ProductVersion FROM _MetaData LIMIT 1").Scan(&s.Version)
	var v1 int
	_ = db.QueryRow("SELECT count(*) FROM pragma_table_info('Unit') WHERE name = 'Contents'").Scan(&v1)
	if v1 > 0 {
		return nil, fmt.Errorf("%s at %s is MPR v1 (contents inside the .mpr); only MPR v2 is supported — commit it from Studio Pro", r.MPRPath, rev)
	}
	rows, err := db.Query("SELECT UnitID, ContainerID, ContentsHash FROM Unit")
	if err != nil {
		return nil, fmt.Errorf("%s at %s has no MPR v2 unit index (MPR v1 projects are not supported; commit them from Studio Pro): %w", r.MPRPath, rev, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, container []byte
		var hash sql.NullString
		if err := rows.Scan(&id, &container, &hash); err != nil {
			return nil, err
		}
		s.Units[mpr.BlobToUUID(id)] = noteUnit{Container: mpr.BlobToUUID(container), Hash: hash.String}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	s.Read = func(id string) ([]byte, error) {
		if len(id) < 4 {
			return nil, os.ErrNotExist
		}
		return r.blob(rev, path.Join(r.Contents, id[:2], id[2:4], id+".mxunit"))
	}
	return s, nil
}

// buildNote computes the note for commit rev on branch.
func (r *gitNoteRepo) buildNote(rev, branch string) (mxNote, error) {
	child, err := r.snapshot(rev)
	if err != nil {
		return mxNote{}, err
	}
	var parent *noteSnapshot
	touched := map[string]bool{}
	if _, err := gitOut(r.Root, "rev-parse", "--verify", "--quiet", rev+"^"); err == nil {
		if parent, err = r.snapshot(rev + "^"); err != nil {
			return mxNote{}, err
		}
		files, err := gitOut(r.Root, "diff-tree", "-r", "--no-commit-id", "--name-only", rev+"^", rev, "--", r.Contents)
		if err != nil {
			return mxNote{}, err
		}
		for _, f := range strings.Fields(files) {
			if strings.HasSuffix(f, ".mxunit") {
				touched[strings.TrimSuffix(path.Base(f), ".mxunit")] = true
			}
		}
	}
	return mxNote{
		BranchName:        noteBranchName(branch),
		ModelerVersion:    child.Version,
		ModelChanges:      computeModelChanges(parent, child, touched),
		MPRFormatVersion:  "Version2",
		HasModelerVersion: true,
	}, nil
}

// noteBranchName is what Studio Pro writes for branch: "" on the main line.
func noteBranchName(branch string) string {
	if branch == "main" || branch == "master" {
		return ""
	}
	return branch
}

// existingNote returns the note on rev, or "" when it has none.
func (r *gitNoteRepo) existingNote(rev string) string {
	out, err := gitOut(r.Root, "notes", "--ref="+mxMetadataRef, "show", rev)
	if err != nil {
		return ""
	}
	return out
}

func (r *gitNoteRepo) writeNote(rev, text string) error {
	_, err := gitOutIn(r.Root, []byte(text), "notes", "--ref="+mxMetadataRef, "add", "-f", "-F", "-", rev)
	return err
}
