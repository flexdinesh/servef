package walkthrough

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

// Targets checks a draft's steps against its project. Check returns the
// project's canonical ID and the steps with abbreviated hashes resolved and
// missing Expect prefixes recorded where the target can drift. A Problems
// error rejects the draft; any other error is a failure to check.
type Targets interface {
	Check(ctx context.Context, project string, steps []Step) (string, []Step, error)
}

// Service publishes, lists, reads, and deletes walkthroughs. Every transport
// (HTTP now; MCP later) goes through it.
type Service struct {
	store   *Store
	targets Targets
	now     func() time.Time
}

// NewService composes a store with the target checks of a project host.
func NewService(store *Store, targets Targets) *Service {
	return &Service{store: store, targets: targets, now: time.Now}
}

// Published is the outcome of Publish.
type Published struct {
	Walkthrough Walkthrough
	// Created reports a new ID rather than a replaced walkthrough.
	Created bool
}

// Publish validates a draft, checks its targets, and stores it, replacing a
// walkthrough with the same ID in the same project.
func (s *Service) Publish(ctx context.Context, draft Draft) (Published, error) {
	draft, err := draft.Normalize()
	if err != nil {
		return Published{}, err
	}
	project, steps, err := s.targets.Check(ctx, draft.Project, draft.Steps)
	if err != nil {
		return Published{}, err
	}
	now := s.now().UTC()
	w := Walkthrough{ID: draft.ID, Project: project, Title: draft.Title, Summary: draft.Summary, Steps: steps, CreatedAt: now, UpdatedAt: now}
	if w.ID == "" {
		w.ID, err = newID(draft.Title)
		if err != nil {
			return Published{}, err
		}
	} else if previous, err := s.store.Get(project, w.ID); err == nil {
		w.CreatedAt = previous.CreatedAt
	} else if !errors.Is(err, ErrNotFound) {
		return Published{}, err
	}
	created, err := s.store.Put(w)
	if err != nil {
		return Published{}, err
	}
	return Published{Walkthrough: w, Created: created}, nil
}

// List returns a project's walkthroughs, most recently updated first.
func (s *Service) List(project string) ([]Summary, error) { return s.store.List(project) }

// Get returns a project's walkthrough, or ErrNotFound.
func (s *Service) Get(project, id string) (Walkthrough, error) { return s.store.Get(project, id) }

// Delete removes a project's walkthrough, or returns ErrNotFound.
func (s *Service) Delete(project, id string) error { return s.store.Delete(project, id) }

// newID is the title's slug with a random suffix, so drafts without IDs
// never replace each other.
func newID(title string) (string, error) {
	suffix := make([]byte, 3)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}
	prefix := slug(title)
	if prefix == "" {
		prefix = "walkthrough"
	}
	return prefix + "-" + hex.EncodeToString(suffix), nil
}
