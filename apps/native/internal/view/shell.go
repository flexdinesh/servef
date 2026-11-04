package view

import (
	"runtime"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/flexdinesh/servef/apps/native/internal/docview"
	"github.com/flexdinesh/servef/apps/native/internal/shell"
	"github.com/flexdinesh/servef/apps/native/internal/tabs"
	"github.com/flexdinesh/servef/apps/native/internal/theme"
)

// mac picks the platform's shortcut labels.
var mac = runtime.GOOS == "darwin"

// Shell sizes in DIPs, from tokens.css: --activity-width, --chrome-height,
// --icon-lg, the 12.5rem tab maximum, and the 9px badge.
const (
	activityWidth = 40
	chromeHeight  = 32
	iconLarge     = 18
	tabMaxWidth   = 200
)

// activityIcons are the activity bar's Lucide icons, as Sidebar.tsx's.
var activityIcons = map[shell.View]*ui.SVG{
	shell.Explorer: lucideFiles, shell.Search: lucideSearch, shell.SourceControl: lucideGitBranch, shell.Walkthroughs: lucideRoute,
}

// shortcuts are the web's workspace keys (use-sidebar.ts): Cmd/Ctrl+B
// toggles the sidebar, Cmd/Ctrl+Shift+E shows the Explorer, and
// Cmd/Ctrl+Shift+G Source Control (the web takes Ctrl or Command for
// each, as here on macOS).
func (a *App) shortcuts(c *ui.Context) {
	m := a.Model
	for _, mods := range modifiers() {
		c.OnShortcut(mods, ui.KeyB, m.ToggleSidebar)
		c.OnShortcut(mods|ui.Shift, ui.KeyE, func() { m.RevealView(shell.Explorer) })
		c.OnShortcut(mods|ui.Shift, ui.KeyG, func() { m.RevealView(shell.SourceControl) })
	}
}

func modifiers() []ui.Modifiers {
	if ui.Cmd == ui.Ctrl {
		return []ui.Modifiers{ui.Ctrl}
	}
	return []ui.Modifiers{ui.Cmd, ui.Ctrl}
}

// activityBar shows the sidebar's views, the shown one on a raised tile,
// and Source Control's pending change count.
func (a *App) activityBar(c *ui.Context, palette theme.Palette) {
	m := a.Model
	t := c.Theme()
	ui.Column(c).Width(activityWidth).FillHeight().Shrink(0).Background(ui.Hex(palette.Chrome())).
		BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Label("Views").Children(func() {
		for _, activity := range shell.Activities(mac) {
			count := 0
			if activity.View == shell.SourceControl && m.ChangesRead {
				count = len(m.Changes.Files)
			}
			// The label stays the view's name; the count goes in the
			// description and the tooltip, as the web's label carries it.
			counted := shell.BadgeLabel(activity, count)
			tooltip := counted
			if activity.Shortcut != "" {
				tooltip += " (" + activity.Shortcut + ")"
			}
			shown := m.Sidebar.Shown() == activity.View
			item := ui.Box(c.Key(activity.View)).Size(activityWidth, activityWidth).Padding(4).Role(ui.RoleButton).Focusable().
				Label(activity.Label).Tooltip(tooltip).Checked(shown)
			if counted != activity.Label {
				item.Description(counted)
			}
			if item.Clicked() {
				m.SelectView(activity.View)
			}
			hovered := item.Hovered()
			item.Children(func() {
				tile := ui.Box(c).Fill().Radius(4).Center()
				switch {
				case shown:
					tile.Background(ui.Hex(palette.Surface)).Shadow(0, 1, 2, 0, shadowControl(t.Dark))
				case hovered:
					tile.Background(ui.Hex(palette.ChromeHover()))
				}
				ink := ui.Hex(palette.TextSecondary)
				if shown || hovered {
					ink = ui.Hex(palette.TextPrimary)
				}
				tile.Children(func() {
					ui.Icon(c, activityIcons[activity.View]).Size(iconLarge, iconLarge).TextColor(ink)
				})
				if badge := shell.Badge(count); badge != "" {
					ui.Text(c, badge).FontSize(9).Bold().SingleLine().TextColor(ui.Hex(palette.Surface)).
						Background(ui.Hex(palette.Accent)).Radius(7).Padding(0, 3).MinWidth(14).Height(14).TextAlign(ui.Center).
						Absolute().Left(activityWidth/2 + 1).Top(activityWidth/2 + 1).IgnoreInput(true)
				}
			})
		}
	})
}

