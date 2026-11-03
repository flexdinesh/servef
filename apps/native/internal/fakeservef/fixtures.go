package fakeservef

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/flexdinesh/servef/pkg/contract"
)

// fixtureDir is testdata/api/default: the shared API fixtures, which the
// server's tests keep valid (diffs carry the line diff the server computes
// for their contents).
func fixtureDir(t testing.TB) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("fakeservef: no caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "testdata", "api", "default")
}

// Fixture decodes testdata/api/default/name into value.
func Fixture(t testing.TB, name string, value any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir(t), name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

// Diff decodes a diff fixture, as "diff-hunks.json".
func Diff(t testing.TB, name string) contract.DiffResponse {
	t.Helper()
	var diff contract.DiffResponse
	Fixture(t, name, &diff)
	return diff
}

// LoadChanges installs the fixtures' working-tree changes (changes.json)
// for project, and their diffs by path.
func (s *Server) LoadChanges(t testing.TB, project string) {
	t.Helper()
	var changes contract.ChangesResponse
	Fixture(t, "changes.json", &changes)
	diffs := map[string]contract.DiffResponse{}
	for _, name := range []string{"diff-binary.json", "diff-added.json", "diff-modified.json", "diff-deleted.json", "diff-renamed.json", "diff-untracked.json", "diff-hunks.json"} {
		diff := Diff(t, name)
		diffs[diff.Path] = diff
	}
	s.Set(func(s *Server) {
		s.Changes[project] = changes
		for path, diff := range diffs {
			s.Diffs[path] = diff
		}
	})
}
