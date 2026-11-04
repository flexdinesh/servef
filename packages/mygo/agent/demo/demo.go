// Package demo is a tiny counter view that exercises the agent tooling
// without an app: a count, buttons, a text input, a context menu, and work
// that finishes on another goroutine.
package demo

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
)

// Runner runs work off the view's goroutine, as session.Session does.
type Runner interface {
	Go(work func() func())
}

// Counter is the view's state.
type Counter struct {
	N      int
	Name   string
	Loaded string
	Menu   string
	run    Runner
}

// New returns a counter whose Load button runs on run.
func New(run Runner) *Counter { return &Counter{run: run} }

// View builds the window.
func (app *Counter) View(c *ui.Context) {
	ui.Column(c).Fill().Gap(12).Children(func() {
		ui.Text(c, fmt.Sprintf("Count: %d", app.N))
		if ui.Button(c, "Add").Clicked() {
			app.N++
		}
		ui.TextInput(c, &app.Name).Label("Name")
		ui.Text(c, "Hello, "+app.Name)
		if ui.Button(c, "Load").Clicked() {
			app.run.Go(func() func() {
				time.Sleep(20 * time.Millisecond)
				return func() { app.Loaded = "Loaded" }
			})
		}
		if app.Loaded != "" {
			ui.Text(c, app.Loaded)
		}
		ui.Text(c, "Row").ContextMenu(func(m *ui.Menu) {
			if m.Item("Rename").Chosen() {
				app.Menu = "Rename"
			}
			if m.Item("Delete").Chosen() {
				app.Menu = "Delete"
			}
		})
		if app.Menu != "" {
			ui.Text(c, "Chose "+app.Menu)
		}
		ui.Text(c, "Repeat")
		ui.Text(c, "Repeat")
	})
}
