// Package memory is native's workspace memory, as the web's
// workspace-memory.ts keeps it in localStorage (decisions 0019, 0061):
// per project, its tabs, active tab, and open folders; per session key
// (contract.ProjectsResponse.SessionKey), the project last open. Browsers
// keep it per origin; native keeps it per connection. The sidebar's view,
// visibility, and width are kept once, as the web's use-sidebar.ts and
// use-sidebar-resize.ts keep them. Functions return new values and never
// change their arguments. Store reads and writes the file.
package memory

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/flexdinesh/servef/apps/native/internal/tabs"
)

// Caps on remembered entries, as the web's.
const (
	maxProjects    = 100
	maxSessions    = 20
	maxConnections = 20
)

// Project is a project's remembered workspace.
type Project struct {
	Tabs []tabs.Tab
	// Active is the active tab, none for no tab.
	Active tabs.Tab
	// Expanded are the open folders; HasExpanded false uses the server's
	// open folders, while an empty Expanded remembers all closed.
	Expanded    []string
	HasExpanded bool
}

// entry is a keyed value; entries are ordered least recent first.
type entry[T any] struct {
	key   string
	value T
}

func find[T any](entries []entry[T], key string) (T, bool) {
	for _, e := range entries {
		if e.key == key {
			return e.value, true
		}
	}
	var zero T
	return zero, false
}

// with moves key to the end with value, dropping the least recent entries
// beyond max.
func with[T any](entries []entry[T], key string, value T, max int) []entry[T] {
	out := make([]entry[T], 0, len(entries)+1)
	for _, e := range entries {
		if e.key != key {
			out = append(out, e)
		}
	}
	out = append(out, entry[T]{key, value})
	if len(out) > max {
		out = out[len(out)-max:]
	}
	return out
}

func without[T any](entries []entry[T], drop func(key string, value T) bool) []entry[T] {
	out := make([]entry[T], 0, len(entries))
	for _, e := range entries {
		if !drop(e.key, e.value) {
			out = append(out, e)
		}
	}
	return out
}

// Workspace is one connection's memory.
type Workspace struct {
	last     []entry[string]
	projects []entry[Project]
}

// LastProject is the project last open in a session, "" for none.
func (w Workspace) LastProject(session string) string {
	project, _ := find(w.last, session)
	return project
}

// Project is a project's memory.
func (w Workspace) Project(id string) (Project, bool) { return find(w.projects, id) }

// RememberProject remembers a project's tabs and active tab, keeping its
// folders, and makes it the session's last project. An active tab that is
// not open is remembered as none.
func (w Workspace) RememberProject(session, project string, open []tabs.Tab, active tabs.Tab) Workspace {
	memory, _ := w.Project(project)
	memory.Tabs = slices.Clone(open)
	memory.Active = tabs.Tab{}
	if tabs.Includes(open, active) {
		memory.Active = active
	}
	return Workspace{last: with(w.last, session, project, maxSessions), projects: with(w.projects, project, memory, maxProjects)}
}

// RememberExpanded remembers a project's open folders, keeping its tabs.
func (w Workspace) RememberExpanded(project string, expanded []string) Workspace {
	memory, _ := w.Project(project)
	memory.Expanded, memory.HasExpanded = unique(expanded), true
	return Workspace{last: w.last, projects: with(w.projects, project, memory, maxProjects)}
}

// Forget drops a project's memory; a forgotten last project no longer
// opens on landing.
func (w Workspace) Forget(project string) Workspace {
	return Workspace{
		last:     without(w.last, func(_ string, id string) bool { return id == project }),
		projects: without(w.projects, func(id string, _ Project) bool { return id == project }),
	}
}

// Remembered lists remembered projects, most recent first.
func (w Workspace) Remembered() []string {
	ids := make([]string, len(w.projects))
	for i, e := range w.projects {
		ids[len(ids)-1-i] = e.key
	}
	return ids
}

