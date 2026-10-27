package walkthrough

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrNotFound is returned for a walkthrough the store does not hold.
var ErrNotFound = errors.New("walkthrough not found")

// DefaultDirectory returns the per-user directory walkthroughs are stored in.
func DefaultDirectory() (string, error) {
	directory, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate user cache: %w", err)
	}
	return filepath.Join(directory, "servef", "walkthroughs"), nil
}

// Store keeps walkthroughs as private JSON files, one directory per project
// named by a hash of its ID, never inside the project. Writes replace files
// atomically. Listing decodes only files changed since the last listing.
type Store struct {
	directory string
	limit     int

	mu        sync.Mutex
	summaries map[string]cachedSummary
}

type cachedSummary struct {
	modified time.Time
	size     int64
	summary  Summary
}

// NewStore returns a store rooted at directory that keeps at most
// MaxPerProject walkthroughs per project.
func NewStore(directory string) *Store {
	return &Store{directory: directory, limit: MaxPerProject, summaries: map[string]cachedSummary{}}
}

// Put stores w, replacing a walkthrough with its ID, then evicts the least
// recently updated past the per-project limit. It reports whether w is new.
func (s *Store) Put(w Walkthrough) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	directory := s.projectDirectory(w.Project)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return false, fmt.Errorf("create walkthrough directory: %w", err)
	}
	name := filepath.Join(directory, w.ID+".json")
	_, err := os.Stat(name)
	created := errors.Is(err, fs.ErrNotExist)
	contents, err := json.Marshal(w)
	if err != nil {
		return false, fmt.Errorf("encode walkthrough: %w", err)
	}
	if err := writeAtomic(name, contents); err != nil {
		return false, err
	}
	if err := s.evict(w.Project, w.ID); err != nil {
		return created, err
	}
	return created, nil
}

// writeAtomic replaces name with contents through a private temporary file.
func writeAtomic(name string, contents []byte) error {
	file, err := os.CreateTemp(filepath.Dir(name), ".walkthrough-*")
	if err != nil {
		return fmt.Errorf("write walkthrough: %w", err)
	}
	temporary := file.Name()
	defer func() { _ = os.Remove(temporary) }()
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return fmt.Errorf("write walkthrough: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("write walkthrough: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("write walkthrough: %w", err)
	}
	if err := os.Rename(temporary, name); err != nil {
		return fmt.Errorf("write walkthrough: %w", err)
	}
	return nil
}

// evict removes the least recently updated walkthroughs past the limit,
// never keep.
func (s *Store) evict(project, keep string) error {
	summaries, err := s.list(project)
	if err != nil {
		return err
	}
	others := slices.DeleteFunc(summaries, func(summary Summary) bool { return summary.ID == keep })
	for i := s.limit - 1; i < len(others); i++ {
		if err := s.remove(project, others[i].ID); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
	}
	return nil
}

// List returns a project's walkthroughs, most recently updated first.
func (s *Store) List(project string) ([]Summary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list(project)
}

func (s *Store) list(project string) ([]Summary, error) {
	directory := s.projectDirectory(project)
	entries, err := os.ReadDir(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return []Summary{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list walkthroughs: %w", err)
	}
	summaries := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || !entry.Type().IsRegular() || !idPattern.MatchString(id) {
			continue
		}
		name := filepath.Join(directory, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		cached, ok := s.summaries[name]
		if !ok || !cached.modified.Equal(info.ModTime()) || cached.size != info.Size() {
			w, err := readWalkthrough(name)
			if err != nil || w.Project != project || w.ID != id {
				// Unreadable or foreign files are skipped, not fatal.
				continue
			}
			cached = cachedSummary{modified: info.ModTime(), size: info.Size(), summary: Summary{ID: w.ID, Title: w.Title, StepCount: len(w.Steps), UpdatedAt: w.UpdatedAt}}
			s.summaries[name] = cached
		}
		summaries = append(summaries, cached.summary)
	}
	sort.SliceStable(summaries, func(a, b int) bool {
		if !summaries[a].UpdatedAt.Equal(summaries[b].UpdatedAt) {
			return summaries[a].UpdatedAt.After(summaries[b].UpdatedAt)
		}
		return summaries[a].ID < summaries[b].ID
	})
	return summaries, nil
}

// Get returns a project's walkthrough by ID.
func (s *Store) Get(project, id string) (Walkthrough, error) {
	if !idPattern.MatchString(id) {
		return Walkthrough{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := readWalkthrough(filepath.Join(s.projectDirectory(project), id+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return Walkthrough{}, ErrNotFound
	}
	if err != nil {
		return Walkthrough{}, err
	}
	if w.Project != project || w.ID != id {
		return Walkthrough{}, ErrNotFound
	}
	return w, nil
}

// Delete removes a project's walkthrough by ID.
func (s *Store) Delete(project, id string) error {
	if !idPattern.MatchString(id) {
		return ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.remove(project, id)
}

func (s *Store) remove(project, id string) error {
	name := filepath.Join(s.projectDirectory(project), id+".json")
	err := os.Remove(name)
	delete(s.summaries, name)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete walkthrough: %w", err)
	}
	return nil
}

func (s *Store) projectDirectory(project string) string {
	sum := sha256.Sum256([]byte(project))
	return filepath.Join(s.directory, hex.EncodeToString(sum[:8]))
}

func readWalkthrough(name string) (Walkthrough, error) {
	contents, err := os.ReadFile(name)
	if err != nil {
		return Walkthrough{}, err
	}
	var w Walkthrough
	if err := json.Unmarshal(contents, &w); err != nil {
		return Walkthrough{}, fmt.Errorf("read walkthrough %s: %w", filepath.Base(name), err)
	}
	return w, nil
}
