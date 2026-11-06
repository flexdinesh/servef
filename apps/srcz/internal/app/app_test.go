package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/srcz/internal/document"
	"github.com/flexdinesh/srcz/internal/projects"
	"github.com/flexdinesh/srcz/pkg/contract"
)

func newTestApp(t *testing.T) (*App, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data.json"), []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	roots, err := projects.NewRootSet([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	application, err := New(Config{Roots: roots, Depth: 5, Types: document.Builtins()})
	if err != nil {
		t.Fatal(err)
	}
	return application, root
}

func kindOf(t *testing.T, err error) contract.ErrorKind {
	t.Helper()
	var failure *Error
	if !errors.As(err, &failure) {
		t.Fatalf("error %v is not an *Error", err)
	}
	return failure.Kind
}

func TestProjectResolutionFailuresAreTyped(t *testing.T) {
	application, root := newTestApp(t)
	cases := map[string]contract.ErrorKind{
		"":                               contract.KindInvalid,
		filepath.Dir(root):               contract.KindInvalid,
		filepath.Join(root, "missing"):   contract.KindNotFound,
		filepath.Join(root, "data.json"): contract.KindNotFound,
	}
	for id, want := range cases {
		if _, err := application.Projects.Resolve(id); kindOf(t, err) != want {
			t.Errorf("Resolve(%q) error = %v, want %s", id, err, want)
		}
	}
	if project, err := application.Projects.Resolve(root); err != nil || project.Root != root {
		t.Errorf("Resolve(root) = %+v, %v", project, err)
	}
}

func TestOperationsReturnTypedErrorsWithoutTransport(t *testing.T) {
	application, root := newTestApp(t)
	if _, err := application.Documents.Format(contract.FormatRequest{Project: root, Path: "data.json", At: "HEAD"}); kindOf(t, err) != contract.KindInvalid {
		t.Errorf("Format at an invalid hash error = %v, want invalid", err)
	}
	page, err := application.Documents.Page(contract.PageRequest{Project: root, Path: "gone.md", Required: true})
	if kindOf(t, err) != contract.KindNotFound || page.Error != err.Error() || page.Project != root {
		t.Errorf("Page of a missing file = %+v, %v; want not found carried in the page", page, err)
	}
	// The lease is checked before the project: it needs no file system.
	_, err = application.Events.Subscribe(contract.EventsRequest{Lease: "forever"})
	var failure *Error
	if !errors.As(err, &failure) || failure.Kind != contract.KindInvalid || failure.Code != contract.CodeInvalidLease {
		t.Errorf("Subscribe with an invalid lease error = %v, want invalid %s", err, contract.CodeInvalidLease)
	}
	if _, err := application.Events.Subscribe(contract.EventsRequest{Project: root}); kindOf(t, err) != contract.KindUnavailable {
		t.Errorf("Subscribe without a hub error = %v, want unavailable", err)
	}
}
