package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/flexdinesh/servef/internal/files"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/internal/watch"
)

// eventHeartbeat writes a comment to idle streams so a closed tab's stream
// ends on the next write.
const eventHeartbeat = 25 * time.Second

// statusEvent is the event stream's "status": how the project is followed.
type statusEvent struct {
	Mode   watch.Mode `json:"mode"`
	Reason string     `json:"reason"`
}

// changeEvent is the event stream's "change": one debounced batch of
// project-relative paths (decision 0050).
type changeEvent struct {
	Paths    []string `json:"paths"`
	Tree     bool     `json:"tree"`
	Git      bool     `json:"git"`
	Overflow bool     `json:"overflow"`
}

// serveEvents streams a project's changes as server-sent events while the
// browser shows it. The subscription is the project's interest: the hub
// watches a project only while some stream follows it.
func (a *App) serveEvents(w http.ResponseWriter, r *http.Request) {
	project, status, message := a.project(r.URL.Query().Get("project"))
	if status != http.StatusOK {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, status, errorResponse{Error: message})
		return
	}
	subscription, err := a.watches.Subscribe(a.watchTarget(project.root))
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "Live updates are unavailable."})
		return
	}
	defer subscription.Close()

	// Streams outlive the server's write timeout; each write extends it.
	controller := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	write := func(chunk string) bool {
		_ = controller.SetWriteDeadline(time.Now().Add(eventHeartbeat + 5*time.Second))
		if _, err := fmt.Fprint(w, chunk); err != nil {
			return false
		}
		return controller.Flush() == nil
	}
	if !write(": servef\n\n") {
		return
	}
	heartbeat := time.NewTicker(eventHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-subscription.Done():
			return
		case <-heartbeat.C:
			if !write(": heartbeat\n\n") {
				return
			}
		case <-subscription.Ready():
			next, change := subscription.Next()
			if next != nil && !write(serverEvent("status", statusEvent{Mode: next.Mode, Reason: next.Reason})) {
				return
			}
			if change != nil && !write(serverEvent("change", changeEvent{Paths: change.Paths, Tree: change.Tree, Git: change.Git, Overflow: change.Overflow})) {
				return
			}
		}
	}
}

// watchTarget is what following a project watches: the directories its scan
// reads and, for a checkout, its git state.
func (a *App) watchTarget(root string) watch.Target {
	return watch.Target{
		Root: root,
		Directories: func() ([]string, int, error) {
			return files.Directories(root, a.config.Depth, a.config.Exclusions)
		},
		Git: func() ([]string, func(string) bool) {
			state, ok := projects.ReadGitState(root)
			if !ok {
				return nil, nil
			}
			return state.Directories, state.Records
		},
	}
}

func serverEvent(name string, value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		data = []byte("{}")
	}
	return "event: " + name + "\ndata: " + string(data) + "\n\n"
}
