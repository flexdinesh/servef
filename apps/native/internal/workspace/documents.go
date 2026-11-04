package workspace

import (
	"context"
	"errors"
	"fmt"

	"github.com/flexdinesh/servef/apps/native/internal/docview"
	"github.com/flexdinesh/servef/apps/native/internal/highlight"
	"github.com/flexdinesh/servef/apps/native/internal/tabs"
	"github.com/flexdinesh/servef/pkg/client"
	"github.com/flexdinesh/servef/pkg/contract"
)

// documents is the open documents' state beyond the page: each document's
// remembered place, Format, and the navigation history.
type documents struct {
	// memory is behind an interface so the owner of open documents can
	// hold it (decision 0008, use-document-navigation.ts).
	memory  docview.Memory
	formats docview.FormatCache
	history docview.History
	// preview is the Preview preference per project (decision 0010): in
	// memory, for the app's run.
	preview map[string]bool
	// code is what CodeVersion stands for; highlighting the CodeVersion a
	// highlight started for.
	code         codeIdentity
	highlighting int
}

// codeIdentity tells one shown code from another.
type codeIdentity struct {
	document int
	kind     docview.Kind
}

func (d *documents) close() {
	d.formats.Clear()
	d.history.Reset()
}

// seed records the first document a project shows, when nothing opened it
// through the history; every later page leaves the history alone.
func (d *documents) seed(page contract.PageData) {
	if page.HasFile && page.Selected != "" && d.history.Empty() {
		d.history.Push(docview.Location{Path: page.Selected})
	}
}

// DocumentKey is the open document's key in format and memory, "" for none.
func (m *Model) DocumentKey() string {
	if m.Page.Selected == "" {
		return ""
	}
	return docview.DocumentKey(m.Page.Selected, m.Page.At)
}

// PlaceKey is a document's key in memory: per connection and project.
func (m *Model) PlaceKey(path, at string) string {
	connection := ""
	if m.Connection != nil {
		connection = m.Connection.ID
	}
	return connection + "\x00" + m.Project + "\x00" + docview.DocumentKey(path, at)
}

// OpenPlaceKey is the open document's key in memory.
func (m *Model) OpenPlaceKey() string { return m.PlaceKey(m.Page.Selected, m.Page.At) }

// Place is what a document remembers.
func (m *Model) Place(key string) docview.Place { return m.documents.memory.Place(key) }

// RememberScroll keeps where a document was scrolled.
func (m *Model) RememberScroll(key string, y float64) {
	place := m.documents.memory.Place(key)
	place.Scroll = y
	m.documents.memory.SetPlace(key, place)
}

// visit records a newly opened location and selects its lines, if any.
func (m *Model) visit(path string, lines *docview.LineRange) {
	key := m.PlaceKey(path, "")
	if lines == nil {
		lines = m.documents.memory.Place(key).Lines
	} else {
		m.setPlaceLines(key, lines)
		m.RevealLine, m.RevealKey = lines.Start, key
		m.LineRevealVersion++
	}
	m.documents.history.Push(docview.Location{Path: path, Lines: lines})
}

func (m *Model) setPlaceLines(key string, lines *docview.LineRange) {
	place := m.documents.memory.Place(key)
	place.Lines = lines
	m.documents.memory.SetPlace(key, place)
}

// openLocation opens a project document in its tab, with lines to select (nil keeps what it remembers). A line link to the open
// document only moves the selection: the server is not asked again.
func (m *Model) openLocation(path string, lines *docview.LineRange) {
	if m.Project == "" {
		return
	}
	m.showTab(tabs.DocumentTab(path))
	m.visit(path, lines)
	m.show(path, lines != nil)
}

func (m *Model) show(path string, keepPage bool) {
	// A load in flight would replace the page kept.
	if keepPage && m.pageLoading == nil && path == m.Page.Selected && m.Page.HasFile && m.Page.At == "" {
		m.decide()
		return
	}
	m.loadPage(pageLoad{path: path, required: true, reveal: true})
}

// CanGoBack reports an earlier document in the history.
func (m *Model) CanGoBack() bool { return m.documents.history.CanGoBack() }

// CanGoForward reports a later document in the history.
func (m *Model) CanGoForward() bool { return m.documents.history.CanGoForward() }

// GoBack returns to the previous document, with the lines it had selected;
// from a review or the empty pane, to the document last shown.
func (m *Model) GoBack() {
	if m.Active.Kind != tabs.Document {
		if location, ok := m.documents.history.Current(); ok {
			m.restore(location)
		}
		return
	}
	if location, ok := m.documents.history.Back(); ok {
		m.restore(location)
	}
}

// GoForward undoes GoBack.
func (m *Model) GoForward() {
	if location, ok := m.documents.history.Forward(); ok {
		m.restore(location)
	}
}

func (m *Model) restore(location docview.Location) {
	m.setPlaceLines(m.PlaceKey(location.Path, ""), location.Lines)
	m.showTab(tabs.DocumentTab(location.Path))
	m.show(location.Path, true)
}

// Selection is the open document's selected lines, nil for none. Lines
// exist in Source only; the view clamps them to the document.
func (m *Model) Selection() *docview.LineRange {
	if m.Page.Selected == "" {
		return nil
	}
	return m.documents.memory.Place(m.OpenPlaceKey()).Lines
}

