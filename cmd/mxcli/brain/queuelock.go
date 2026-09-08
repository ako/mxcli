// SPDX-License-Identifier: Apache-2.0

// queuelock.go - serialising read-modify-write on the staged queue.
//
// Capture is an append and needs no help: a short O_APPEND write lands whole
// beside another one, which is why three simultaneous captures were measured
// arriving as three well-formed lines. Promote and drop are the problem. Both
// load the queue, rebuild it in memory and write it back, so a capture landing
// in that window is overwritten by a snapshot taken before it existed. The
// queue stays well-formed and one line is simply gone — there is nothing to
// notice, which is what makes it worth preventing rather than detecting.
//
// The cost of that is decided by what the queue is FOR. In one long session a
// lost capture is a note whose author can still recall it. Under one sub-agent
// per slice the queue is the only channel between slices, so a capture is the
// agent's return value and losing one loses the decision itself.
//
// A lock file rather than flock(2): the queue is a handful of short-lived CLI
// processes on one machine, mxcli ships for Windows as well as Linux, and
// O_EXCL is the one primitive that means the same thing everywhere without
// taking a dependency. The cost is that a process killed mid-write leaves the
// file behind, so the lock is taken over once it is old enough to be certain
// nobody is still working — a delay is recoverable, a permanently unwritable
// queue is not.
package brain

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	// staleQueueLock is how old a lock must be before it is assumed abandoned.
	// Every operation under it is a load, an in-memory rebuild and a write of a
	// file that holds tens of entries, so it is orders of magnitude longer than
	// a real hold and still short enough not to strand an agent.
	staleQueueLock = 30 * time.Second
	// queueLockWait bounds the wait for a live holder. Reaching it means real
	// contention rather than a crash, and failing is better than a capture that
	// appears to have worked.
	queueLockWait = 20 * time.Second
	queueLockPoll = 5 * time.Millisecond
)

func queueLockPath(queuePath string) string { return queuePath + ".lock" }

// acquireQueueLock blocks until it owns the queue's lock, and returns the
// function that releases it. The returned function is safe to call more than
// once so a deferred release cannot double-remove a lock someone else has since
// taken.
func acquireQueueLock(queuePath string) (func(), error) {
	lock := queueLockPath(queuePath)
	if err := os.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(queueLockWait)
	for {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			// The pid is for a human reading a stuck lock, not for the takeover
			// decision: a pid means nothing across containers, and this queue is
			// written from inside them.
			fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
			released := false
			return func() {
				if released {
					return
				}
				released = true
				_ = os.Remove(lock)
			}, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}

		if st, err := os.Stat(lock); err == nil && time.Since(st.ModTime()) > staleQueueLock {
			// Abandoned. Remove it and try again; if two processes both decide
			// this, one of them still loses the O_EXCL race on the next pass.
			_ = os.Remove(lock)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%s is locked by another mxcli process (waited %s); "+
				"if nothing else is running, delete %s", filepath.Base(queuePath), queueLockWait, lock)
		}
		time.Sleep(queueLockPoll)
	}
}

// writeFileAtomic replaces path in one step, so a reader either sees the whole
// old file or the whole new one. Load parses a JSON object per line, so a
// half-written queue does not read as a shorter queue — it reads as a corrupt
// one, and blames capture for what an interrupted promote did.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-"+strconv.Itoa(os.Getpid())+"-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op once the rename has succeeded

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	// Durability matters more than speed here: the queue is the only copy of
	// an agent's captures until someone promotes them.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		return err
	}
	return os.Rename(name, path)
}