func unique(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// Restored are a project's tabs restored against its fresh tree.
type Restored struct {
	Tabs   []tabs.Tab
	Active tabs.Tab
}

// RestoreTabs keeps remembered documents that still exist in the fresh
// tree (file reports them); the review restores regardless.
func RestoreTabs(memory Project, file func(path string) bool) Restored {
	var restored Restored
	for _, tab := range memory.Tabs {
		if tab.None() || tab.Kind == tabs.Document && !file(tab.Path) || tabs.Includes(restored.Tabs, tab) {
			continue
		}
		restored.Tabs = append(restored.Tabs, tab)
	}
	if tabs.Includes(restored.Tabs, memory.Active) {
		restored.Active = memory.Active
	}
	return restored
}

// StartupTab is the tab a project opens on when its link names none: the
// remembered active tab, else Welcome when nothing was restored and
// Welcome shows on startup (decision 0052), else none.
func StartupTab(routed bool, restored Restored, welcome bool) tabs.Tab {
	switch {
	case routed:
		return tabs.Tab{}
	case !restored.Active.None():
		return restored.Active
	case welcome && len(restored.Tabs) == 0:
		return tabs.WelcomeTab()
	}
	return tabs.Tab{}
}

// RestoreExpanded keeps remembered folders still in the fresh tree (dir
// reports them); false keeps the server's open folders.
func RestoreExpanded(memory Project, dir func(path string) bool) ([]string, bool) {
	if !memory.HasExpanded {
		return nil, false
	}
	open := []string{}
	for _, p := range unique(memory.Expanded) {
		if dir(p) {
			open = append(open, p)
		}
	}
	return open, true
}

// Sidebar is the sidebar's remembered state; the shell package reads it.
type Sidebar struct {
	View   string
	Hidden bool
	// Width is in DIPs, 0 for the default.
	Width float32
}

// File is everything remembered.
type File struct {
	Sidebar     Sidebar
	connections []entry[Workspace]
}

// Workspace is a connection's memory.
func (f File) Workspace(connection string) Workspace {
	w, _ := find(f.connections, connection)
	return w
}

// WithWorkspace replaces a connection's memory.
func (f File) WithWorkspace(connection string, w Workspace) File {
	return File{Sidebar: f.Sidebar, connections: with(f.connections, connection, w, maxConnections)}
}

// The file's JSON: entries as arrays, least recent first, tabs as the
// web stores them.
type (
	fileJSON struct {
		Version     int              `json:"version"`
		Sidebar     sidebarJSON      `json:"sidebar"`
		Connections []connectionJSON `json:"connections"`
	}
	sidebarJSON struct {
		View   string  `json:"view"`
		Hidden bool    `json:"hidden"`
		Width  float32 `json:"width,omitempty"`
	}
	connectionJSON struct {
		ID       string        `json:"id"`
		Last     []lastJSON    `json:"last"`
		Projects []projectJSON `json:"projects"`
	}
	lastJSON struct {
		Session string `json:"session"`
		Project string `json:"project"`
	}
	projectJSON struct {
		ID       string            `json:"id"`
		Tabs     []json.RawMessage `json:"tabs"`
		Active   json.RawMessage   `json:"active"`
		Expanded *[]string         `json:"expanded,omitempty"`
	}
)

// Marshal encodes the file.
func (f File) Marshal() []byte {
	out := fileJSON{Version: 1, Sidebar: sidebarJSON(f.Sidebar), Connections: []connectionJSON{}}
	for _, c := range f.connections {
		connection := connectionJSON{ID: c.key, Last: []lastJSON{}, Projects: []projectJSON{}}
		for _, l := range c.value.last {
			connection.Last = append(connection.Last, lastJSON{Session: l.key, Project: l.value})
		}
		for _, p := range c.value.projects {
			project := projectJSON{ID: p.key, Tabs: []json.RawMessage{}, Active: raw(p.value.Active)}
			for _, tab := range p.value.Tabs {
				project.Tabs = append(project.Tabs, raw(tab))
			}
			if p.value.HasExpanded {
				expanded := slices.Clone(p.value.Expanded)
				if expanded == nil {
					expanded = []string{}
				}
				project.Expanded = &expanded
			}
			connection.Projects = append(connection.Projects, project)
		}
		out.Connections = append(out.Connections, connection)
	}
	data, _ := json.Marshal(out)
	return data
}

func raw(tab tabs.Tab) json.RawMessage {
	data, _ := json.Marshal(tab)
	return data
}

// foreign reports a JSON object that is not this version's file: it may
// hold what another build wrote, so it is not this build's to replace.
func foreign(data []byte) bool {
	var probe map[string]json.RawMessage
	if json.Unmarshal(data, &probe) != nil || probe == nil {
		return false
	}
	return string(probe["version"]) != "1"
}

// Parse keeps every well-formed entry of untrusted stored data, dropping
// each mistyped field or entry alone; anything else reads as nothing
// remembered.
func Parse(data []byte) File {
	var in fileJSON
	// A type mismatch leaves that value empty and decodes the rest.
	var mistyped *json.UnmarshalTypeError
	if err := json.Unmarshal(data, &in); err != nil && !errors.As(err, &mistyped) || in.Version != 1 {
		return File{}
	}
	f := File{Sidebar: Sidebar(in.Sidebar)}
	for _, c := range in.Connections {
		if c.ID == "" {
			continue
		}
		var w Workspace
		for _, l := range c.Last {
			if l.Project != "" {
				w.last = with(w.last, l.Session, l.Project, maxSessions)
			}
		}
		for _, p := range c.Projects {
			if p.ID == "" {
				continue
			}
			project := Project{}
			for _, stored := range p.Tabs {
				if tab, ok := tabs.Parse(stored); ok {
					project.Tabs = append(project.Tabs, tab)
				}
			}
			if active, ok := tabs.Parse(p.Active); ok {
				project.Active = active
			}
			if p.Expanded != nil && *p.Expanded != nil {
				expanded := slices.DeleteFunc(slices.Clone(*p.Expanded), func(path string) bool { return path == "" })
				project.Expanded, project.HasExpanded = unique(expanded), true
			}
			w.projects = with(w.projects, p.ID, project, maxProjects)
		}
		f.connections = with(f.connections, c.ID, w, maxConnections)
	}
	return f
}
