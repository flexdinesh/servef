package watch

import (
	"errors"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

type op uint8

const (
	opCreate op = 1 << iota
	opWrite
	opRemove
	opRename
	opChmod
)

// event is one change to an entry of a watched directory.
type event struct {
	name string
	op   op
}

// backend watches single directories, not their subdirectories.
type backend interface {
	Add(dir string) error
	Remove(dir string) error
	Close() error
}

// queueSize bounds events read but not yet dispatched; more are lost and
// reported as overflow.
const queueSize = 4096

type notifyBackend struct{ watcher *fsnotify.Watcher }

// newNotifyBackend starts fsnotify. Events reach handle from one goroutine;
// a reader drains fsnotify into a bounded queue so the hub's lock never
// blocks the watcher.
func newNotifyBackend(handle func(event), overflow func()) (backend, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	queue := make(chan event, queueSize)
	lost := make(chan struct{}, 1)
	signalLost := func() {
		select {
		case lost <- struct{}{}:
		default:
		}
	}
	go func() {
		defer close(queue)
		for {
			select {
			case e, ok := <-watcher.Events:
				if !ok {
					return
				}
				select {
				case queue <- convert(e):
				default:
					signalLost()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				if errors.Is(err, fsnotify.ErrEventOverflow) {
					signalLost()
				}
			}
		}
	}()
	go func() {
		for {
			select {
			case e, ok := <-queue:
				if !ok {
					return
				}
				handle(e)
			case <-lost:
				overflow()
			}
		}
	}()
	return notifyBackend{watcher: watcher}, nil
}

func (b notifyBackend) Add(dir string) error    { return b.watcher.Add(dir) }
func (b notifyBackend) Remove(dir string) error { return b.watcher.Remove(dir) }
func (b notifyBackend) Close() error            { return b.watcher.Close() }

func convert(e fsnotify.Event) event {
	var converted op
	for _, pair := range []struct {
		from fsnotify.Op
		to   op
	}{{fsnotify.Create, opCreate}, {fsnotify.Write, opWrite}, {fsnotify.Remove, opRemove}, {fsnotify.Rename, opRename}, {fsnotify.Chmod, opChmod}} {
		if e.Has(pair.from) {
			converted |= pair.to
		}
	}
	return event{name: filepath.Clean(e.Name), op: converted}
}

func parentDir(name string) string { return filepath.Dir(name) }
