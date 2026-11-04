// Package view is native's thin mygo layer: it draws the workspace model
// with mygo's native UI (no web view) and turns input into model calls.
// Decisions live in the model and the pure packages it uses.
package view

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/flexdinesh/servef/apps/native/internal/docview"
	"github.com/flexdinesh/servef/apps/native/internal/mdlink"
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
	imageVersion  int
	image         ui.ImageSource
	imageErr      string
	mode          int
	modeDocument  int
	panel         int
	changeList    ui.ListState
	changeRow     int
	review        reviewState
	lastTitle     string
	onTitleChange func(string)
}

// New returns the view of model; run runs background work.
func New(model *workspace.Model, run workspace.Runner) *App {
	app := &App{Model: model, run: run, sidebar: 280, projectRow: -1, codeVersion: -1, imageVersion: -1, modeDocument: -1, changeRow: -1}
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
	a.confirm(c)
	ui.Column(c).Fill().Children(func() {
		switch {
		case m.Connection == nil:
			a.connectPage(c)
		case m.Project == "":
			a.projectsPage(c)
		default:
			a.workspace(c, code, palette.Surface)
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

// workspace is the tree beside the document, over a status bar.
func (a *App) workspace(c *ui.Context, code *editor.Theme, surface string) {
	m := a.Model
	a.syncTree()
	ui.Split(c, &a.sidebar, func() {
		ui.Column(c).Fill().Background(c.Theme().Surface).Children(func() {
			ui.Row(c).Padding(6, 8).Gap(6).AlignItems(ui.Center).Children(func() {
				if ui.Button(c, "Projects").Clicked() {
					m.CloseProject()
				}
				name := m.Page.RootName
				if m.Page.Branch != "" {
					name += " · " + m.Page.Branch
				}
				ui.Text(c, name).Bold().SingleLine().Grow(1)
			})
			a.panel = int(m.Panel)
			ui.Row(c).Padding(0, 8, 6, 8).Children(func() {
				if ui.Segmented(c, &a.panel, "Files", "Changes").Label("Sidebar").Changed() {
					m.ShowPanel(workspace.Panel(a.panel))
				}
			})
			ui.Divider(c)
			if m.Panel == workspace.SourceControl {
				a.changes(c)
				return
			}
			a.files(c)
		})
	}, func() {
		ui.Column(c).Fill().Background(ui.Hex(surface)).Children(func() {
			if m.Review != nil {
				a.diff(c, code)
				return
			}
			a.document(c, code)
		})
	}).Grow(1)
	a.statusBar(c)
}

func (a *App) statusBar(c *ui.Context) {
	m := a.Model
	t := c.Theme()
	ui.Row(c).Padding(4, 10).Gap(12).AlignItems(ui.Center).Background(t.Surface).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, m.Connection.Label).TextColor(t.TextMuted).SingleLine()
		if m.Head != nil {
			ui.Text(c, headLabel(*m.Head)).TextColor(t.TextMuted).SingleLine()
		}
		ui.Spacer(c)
		if m.View.Language != "" && m.View.Kind == docview.Code {
			ui.Text(c, m.View.Language).TextColor(t.TextMuted)
		}
		if m.Live.Label != "" {
			ui.Text(c, m.Live.Label).TextColor(t.TextMuted).Tooltip(m.Live.Description)
		}
	})
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

// document shows the open document as the model decided.
func (a *App) document(c *ui.Context, code *editor.Theme) {
	m := a.Model
	view := m.View
	t := c.Theme()
	if m.Page.Selected != "" {
		ui.Row(c).Padding(6, 12).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, m.Page.Selected).SingleLine().Grow(1)
			a.modeSwitch(c, view)
		})
		if view.Message != "" && view.Kind != docview.Failed && view.Kind != docview.Binary && view.Kind != docview.TooLarge {
			ui.Text(c, view.Message).TextColor(t.TextMuted).Padding(4, 12)
		}
	}
	a.errorLine(c)
	switch view.Kind {
	case docview.None:
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, docview.Empty(m.Page.FileCount)).TextColor(t.TextMuted)
		})
	case docview.Failed, docview.Binary, docview.TooLarge:
		ui.Column(c).Fill().Center().Padding(24).Children(func() {
			ui.Text(c, view.Message).TextColor(t.TextMuted).Wrap()
		})
	case docview.Code:
		a.source(c, code)
	case docview.Markdown:
		a.preview(c, code)
	case docview.Image:
		a.imageView(c)
	}
}

