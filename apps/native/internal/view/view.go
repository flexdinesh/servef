// Package view is native's thin mygo layer: it draws the workspace model
// with mygo's native UI (no web view) and turns input into model calls.
// Decisions live in the model and the pure packages it uses.
package view

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/flexdinesh/servef/apps/native/internal/docview"
	"github.com/flexdinesh/servef/apps/native/internal/tabs"
	"github.com/flexdinesh/servef/apps/native/internal/workspace"
	"github.com/flexdinesh/servef/pkg/contract"
)

// App is the window's view state around the model.
type App struct {
	Model *workspace.Model
	run   workspace.Runner
	// OpenFolder asks the user for a folder to open through the daemon;
	// nil hides the action (tests, no dialog).
	OpenFolder func(done func(path string))

	address     string
	projectRow  int
	projectList ui.ListState
	explorer    explorer
	sidebar     float32

	code          *editor.State
	codeVersion   int
	codeLines     int
	markdown      markdown.Options
	blocks        markdownBlocks
	confirmOpen   bool
	doc           documentState
	tabChoice     int
	tabShown      string
	tabStrip      float32
	tabStripShown float32
	changeList    ui.ListState
	changeRow     int
	review        reviewState
	lastTitle     string
	onTitleChange func(string)
}

// New returns the view of model; run runs background work.
func New(model *workspace.Model, run workspace.Runner) *App {
	app := &App{Model: model, run: run, projectRow: -1, codeVersion: -1, changeRow: -1}
	app.projectList.Selected = &app.projectRow
	app.changeList.Selected = &app.changeRow
	return app
}

// OnTitle reports the window title when it changes.
func (a *App) OnTitle(fn func(title string)) { a.onTitleChange = fn }

// View builds the window.
func (a *App) View(c *ui.Context) {
	t, palette := windowTheme(c.Theme())
	c.SetTheme(t)
	code := codeTheme(t, palette)
	m := a.Model
	if title := m.Title(); title != a.lastTitle {
		a.lastTitle = title
		if a.onTitleChange != nil {
			a.onTitleChange(title)
		}
	}
	a.release()
	a.confirm(c)
	a.navigationKeys(c)
	ui.Column(c).Fill().Children(func() {
		switch {
		case m.Connection == nil:
			a.connectPage(c)
		case m.Project == "":
			a.projectsPage(c)
		default:
			a.workspace(c, code, palette)
		}
	})
}

// confirm asks before opening a link to a server on another computer.
func (a *App) confirm(c *ui.Context) {
	m := a.Model
	if m.Confirm == nil {
		a.confirmOpen = false
		return
	}
	a.confirmOpen = true
	host := m.Confirm.Origin()
	switch ui.AlertDialog(c, &a.confirmOpen, "Open "+host[len("http://"):]+"?",
		"A link asked servef to open the server at "+host[len("http://"):]+". It is not on this computer. Open only servers you trust.", "Cancel", "Open") {
	case 1:
		m.AcceptLink()
	case 0, ui.AlertDismissed:
		m.RejectLink()
	}
}

func (a *App) errorLine(c *ui.Context) {
	if a.Model.Error != "" {
		ui.Text(c, a.Model.Error).TextColor(c.Theme().Danger).Wrap().Selectable()
	}
}

// connectPage chooses a connection: this computer's daemon, a CLI server
// running here, or a server address.
func (a *App) connectPage(c *ui.Context) {
	m := a.Model
	ui.Column(c).Fill().Padding(32).Gap(16).Children(func() {
		ui.Text(c, "servef").FontSize(28).Bold()
		ui.Text(c, "Experimental native app").TextColor(c.Theme().TextMuted)
		a.errorLine(c)
		if m.Connecting != "" {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "Connecting to "+m.Connecting+"…")
			})
			return
		}
		if ui.PrimaryButton(c, "Open this computer").Clicked() {
			m.ConnectLocal(nil)
		}
		if len(m.Servers) > 0 {
			ui.Text(c, "Servers on this computer").Bold()
			for _, server := range m.Servers {
				if ui.Button(c.Key(server.Origin), server.Origin).Clicked() {
					m.ConnectAddress(server.Origin)
				}
			}
		}
		ui.Text(c, "Open a server").Bold()
		ui.Row(c).Gap(8).Children(func() {
			submitted := ui.TextInput(c, &a.address).Placeholder("http://localhost:7971").Label("Server address").Width(320).Submitted()
			if ui.Button(c, "Connect").Clicked() || submitted {
				m.ConnectAddress(a.address)
			}
		})
	})
}

