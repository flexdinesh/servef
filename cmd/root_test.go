package cmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatih/color"
	"github.com/flexdinesh/servef/internal/control"
	"github.com/spf13/cobra"
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
	if strings.Join(got.paths, ",") != "." || got.depth != 5 || got.scanDepth != 10 || got.host != defaultHost || got.port != 0 || got.noOpen {
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
	command.SetArgs([]string{"docs", "--depth", "3", "--host", "192.0.2.10", "--port", "8080", "--no-open"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.paths, ",") != "docs" || got.depth != 3 || got.host != "192.0.2.10" || got.port != 8080 || !got.portSet || !got.noOpen {
		t.Fatalf("parsed options = %+v", got)
	}
}

func TestRootCommandMergesPaths(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"--path", "work", "--path", "personal"}, "work,personal"},
		{[]string{"docs", "--path", "work"}, "docs,work"},
		{[]string{"--path", "work", "docs"}, "docs,work"},
	} {
		var got options
		command := newRootCommand(func(_ context.Context, opts options, _ io.Reader, _, _ io.Writer) error {
			got = opts
			return nil
		})
		command.SetArgs(test.args)
		if err := command.Execute(); err != nil {
			t.Fatal(err)
		}
		if paths := strings.Join(got.paths, ","); paths != test.want {
			t.Errorf("Execute(%v) paths = %s, want %s", test.args, paths, test.want)
		}
	}
}

func TestRootCommandValidatesArguments(t *testing.T) {
	tests := [][]string{
		{"one", "two"},
		{"--depth", "-1"},
		{"--scan-depth", "-1"},
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

func TestRunSelectsFreePortAndStopsWithContext(t *testing.T) {
	disableColor(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	if err := run(ctx, options{paths: []string{root}, depth: 5, noOpen: true, controlDir: t.TempDir()}, strings.NewReader(""), &stdout, &stderr); err != nil {
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
		!strings.Contains(stdout.String(), "  projects discovered: 1  (") ||
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
		Version:      "servef 1.2.3",
		Directories:  []string{"/tmp/notes"},
		URL:          "http://localhost:4173/",
		ProjectCount: 12,
		Discovery:    42 * time.Millisecond,
	})

	want := "  servef 1.2.3\n" +
		"  serving directory:  /tmp/notes\n" +
		"  url:  http://localhost:4173/\n" +
		"\n" +
		"  projects discovered: 12  (42ms)\n" +
		"  ctrl-c to stop.\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}

	output.Reset()
	writeStartup(&output, startupInfo{Version: "servef 1.2.3", Directories: []string{"/tmp/a", "/tmp/b"}, URL: "http://localhost:4173/"})
	if got := output.String(); !strings.Contains(got, "  serving directories:  /tmp/a\n                        /tmp/b\n") {
		t.Fatalf("multi-root output = %q", got)
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
	err = run(context.Background(), options{paths: []string{root}, depth: 5, host: "127.0.0.1", port: port, portSet: true, noOpen: true, controlDir: t.TempDir()}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "listen on 127.0.0.1 port") {
		t.Fatalf("run() error = %v", err)
	}
}

func TestEphemeralServersStayIndependent(t *testing.T) {
	disableColor(t)
	controlDir := t.TempDir()
	start := func(content string) (string, string, func()) {
		t.Helper()
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		reader, writer := io.Pipe()
		urls := make(chan string, 1)
		go func() {
			defer reader.Close()
			scanner := bufio.NewScanner(reader)
			for scanner.Scan() {
				if url, ok := strings.CutPrefix(scanner.Text(), "  url:  "); ok {
					urls <- url
				}
			}
		}()
		done := make(chan error, 1)
		go func() {
			err := run(ctx, options{paths: []string{root}, depth: 5, host: "127.0.0.1", port: 0, portSet: true, noOpen: true, controlDir: controlDir}, strings.NewReader(""), writer, io.Discard)
			writer.Close()
			done <- err
		}()
		var once sync.Once
		stop := func() {
			once.Do(func() {
				cancel()
				select {
				case err := <-done:
					if err != nil {
						t.Error(err)
					}
				case <-time.After(10 * time.Second):
					t.Error("server did not stop")
				}
			})
		}
		t.Cleanup(stop)
		select {
		case address := <-urls:
			return address, root, stop
		case <-time.After(10 * time.Second):
			t.Fatal("server did not start")
			return "", root, stop
		}
	}
	read := func(address, root, want string) {
		t.Helper()
		client := http.Client{Timeout: 2 * time.Second}
		response, err := client.Get(address + "api/page?path=README.md&project=" + url.QueryEscape(root))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(want)) {
			t.Fatalf("page = %s, status %d, error %v; want %q", body, response.StatusCode, err, want)
		}
	}
	first, firstRoot, stopFirst := start("first worktree")
	second, secondRoot, _ := start("second worktree")
	if first == second {
		t.Fatal("servers share a port")
	}
	read(first, firstRoot, "first worktree")
	read(second, secondRoot, "second worktree")
	stopFirst()
	read(second, secondRoot, "second worktree")
}

func disableColor(t *testing.T) {
	t.Helper()
	previous := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = previous })
}

