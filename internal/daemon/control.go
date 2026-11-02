package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/internal/walkthrough"
	"github.com/flexdinesh/servef/internal/web"
)

// controlPrefix marks the control API; its requests never reach the web app.
const controlPrefix = "/_daemon/"

// maxDirectoryEntries bounds a GET /_daemon/directories listing.
const maxDirectoryEntries = 2000

// maxControlRequest bounds a POST /_daemon/* body.
const maxControlRequest = 4 << 10

// controlHeartbeat writes a comment to an idle control stream so a closed
// client's stream ends on the next write.
const controlHeartbeat = 25 * time.Second

// Status describes a running daemon. Its shape is frozen across protocols.
type Status struct {
	Protocol  int       `json:"protocol"`
	Version   string    `json:"version"`
	PID       int       `json:"pid"`
	Socket    string    `json:"socket"`
	Roots     []string  `json:"roots"`
	Depth     int       `json:"depth"`
	ScanDepth int       `json:"scanDepth"`
	IdleExit  string    `json:"idleExit"` // time.Duration.String(); "0s" never exits
	StartedAt time.Time `json:"startedAt"`
}

type rootsResponse struct {
	Roots   []string `json:"roots"`
	Skipped []string `json:"skipped"`
}

type addRootRequest struct {
	Path string `json:"path"`
}

type addRootResponse struct {
	Root  string `json:"root"`
	Added bool   `json:"added"`
}

// OpenResult is POST /_daemon/open's answer: the root serving the path,
// whether it was added, the roots it replaced (inside it), and the project
// that shows the path.
type OpenResult struct {
	Root     string   `json:"root"`
	Added    bool     `json:"added"`
	Replaced []string `json:"replaced"`
	Project  string   `json:"project"`
}

// directoriesResponse is GET /_daemon/directories' answer.
type directoriesResponse struct {
	Path      string                 `json:"path"`
	Parent    string                 `json:"parent"`
	Entries   []projects.BrowseEntry `json:"entries"`
	Truncated bool                   `json:"truncated"`
}

type announceRequest struct {
	Project string `json:"project"`
	ID      string `json:"id"`
	Created bool   `json:"created"`
}

type announceResponse struct {
	Announced bool `json:"announced"`
}

// controlStatusEvent is the control stream's first event.
type controlStatusEvent struct {
	Protocol int    `json:"protocol"`
	Version  string `json:"version"`
}

