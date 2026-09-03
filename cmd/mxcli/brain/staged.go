// SPDX-License-Identifier: Apache-2.0

// staged.go - the queue an agent writes to.
//
// The queue is deliberately NOT sharded. Staging is a queue, not a store, and
// routing it would force the shard decision before a human has looked at the
// entry — which is exactly the decision promotion exists to make. It lives
// under .mxcli/, which `mxcli init` git-ignores, so nothing reaches a pull
// request until someone promotes it.
package brain

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// StagedPath is the queue's location relative to the project directory.
const StagedPath = ".mxcli/brain/staged.jsonl"

// Queue is the staged half of the brain.
type Queue struct{ Path string }

// NewQueue locates the queue for a project directory.
func NewQueue(projectDir string) *Queue {
	return &Queue{Path: filepath.Join(projectDir, filepath.FromSlash(StagedPath))}
}

// Load reads the queue. A missing file is an empty queue, not an error.
func (q *Queue) Load() ([]Entry, error) {
	f, err := os.Open(q.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(text), &e); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", q.Path, line, err)
		}
		entries = append(entries, e)
	}
	return entries, sc.Err()
}

// Append queues an entry, refusing one whose id is already present.
//
// The id is content-derived, so this duplicate check costs nothing and needs no
// prose comparison: capturing the same fact twice produces the same id. It is
// cheap insurance rather than a load-bearing guard — the duplicate flood that
// motivated it in mxcli's own findings store was a many-parallel-writers
// problem, and one developer on one project has little exposure to it (A5).
func (q *Queue) Append(e Entry) (added bool, err error) {
	entries, err := q.Load()
	if err != nil {
		return false, err
	}
	for _, existing := range entries {
		if existing.ID == e.ID {
			return false, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(q.Path), 0755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(q.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return false, err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return false, err
	}
	return true, nil
}

// Drop removes an entry from the queue by id.
func (q *Queue) Drop(id string) (bool, error) {
	entries, err := q.Load()
	if err != nil {
		return false, err
	}
	kept := make([]Entry, 0, len(entries))
	found := false
	for _, e := range entries {
		if e.ID == id {
			found = true
			continue
		}
		kept = append(kept, e)
	}
	if !found {
		return false, nil
	}
	return true, q.write(kept)
}

func (q *Queue) write(entries []Entry) error {
	if len(entries) == 0 {
		err := os.Remove(q.Path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var b strings.Builder
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			return err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(q.Path), 0755); err != nil {
		return err
	}
	return os.WriteFile(q.Path, []byte(b.String()), 0644)
}

// Get returns the queued entry with the given id.
func (q *Queue) Get(id string) (Entry, bool, error) {
	entries, err := q.Load()
	if err != nil {
		return Entry{}, false, err
	}
	for _, e := range entries {
		if e.ID == id {
			return e, true, nil
		}
	}
	return Entry{}, false, nil
}
