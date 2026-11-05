package daemon

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/flexdinesh/servef/internal/app"
	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/pkg/contract"
)

// controlPrefix marks the control API; its requests never reach the web app.
const controlPrefix = "/_daemon/"

// maxControlRequest bounds a POST /_daemon/* body.
const maxControlRequest = 4 << 10

// controlHeartbeat writes a comment to an idle control stream so a closed
// client's stream ends on the next write.
const controlHeartbeat = 25 * time.Second

// Status describes a running daemon. Its shape is frozen across protocols.
type Status = contract.DaemonStatus

type rootsResponse = contract.DaemonRoots

type addRootRequest = contract.DaemonPathRequest

type addRootResponse struct {
	Root  string `json:"root"`
	Added bool   `json:"added"`
}

// OpenResult is POST /_daemon/open's answer: the root serving the path,
// whether it was added, the roots it replaced (inside it), and the project
// that shows the path.
type OpenResult = contract.DaemonOpenResult

type announceRequest struct {
	Project string `json:"project"`
	ID      string `json:"id"`
	Created bool   `json:"created"`
}

type announceResponse struct {
	Announced bool `json:"announced"`
}

type stopResponse struct {
	Stopping bool `json:"stopping"`
}

// WalkthroughNotices announces published walkthroughs; app.Walkthroughs
// implements it.
type WalkthroughNotices interface {
	Announce(project, id string, created bool) (contract.WalkthroughEvent, error)
	Notices() (<-chan contract.WalkthroughEvent, func())
}

// errorResponse matches the web app's JSON errors: {error, code?}.
type errorResponse = contract.ErrorResponse

// Controller owns the daemon control operations and serves them as
// /_daemon/* (Handler); in-process clients call its typed operations
// directly (pkg/client). Adds and saves serialize under mu; a save writes the
// served roots plus the roots skipped at load, so an unmounted drive is not
// forgotten. Control streams end when done closes.
type Controller struct {
	mu        sync.Mutex
	roots     *projects.RootSet
	skipped   []string
	rootsFile string
	status    Status
	stop      func()
	done      <-chan struct{}
	notices   WalkthroughNotices
}

// ControllerConfig configures NewController.
type ControllerConfig struct {
	// Roots is the served root set, required.
	Roots *projects.RootSet
	// Skipped are saved roots skipped at start; saves keep them.
	Skipped []string
	// RootsFile receives the roots after each add.
	RootsFile string
	// Status is the daemon's status; Roots is filled from Roots per call.
	Status Status
	// Stop asks the daemon to stop; nil does nothing.
	Stop func()
	// Done closes when the daemon stops, ending control streams.
	Done <-chan struct{}
	// Notices announces walkthroughs, required.
	Notices WalkthroughNotices
}

// NewController returns the control operations over config.
func NewController(config ControllerConfig) *Controller {
	stop := config.Stop
	if stop == nil {
		stop = func() {}
	}
	return &Controller{roots: config.Roots, skipped: config.Skipped, rootsFile: config.RootsFile, status: config.Status, stop: stop, done: config.Done, notices: config.Notices}
}

func pid() int { return os.Getpid() }