// walkthroughEvent is the control stream's "walkthrough" event.
type walkthroughEvent struct {
	Project string `json:"project"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Created bool   `json:"created"`
}

type stopResponse struct {
	Stopping bool `json:"stopping"`
}

// walkthroughNotices announces published walkthroughs; runtime.Runtime
// implements it.
type walkthroughNotices interface {
	AnnounceWalkthrough(project, id string, created bool) (web.WalkthroughNotice, error)
	WalkthroughNotices() (<-chan web.WalkthroughNotice, func())
}

// errorResponse matches the web app's JSON errors: {error, code?}.
type errorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// control serves /_daemon/*. Adds and saves serialize under mu; a save
// writes the served roots plus the roots skipped at load, so an unmounted
// drive is not forgotten. Control streams end when done closes.
type control struct {
	mu        sync.Mutex
	roots     *projects.RootSet
	skipped   []string
	rootsFile string
	status    Status
	stop      func()
	done      <-chan struct{}
	notices   walkthroughNotices
}

func pid() int { return os.Getpid() }

// wrap serves the control API before next. Browser-originated requests are
// refused: the socket is the trust boundary, and a future proxy must not
// forward browser requests to it.
func (c *control) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, controlPrefix) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "The daemon control API refuses browser requests.", Code: "browserRequest"})
			return
		}
		switch name := strings.TrimPrefix(r.URL.Path, controlPrefix); {
		case name == "status" && r.Method == http.MethodGet:
			writeJSON(w, http.StatusOK, c.currentStatus())
		case name == "roots" && r.Method == http.MethodGet:
			c.mu.Lock()
			response := rootsResponse{Roots: nonNil(c.roots.List()), Skipped: nonNil(slices.Clone(c.skipped))}
			c.mu.Unlock()
			writeJSON(w, http.StatusOK, response)
		case name == "roots" && r.Method == http.MethodPost:
			c.addRoot(w, r)
		case name == "open" && r.Method == http.MethodPost:
			c.open(w, r)
		case name == "directories" && r.Method == http.MethodGet:
			serveDirectories(w, r.URL.Query().Get("path"))
		case name == "events" && r.Method == http.MethodGet:
			c.serveEvents(w, r)
		case name == "walkthroughs" && r.Method == http.MethodPost:
			c.announce(w, r)
		case name == "stop" && r.Method == http.MethodPost:
			writeJSON(w, http.StatusAccepted, stopResponse{Stopping: true})
			c.stop()
		case name == "status", name == "events", name == "directories":
			methodNotAllowed(w, http.MethodGet)
		case name == "roots":
			methodNotAllowed(w, "GET, POST")
		case name == "stop", name == "open", name == "walkthroughs":
			methodNotAllowed(w, http.MethodPost)
		default:
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Unknown daemon endpoint."})
		}
	})
}

func (c *control) currentStatus() Status {
	status := c.status
	status.Roots = nonNil(c.roots.List())
	return status
}

// decodeRequest reads a strict JSON body of at most maxControlRequest into
// request. Otherwise it answers 415 or 400 (usage names the expected body)
// and returns false.
func decodeRequest(w http.ResponseWriter, r *http.Request, request any, usage string) bool {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, errorResponse{Error: "Send a JSON body.", Code: "unsupportedMediaType"})
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxControlRequest))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(request); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Send " + usage + ".", Code: "invalidRequest"})
		return false
	}
	return true
}

func (c *control) addRoot(w http.ResponseWriter, r *http.Request) {
	var request addRootRequest
	if !decodeRequest(w, r, &request, `{"path": "/absolute/path"}`) {
		return
	}
	if !filepath.IsAbs(request.Path) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Root path must be absolute.", Code: "invalidRoot"})
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	root, added, err := c.roots.Add(request.Path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "invalidRoot"})
		return
	}
	if !added {
		writeJSON(w, http.StatusOK, addRootResponse{Root: root})
		return
	}
	c.skipped = slices.DeleteFunc(c.skipped, func(skipped string) bool { return skipped == root })
	if err := SaveRoots(c.rootsFile, append(c.roots.List(), c.skipped...)); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "The root is served until the daemon stops but was not saved: " + err.Error(), Code: "rootsNotSaved"})
		return
	}
	writeJSON(w, http.StatusCreated, addRootResponse{Root: root, Added: true})
}

// open serves a path: within a served root nothing changes; otherwise the
// path becomes a root, replacing the roots inside it, and is saved. It
// answers the project that shows the path.
func (c *control) open(w http.ResponseWriter, r *http.Request) {
	var request addRootRequest
	if !decodeRequest(w, r, &request, `{"path": "/absolute/path"}`) {
		return
	}
	if !filepath.IsAbs(request.Path) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Path must be absolute.", Code: "invalidRoot"})
		return
	}
	c.mu.Lock()
	root, added, replaced, err := c.roots.Cover(request.Path)
	if err != nil {
		c.mu.Unlock()
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "invalidRoot"})
		return
	}
	if added {
		c.skipped = slices.DeleteFunc(c.skipped, func(skipped string) bool { return files.Within(root, skipped) })
		if err := SaveRoots(c.rootsFile, append(c.roots.List(), c.skipped...)); err != nil {
			c.mu.Unlock()
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "The root is served until the daemon stops but was not saved: " + err.Error(), Code: "rootsNotSaved"})
			return
		}
	}
	roots := c.roots.List()
	c.mu.Unlock()
	project, err := projects.ProjectFor(roots, filepath.Clean(request.Path))
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "No project shows " + request.Path + ": " + err.Error(), Code: "projectUnavailable"})
		return
	}
	status := http.StatusOK
	if added {
		status = http.StatusCreated
	}
	writeJSON(w, status, OpenResult{Root: root, Added: added, Replaced: nonNil(replaced), Project: project})
}

// serveDirectories lists a directory's child directories so a client can
// choose a folder to open (decision 0076); an empty path lists the home
// directory.
func serveDirectories(w http.ResponseWriter, path string) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "The home directory is unknown: " + err.Error(), Code: "pathUnavailable"})
			return
		}
		path = home
	}
	if !filepath.IsAbs(path) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Path must be absolute.", Code: "invalidPath"})
		return
	}
	path = filepath.Clean(path)
	entries, truncated, err := projects.Browse(path, maxDirectoryEntries)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: path + " does not exist.", Code: "pathUnavailable"})
	case errors.Is(err, projects.ErrNotDirectory):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: path + " is not a directory.", Code: "notDirectory"})
	case errors.Is(err, fs.ErrPermission):
		writeJSON(w, http.StatusForbidden, errorResponse{Error: path + " cannot be read.", Code: "pathUnreadable"})
	case err != nil:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Read " + path + ": " + err.Error(), Code: "pathUnreadable"})
	default:
		parent := filepath.Dir(path)
		if parent == path {
			parent = ""
		}
		writeJSON(w, http.StatusOK, directoriesResponse{Path: path, Parent: parent, Entries: entries, Truncated: truncated})
	}
}

// announce tells control streams that a walkthrough was published to the
// daemon's store, as the CLI's publish does after writing it.
func (c *control) announce(w http.ResponseWriter, r *http.Request) {
	var request announceRequest
	if !decodeRequest(w, r, &request, `{"project": "/absolute/path", "id": "id", "created": true}`) {
		return
	}
	if request.Project == "" || request.ID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Send a project and an id.", Code: "invalidRequest"})
		return
	}
	_, err := c.notices.AnnounceWalkthrough(request.Project, request.ID, request.Created)
	switch {
	case err == nil:
		writeJSON(w, http.StatusAccepted, announceResponse{Announced: true})
	case errors.Is(err, web.ErrProjectUnavailable):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "No served root shows the project.", Code: "projectUnavailable"})
	case errors.Is(err, walkthrough.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "The daemon's walkthrough store has no such walkthrough.", Code: "walkthroughUnavailable"})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Announce walkthrough: " + err.Error()})
	}
}

// serveEvents streams daemon-level events: status first, then each
// walkthrough announcement. It counts as in flight for idle exit while open,
// and ends with the request, the daemon, or the notices.
func (c *control) serveEvents(w http.ResponseWriter, r *http.Request) {
	notices, cancel := c.notices.WalkthroughNotices()
	defer cancel()
	// Streams outlive the server's write timeout; each write extends it.
	controller := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	write := func(chunk string) bool {
		_ = controller.SetWriteDeadline(time.Now().Add(controlHeartbeat + 5*time.Second))
		if _, err := fmt.Fprint(w, chunk); err != nil {
			return false
		}
		return controller.Flush() == nil
	}
	if !write(": servef\n\n") || !write(controlEvent("status", controlStatusEvent{Protocol: c.status.Protocol, Version: c.status.Version})) {
		return
	}
	heartbeat := time.NewTicker(controlHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-c.done:
			return
		case <-heartbeat.C:
			if !write(": heartbeat\n\n") {
				return
			}
		case notice, ok := <-notices:
			if !ok {
				return
			}
			if !write(controlEvent("walkthrough", walkthroughEvent{Project: notice.Project, ID: notice.ID, Title: notice.Title, Created: notice.Created})) {
				return
			}
		}
	}
}

func controlEvent(name string, value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return "event: " + name + "\ndata: " + string(data) + "\n\n"
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "Method not allowed."})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
