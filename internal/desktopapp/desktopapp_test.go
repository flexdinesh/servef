package desktopapp

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fakeRunner records commands and answers each by its name.
type fakeRunner struct {
	calls   [][]string
	outputs map[string]string
	errs    map[string]error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return []byte(f.outputs[name]), f.errs[name]
}

func (f *fakeRunner) Launch(name string, args ...string) error {
	f.calls = append(f.calls, append([]string{name}, args...))
	return f.errs[name]
}

// exitError mimics *exec.ExitError's exit code.
type exitError int

func (e exitError) Error() string { return fmt.Sprintf("exit status %d", int(e)) }
func (e exitError) ExitCode() int { return int(e) }

const target = "servef://local/?project=%2Ftmp%2Fp"

var query = []string{"xdg-mime", "query", "default", "x-scheme-handler/servef"}

func TestInstalledPerOS(t *testing.T) {
	for _, test := range []struct {
		name   string
		goos   string
		runner *fakeRunner
		want   bool
		err    bool
		calls  [][]string
	}{
		{"darwin lets open decide", "darwin", &fakeRunner{}, true, false, nil},
		{"linux with a handler", "linux", &fakeRunner{outputs: map[string]string{"xdg-mime": "servef-desktop.desktop\n"}}, true, false, [][]string{query}},
		{"linux without a handler", "linux", &fakeRunner{outputs: map[string]string{"xdg-mime": "\n"}}, false, false, [][]string{query}},
		{"linux without xdg-mime", "linux", &fakeRunner{errs: map[string]error{"xdg-mime": &exec.Error{Name: "xdg-mime", Err: exec.ErrNotFound}}}, false, false, [][]string{query}},
		{"linux query failure", "linux", &fakeRunner{errs: map[string]error{"xdg-mime": exitError(1)}}, false, true, [][]string{query}},
		{"windows", "windows", &fakeRunner{}, false, false, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := installed(context.Background(), test.goos, test.runner)
			if got != test.want || (err != nil) != test.err {
				t.Errorf("installed = %v, %v; want %v, error %v", got, err, test.want, test.err)
			}
			if !reflect.DeepEqual(test.runner.calls, test.calls) {
				t.Errorf("calls = %v, want %v", test.runner.calls, test.calls)
			}
		})
	}
}

func TestOpenPerOS(t *testing.T) {
	for _, test := range []struct {
		name         string
		goos         string
		runner       *fakeRunner
		notInstalled bool
		err          bool
		calls        [][]string
	}{
		{"darwin opens", "darwin", &fakeRunner{}, false, false, [][]string{{"open", target}}},
		{"darwin without a handler", "darwin", &fakeRunner{errs: map[string]error{"open": exitError(1)}}, true, true, [][]string{{"open", target}}},
		{"darwin open missing", "darwin", &fakeRunner{errs: map[string]error{"open": exec.ErrNotFound}}, false, true, [][]string{{"open", target}}},
		{"linux opens", "linux", &fakeRunner{outputs: map[string]string{"xdg-mime": "servef-desktop.desktop\n"}}, false, false, [][]string{query, {"xdg-open", target}}},
		{"linux without a handler", "linux", &fakeRunner{}, true, true, [][]string{query}},
		{"linux xdg-open failure", "linux", &fakeRunner{outputs: map[string]string{"xdg-mime": "servef-desktop.desktop\n"}, errs: map[string]error{"xdg-open": exitError(4)}}, false, true, [][]string{query, {"xdg-open", target}}},
		{"windows", "windows", &fakeRunner{}, true, true, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := open(context.Background(), target, test.goos, test.runner)
			if (err != nil) != test.err || errors.Is(err, ErrNotInstalled) != test.notInstalled {
				t.Errorf("open = %v; want error %v, not installed %v", err, test.err, test.notInstalled)
			}
			if !reflect.DeepEqual(test.runner.calls, test.calls) {
				t.Errorf("calls = %v, want %v", test.runner.calls, test.calls)
			}
		})
	}
}

func TestInstallHintPerOS(t *testing.T) {
	footer := []string{
		"curl -fsSL https://raw.githubusercontent.com/flexdinesh/servef/main/scripts/install-desktop.sh | sh",
		"`servef browser` serves in a web browser",
		"https://github.com/flexdinesh/servef/blob/main/site/src/pages/desktop.mdx",
	}
	for _, test := range []struct {
		goos string
		want []string
	}{
		{"darwin", append([]string{
			"servef-desktop_<version>_darwin_<arm64|amd64>.dmg",
			"https://github.com/flexdinesh/servef/releases",
			"Applications",
			"right-click",
			"xattr -dr com.apple.quarantine /Applications/servef.app",
		}, footer...)},
		{"linux", append([]string{
			"sudo apt install ./servef-desktop_<version>_linux_<amd64|arm64>.deb",
			"https://github.com/flexdinesh/servef/releases",
			"AppImageLauncher",
		}, footer...)},
		{"windows", []string{"not available on this OS"}},
	} {
		hint := InstallHint(test.goos)
		for _, want := range test.want {
			if !strings.Contains(hint, want) {
				t.Errorf("InstallHint(%s) lacks %q:\n%s", test.goos, want, hint)
			}
		}
		if !strings.HasSuffix(hint, "\n") {
			t.Errorf("InstallHint(%s) does not end with a newline", test.goos)
		}
	}
}

func TestLaunchReportsEarlyExitAndReleasesRunningCommands(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	run := execRunner{wait: 2 * time.Second}
	if err := run.Launch("sh", "-c", "exit 3"); err == nil {
		t.Error("Launch of a failing command = nil, want its exit error")
	}
	if err := run.Launch("sh", "-c", "exit 0"); err != nil {
		t.Errorf("Launch of a succeeding command = %v", err)
	}
	started := time.Now()
	if err := (execRunner{wait: 100 * time.Millisecond}).Launch("sh", "-c", "sleep 5"); err != nil {
		t.Errorf("Launch of a running command = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("Launch waited %v for a running command", elapsed)
	}
}
