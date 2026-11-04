package memory

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// saveDelay is how long a Save waits for later ones: a drag saves every
// frame, and the file is written once it settles.
const saveDelay = 500 * time.Millisecond

// Store keeps the file at Path: Load reads it once, and Save writes the
// latest memory soon, off the caller's thread, in order. Memory is a
// convenience: failures are logged and the app keeps working. A file of
// another version is never overwritten.
type Store struct {
	Path string
	// Delay overrides saveDelay.
	Delay time.Duration

	// writeFile overrides write in tests.
	writeFile func(path string, data []byte) error

	mu      sync.Mutex
	pending *File
	frozen  bool
	// wake ends the writer's wait; idle closes when it stops, nil when none
	// runs; flushing is a Flush waiting for it.
	wake     chan struct{}
	idle     chan struct{}
	flushing bool
}

// Load reads the file; a missing or malformed file is nothing remembered.
// A file of another version is nothing remembered too, and stays as it is.
func (s *Store) Load() File {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return File{}
	}
	if foreign(data) {
		log.Printf("workspace memory: %s is another version; not reading or writing it", s.Path)
		s.mu.Lock()
		s.frozen = true
		s.mu.Unlock()
		return File{}
	}
	return Parse(data)
}

// Save writes f soon; a later Save before the write replaces it. It does no
// work on the caller's thread, so a drag can save every frame.
func (s *Store) Save(f File) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.frozen {
		return
	}
	s.pending = &f
	if s.idle != nil {
		return
	}
	s.wake, s.idle = make(chan struct{}, 1), make(chan struct{})
	go s.drain(s.wake)
}

// drain waits out the delay, or a Flush, and writes the latest, until a
// write leaves nothing pending.
func (s *Store) drain(wake <-chan struct{}) {
	delay := s.Delay
	if delay == 0 {
		delay = saveDelay
	}
	writeFile := s.writeFile
	if writeFile == nil {
		writeFile = write
	}
	for {
		select {
		case <-time.After(delay):
		case <-wake:
		}
		s.mu.Lock()
		f := s.pending
		s.pending = nil
		s.mu.Unlock()
		if err := writeFile(s.Path, f.Marshal()); err != nil {
			log.Printf("workspace memory: %v", err)
		}
		s.mu.Lock()
		if s.pending == nil {
			close(s.idle)
			s.idle = nil
			s.mu.Unlock()
			return
		}
		if s.flushing {
			s.nudge()
		}
		s.mu.Unlock()
	}
}

// nudge ends the writer's wait; s.mu is held.
func (s *Store) nudge() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Flush writes what is pending now and waits for it, as at quit.
func (s *Store) Flush() {
	s.mu.Lock()
	idle := s.idle
	if idle == nil {
		s.mu.Unlock()
		return
	}
	s.flushing = true
	s.nudge()
	s.mu.Unlock()
	<-idle
	s.mu.Lock()
	s.flushing = false
	s.mu.Unlock()
}

// write replaces the file atomically, private to the user. mygo makes the
// user data directory.
func write(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".workspaces-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
