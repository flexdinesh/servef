package cmd

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/flexdinesh/servef/internal/features"
)

func TestRootCommandDefaults(t *testing.T) {
	var got options
	command := newRootCommand(func(_ context.Context, opts options, _ io.Reader, _, _ io.Writer) error {
		got = opts
		return nil
	})
	command.SetArgs(nil)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.path != "." || got.depth != 5 || got.host != defaultHost || got.port != 0 || got.noOpen || len(got.exclusions) != 0 || got.features.BrowserData().MermaidTldraw {
		t.Fatalf("default options = %+v", got)
	}
}

func TestRootCommandHelpUsesServef(t *testing.T) {
	var output bytes.Buffer
	command := newRootCommand(func(context.Context, options, io.Reader, io.Writer, io.Writer) error {
		t.Fatal("runner called for help")
		return nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !strings.Contains(got, "Usage:\n  servef [path] [flags]") {
		t.Fatalf("help output = %q", got)
	}
}

func TestRootCommandPrintsVersionWithoutRunning(t *testing.T) {
	var output bytes.Buffer
	command := newRootCommand(func(context.Context, options, io.Reader, io.Writer, io.Writer) error {
		t.Fatal("runner called for version")
		return nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{"--version"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "servef dev\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestRootCommandParsesAdditiveFlags(t *testing.T) {
	var got options
	command := newRootCommand(func(_ context.Context, opts options, _ io.Reader, _, _ io.Writer) error {
		got = opts
		return nil
	})
	command.SetArgs([]string{"docs", "--depth", "3", "--host", "192.0.2.10", "--port", "8080", "--no-open", "--exclude", "drafts", "--exclude", "generated", "--feature", features.MermaidTldraw, "--feature", features.MermaidTldraw})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.path != "docs" || got.depth != 3 || got.host != "192.0.2.10" || got.port != 8080 || !got.portSet || !got.noOpen {
		t.Fatalf("parsed options = %+v", got)
	}
	if strings.Join(got.exclusions, ",") != "drafts,generated" {
		t.Fatalf("exclusions = %v", got.exclusions)
	}
	if !got.features.BrowserData().MermaidTldraw {
		t.Fatal("mermaid-tldraw feature not enabled")
	}
}

func TestRootCommandValidatesArguments(t *testing.T) {
	tests := [][]string{
		{"one", "two"},
		{"--depth", "-1"},
		{"--host", "example.com"},
		{"--port", "65536"},
	}
	for _, args := range tests {
		command := newRootCommand(func(context.Context, options, io.Reader, io.Writer, io.Writer) error {
			t.Fatal("runner called for invalid arguments")
			return nil
		})
		command.SetArgs(args)
		if err := command.Execute(); err == nil {
			t.Fatalf("Execute(%v) error = nil", args)
		}
	}
}

func TestRootCommandRejectsUnknownFeatureBeforeRun(t *testing.T) {
	command := newRootCommand(func(context.Context, options, io.Reader, io.Writer, io.Writer) error {
		t.Fatal("runner called for unknown feature")
		return nil
	})
	command.SetArgs([]string{"--feature", "unknown"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown feature "unknown"`) || !strings.Contains(err.Error(), features.MermaidTldraw) {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestRunSelectsFreePortAndStopsWithContext(t *testing.T) {
	disableColor(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	if err := run(ctx, options{path: root, depth: 5, noOpen: true, controlDir: t.TempDir()}, strings.NewReader(""), &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	localPort := ""
	for _, line := range strings.Split(stdout.String(), "\n") {
		if strings.HasPrefix(line, "  url:  http://localhost:") {
			localPort = strings.TrimSuffix(strings.TrimPrefix(line, "  url:  http://localhost:"), "/")
		}
	}
	port, err := strconv.Atoi(localPort)
	if err != nil || port < defaultPortStart || port > defaultPortEnd {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "  servef dev\n") ||
		!strings.Contains(stdout.String(), "  serving directory:  "+root+"\n") ||
		!strings.Contains(stdout.String(), "  files discovered: 1  (") ||
		!strings.Contains(stdout.String(), "ms)\n") ||
		!strings.HasSuffix(stdout.String(), "  ctrl-c to stop.\n") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestListenDefaultsToLoopback(t *testing.T) {
	listener, err := listen(defaultHost, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address type = %T", listener.Addr())
	}
	if !address.IP.IsLoopback() {
		t.Fatalf("listener address = %s, want loopback", address)
	}
}

func TestListenBindsPassedIP(t *testing.T) {
	listener, err := listen("0.0.0.0", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address type = %T", listener.Addr())
	}
	if !address.IP.IsUnspecified() {
		t.Fatalf("listener address = %s, want wildcard", address)
	}
}

func TestWriteStartup(t *testing.T) {
	disableColor(t)
	var output bytes.Buffer
	writeStartup(&output, startupInfo{
		Version:   "servef 1.2.3",
		Directory: "/tmp/notes",
		URL:       "http://localhost:4173/",
		FileCount: 12,
		Discovery: 42 * time.Millisecond,
	})

	want := "  servef 1.2.3\n" +
		"  serving directory:  /tmp/notes\n" +
		"  url:  http://localhost:4173/\n" +
		"\n" +
		"  files discovered: 12  (42ms)\n" +
		"  ctrl-c to stop.\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestShouldOpenBrowser(t *testing.T) {
	tests := []struct {
		name   string
		noOpen bool
		env    map[string]string
		want   bool
	}{
		{name: "local session", want: true},
		{name: "no-open flag", noOpen: true, want: false},
		{name: "SSH connection", env: map[string]string{"SSH_CONNECTION": "client server"}, want: false},
		{name: "SSH client", env: map[string]string{"SSH_CLIENT": "client"}, want: false},
		{name: "SSH TTY", env: map[string]string{"SSH_TTY": "/dev/pts/1"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			getenv := func(name string) string { return test.env[name] }
			if got := shouldOpenBrowser(test.noOpen, getenv); got != test.want {
				t.Fatalf("shouldOpenBrowser() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestRunReportsOccupiedExplicitPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	root := t.TempDir()
	err = run(context.Background(), options{path: root, depth: 5, host: "127.0.0.1", port: port, portSet: true, noOpen: true, controlDir: t.TempDir()}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "listen on 127.0.0.1 port") {
		t.Fatalf("run() error = %v", err)
	}
}

func disableColor(t *testing.T) {
	t.Helper()
	previous := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = previous })
}

func TestRootCommandFilenameScope(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		patterns []string
		included []string
		excluded []string
	}{
		{name: "implicit all", patterns: []string{"*"}, included: []string{"README.md", "main.go", ".env", "Dockerfile", "data.bin"}},
		{name: "explicit all", args: []string{"--filename", "*"}, patterns: []string{"*"}, included: []string{"notes.txt"}},
		{name: "one glob", args: []string{"--filename", "*.md"}, patterns: []string{"*.md"}, included: []string{"README.md", "docs/guide.MD"}, excluded: []string{"main.go", "README.markdown"}},
		{name: "multiple globs and names", args: []string{"--filename", "*.md,*.go,Dockerfile"}, patterns: []string{"*.md", "*.go", "Dockerfile"}, included: []string{"a.md", "b.go", "deploy/Dockerfile"}, excluded: []string{"Makefile", "c.ts"}},
		{name: "case-insensitive and trimmed", args: []string{"--filename= *.GO , dockerFILE "}, patterns: []string{"*.GO", "dockerFILE"}, included: []string{"main.go", "Dockerfile"}, excluded: []string{"README.md"}},
		{name: "character class", args: []string{"--filename", "[mn]*.txt"}, patterns: []string{"[mn]*.txt"}, included: []string{"notes.txt"}, excluded: []string{"other.txt"}},
	}
	for _, entry := range tests {
		t.Run(entry.name, func(t *testing.T) {
			var got options
			command := newRootCommand(func(_ context.Context, opts options, _ io.Reader, _, _ io.Writer) error { got = opts; return nil })
			command.SetArgs(append([]string{"docs"}, entry.args...))
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if strings.Join(got.filenames.Patterns(), "|") != strings.Join(entry.patterns, "|") {
				t.Fatalf("patterns = %v, want %v", got.filenames.Patterns(), entry.patterns)
			}
			for _, name := range entry.included {
				if !got.filenames.Match(name) {
					t.Errorf("%s excluded", name)
				}
			}
			for _, name := range entry.excluded {
				if got.filenames.Match(name) {
					t.Errorf("%s included", name)
				}
			}
		})
	}
}

func TestRootCommandRejectsInvalidFilenameLists(t *testing.T) {
	for _, value := range []string{"", " ", ",*.md", "*.md,", "*.md,,*.go", "[a-", "*.md,[", "docs/*.md"} {
		t.Run(value, func(t *testing.T) {
			command := newRootCommand(func(context.Context, options, io.Reader, io.Writer, io.Writer) error {
				t.Fatal("runner called for invalid filename scope")
				return nil
			})
			command.SetArgs([]string{"--filename", value})
			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), "--filename") {
				t.Fatalf("Execute() error = %v", err)
			}
		})
	}
}

func TestRootCommandRemovesFileTypeFlag(t *testing.T) {
	command := newRootCommand(func(context.Context, options, io.Reader, io.Writer, io.Writer) error {
		t.Fatal("runner called for removed flag")
		return nil
	})
	command.SetArgs([]string{"--file-type", "md"})
	if err := command.Execute(); err == nil {
		t.Fatal("--file-type accepted")
	}
}

func TestRunScopesDiscoveryByFilename(t *testing.T) {
	disableColor(t)
	root := t.TempDir()
	for _, name := range []string{"README.md", "main.go", "Dockerfile"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	filter, err := parseFilenames("*.go,dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	if err := run(ctx, options{path: root, depth: 5, noOpen: true, filenames: filter, controlDir: t.TempDir()}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "  files discovered: 2  (") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}
