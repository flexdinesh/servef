// Package fakeservef is a test double of a servef server speaking the HTTP
// shapes of pkg/contract, so tests drive native through the real
// pkg/client transports: TCP (a CLI server) and a Unix socket (the
// daemon). Tests and the headless agent host (internal/agenthost) only.
package fakeservef

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/flexdinesh/servef/pkg/contract"
)

// Server is a fake servef. Fields are guarded by mu; set them with Set.
type Server struct {
	mu       sync.Mutex
	Projects contract.ProjectsResponse
	// Pages answers /api/page by project and path ("" for no document).
	Pages map[string]map[string]contract.PageData
	// Images answers /api/image by path.
	Images map[string][]byte
	// Opened records /_daemon/open paths; OpenResult answers them.
	Opened     []string
	OpenResult contract.DaemonOpenResult
	// PageReads counts /api/page requests.
	PageReads int
	// Changes answers /api/changes by project; Diffs answers /api/diff by
	// path (a missing path is unchanged). ChangesReads counts the former;
	// DiffQueries records the latter's queries.
	Changes      map[string]contract.ChangesResponse
	Diffs        map[string]contract.DiffResponse
	ChangesReads int
	DiffQueries  []url.Values
	// Formats answers /api/format by path; FormatErrors refuses paths with
	// a message (422); others are not formattable (400). FormatQueries
	// records the queries.
	Formats       map[string]string
	FormatErrors  map[string]string
	FormatQueries []url.Values
	// FormatRaw answers paths with these bytes, as a proxy or a broken
	// server might.
	FormatRaw map[string]RawAnswer

	events chan contract.Event
	server *httptest.Server
	// URL is the TCP origin; Socket the Unix socket path (daemon mode).
	URL    string
	Socket string
}

// New starts a fake CLI server over TCP.
func New(t testing.TB) *Server {
	s := newServer()
	s.server = httptest.NewServer(s.handler(false))
	s.URL = s.server.URL
	t.Cleanup(s.Close)
	return s
}

func newServer() *Server {
	return &Server{
		Pages:   map[string]map[string]contract.PageData{},
		Images:  map[string][]byte{},
		Changes: map[string]contract.ChangesResponse{},
		Diffs:   map[string]contract.DiffResponse{},
		Formats: map[string]string{}, FormatErrors: map[string]string{}, FormatRaw: map[string]RawAnswer{},
		events: make(chan contract.Event, 64),
	}
}

// RawAnswer is a response that is not the contract's.
type RawAnswer struct {
	Status int
	Body   string
}

// NewDaemon starts a fake daemon on a Unix socket.
func NewDaemon(t testing.TB) *Server {
	dir, err := os.MkdirTemp("", "fsd")
	if err != nil {
		t.Fatal(err)
	}
	s, err := StartDaemon(filepath.Join(dir, "d.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.Close()
		_ = os.RemoveAll(dir)
	})
	return s
}

// StartServer starts a fake CLI server over TCP at addr (host:port);
// Close stops it.
func StartServer(addr string) (*Server, error) {
	s := newServer()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s.server = httptest.NewUnstartedServer(s.handler(false))
	s.server.Listener = listener
	s.server.Start()
	s.URL = s.server.URL
	return s, nil
}

// StartDaemon starts a fake daemon on a Unix socket at socket; Close
// stops it.
func StartDaemon(socket string) (*Server, error) {
	s := newServer()
	s.Socket = socket
	listener, err := net.Listen("unix", s.Socket)
	if err != nil {
		return nil, err
	}
	s.server = httptest.NewUnstartedServer(s.handler(true))
	s.server.Listener = listener
	s.server.Start()
	return s, nil
}

// Close stops the server and ends event streams.
func (s *Server) Close() {
	s.server.CloseClientConnections()
	s.server.Close()
}

// Set changes the server's data under its lock.
func (s *Server) Set(change func(s *Server)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	change(s)
}

// Get reads the server's data under its lock.
func (s *Server) Get(read func(s *Server)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	read(s)
}

// Page registers a page.
func (s *Server) Page(page contract.PageData) {
	s.Set(func(s *Server) {
		pages := s.Pages[page.Project]
		if pages == nil {
			pages = map[string]contract.PageData{}
			s.Pages[page.Project] = pages
		}
		pages[page.Selected] = page
	})
}

// Send queues an event for the open event stream.
func (s *Server) Send(event contract.Event) { s.events <- event }