// shadowControl is --shadow-control's color.
func shadowControl(dark bool) ui.Color {
	if dark {
		return ui.RGBA(0, 0, 0, 61)
	}
	return ui.RGBA(15, 18, 23, 20)
}

// sidebarView shows the sidebar's view; Search and Walkthroughs come in
// later milestones (M6, M9).
func (a *App) sidebarView(c *ui.Context) {
	m := a.Model
	ui.Column(c).Fill().Background(c.Theme().Surface).Children(func() {
		switch m.Sidebar.View {
		case shell.SourceControl:
			a.changes(c)
		case shell.Search, shell.Walkthroughs:
			a.placeholder(c, m.Sidebar.View)
		default:
			a.explorerHeader(c)
			ui.Divider(c)
			a.files(c)
		}
	})
}

// explorerHeader names the project and its branch, with the way back to
// the project list.
func (a *App) explorerHeader(c *ui.Context) {
	m := a.Model
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
}

func (a *App) placeholder(c *ui.Context, view shell.View) {
	t := c.Theme()
	ui.Row(c).Padding(6, 8).Children(func() { ui.Text(c, view.Name()).Bold().SingleLine() })
	ui.Divider(c)
	ui.Text(c, view.Name()+" is not in the native app yet.").TextColor(t.TextMuted).Wrap().Padding(12)
}

// editor is the tab strip and the active tab's toolbar over its content,
// or the empty pane.
func (a *App) editor(c *ui.Context, code *editor.Theme, palette theme.Palette) {
	m := a.Model
	ui.Column(c).Fill().Background(ui.Hex(palette.Surface)).Children(func() {
		// The band stays with no tabs, as the web's: content does not shift
		// when the first tab opens.
		ui.Row(c).FillWidth().Height(chromeHeight).Shrink(0).AlignItems(ui.Stretch).Background(ui.Hex(palette.Chrome())).Children(func() {
			a.tabBar(c, palette)
		})
		switch m.Active.Kind {
		case tabs.Review:
			if m.Review != nil {
				a.diff(c, palette)
				return
			}
			a.errorLine(c)
			ui.Column(c).Fill().Center().Padding(24).Children(func() {
				text := "No uncommitted changes."
				switch {
				case m.ChangesError != "":
					text = m.ChangesError
				case !m.ChangesRead:
					text = "Reading changes…"
				}
				ui.Text(c, text).TextColor(c.Theme().TextMuted).Wrap()
			})
		case tabs.Document:
			a.document(c, code, palette)
		default:
			a.errorLine(c)
			a.emptyPane(c)
		}
	})
}

// release settles the document's place and drops what the view holds for
// documents and reviews it no longer needs. The page's document keeps its
// editor under a review, so it comes back as it was; its decoded image, up to
// 32 MiB of file, is read again. Another document's editor and image go when a
// page replaces it. A preview's blocks stay until the next preview or their
// tab closes: a remembered scroll needs diagrams at the size they had. A
// closed tab's document stays until the next active tab's page takes the pane,
// and goes at once when none will. A closed review keeps only its layout choice.
func (a *App) release() {
	m, d := a.Model, &a.doc
	a.place()
	if m.Review == nil && a.review.of != nil {
		a.review = reviewState{layout: a.review.layout}
	}
	key := m.OpenPlaceKey()
	open := m.Page.Selected != "" && tabs.Includes(m.Tabs, tabs.DocumentTab(m.Page.Selected))
	gone := !open && m.Active.Kind != tabs.Document
	if a.code != nil && (gone || d.codeKey != key) {
		a.code, a.codeVersion, d.codeKey, d.codeKind = nil, -1, "", docview.None
	}
	if d.imageKey != "" && (m.Active.Kind != tabs.Document || d.imageKey != key) {
		d.image, d.imageKey, d.imageVersion = nil, "", -1
	}
	if a.blocks.document != "" && (gone || !tabs.Includes(m.Tabs, tabs.DocumentTab(d.blocksPath))) {
		a.blocks, d.blocksPath = markdownBlocks{}, ""
	}
	if gone {
		// Reopening starts over, as its memory did when the tab closed.
		d.shown, d.shownPath, d.scrolled, d.diagram = "", "", docview.None, nil
	}
}