// SelectLines selects lines of the open document, nil for none, replacing
// its place in the history as the web replaces its URL.
func (m *Model) SelectLines(lines *docview.LineRange) {
	if m.Page.Selected == "" {
		return
	}
	m.setPlaceLines(m.OpenPlaceKey(), lines)
	m.documents.history.Replace(docview.Location{Path: m.Page.Selected, Lines: lines})
	// Lines ask for Source, so clearing them can change the view (App.tsx
	// derives the mode each render).
	m.decide()
}

// DocumentLink is a link opening the open document, at its selected lines.
func (m *Model) DocumentLink() string {
	if m.Connection == nil || m.Page.Selected == "" {
		return ""
	}
	target := m.Connection.Target
	target.Project, target.Path = m.Project, m.Page.Selected
	if lines := m.Selection(); lines != nil {
		target.Fragment = docview.LineAnchor(*lines)
	}
	return target.String()
}

// SetMode switches the open document's view, as App.tsx's
// changeDocumentMode: Format asks for its text; a view other than Source
// clears the selection, since other line numbers do not address the file;
// Preview and Source set the project's preference; the scroll starts over.
func (m *Model) SetMode(mode docview.Mode) {
	if !m.Page.HasFile || m.Page.Selected == "" || mode == m.View.Mode {
		return
	}
	key := m.OpenPlaceKey()
	place := m.documents.memory.Place(key)
	if mode != docview.Source && place.Lines != nil {
		place.Lines = nil
		m.documents.history.Replace(docview.Location{Path: m.Page.Selected})
	}
	place.Mode, place.Scroll = mode, 0
	m.documents.memory.SetPlace(key, place)
	if mode != docview.Format {
		if m.documents.preview == nil {
			m.documents.preview = map[string]bool{}
		}
		m.documents.preview[m.Project] = mode == docview.Preview
	}
	if mode == docview.Format {
		m.requestFormat()
	}
	m.ScrollResetVersion++
	m.decide()
}

// decide recomputes the document view, asks for Format when it is chosen
// and missing, and starts highlighting the code shown.
func (m *Model) decide() {
	place := docview.Place{}
	if m.Page.Selected != "" {
		place = m.documents.memory.Place(m.OpenPlaceKey())
	}
	requested := docview.Requested(place.Mode, place.Lines != nil, m.documents.preview[m.Project])
	mode := docview.Resolve(m.Page, requested)
	key := m.DocumentKey()
	format := m.documents.formats.Get(key, m.Page.Source)
	m.View = docview.Decide(m.Page, mode, format)
	if mode == docview.Format && format == nil {
		m.requestFormat()
	}
	m.highlightCode()
}

// CodeText is the code the view shows and highlights: the formatted text or
// the source, with its lines as the web splits them. Page.Source stays the
// original text.
func (m *Model) CodeText() string {
	if m.View.Kind == docview.Formatted {
		return docview.DisplayText(m.View.Text)
	}
	return docview.DisplayText(m.Page.Source)
}

// highlightCode gives the shown code its CodeVersion and highlights it off
// the UI thread. Live rereads keep the old lines until the new are ready.
func (m *Model) highlightCode() {
	kind := m.View.Kind
	if kind != docview.Code && kind != docview.Formatted {
		return
	}
	d := &m.documents
	if identity := (codeIdentity{m.DocumentVersion, kind}); identity != d.code {
		d.code = identity
		m.CodeVersion++
		m.Lines = nil
	}
	if d.highlighting == m.CodeVersion {
		return
	}
	d.highlighting = m.CodeVersion
	version, text, language := m.CodeVersion, m.CodeText(), m.View.Language
	if !m.View.Highlight {
		language = "text"
	}
	m.run.Go(func() func() {
		lines := highlight.Lines(text, language)
		return func() {
			if version == m.CodeVersion {
				m.Lines, m.LinesVersion = lines, version
			}
		}
	})
}

// requestFormat reads the open document's Format, as useDocumentFormats:
// one request per source; a failure shows Source with its notice and can be
// retried.
func (m *Model) requestFormat() {
	connection, project := m.Connection, m.Project
	if connection == nil || project == "" {
		return
	}
	key, placeKey, source := m.DocumentKey(), m.OpenPlaceKey(), m.Page.Source
	d := &m.documents
	if !d.formats.Wanted(key, source) {
		return
	}
	d.formats.Set(key, source, docview.FormatState{Kind: docview.FormatLoading})
	request := contract.FormatRequest{Project: project, Path: m.Page.Selected, At: m.Page.At}
	m.run.Go(func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		result, err := connection.Client.Format(ctx, request)
		return func() {
			if m.Connection != connection || m.Project != project || d.formats.Get(key, source) == nil {
				return
			}
			state := docview.FormatState{Kind: docview.FormatReady, Text: result.Formatted}
			if err != nil {
				state = docview.FormatState{Kind: docview.FormatFailed, Message: formatReason(err)}
				place := d.memory.Place(placeKey)
				place.Mode = docview.Source
				d.memory.SetPlace(placeKey, place)
			}
			d.formats.Set(key, source, state)
			if key == m.DocumentKey() {
				m.decide()
			}
		}
	})
}

// formatReason is a failed Format's reason, as document-format.ts words it.
func formatReason(err error) string {
	var clientErr *client.Error
	if errors.As(err, &clientErr) && clientErr.Message != "" {
		return clientErr.Message
	}
	var response *client.ResponseError
	if errors.As(err, &response) {
		if response.Status >= 200 && response.Status < 300 {
			return "the server returned an invalid response"
		}
		return fmt.Sprintf("the server responded with %d", response.Status)
	}
	return "the server could not be reached"
}
