package connection

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

const (
	storeVersion = 1
	// lockWait bounds waiting for another process's save.
	lockWait = 2 * time.Second
	// lockStale is when a lock's owner is presumed dead: saves take
	// milliseconds.
	lockStale = 10 * time.Second
)

// Saved is a paired remote server. Credential is a secret; the file holding it
// is private to the user.
type Saved struct {
	ID         string    `json:"id"`
	Label      string    `json:"label"`
	Hosts      []string  `json:"hosts"`
	Port       int       `json:"port"`
	Credential string    `json:"credential"`
	AddedAt    time.Time `json:"addedAt"`
}

// Endpoint is how to reach the saved server.
func (s Saved) Endpoint() (TLS, error) {
	credential, err := ParseCredential(s.Credential)
	if err != nil {
		return TLS{}, err
	}
	return TLS{Hosts: s.Hosts, Port: s.Port, Credential: credential}, nil
}

type storeDocument struct {
	Version     int     `json:"version"`
	Connections []Saved `json:"connections"`
}

// Store is the saved connections. `srcz connect` writes it and the desktop
// app reads it, so a link can name a connection without carrying its secret.
type Store struct{ path string }

// DefaultStorePath is os.UserConfigDir()/srcz/connections.json.
func DefaultStorePath() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config: %w", err)
	}
	return filepath.Join(directory, "srcz", "connections.json"), nil
}

// NewStore returns the store at path.
func NewStore(path string) Store { return Store{path: path} }

// List returns the saved connections; a missing file is an empty list. A
// malformed file or an unknown version is an error, so saves never overwrite
// what this build cannot read.
func (s Store) List() ([]Saved, error) {
	content, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read connections: %w", err)
	}
	var document storeDocument
	if err := json.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("connections file %s is malformed: %w", s.path, err)
	}
	if document.Version != storeVersion {
		return nil, fmt.Errorf("connections file %s has version %d; this srcz reads version %d", s.path, document.Version, storeVersion)
	}
	return document.Connections, nil
}

// Lookup finds a saved connection by id.
func (s Store) Lookup(id string) (Saved, bool, error) {
	saved, err := s.List()
	if err != nil {
		return Saved{}, false, err
	}
	index := slices.IndexFunc(saved, func(entry Saved) bool { return entry.ID == id })
	if index < 0 {
		return Saved{}, false, nil
	}
	return saved[index], true, nil
}

// Put saves a connection, replacing one with its id. Concurrent saves from
// other processes do not lose each other's entries.
func (s Store) Put(entry Saved) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create connections directory: %w", err)
	}
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	saved, err := s.List()
	if err != nil {
		return err
	}
	saved = slices.DeleteFunc(saved, func(existing Saved) bool { return existing.ID == entry.ID })
	return s.write(append(saved, entry))
}

func (s Store) write(saved []Saved) error {
	content, err := json.MarshalIndent(storeDocument{Version: storeVersion, Connections: saved}, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".connections-*")
	if err != nil {
		return fmt.Errorf("write connections: %w", err)
	}
	defer func() { _ = os.Remove(temporary.Name()) }()
	// CreateTemp's file is 0600, so the secret is never briefly world-readable.
	if _, err := temporary.Write(append(content, '\n')); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write connections: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("write connections: %w", err)
	}
	if err := os.Rename(temporary.Name(), s.path); err != nil {
		return fmt.Errorf("write connections: %w", err)
	}
	return nil
}

// lock takes a lock file, portably: O_EXCL works on every platform the CLI
// builds for. A lock older than lockStale is presumed abandoned.
func (s Store) lock() (func(), error) {
	path := s.path + ".lock"
	deadline := time.Now().Add(lockWait)
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = file.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("lock connections: %w", err)
		}
		if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) > lockStale {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("lock connections: %s is held", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
