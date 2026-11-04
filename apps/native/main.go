// Command servef-native is servef's experimental native app (decision
// 0082): mygo native UI, no web view, a client of the servef daemon and of
// servef http servers through pkg/client. It runs once per user: links
// (servef-native://) and `servef-native [path]` reach the first instance.
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/flexdinesh/servef/apps/native/internal/connect"
	"github.com/flexdinesh/servef/apps/native/internal/highlight"
	"github.com/flexdinesh/servef/apps/native/internal/link"
	"github.com/flexdinesh/servef/apps/native/internal/view"
	"github.com/flexdinesh/servef/apps/native/internal/workspace"
	"github.com/flexdinesh/servef/packages/mygo/wasmcache"
)

// Stamped by release builds with -ldflags -X.
var (
	version = "0.0.0-dev"
	commit  = ""
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println(versionLine())
		return
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return // the first instance handles our arguments
	}
	// Highlights and Ondine compile once per wazero release into the
	// user cache; later launches load the compiled code. Highlights
	// compiles now, off the UI thread; Ondine on its first diagram.
	if dir, err := os.UserCacheDir(); err == nil {
		if err := wasmcache.SetDir(filepath.Join(dir, "servef-native", "wasm")); err != nil {
			log.Printf("wasm cache: %v", err)
		}
	}
	go func() {
		if err := highlight.Warm(); err != nil {
			log.Printf("highlights: %v", err)
		}
	}()
	workingDir, _ := os.Getwd()
	request, requestErr := link.FromArgs(os.Args[1:], workingDir)

	resources, _ := mygo.App.Path(mygo.PathResources)
	binary, binaryErr := connect.Locate(mygo.App.IsPackaged(), resources, os.Getenv, exec.LookPath)
	system := connect.System{Binary: binary, Flags: connect.DevFlags(os.Getenv), Run: connect.Exec, BinaryErr: binaryErr}

	runner := &windowRunner{}
	model := workspace.New(runner, system)
	app := view.New(model, runner)
	app.OpenFolder = func(done func(path string)) {
		go func() {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: runner.window.Load(), Title: "Open a folder in servef", Directory: true})
			path := ""
			if err == nil && len(paths) > 0 {
				path = paths[0]
			}
			runner.Post(func() { done(path) })
		}()
	}

	// Windows and Linux pass links as arguments, handled with the rest of
	// them; macOS sends them as Apple Events.
	if runtime.GOOS == "darwin" {
		mygo.App.OnOpenURL(func(raw string) {
			runner.Post(func() { openLink(model, raw) })
		})
	}
	mygo.App.OnSecondInstance(func(args []string, workingDir string) {
		runner.Post(func() {
			if w := runner.window.Load(); w != nil {
				w.Restore()
				w.Focus()
			}
			second, err := link.FromArgs(args, workingDir)
			if err != nil {
				model.Error = "Not a servef-native link."
				return
			}
			if second.Link != nil || second.Path != "" {
				model.Handle(second)
			}
		})
	})
	mygo.App.WhenReady(func() {
		window := mygo.NewWindow(mygo.WindowOptions{
			Title:     "servef",
			Width:     1200,
			Height:    800,
			MinWidth:  640,
			MinHeight: 400,
			StateKey:  "main",
			Content:   ui.View(app.View),
		})
		runner.window.Store(window)
		app.OnTitle(window.SetTitle)
		// A return to the window rereads the changes, as the web's
		// useRefreshOnReturn does.
		window.OnFocus(func() { runner.Post(model.Returned) })
		if requestErr != nil {
			model.Error = "Not a servef-native link."
		}
		model.Handle(request)
		model.RefreshServers()
	})
	mygo.App.OnQuit(model.Close)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func openLink(model *workspace.Model, raw string) {
	target, err := link.Parse(raw)
	if err != nil {
		model.Error = "Not a servef-native link."
		return
	}
	model.OpenLink(target)
}

func versionLine() string {
	if commit == "" {
		return "servef-native " + version
	}
	return "servef-native " + version + " " + commit
}

// windowRunner runs work on goroutines and applies results on the main
// thread through the window, which then draws a new frame.
type windowRunner struct {
	window atomic.Pointer[mygo.Window]
}

func (r *windowRunner) Go(work func() func()) {
	go func() {
		if apply := work(); apply != nil {
			r.Post(apply)
		}
	}()
}

func (r *windowRunner) Post(apply func()) {
	if w := r.window.Load(); w != nil {
		w.Update(apply)
	}
}
