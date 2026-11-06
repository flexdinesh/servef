package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/srcz/internal/document"
	"github.com/flexdinesh/srcz/internal/lsp"
	"github.com/flexdinesh/srcz/internal/projects"
	"github.com/flexdinesh/srcz/pkg/contract"
)

// navigationApp serves a Go project whose language server is never found.
func navigationApp(t *testing.T) (*App, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		".gitignore":       "generated/\n",
		"main.go":          "package app\n\nfunc main() { Build() }\n",
		"target.go":        "package app\n\n// Build is the target.\n\nfunc Build() {}\n",
		"generated/gen.go": "package app\n\nfunc Generated() {}\n",
		"notes.txt":        "notes\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	roots, err := projects.NewRootSet([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	servers := lsp.NewManager([]lsp.Server{{Name: "fake", Language: "Go", Families: map[string]string{"go": "go"}}}, lsp.Options{
		LookPath: func(string) (string, bool) { return "", false },
	})
	t.Cleanup(servers.Close)
	application, err := New(Config{Roots: roots, Depth: 5, Types: document.Builtins(), LanguageServers: servers})
	if err != nil {
		t.Fatal(err)
	}
	return application, root
}

func TestNavigationLocationsKeepRangesOrderAndScope(t *testing.T) {
	application, root := navigationApp(t)
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.go")); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"binary.go": "a\x00b", "large.go": strings.Repeat("x", (4<<20)+1)} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	location := func(name string, line, column int) lsp.Location {
		return lsp.Location{Path: filepath.Join(root, name), Position: lsp.Position{Line: line, Column: column}, End: lsp.Position{Line: line, Column: column + 4}}
	}
	found := []lsp.Location{location("target.go", 5, 2), location("main.go", 3, 14), location("target.go", 5, 2), location("generated/gen.go", 1, 0), location("escape.go", 1, 0), location("binary.go", 1, 0), location("large.go", 1, 0), location("../secret.go", 1, 0)}
	navigation := application.Navigation
	response := navigation.source(root).locations(found, "definition", 50)
	if len(response.Locations) != 2 || response.Filtered != 5 || response.Truncated || response.Locations[0].Path != "target.go" || response.Locations[0].EndColumn != 6 || response.Locations[0].EndLine != 5 {
		t.Fatalf("definition = %+v", response)
	}
	response = navigation.source(root).locations(found, "reference", 1000)
	if len(response.Locations) != 2 || response.Locations[0].Path != "main.go" {
		t.Fatalf("references = %+v", response)
	}
	response = navigation.source(root).locations(found, "definition", 1)
	if len(response.Locations) != 1 || !response.Truncated {
		t.Fatalf("limited = %+v", response)
	}
}

func TestNavigationFailuresCarryKindsAndCodes(t *testing.T) {
	application, root := navigationApp(t)
	ctx := context.Background()
	cases := []struct {
		name    string
		request contract.NavigationRequest
		kind    contract.ErrorKind
		code    string
	}{
		{"no project", contract.NavigationRequest{Path: "main.go", Line: 1}, contract.KindInvalid, contract.CodeNavigationUnavailable},
		{"missing project", contract.NavigationRequest{Project: filepath.Join(root, "gone"), Path: "main.go", Line: 1}, contract.KindNotFound, contract.CodeNavigationUnavailable},
		{"line 0", contract.NavigationRequest{Project: root, Path: "main.go", LanguageServers: true}, contract.KindInvalid, contract.CodeNavigationInvalid},
		{"no path", contract.NavigationRequest{Project: root, Line: 1, LanguageServers: true}, contract.KindInvalid, contract.CodeNavigationInvalid},
		{"missing file", contract.NavigationRequest{Project: root, Path: "gone.go", Line: 1, LanguageServers: true}, contract.KindNotFound, contract.CodeNavigationUnavailable},
		{"ignored", contract.NavigationRequest{Project: root, Path: "generated/gen.go", Line: 1, LanguageServers: true}, contract.KindNotFound, contract.CodeNavigationUnavailable},
		{"unsupported", contract.NavigationRequest{Project: root, Path: "notes.txt", Line: 1, LanguageServers: true}, contract.KindInvalid, contract.CodeNavigationUnsupported},
		{"disabled", contract.NavigationRequest{Project: root, Path: "main.go", Line: 1}, contract.KindInvalid, contract.CodeNavigationDisabled},
		{"missing server", contract.NavigationRequest{Project: root, Path: "main.go", Line: 1, LanguageServers: true}, contract.KindUnavailable, contract.CodeNavigationMissing},
	}
	for _, test := range cases {
		for name, navigate := range map[string]func(context.Context, contract.NavigationRequest) (contract.NavigationResult, error){
			"Definition": application.Navigation.Definition,
			"References": application.Navigation.References,
		} {
			_, err := navigate(ctx, test.request)
			var failure *Error
			if !errors.As(err, &failure) || failure.Kind != test.kind || failure.Code != test.code || failure.Message == "" {
				t.Errorf("%s %s error = %#v, want %s %s", name, test.name, err, test.kind, test.code)
			}
		}
	}
}

func TestNavigationStatusNeverStartsServers(t *testing.T) {
	application, root := navigationApp(t)
	status, err := application.Navigation.Status(contract.NavigationStatusRequest{Project: root, Path: "main.go", Refresh: true, LanguageServers: true})
	if err != nil || !status.Supported || !status.Enabled || !status.Eligible || status.Server == nil || status.Server.State != contract.LanguageServerMissing || status.Server.Capabilities != nil || status.Server.IdleMinutes != 10 {
		t.Fatalf("status = %+v, %v", status, err)
	}
	status, err = application.Navigation.Status(contract.NavigationStatusRequest{Project: root, Path: "notes.txt"})
	if err != nil || status.Supported || status.Server != nil || status.Reason == "" {
		t.Fatalf("unsupported status = %+v, %v", status, err)
	}
	_, err = application.Navigation.Status(contract.NavigationStatusRequest{Project: root})
	var failure *Error
	if !errors.As(err, &failure) || failure.Kind != contract.KindInvalid || failure.Code != contract.CodeNavigationInvalid {
		t.Fatalf("status without a file error = %v", err)
	}
}