func (s *Server) handler(daemon bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, _ *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		writeJSON(w, http.StatusOK, s.Projects)
	})
	mux.HandleFunc("GET /api/page", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.PageReads++
		query := r.URL.Query()
		pages, ok := s.Pages[query.Get("project")]
		if !ok {
			writeJSON(w, http.StatusNotFound, contract.PageData{ProjectUnavailable: true, Error: "Project unavailable"})
			return
		}
		page, ok := pages[query.Get("path")]
		if !ok {
			base := pages[""]
			base.Selected, base.Error = query.Get("path"), "File not found"
			writeJSON(w, http.StatusNotFound, base)
			return
		}
		writeJSON(w, http.StatusOK, page)
	})
	mux.HandleFunc("GET /api/changes", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.ChangesReads++
		changes, ok := s.Changes[r.URL.Query().Get("project")]
		if !ok {
			writeJSON(w, http.StatusNotFound, contract.ErrorResponse{Error: "Project unavailable", Code: contract.CodeProjectUnavailable})
			return
		}
		writeJSON(w, http.StatusOK, changes)
	})
	mux.HandleFunc("GET /api/diff", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		query := r.URL.Query()
		s.DiffQueries = append(s.DiffQueries, query)
		diff, ok := s.Diffs[query.Get("path")]
		if !ok {
			writeJSON(w, http.StatusNotFound, contract.ErrorResponse{Error: "No changes to " + query.Get("path"), Code: contract.CodeUnchanged})
			return
		}
		writeJSON(w, http.StatusOK, diff)
	})
	mux.HandleFunc("GET /api/format", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		query := r.URL.Query()
		s.FormatQueries = append(s.FormatQueries, query)
		path := query.Get("path")
		if raw, ok := s.FormatRaw[path]; ok {
			w.WriteHeader(raw.Status)
			_, _ = w.Write([]byte(raw.Body))
			return
		}
		if message, ok := s.FormatErrors[path]; ok {
			writeJSON(w, http.StatusUnprocessableEntity, contract.ErrorResponse{Error: message})
			return
		}
		formatted, ok := s.Formats[path]
		if !ok {
			writeJSON(w, http.StatusBadRequest, contract.ErrorResponse{Error: "This file type cannot be formatted"})
			return
		}
		writeJSON(w, http.StatusOK, contract.FormatResult{Formatted: formatted})
	})
	mux.HandleFunc("GET /api/image", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		data, ok := s.Images[r.URL.Query().Get("path")]
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", http.DetectContentType(data))
		_, _ = w.Write(data)
	})
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "no flush", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, ": servef\n\n")
		flusher.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case event := <-s.events:
				data, err := json.Marshal(event.Payload())
				if err != nil {
					return
				}
				_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Name, data)
				flusher.Flush()
			}
		}
	})
	if daemon {
		mux.HandleFunc("POST /_daemon/open", func(w http.ResponseWriter, r *http.Request) {
			var body contract.DaemonPathRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeJSON(w, http.StatusBadRequest, contract.ErrorResponse{Error: "bad body", Code: contract.CodeInvalidRequest})
				return
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			s.Opened = append(s.Opened, body.Path)
			writeJSON(w, http.StatusOK, s.OpenResult)
		})
	}
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// Sample is a small project: a repository with a README, docs, and code.
func Sample(project string) (contract.ProjectsResponse, []contract.PageData) {
	markdown := &contract.FileType{ID: "markdown", Label: "Markdown", Language: "markdown", PreviewKind: contract.PreviewMarkdown, PreviewLabel: "Preview", DefaultMode: contract.ModePreview}
	golang := &contract.FileType{ID: "go", Label: "Go", Language: "go", DefaultMode: contract.ModeSource}
	treeNodes := []contract.TreeNode{
		{Name: "docs", Path: "docs", IsDir: true, Open: true, Children: []contract.TreeNode{
			{Name: "guide.md", Path: "docs/guide.md", FileType: "markdown"},
		}},
		{Name: "src", Path: "src", IsDir: true, Children: []contract.TreeNode{
			{Name: "main.go", Path: "src/main.go", FileType: "go"},
		}},
		{Name: "README.md", Path: "README.md", FileType: "markdown"},
	}
	base := contract.PageData{Project: project, RootName: "demo", ProjectKind: contract.ProjectRepo, Branch: "main", Tree: treeNodes, FileCount: 3}
	readme := base
	readme.Selected, readme.HasFile, readme.DocumentType = "README.md", true, markdown
	readme.Source = "# Demo\n\nSee the [guide](docs/guide.md) and ![logo](docs/logo.png).\n"
	guide := base
	guide.Selected, guide.HasFile, guide.DocumentType = "docs/guide.md", true, markdown
	guide.Source = "# Guide\n\n## Install\n\nBack to [readme](../README.md#demo).\n\n```mermaid\ngraph TD; A-->B\n```\n"
	main := base
	main.Selected, main.HasFile, main.DocumentType = "src/main.go", true, golang
	main.Source = "package main\n\n// main greets.\nfunc main() {\n\tprintln(\"hi\", 42)\n}\n"
	projects := contract.ProjectsResponse{
		SessionKey: "daemon",
		Roots:      []contract.Root{{Path: project, Name: "demo", IsRepo: true}},
		Projects:   []contract.Project{{ID: project, Name: "demo", Path: project, Kind: contract.ProjectRepo, Group: project, Root: project, Branch: "main", Repository: "demo", Available: true}},
	}
	return projects, []contract.PageData{base, readme, guide, main}
}

// Load installs Sample(project).
func (s *Server) Load(project string) {
	projects, pages := Sample(project)
	s.Set(func(s *Server) { s.Projects = projects })
	for _, page := range pages {
		s.Page(page)
	}
}
