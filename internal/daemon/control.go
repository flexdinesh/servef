package daemon

import (
	"encoding/json"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/flexdinesh/servef/internal/projects"
)

// controlPrefix marks the control API; its requests never reach the web app.
const controlPrefix = "/_daemon/"

// maxRootRequest bounds a POST /_daemon/roots body.
const maxRootRequest = 4 << 10

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

type stopResponse struct {
	Stopping bool `json:"stopping"`
}

// errorResponse matches the web app's JSON errors: {error, code?}.
type errorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// control serves /_daemon/*. Adds and saves serialize under mu; a save
// writes the served roots plus the roots skipped at load, so an unmounted
// drive is not forgotten.
type control struct {
	mu        sync.Mutex
	roots     *projects.RootSet
	skipped   []string
	rootsFile string
	status    Status
	stop      func()
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
		case name == "stop" && r.Method == http.MethodPost:
			writeJSON(w, http.StatusAccepted, stopResponse{Stopping: true})
			c.stop()
		case name == "status":
			methodNotAllowed(w, http.MethodGet)
		case name == "roots":
			methodNotAllowed(w, "GET, POST")
		case name == "stop":
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

func (c *control) addRoot(w http.ResponseWriter, r *http.Request) {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		writeJSON(w, http.StatusUnsupportedMediaType, errorResponse{Error: "Send a JSON body.", Code: "unsupportedMediaType"})
		return
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRootRequest))
	decoder.DisallowUnknownFields()
	var request addRootRequest
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Send {\"path\": \"/absolute/path\"}.", Code: "invalidRequest"})
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