// Handler serves the control API before next. Browser-originated requests
// are refused: the socket is the trust boundary, and a future proxy must not
// forward browser requests to it.
func (c *Controller) Handler(next http.Handler) http.Handler {
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
			writeJSON(w, http.StatusOK, c.Status())
		case name == "roots" && r.Method == http.MethodGet:
			writeJSON(w, http.StatusOK, c.Roots())
		case name == "roots" && r.Method == http.MethodPost:
			c.addRoot(w, r)
		case name == "open" && r.Method == http.MethodPost:
			c.serveOpen(w, r)
		case name == "events" && r.Method == http.MethodGet:
			c.serveEvents(w, r)
		case name == "walkthroughs" && r.Method == http.MethodPost:
			c.announce(w, r)
		case name == "stop" && r.Method == http.MethodPost:
			writeJSON(w, http.StatusAccepted, stopResponse{Stopping: true})
			c.stop()
		case name == "status", name == "events":
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

// Status is the daemon's status with the current roots.
func (c *Controller) Status() Status {
	status := c.status
	status.Roots = nonNil(c.roots.List())
	return status
}

// Roots returns the served roots and the saved roots skipped at start.
func (c *Controller) Roots() contract.DaemonRoots {
	c.mu.Lock()
	defer c.mu.Unlock()
	return rootsResponse{Roots: nonNil(c.roots.List()), Skipped: nonNil(slices.Clone(c.skipped))}
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

func (c *Controller) addRoot(w http.ResponseWriter, r *http.Request) {
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

// serveOpen answers POST /_daemon/open (Open): 201 when the path became a
// root, else 200.
func (c *Controller) serveOpen(w http.ResponseWriter, r *http.Request) {
	var request addRootRequest
	if !decodeRequest(w, r, &request, `{"path": "/absolute/path"}`) {
		return
	}
	result, err := c.Open(request.Path)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if result.Added {
		status = http.StatusCreated
	}
	writeJSON(w, status, result)
}

// Open serves a path, an absolute directory: within a served root nothing
// changes; otherwise the path becomes a root, replacing the roots inside it,
// and is saved. It returns the project that shows the path.
func (c *Controller) Open(path string) (OpenResult, error) {
	if !filepath.IsAbs(path) {
		return OpenResult{}, &app.Error{Kind: contract.KindInvalid, Code: contract.CodeInvalidRoot, Message: "Path must be absolute."}
	}
	c.mu.Lock()
	root, added, replaced, err := c.roots.Cover(path)
	if err != nil {
		c.mu.Unlock()
		return OpenResult{}, &app.Error{Kind: contract.KindInvalid, Code: contract.CodeInvalidRoot, Message: err.Error()}
	}
	if added {
		c.skipped = slices.DeleteFunc(c.skipped, func(skipped string) bool { return files.Within(root, skipped) })
		if err := SaveRoots(c.rootsFile, append(c.roots.List(), c.skipped...)); err != nil {
			c.mu.Unlock()
			return OpenResult{}, &app.Error{Kind: contract.KindInternal, Code: contract.CodeRootsNotSaved, Message: "The root is served until the daemon stops but was not saved: " + err.Error()}
		}
	}
	roots := c.roots.List()
	c.mu.Unlock()
	project, err := projects.ProjectFor(roots, filepath.Clean(path))
	if err != nil {
		return OpenResult{}, &app.Error{Kind: contract.KindNotFound, Code: contract.CodeProjectUnavailable, Message: "No project shows " + path + ": " + err.Error()}
	}
	return OpenResult{Root: root, Added: added, Replaced: nonNil(replaced), Project: project}, nil
}

// announce tells control streams that a walkthrough was published to the
// daemon's store, as the CLI's publish does after writing it.
func (c *Controller) announce(w http.ResponseWriter, r *http.Request) {
	var request announceRequest
	if !decodeRequest(w, r, &request, `{"project": "/absolute/path", "id": "id", "created": true}`) {
		return
	}
	if request.Project == "" || request.ID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Send a project and an id.", Code: "invalidRequest"})
		return
	}
	_, err := c.notices.Announce(request.Project, request.ID, request.Created)
	if err == nil {
		writeJSON(w, http.StatusAccepted, announceResponse{Announced: true})
		return
	}
	switch failure := app.AsError(err); failure.Code {
	case contract.CodeProjectUnavailable:
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "No served root shows the project.", Code: contract.CodeProjectUnavailable})
	case contract.CodeWalkthroughUnavailable:
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "The daemon's walkthrough store has no such walkthrough.", Code: contract.CodeWalkthroughUnavailable})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Announce walkthrough: " + failure.Message})
	}
}

// Subscribe starts a control stream: its first event (status), then each
// walkthrough announcement on notices until cancel, which the caller must
// call. Done closes when the daemon stops.
func (c *Controller) Subscribe() (first contract.DaemonEvent, notices <-chan contract.WalkthroughEvent, cancel func()) {
	notices, cancel = c.notices.Notices()
	status := contract.DaemonStatusEvent{Protocol: c.status.Protocol, Version: c.status.Version}
	return contract.DaemonEvent{Name: contract.DaemonEventStatus, Status: &status}, notices, cancel
}

// Done closes when the daemon stops.
func (c *Controller) Done() <-chan struct{} { return c.done }

// WalkthroughEvent is a notice as a control stream event.
func WalkthroughEvent(notice contract.WalkthroughEvent) contract.DaemonEvent {
	return contract.DaemonEvent{Name: contract.DaemonEventWalkthrough, Walkthrough: &notice}
}

// serveEvents streams daemon-level events: status first, then each
// walkthrough announcement. It counts as in flight for idle exit while open,
// and ends with the request, the daemon, or the notices.
func (c *Controller) serveEvents(w http.ResponseWriter, r *http.Request) {
	first, notices, cancel := c.Subscribe()
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
	if !write(": servef\n\n") || !write(controlEvent(first.Name, first.Payload())) {
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
			event := WalkthroughEvent(notice)
			if !write(controlEvent(event.Name, event.Payload())) {
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

// writeError writes err as an errorResponse with its kind's status.
func writeError(w http.ResponseWriter, err error) {
	failure := app.AsError(err)
	writeJSON(w, contract.HTTPStatus(failure.Kind), failure.Response())
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