// modeSwitch shows the document's views when it has more than one.
func (a *App) modeSwitch(c *ui.Context, view docview.View) {
	if len(view.Modes) < 2 {
		return
	}
	labels := make([]string, len(view.Modes))
	for i, mode := range view.Modes {
		labels[i] = map[docview.Mode]string{docview.Source: "Source", docview.Preview: "Preview"}[mode]
		if mode == view.Mode {
			a.mode = i
		}
	}
	if ui.Segmented(c, &a.mode, labels...).Label("View").Changed() && a.mode >= 0 && a.mode < len(view.Modes) {
		a.Model.SetMode(view.Modes[a.mode])
	}
}

// source shows the read-only source with line numbers; find and copy are
// the editor's.
func (a *App) source(c *ui.Context, code *editor.Theme) {
	m := a.Model
	if a.code == nil || a.codeVersion != m.DocumentVersion {
		buffer := editor.NewBuffer(m.Page.Source)
		buffer.SetLanguage(syntaxLanguage(m.View.Language, nil))
		a.code = &editor.State{Buffer: buffer, ReadOnly: true, LineNumbers: editor.LineNumbersOn, Label: m.Page.Selected}
		a.codeVersion, a.codeLines = m.DocumentVersion, -1
	}
	if m.Lines != nil && m.LinesVersion == a.codeVersion && a.codeLines != a.codeVersion {
		a.code.Buffer.SetLanguage(syntaxLanguage(m.View.Language, m.Lines))
		a.codeLines = a.codeVersion
	}
	a.code.Theme = code
	editor.View(c.Key(a.codeVersion), a.code).Grow(1).FillWidth()
}

// preview shows a Markdown preview: links open through the model, images
// load from the server's image endpoint, fences highlight with
// Highlights, and Mermaid draws as diagrams.
func (a *App) preview(c *ui.Context, code *editor.Theme) {
	m := a.Model
	t := c.Theme()
	_, palette := windowTheme(t)
	th := markdownTheme(t, palette, code)
	a.markdown.Theme = th
	a.markdown.Anchor, a.markdown.AnchorVersion = m.Anchor, m.AnchorVersion
	a.markdown.OnLink = func(url string) {
		if external := m.FollowLink(url); external != "" && ui.SafeToOpen(external) {
			c.OpenURL(external)
		}
	}
	read := m.ImageReader()
	a.markdown.LoadImage = func(url string) (ui.ImageSource, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		data, err := read(ctx, url)
		if err != nil {
			return nil, err
		}
		return ui.DecodeImage(data)
	}
	a.blocks.use(fmt.Sprint(m.Page.Project, "\x00", m.Page.Selected, "\x00", m.DocumentVersion))
	a.markdown.CodeBlock = func(c *ui.Context, b *markdown.CodeBlockView) { a.codeBlock(c, b, th) }
	a.markdown.Code = map[string]func(*ui.Context, *markdown.CodeBlockView){
		"mermaid": func(c *ui.Context, b *markdown.CodeBlockView) { a.mermaidBlock(c, b, th, palette) },
	}
	source := mdlink.Preview(m.Page.Source, m.Page.DocumentType)
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).Padding(16, 24).FillWidth().Children(func() {
			markdown.View(c.Key(m.Page.Selected), source, &a.markdown)
		})
	})
}

// imageView shows an image file, read from the server's image endpoint.
func (a *App) imageView(c *ui.Context) {
	m := a.Model
	if a.imageVersion != m.DocumentVersion {
		a.imageVersion, a.image, a.imageErr = m.DocumentVersion, nil, ""
		version, read := m.DocumentVersion, m.ImageReader()
		a.run.Go(func() func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			data, err := read(ctx, "")
			var source ui.ImageSource
			if err == nil {
				source, err = ui.DecodeImage(data)
			}
			return func() {
				if a.imageVersion != version {
					return
				}
				a.image = source
				if err != nil {
					a.imageErr = "Could not show " + path.Base(m.Page.Selected) + ": " + err.Error()
				}
			}
		})
	}
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).Padding(16).Center().Children(func() {
			switch {
			case a.imageErr != "":
				ui.Text(c, a.imageErr).TextColor(c.Theme().TextMuted).Wrap()
			case a.image != nil:
				ui.Image(c, a.image).Label(path.Base(m.Page.Selected))
			default:
				ui.Spinner(c)
			}
		})
	})
}