// tabBar shows the open tabs, square on the chrome, the active one on the
// document surface (decision 0056): a click selects, the arrows move the
// choice, Delete and Backspace close the focused tab, and each tab's menu
// closes it, the others, those to its right, or all. It scrolls sideways
// when the tabs overflow, keeping the active one in view.
func (a *App) tabBar(c *ui.Context, palette theme.Palette) {
	m := a.Model
	t := c.Theme()
	open := m.Tabs
	a.tabChoice = max(tabs.Index(open, m.Active), 0)
	// Reveal the active tab when it changes and when the strip resizes, as
	// TabBar.tsx does.
	reveal := m.Active.Key() != a.tabShown || a.tabStrip != a.tabStripShown
	a.tabShown, a.tabStripShown = m.Active.Key(), a.tabStrip
	var selected, closing = -1, -1
	var menu tabs.Action
	menuOn := -1
	strip := ui.ScrollHorizontal(c).Grow(1).FillHeight().Scrollbars(false)
	a.tabStrip = strip.Bounds().W
	strip.Children(func() {
		bar := ui.TabsBase(c, &a.tabChoice, len(open))
		bar.List.FillHeight().AlignItems(ui.Stretch).Label("Open documents").Children(func() {
			for i, tab := range open {
				name := tab.Name()
				active := tab == m.Active
				// The tab and its close action are siblings, as the web's
				// buffer-tab: assistive technology sees a tab's content as its
				// name, so an action inside one would not show.
				holder := ui.Row(c).MaxWidth(tabMaxWidth).Shrink(0).AlignItems(ui.Center).Padding(0, 4, 0, 0)
				hovered := holder.Hovered()
				ink := palette.TextSecondary
				switch {
				case active:
					holder.Background(ui.Hex(palette.Surface))
					ink = palette.TextPrimary
				case hovered:
					holder.Background(ui.Hex(palette.ChromeHover()))
					ink = palette.TextPrimary
				}
				holder.ContextMenu(func(mm *ui.Menu) {
					for _, item := range tabs.MenuItems(tab, i, len(open), false) {
						if item.Separator {
							mm.Separator()
							continue
						}
						if mm.Item(item.Label).Disabled(!item.Enabled).Chosen() {
							menu, menuOn = item.Action, i
						}
					}
				})
				if active && reveal {
					holder.ScrollIntoView()
				}
				focus := holder.FocusWithin()
				holder.Children(func() {
					el := bar.Tab(i).FillHeight().Shrink(1).MinWidth(0).AlignItems(ui.Center).Gap(4).Padding(0, 4, 0, 8).
						Tooltip(name.Title).Label(name.Title).TextColor(ui.Hex(ink))
					if el.Clicked() {
						selected = i
					}
					if el.Shortcut(0, ui.KeyDelete) || el.Shortcut(0, ui.KeyBackspace) {
						closing = i
					}
					el.Children(func() {
						a.tabIcon(c, tab)
						ui.Text(c, name.Label).FontSize(13).SingleLine().Ellipsis("…").Shrink(1)
					})
					// The close action keeps its room; it shows on the active
					// tab, on hover, and with the focus inside (0056).
					x := ui.Box(c).Size(16, 16).Radius(3).Center().Role(ui.RoleButton).Label("Close " + name.Title).Shrink(0)
					if x.Clicked() {
						closing = i
					}
					if x.Hovered() {
						x.Background(ui.Hex(palette.ChromeHover()))
					}
					if !active && !hovered && !focus {
						x.Opacity(0)
					}
					x.Children(func() { ui.Icon(c, lucideX).Size(12, 12).TextColor(t.TextMuted) })
				})
			}
		})
		if bar.List.Changed() && a.tabChoice < len(open) && selected < 0 {
			selected = a.tabChoice
		}
	})
	switch {
	case closing >= 0:
		m.CloseTabs([]tabs.Tab{open[closing]})
	case menuOn >= 0:
		m.TabAction(menu, menuOn)
	case selected >= 0 && selected < len(open):
		m.SelectTab(open[selected])
	}
}