func TestRootCommandRejectsRemovedFlags(t *testing.T) {
	for _, args := range [][]string{{"--file-type", "md"}, {"--filename", "*.md"}, {"--exclude", "drafts"}} {
		command := newRootCommand(func(context.Context, options, io.Reader, io.Writer, io.Writer) error {
			t.Fatal("runner called for removed flag")
			return nil
		})
		command.SetArgs(args)
		if err := command.Execute(); err == nil {
			t.Fatalf("%s accepted", args[0])
		}
	}
}

func TestRunDiscoversProjects(t *testing.T) {
	disableColor(t)
	root := t.TempDir()
	for _, repo := range []string{"api", "web", filepath.Join("deep", "a", "b", "c")} {
		if err := os.MkdirAll(filepath.Join(root, repo, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	if err := run(ctx, options{paths: []string{root}, depth: 5, scanDepth: 3, noOpen: true, controlDir: t.TempDir()}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	// The root folder plus api and web; deep/a/b/c is beyond --scan-depth.
	if !strings.Contains(stdout.String(), "  projects discovered: 3  (") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestRunServesEveryPathAndRejectsOverlap(t *testing.T) {
	disableColor(t)
	parent := t.TempDir()
	work, personal := filepath.Join(parent, "work"), filepath.Join(parent, "personal")
	for _, repo := range []string{filepath.Join(work, "api"), filepath.Join(personal, "blog")} {
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout bytes.Buffer
	if err := run(ctx, options{paths: []string{work, personal, work}, depth: 5, scanDepth: 3, noOpen: true, controlDir: t.TempDir()}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); !strings.Contains(got, "serving directories:  ") || !strings.Contains(got, "  projects discovered: 4  (") {
		t.Fatalf("stdout = %q", got)
	}

	stdout.Reset()
	err := run(context.Background(), options{paths: []string{parent, work}, depth: 5, noOpen: true, controlDir: t.TempDir()}, strings.NewReader(""), &stdout, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "overlap") || stdout.Len() != 0 {
		t.Fatalf("run(nested roots) = %v, stdout %q; want overlap error before serving", err, stdout.String())
	}
}

func executeCommand(t *testing.T, args []string) (options, error) {
	t.Helper()
	var got options
	command := newRootCommand(func(_ context.Context, opts options, _ io.Reader, _, _ io.Writer) error {
		got = opts
		return nil
	})
	command.SetArgs(args)
	return got, command.Execute()
}

func TestBrowserCommandMatchesRoot(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"docs"},
		{"docs", "--path", "work", "--path", "personal", "--depth", "3", "--scan-depth", "2", "--host", "192.0.2.10", "--port", "8080", "--no-open"},
		{"--port", "0"},
	} {
		root, rootErr := executeCommand(t, args)
		browser, browserErr := executeCommand(t, append([]string{"browser"}, args...))
		if rootErr != nil || browserErr != nil {
			t.Fatalf("Execute(%v) errors = %v, %v", args, rootErr, browserErr)
		}
		if !reflect.DeepEqual(root, browser) {
			t.Errorf("Execute(%v) root = %+v, browser = %+v", args, root, browser)
		}
	}
}

func TestBrowserCommandDefaultsToCurrentDirectory(t *testing.T) {
	got, err := executeCommand(t, []string{"browser"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.paths, ",") != "." || got.depth != 5 || got.scanDepth != 10 || got.host != defaultHost || got.port != 0 || got.portSet || got.noOpen {
		t.Fatalf("browser options = %+v", got)
	}
}

func TestRootCommandServesPositionalDirectories(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"somedir"}, "somedir"},
		{[]string{"./browser"}, "./browser"},
		{[]string{"./walkthrough"}, "./walkthrough"},
		{[]string{"browser", "browser"}, "browser"},
	} {
		got, err := executeCommand(t, test.args)
		if err != nil {
			t.Fatalf("Execute(%v) error = %v", test.args, err)
		}
		if paths := strings.Join(got.paths, ","); paths != test.want {
			t.Errorf("Execute(%v) paths = %s, want %s", test.args, paths, test.want)
		}
	}
}

func TestBrowserCommandValidatesArgumentsLikeRoot(t *testing.T) {
	for _, args := range [][]string{
		{"one", "two"},
		{"--depth", "-1"},
		{"--scan-depth", "-1"},
		{"--host", "example.com"},
		{"--port", "65536"},
		{"--filename", "*.md"},
	} {
		command := newRootCommand(func(context.Context, options, io.Reader, io.Writer, io.Writer) error {
			t.Fatal("runner called for invalid arguments")
			return nil
		})
		command.SetArgs(args)
		rootErr := command.Execute()
		command = newRootCommand(func(context.Context, options, io.Reader, io.Writer, io.Writer) error {
			t.Fatal("runner called for invalid arguments")
			return nil
		})
		command.SetArgs(append([]string{"browser"}, args...))
		browserErr := command.Execute()
		if rootErr == nil || browserErr == nil {
			t.Fatalf("Execute(%v) errors = %v, %v", args, rootErr, browserErr)
		}
		if rootErr.Error() != browserErr.Error() {
			t.Errorf("Execute(%v) root error = %q, browser error = %q", args, rootErr, browserErr)
		}
	}
}

func TestBrowserCommandHelp(t *testing.T) {
	var output bytes.Buffer
	command := newRootCommand(func(context.Context, options, io.Reader, io.Writer, io.Writer) error {
		t.Fatal("runner called for help")
		return nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{"browser", "--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !strings.Contains(got, "Usage:\n  servef browser [path] [flags]") {
		t.Fatalf("help output = %q", got)
	}
}

func TestWalkthroughSubcommandIsNotServed(t *testing.T) {
	newCommand := func() *cobra.Command {
		return newRootCommand(func(context.Context, options, io.Reader, io.Writer, io.Writer) error {
			t.Fatal("browser runner called for walkthrough")
			return nil
		})
	}
	command := newCommand()
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	command.SetArgs([]string{"walkthrough", "publish", filepath.Join(t.TempDir(), "missing.json")})
	if err := command.Execute(); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("publish missing file error = %v", err)
	}

	var output bytes.Buffer
	command = newCommand()
	command.SetOut(&output)
	command.SetArgs([]string{"walkthrough", "--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"servef walkthrough [command]", "publish", "schema"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("help lacks %q:\n%s", want, output.String())
		}
	}

	command = newCommand()
	command.SetOut(io.Discard)
	command.SetArgs([]string{"walkthrough", "publsh"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), `unknown command "publsh"`) {
		t.Errorf("unknown walkthrough subcommand error = %v", err)
	}
}

func TestRunStopsOnControlRequest(t *testing.T) {
	disableColor(t)
	controlDir := t.TempDir()
	root := t.TempDir()
	reader, writer := io.Pipe()
	urls := make(chan string, 1)
	go func() {
		defer reader.Close()
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			if address, ok := strings.CutPrefix(scanner.Text(), "  url:  "); ok {
				urls <- address
			}
		}
	}()
	done := make(chan error, 1)
	go func() {
		err := run(context.Background(), options{paths: []string{root}, depth: 5, host: "127.0.0.1", port: 0, portSet: true, noOpen: true, controlDir: controlDir}, strings.NewReader(""), writer, io.Discard)
		writer.Close()
		done <- err
	}()
	var address string
	select {
	case address = <-urls:
	case <-time.After(10 * time.Second):
		t.Fatal("server did not start")
	}
	parsed, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	remote, err := control.NewRegistry(controlDir).Lookup("127.0.0.1", port)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := remote.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run() = %v, want nil after control stop", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not stop on control request")
	}
	if _, err := control.NewRegistry(controlDir).Lookup("127.0.0.1", port); err == nil {
		t.Fatal("control record remains after stop")
	}
}

func TestRunRejectsUnreadableRootBeforeServing(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads any directory")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	var stdout bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := run(ctx, options{paths: []string{root}, depth: 5, scanDepth: 3, noOpen: true, controlDir: t.TempDir()}, strings.NewReader(""), &stdout, &bytes.Buffer{})
	if err == nil || !strings.HasPrefix(err.Error(), "read discovery root: ") || stdout.Len() != 0 {
		t.Fatalf("run(unreadable root) = %v, stdout %q; want read discovery root error before serving", err, stdout.String())
	}
}
