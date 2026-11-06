// Package browser opens URLs and other browser-compatible targets using the
// operating system's default handler.
package browser

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
)

// Runner executes a command. Implementations can be injected with
// OpenWithRunner, which is useful for testing and for callers that need custom
// process handling.
type Runner interface {
	Run(name string, args ...string) error
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

// Open opens target with the operating system's default browser or handler.
func Open(target string) error {
	return OpenWithRunner(target, execRunner{})
}

// OpenWithRunner opens target using runner to execute the platform command.
func OpenWithRunner(target string, runner Runner) error {
	return open(target, runtime.GOOS, runner)
}

func open(target, goos string, runner Runner) error {
	if target == "" {
		return errors.New("open browser: target is empty")
	}
	if runner == nil {
		return errors.New("open browser: runner is nil")
	}

	name, args, err := command(target, goos)
	if err != nil {
		return err
	}
	if err := runner.Run(name, args...); err != nil {
		return fmt.Errorf("open browser: run %q for %q: %w", name, target, err)
	}
	return nil
}

func command(target, goos string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "open", []string{target}, nil
	case "linux":
		return "xdg-open", []string{target}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", target}, nil
	default:
		return "", nil, fmt.Errorf("open browser: unsupported operating system %q", goos)
	}
}