// tabIcon is a tab's icon: the file's Pierre icon, or the review's.
func (a *App) tabIcon(c *ui.Context, tab tabs.Tab) {
	if tab.Kind == tabs.Document {
		fileIcon(c, tab.Path)
		return
	}
	ui.Icon(c, lucideGitCompareArrows).Size(14, 14).TextColor(c.Theme().TextMuted).Shrink(0)
}

// emptyPane names what the shown sidebar view offers with no tab open
// (decision 0052), with its action where native has it.
func (a *App) emptyPane(c *ui.Context) {
	m := a.Model
	t := c.Theme()
	input := shell.EmptyInput{View: m.Sidebar.Shown(), Changes: -1, Repository: true, Walkthroughs: -1}
	if m.ChangesRead {
		input.Changes, input.Repository = len(m.Changes.Files), m.Changes.Repository
	}
	empty := shell.EmptyWorkspace(input)
	ui.Column(c).Fill().Center().Gap(8).Padding(24).Children(func() {
		ui.Text(c, empty.Heading).FontSize(20).Bold()
		ui.Text(c, empty.Hint).TextColor(t.TextMuted).Wrap().TextAlign(ui.Center)
		// Find a file waits for quick open (M3).
		if empty.Action == "" || empty.Action == shell.FindFile {
			return
		}
		label, shortcut := shell.ActionLabel(empty.Action, mac)
		if shortcut != "" {
			label += "  " + shortcut
		}
		if ui.Button(c, label).Margin(8, 0, 0, 0).Clicked() {
			switch empty.Action {
			case shell.ReviewUncommitted:
				m.OpenReview()
			case shell.ShowSidebar:
				m.RevealView(m.Sidebar.View)
			}
		}
	})
}

// workspace is the activity bar and the sidebar beside the editor, over
// the status bar.
func (a *App) workspace(c *ui.Context, code *editor.Theme, palette theme.Palette) {
	m := a.Model
	a.syncTree()
	a.shortcuts(c)
	ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
		a.activityBar(c, palette)
		if m.Sidebar.Hidden {
			a.editorAlone(c, code, palette)
			return
		}
		a.sidebar = m.Sidebar.Width
		if ui.Split(c, &a.sidebar, func() { a.sidebarView(c) }, func() { a.editor(c, code, palette) }).Grow(1).Changed() {
			m.SetSidebarWidth(a.sidebar)
		}
	})
	a.statusBar(c)
}

// editorAlone shows the editor with the sidebar hidden, in the element
// shape ui.Split gives it (a row in a row, after a pane and a divider,
// before the handle): elements keep their state by position, so hiding the
// sidebar leaves the document where it is scrolled. TestSidebarToggle checks
// it against mygo's Split.
func (a *App) editorAlone(c *ui.Context, code *editor.Theme, palette theme.Palette) {
	ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
			ui.Box(c).Width(0)
			ui.Box(c).Width(0)
			ui.Box(c).Grow(1).Shrink(1).Clip().MinWidth(0).Children(func() { a.editor(c, code, palette) })
			ui.Box(c).Absolute()
		})
	})
}
