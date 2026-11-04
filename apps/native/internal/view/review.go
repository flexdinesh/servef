package view

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/flexdinesh/servef/apps/native/internal/review"
	"github.com/flexdinesh/servef/apps/native/internal/workspace"
	"github.com/flexdinesh/servef/pkg/contract"
)

// reviewState is the diff viewer's state for the model's open review.
type reviewState struct {
	state   *editor.DiffState
	of      *workspace.Review
	version int
	lines   int
	layout  int // 0 split, 1 unified
	built   int
	full    bool
	space   bool
}

// changes lists the working tree's changes since HEAD; choosing one opens
// its diff.
func (a *App) changes(c *ui.Context) {
	m := a.Model
	t := c.Theme()
	files := m.Changes.Files
	ui.Row(c).Padding(6, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		label := "Changes"
		if m.ChangesRead {
			label = review.ChangeCount(len(files))
		}
		ui.Text(c, label).Bold().SingleLine().Grow(1)
		if ui.Button(c, "Refresh").Clicked() {
			m.LoadChanges()
		}
	})
	if m.ChangesError != "" {
		ui.Text(c, m.ChangesError).TextColor(t.Danger).Wrap().Padding(4, 8)
	}
	for _, warning := range m.Changes.Warnings {
		ui.Text(c, warning).TextColor(t.Warning).Wrap().Padding(4, 8)
	}
	if m.Changes.Truncated {
		ui.Text(c, "Showing the first changes only.").TextColor(t.TextMuted).Wrap().Padding(4, 8)
	}
	a.syncChange()
	list := ui.List(c, &a.changeList, len(files), func(i int) {
		file := files[i]
		ui.Row(c).Padding(4, 8).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, review.Letter(file.Status)).Bold().TextColor(statusColor(t, file.Status)).Tooltip(review.StatusName(file.Status)).Width(14)
			fileIcon(c, file.Path)
			ui.Text(c, review.Label(file)).SingleLine().Grow(1)
		})
	}).Grow(1).Label("Changes").Children(func() {
		switch {
		case !m.ChangesRead:
		case !m.Changes.Repository:
			ui.Text(c, "Source Control needs a git repository.").TextColor(t.TextMuted).Wrap().Padding(12)
		case len(files) == 0:
			ui.Text(c, "No changes since last commit.").TextColor(t.TextMuted).Padding(12)
		}
	})
	if list.Changed() || list.Submitted() {
		if i := a.changeRow; i >= 0 && i < len(files) {
			m.OpenChange(files[i])
		}
	}
}

// syncChange selects the open review's row.
func (a *App) syncChange() {
	m := a.Model
	a.changeRow = -1
	if m.Review == nil {
		return
	}
	for i, file := range m.Changes.Files {
		if file.Path == m.Review.File.Path {
			a.changeRow = i
			return
		}
	}
}

func statusColor(t *ui.Theme, status contract.ChangeStatus) ui.Color {
	switch status {
	case contract.ChangeAdded, contract.ChangeUntracked:
		return t.Success
	case contract.ChangeDeleted:
		return t.Danger
	}
	return t.Warning
}

// diff shows the open review: the server's hunks in mygo's diff viewer.
func (a *App) diff(c *ui.Context, code *editor.Theme) {
	m := a.Model
	open := m.Review
	t := c.Theme()
	layout := open.Layout
	shown := open.Diff != nil && open.Error == "" && layout.Kind == review.Text
	ui.Row(c).Padding(6, 12).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, review.Label(open.File)).SingleLine().Grow(1)
		if shown {
			ui.Text(c, review.Counts(layout.Additions, layout.Deletions)).TextColor(t.TextMuted).SingleLine()
			ui.Segmented(c, &a.review.layout, "Split", "Unified").Label("Diff layout")
		}
		a.review.full, a.review.space = m.FullFile, m.IgnoreWhitespace
		if ui.Toggle(c, &a.review.full, "Full file").Tooltip("Show whole files, not only the changes").Changed() {
			m.SetFullFile(a.review.full)
		}
		if ui.Toggle(c, &a.review.space, "Ignore whitespace").Tooltip("Ignore leading and trailing whitespace").Changed() {
			m.SetIgnoreWhitespace(a.review.space)
		}
		if ui.Button(c, "Close").Clicked() {
			m.CloseReview()
		}
	})
	if m.Review == nil {
		return
	}
	switch {
	case open.Error != "":
		ui.Column(c).Fill().Center().Padding(24).Children(func() {
			ui.Text(c, open.Error).TextColor(t.TextMuted).Wrap()
		})
	case open.Diff == nil:
		ui.Column(c).Fill().Center().Children(func() { ui.Spinner(c) })
	case layout.Kind != review.Text:
		ui.Column(c).Fill().Center().Padding(24).Children(func() {
			ui.Text(c, layout.Message).TextColor(t.TextMuted).Wrap()
		})
	default:
		if layout.Hidden > 0 && !m.FullFile {
			ui.Text(c, review.Unmodified(layout.Hidden)+" hidden").TextColor(t.TextMuted).Padding(4, 12)
		}
		a.diffView(c, code, open)
	}
}

// diffView keeps the viewer on the review's layout: the server's hunks as
// mygo's patch hunks, so the viewer never compares the texts, highlighted
// from the full files in the server's language.
func (a *App) diffView(c *ui.Context, code *editor.Theme, open *workspace.Review) {
	r := &a.review
	if r.state == nil || r.of != open || r.version != open.Version || r.built != r.layout {
		state := &editor.DiffState{Layout: editor.DiffSplit}
		if r.layout == 1 {
			state.Layout = editor.DiffUnified
		}
		state.SetPatch(filePatch(open.Layout))
		state.Old.SetLanguage(syntaxLanguage(open.Layout.Language, nil))
		state.New.SetLanguage(syntaxLanguage(open.Layout.Language, nil))
		r.state, r.of, r.version, r.built, r.lines = state, open, open.Version, r.layout, -1
	}
	if open.Old != nil && open.LinesVersion == r.version && r.lines != r.version {
		oldRows, newRows := open.Layout.Sides()
		r.state.Old.SetLanguage(syntaxLanguage(open.Layout.Language, review.Rows(open.Old, oldRows)))
		r.state.New.SetLanguage(syntaxLanguage(open.Layout.Language, review.Rows(open.New, newRows)))
		r.lines = r.version
	}
	r.state.Theme = code
	editor.DiffView(c.Key(r.version*2+r.built), r.state).Grow(1).FillWidth()
}

// filePatch hands the layout's rows to mygo's viewer, which takes hunks as
// rows marked " ", "-", or "+". The file names stay empty so mygo detects
// no language; the server's applies.
func filePatch(layout review.Layout) editor.FilePatch {
	patch := editor.FilePatch{Hunks: make([]editor.PatchHunk, len(layout.Hunks))}
	for i, hunk := range layout.Hunks {
		rows := make([]string, len(hunk.Lines))
		for j, line := range hunk.Lines {
			rows[j] = map[review.LineKind]string{review.Context: " ", review.Deleted: "-", review.Added: "+"}[line.Kind] + line.Text
		}
		patch.Hunks[i] = editor.PatchHunk{OldStart: hunk.OldStart, OldLines: hunk.OldCount, NewStart: hunk.NewStart, NewLines: hunk.NewCount, Section: hunk.Section(), Lines: rows}
	}
	return patch
}
