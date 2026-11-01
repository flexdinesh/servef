package daemon

import (
	goruntime "runtime"
	"strings"
	"testing"
)

func TestSocketPath(t *testing.T) {
	long := "/" + strings.Repeat("x", 100)
	tests := []struct {
		name    string
		env     map[string]string
		tempDir string
		darwin  bool
		want    string
	}{
		{name: "linux runtime dir", env: map[string]string{"XDG_RUNTIME_DIR": "/run/user/501"}, tempDir: "/tmp", want: "/run/user/501/servef/daemon.sock"},
		{name: "linux without runtime dir", tempDir: "/var/tmp", want: "/var/tmp/servef-501/daemon.sock"},
		{name: "linux relative runtime dir", env: map[string]string{"XDG_RUNTIME_DIR": "run"}, tempDir: "/tmp", want: "/tmp/servef-501/daemon.sock"},
		{name: "macOS TMPDIR", env: map[string]string{"XDG_RUNTIME_DIR": "/run/user/501"}, tempDir: "/var/folders/ab/xyz/T/", darwin: true, want: "/var/folders/ab/xyz/T/servef/daemon.sock"},
		{name: "macOS long TMPDIR", tempDir: long, darwin: true, want: "/tmp/servef-501/daemon.sock"},
		{name: "linux long runtime dir", env: map[string]string{"XDG_RUNTIME_DIR": long}, tempDir: "/tmp", want: "/tmp/servef-501/daemon.sock"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := socketPath(func(name string) string { return test.env[name] }, test.tempDir, 501, test.darwin)
			if err != nil || got != test.want {
				t.Fatalf("socketPath() = %q, %v; want %q", got, err, test.want)
			}
		})
	}
}

func TestFirstFittingErrorsWhenNothingFits(t *testing.T) {
	if got, err := firstFitting(10, "/tmp/servef-501/daemon.sock"); err == nil || !strings.Contains(err.Error(), "--socket") {
		t.Fatalf("firstFitting() = %q, %v; want error naming --socket", got, err)
	}
}

func TestCheckSocketLengthAtTheLimit(t *testing.T) {
	limit := maxSocketLength(goruntime.GOOS == "darwin")
	if err := checkSocketLength("/" + strings.Repeat("s", limit-1)); err != nil {
		t.Fatalf("checkSocketLength(%d bytes) = %v", limit, err)
	}
	if err := checkSocketLength("/" + strings.Repeat("s", limit)); err == nil {
		t.Fatalf("checkSocketLength(%d bytes) = nil", limit+1)
	}
}
