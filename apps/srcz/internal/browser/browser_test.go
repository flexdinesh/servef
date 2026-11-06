package browser

import (
	"errors"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type recordingRunner struct {
	name  string
	args  []string
	err   error
	calls int
}

func (r *recordingRunner) Run(name string, args ...string) error {
	r.calls++
	r.name = name
	r.args = append([]string(nil), args...)
	return r.err
}

func TestOpenUsesPlatformCommand(t *testing.T) {
	t.Parallel()

	const target = "http://localhost:8080/docs"
	tests := []struct {
		name     string
		goos     string
		wantName string
		wantArgs []string
	}{
		{name: "macOS", goos: "darwin", wantName: "open", wantArgs: []string{target}},
		{name: "Linux", goos: "linux", wantName: "xdg-open", wantArgs: []string{target}},
		{name: "Windows", goos: "windows", wantName: "rundll32", wantArgs: []string{"url.dll,FileProtocolHandler", target}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			runner := &recordingRunner{}

			if err := open(target, tt.goos, runner); err != nil {
				t.Fatalf("open() error = %v", err)
			}
			if runner.calls != 1 {
				t.Fatalf("runner calls = %d, want 1", runner.calls)
			}
			if runner.name != tt.wantName {
				t.Errorf("command name = %q, want %q", runner.name, tt.wantName)
			}
			if !reflect.DeepEqual(runner.args, tt.wantArgs) {
				t.Errorf("command args = %#v, want %#v", runner.args, tt.wantArgs)
			}
		})
	}
}

func TestOpenWithRunnerUsesCurrentPlatform(t *testing.T) {
	t.Parallel()

	wantName, wantArgs, err := command("https://example.com", runtime.GOOS)
	if err != nil {
		t.Skipf("current platform is unsupported: %v", err)
	}
	runner := &recordingRunner{}

	if err := OpenWithRunner("https://example.com", runner); err != nil {
		t.Fatalf("OpenWithRunner() error = %v", err)
	}
	if runner.name != wantName || !reflect.DeepEqual(runner.args, wantArgs) {
		t.Errorf("runner got (%q, %#v), want (%q, %#v)", runner.name, runner.args, wantName, wantArgs)
	}
}

func TestOpenRejectsEmptyTarget(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{}
	err := open("", "linux", runner)

	if err == nil || !strings.Contains(err.Error(), "target is empty") {
		t.Fatalf("open() error = %v, want empty-target error", err)
	}
	if runner.calls != 0 {
		t.Errorf("runner calls = %d, want 0", runner.calls)
	}
}

func TestOpenRejectsNilRunner(t *testing.T) {
	t.Parallel()

	err := open("https://example.com", "linux", nil)
	if err == nil || !strings.Contains(err.Error(), "runner is nil") {
		t.Fatalf("open() error = %v, want nil-runner error", err)
	}
}

func TestOpenRejectsUnsupportedPlatform(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{}
	err := open("https://example.com", "plan9", runner)

	if err == nil || !strings.Contains(err.Error(), `unsupported operating system "plan9"`) {
		t.Fatalf("open() error = %v, want unsupported-platform error", err)
	}
	if runner.calls != 0 {
		t.Errorf("runner calls = %d, want 0", runner.calls)
	}
}

func TestOpenWrapsRunnerError(t *testing.T) {
	t.Parallel()

	runErr := errors.New("executable not found")
	runner := &recordingRunner{err: runErr}
	err := open("https://example.com", "linux", runner)

	if !errors.Is(err, runErr) {
		t.Fatalf("open() error = %v, want wrapped runner error", err)
	}
	if !strings.Contains(err.Error(), `run "xdg-open"`) {
		t.Errorf("open() error = %v, want command context", err)
	}
}
