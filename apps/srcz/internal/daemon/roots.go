package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// rootsVersion is the roots file format version.
const rootsVersion = 1

// rootsDocument is the persisted shape: {"version":1,"roots":["/abs",...]}.
type rootsDocument struct {
	Version int      `json:"version"`
	Roots   []string `json:"roots"`
}

// DefaultRootsFile is os.UserConfigDir()/srcz/roots.json.
func DefaultRootsFile() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config: %w", err)
	}
	return filepath.Join(directory, "srcz", "roots.json"), nil
}

// LoadRoots reads the saved roots; a missing file is an empty list. A
// malformed file or an unknown version is an error naming the path, so the
// daemon refuses to start rather than overwrite it.
func LoadRoots(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read roots file: %w", err)
	}
	var document rootsDocument
	if err := json.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("roots file %s is malformed: %w", path, err)
	}
	if document.Version != rootsVersion {
		return nil, fmt.Errorf("roots file %s has version %d; this srcz reads version %d", path, document.Version, rootsVersion)
	}
	return document.Roots, nil
}

// SaveRoots writes roots atomically: the directory is 0700 and the file 0600,
// written to a temporary file in the same directory, synced, and renamed.
func SaveRoots(path string, roots []string) (err error) {
	if roots == nil {
		roots = []string{}
	}
	content, err := json.Marshal(rootsDocument{Version: rootsVersion, Roots: roots})
	if err != nil {
		return fmt.Errorf("encode roots: %w", err)
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create roots directory: %w", err)
	}
	file, err := os.CreateTemp(directory, ".roots-*.json")
	if err != nil {
		return fmt.Errorf("create roots file: %w", err)
	}
	defer func() {
		if err != nil {
			_ = file.Close()
			_ = os.Remove(file.Name())
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("restrict roots file: %w", err)
	}
	if _, err := file.Write(append(content, '\n')); err != nil {
		return fmt.Errorf("write roots file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync roots file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close roots file: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("replace roots file: %w", err)
	}
	if dir, openErr := os.Open(directory); openErr == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}