// projectsPage lists the connection's projects.
func (a *App) projectsPage(c *ui.Context) {
	m := a.Model
	projects := m.Projects.Projects
	ui.Column(c).Fill().Padding(24).Gap(12).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, m.Connection.Label).FontSize(20).Bold().Grow(1)
			if a.OpenFolder != nil && m.Connection.Daemon && ui.Button(c, "Open folder…").Clicked() {
				a.OpenFolder(func(path string) {
					if path != "" {
						m.OpenPath(path)
					}
				})
			}
			if ui.Button(c, "Refresh").Clicked() {
				m.RefreshProjects()
			}
			if ui.Button(c, "Disconnect").Clicked() {
				m.Disconnect()
			}
		})
		a.errorLine(c)
		for _, warning := range m.Projects.Warnings {
			ui.Text(c, warning).TextColor(c.Theme().Warning).Wrap()
		}
		list := ui.List(c, &a.projectList, len(projects), func(i int) {
			project := projects[i]
			ui.Column(c).Padding(6, 8).Gap(2).Children(func() {
				name := project.Name
				if project.Branch != "" {
					name += "  ·  " + project.Branch
				}
				ui.Text(c, name).Bold().SingleLine()
				ui.Text(c, project.Path).TextColor(c.Theme().TextMuted).SingleLine()
			})
		}).Grow(1).Label("Projects").Children(func() {
			if len(projects) == 0 {
				ui.Text(c, "No projects yet. Run servef in a folder, or open one.").TextColor(c.Theme().TextMuted).Padding(12)
			}
		})
		if list.Changed() || list.Submitted() {
			if i := a.projectRow; i >= 0 && i < len(projects) && projects[i].Available {
				a.projectRow = -1
				m.OpenProject(projects[i].ID)
			}
		}
	})
}

func (a *App) statusBar(c *ui.Context) {
	m := a.Model
	t := c.Theme()
	ui.Row(c).Padding(4, 10).Gap(12).AlignItems(ui.Center).Background(t.Surface).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, m.Connection.Label).TextColor(t.TextMuted).SingleLine()
		if m.Head != nil {
			ui.Text(c, headLabel(*m.Head)).TextColor(t.TextMuted).SingleLine()
		}
		// The document's path, as the web's status bar shows it.
		if path := a.statusPath(); path != "" {
			ui.Text(c, path).TextColor(t.TextMuted).SingleLine().Shrink(1).Tooltip(path)
		}
		ui.Spacer(c)
		if m.Active.Kind == tabs.Document && a.doc.shownLines != nil {
			ui.Text(c, docview.LineRangeLabel(*a.doc.shownLines)).TextColor(t.TextMuted).SingleLine()
		}
		if m.View.Language != "" && (m.View.Kind == docview.Code || m.View.Kind == docview.Formatted) && m.Active.Kind == tabs.Document {
			ui.Text(c, m.View.Language).TextColor(t.TextMuted)
		}
		if m.Live.Label != "" {
			ui.Text(c, m.Live.Label).TextColor(t.TextMuted).Tooltip(m.Live.Description)
		}
	})
}

// statusPath is the shown file's path: the review's file or the document.
func (a *App) statusPath() string {
	m := a.Model
	switch m.Active.Kind {
	case tabs.Review:
		if m.Review != nil {
			return m.Review.File.Path
		}
	case tabs.Document:
		return m.Page.Selected
	}
	return ""
}

func headLabel(head contract.HeadEvent) string {
	switch {
	case head.Branch != "":
		return head.Branch
	case len(head.Commit) >= 7:
		return "detached at " + head.Commit[:7]
	}
	return "no commits"
}
