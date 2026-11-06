package client_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/srcz/internal/lsp"
	"github.com/flexdinesh/srcz/internal/lsp/lsptest"
	"github.com/flexdinesh/srcz/pkg/client"
	"github.com/flexdinesh/srcz/pkg/contract"
)

// The test binary doubles as a fake language server.
func TestMain(m *testing.M) {
	if lsptest.Serving() {
		lsptest.Serve()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// newNavigationFixture serves a Go project through the fake language
// server; the first navigation starts it.
func newNavigationFixture(t *testing.T) fixture {
	t.Helper()
	t.Setenv(lsptest.Env, "1")
	root := t.TempDir()
	for name, content := range map[string]string{
		"main.go":   "package app\n\nfunc main() { Build() }\n",
		"target.go": "package app\n\n// Build is the target.\n\nfunc Build() {}\n",
		"notes.txt": "notes\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	servers := lsp.NewManager([]lsp.Server{{
		Name:     "fake",
		Language: "Go",
		Families: map[string]string{"go": "go"},
		Install:  []lsp.Install{{Label: "Go", Command: "go install fake@latest", Prerequisites: []string{"Go"}}},
		Docs:     "https://example.com/fake",
	}}, lsp.Options{LookPath: func(string) (string, bool) { return os.Args[0], true }})
	return serveFixtureWith(t, root, servers)
}

func TestNavigationParityAcrossTransports(t *testing.T) {
	f := newNavigationFixture(t)
	ctx := context.Background()
	at := func(path string, line, column int, enabled bool) contract.NavigationRequest {
		return contract.NavigationRequest{Project: f.root, Path: path, Line: line, Column: column, LanguageServers: enabled}
	}

	status := f.same(t, "NavigationStatus before start", func(c client.Client) outcome {
		return result(c.NavigationStatus(ctx, contract.NavigationStatusRequest{Project: f.root, Path: "main.go", Refresh: true}))
	})
	if got, ok := status.Value.(contract.NavigationStatus); !ok || !got.Supported || got.Enabled || got.Server == nil || got.Server.State != contract.LanguageServerStopped || len(got.Server.Install) != 1 {
		t.Errorf("NavigationStatus = %+v", status)
	}

	failures := []struct {
		name    string
		request contract.NavigationRequest
		kind    contract.ErrorKind
		code    string
	}{
		{"disabled", at("main.go", 3, 15, false), contract.KindInvalid, contract.CodeNavigationDisabled},
		{"invalid", at("main.go", 0, 0, true), contract.KindInvalid, contract.CodeNavigationInvalid},
		{"unsupported", at("notes.txt", 1, 0, true), contract.KindInvalid, contract.CodeNavigationUnsupported},
		{"missing", at("gone.go", 1, 0, true), contract.KindNotFound, contract.CodeNavigationUnavailable},
		{"no project", contract.NavigationRequest{Path: "main.go", Line: 1, LanguageServers: true}, contract.KindInvalid, contract.CodeNavigationUnavailable},
	}
	for _, failure := range failures {
		got := f.same(t, "Definition "+failure.name, func(c client.Client) outcome { return result(c.Definition(ctx, failure.request)) })
		var clientErr *client.Error
		if !errors.As(got.Err, &clientErr) || clientErr.Kind != failure.kind || clientErr.Code != failure.code {
			t.Errorf("Definition %s error = %v, want %s %s", failure.name, got.Err, failure.kind, failure.code)
		}
	}

	definition := f.same(t, "Definition", func(c client.Client) outcome { return result(c.Definition(ctx, at("main.go", 3, 15, true))) })
	if got, ok := definition.Value.(contract.NavigationResult); !ok || definition.Err != nil || got.Symbol != "Build" || got.Server != "fake" || len(got.Locations) != 1 || got.Locations[0].Path != "target.go" || got.Locations[0].Line != 5 {
		t.Errorf("Definition = %+v", definition)
	}
	references := f.same(t, "References", func(c client.Client) outcome { return result(c.References(ctx, at("main.go", 3, 15, true))) })
	if got, ok := references.Value.(contract.NavigationResult); !ok || references.Err != nil || len(got.Locations) == 0 {
		t.Errorf("References = %+v", references)
	}
	ready := f.same(t, "NavigationStatus after start", func(c client.Client) outcome {
		return result(c.NavigationStatus(ctx, contract.NavigationStatusRequest{Project: f.root, Path: "main.go", LanguageServers: true}))
	})
	if got, ok := ready.Value.(contract.NavigationStatus); !ok || got.Server == nil || got.Server.State != contract.LanguageServerReady || got.Server.Capabilities == nil || !got.Server.Capabilities.Definition {
		t.Errorf("NavigationStatus after start = %+v", ready)
	}
}
