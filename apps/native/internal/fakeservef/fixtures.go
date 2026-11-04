package fakeservef

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/flexdinesh/servef/pkg/contract"
)

// FixtureDir is testdata/api/default in the source tree: the shared API
// fixtures, which the server's tests keep valid (diffs carry the line diff
// the server computes for their contents). The web UI's mock mode
// (apps/web/dev-server.ts) serves the same files.
func FixtureDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("testdata", "api", "default")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "testdata", "api", "default")
}

// Fixture decodes testdata/api/default/name into value.
func Fixture(t testing.TB, name string, value any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(FixtureDir(), name))
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

// LoadFixtures installs what the web UI's mock mode serves from dir (a
// copy of testdata/api/default): projects.json, every page-*.json by its
// project and path (page-index.json for no document), changes.json for
// every project, each diff-*.json by its path, and raw-preview.png as the
// image assets/preview.png. page-commit.json (a commit snapshot) and
// page-missing.json (the server's not-found answer) are left out.
func (s *Server) LoadFixtures(dir string) error {
	read := func(name string, value any) error {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, value); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	}
	var projects contract.ProjectsResponse
	if err := read("projects.json", &projects); err != nil {
		return err
	}
	var changes contract.ChangesResponse
	if err := read("changes.json", &changes); err != nil {
		return err
	}
	names, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return err
	}
	var pages []contract.PageData
	diffs := map[string]contract.DiffResponse{}
	for _, path := range names {
		name := filepath.Base(path)
		switch {
		case name == "page-commit.json" || name == "page-missing.json":
		case strings.HasPrefix(name, "page-"):
			var page contract.PageData
			if err := read(name, &page); err != nil {
				return err
			}
			pages = append(pages, page)
		case strings.HasPrefix(name, "diff-"):
			var diff contract.DiffResponse
			if err := read(name, &diff); err != nil {
				return err
			}
			diffs[diff.Path] = diff
		}
	}
	// As apps/web/dev-server.ts's mockFormats.
	var formatted contract.FormatResult
	if err := read("format-settings.json", &formatted); err != nil {
		return err
	}
	var refused contract.ErrorResponse
	if err := read("format-error.json", &refused); err != nil {
		return err
	}
	preview, err := os.ReadFile(filepath.Join(dir, "raw-preview.png"))
	if err != nil {
		return err
	}
	s.Set(func(s *Server) {
		s.Projects = projects
		for _, project := range projects.Projects {
			s.Changes[project.ID] = changes
		}
		for path, diff := range diffs {
			s.Diffs[path] = diff
		}
		s.Images["assets/preview.png"] = preview
		s.Formats["config/settings.json"] = formatted.Formatted
		s.FormatErrors["config/broken.json"] = refused.Error
	})
	for _, page := range pages {
		s.Page(page)
	}
	return nil
}
