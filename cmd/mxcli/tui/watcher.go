package tui

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

// MprChangedMsg signals that the MPR project files were modified externally.
type MprChangedMsg struct{}

// MsgSender abstracts the Send method for testability.
type MsgSender interface {
	Send(msg tea.Msg)
}

// programSender wraps a tea.Program to satisfy MsgSender.
type programSender struct{ prog *tea.Program }

func (p programSender) Send(msg tea.Msg) { p.prog.Send(msg) }

// Watcher monitors MPR project files for changes and notifies the TUI.
type Watcher struct {
	fsw         *fsnotify.Watcher
	done        chan struct{}
	closeOnce   sync.Once
	mu          sync.Mutex
	suppressEnd time.Time

	// debounce is how long a burst of filesystem events is collapsed over. It
	// is a field rather than the constant so a test can choose a window that
	// cannot race with its own burst — see newWatcherDebounced.
	debounce time.Duration
}

const watchDebounce = 500 * time.Millisecond

// NewWatcher creates a file watcher that sends MprChangedMsg to prog.
//   - mprPath: path to the .mpr file
//   - contentsDir: path to mprcontents/ directory (empty string for v1)
func NewWatcher(mprPath, contentsDir string, prog *tea.Program) (*Watcher, error) {
	return newWatcher(mprPath, contentsDir, programSender{prog: prog})
}

func newWatcher(mprPath, contentsDir string, sender MsgSender) (*Watcher, error) {
	return newWatcherDebounced(mprPath, contentsDir, sender, watchDebounce)
}

// newWatcherDebounced is newWatcher with the debounce window chosen by the
// caller.
//
// It exists for tests, and for a specific reason. Asserting that a burst of
// writes collapses into ONE message means the whole burst has to land inside
// the window — with the production 500ms that is true on an idle machine and
// not guaranteed on a loaded CI runner, where the burst itself can outlast the
// window and an intermediate timer fires. Passing a window far longer than any
// plausible burst removes the race from the test instead of narrowing it.
func newWatcherDebounced(mprPath, contentsDir string, sender MsgSender, debounce time.Duration) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		debounce: debounce,
		fsw:      fsw,
		done:     make(chan struct{}),
	}

	if contentsDir != "" {
		// MPR v2: mprcontents/ has a 2-level hash directory structure (e.g. f3/26/).
		// fsnotify does not recurse, so walk all subdirectories and add each one.
		err = filepath.WalkDir(contentsDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return fsw.Add(path)
			}
			return nil
		})
	} else {
		err = fsw.Add(mprPath)
	}
	if err != nil {
		fsw.Close()
		return nil, err
	}

	go w.run(sender)
	return w, nil
}

func (w *Watcher) run(sender MsgSender) {
	var debounceTimer *time.Timer
	var debounceSeq atomic.Uint64

	for {
		select {
		case <-w.done:
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return

		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Remove) {
				continue
			}
			ext := filepath.Ext(event.Name)
			// Allow .mpr, .mxunit, and extensionless files (MPR v2 mprcontents/ hash files).
			if ext != ".mpr" && ext != ".mxunit" && ext != "" {
				continue
			}

			w.mu.Lock()
			suppressed := time.Now().Before(w.suppressEnd)
			w.mu.Unlock()
			if suppressed {
				continue
			}

			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			seq := debounceSeq.Add(1)
			debounceTimer = time.AfterFunc(w.debounce, func() {
				if debounceSeq.Load() != seq {
					return
				}
				sender.Send(MprChangedMsg{})
			})

		case watchErr, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			Trace("watcher: fsnotify error: %v", watchErr)
		}
	}
}

// Suppress causes the watcher to ignore changes for the given duration.
// Use this when mxcli itself modifies the MPR (e.g., exec command).
func (w *Watcher) Suppress(d time.Duration) {
	w.mu.Lock()
	w.suppressEnd = time.Now().Add(d)
	w.mu.Unlock()
}

// Close stops the watcher and releases resources. Safe to call concurrently.
func (w *Watcher) Close() {
	w.closeOnce.Do(func() {
		close(w.done)
		w.fsw.Close()
	})
}
