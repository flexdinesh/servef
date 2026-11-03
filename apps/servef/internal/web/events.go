package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/flexdinesh/servef/pkg/contract"
)

// eventHeartbeat writes a comment to idle streams so a closed tab's stream
// ends on the next write.
const eventHeartbeat = 25 * time.Second

// serveEvents streams a project's live updates (app.Events) as server-sent
// events while the browser shows it; each event's name is its contract name.
func (a *App) serveEvents(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	subscription, err := a.app.Events.Subscribe(contract.EventsRequest{Project: query.Get("project"), Lease: query.Get("lease")})
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		writeError(w, err)
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
			for _, event := range subscription.Next() {
				if !write(serverEvent(event.Name, event.Payload())) {
					return
				}
			}
		}
	}
}

func serverEvent(name string, value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		data = []byte("{}")
	}
	return "event: " + name + "\ndata: " + string(data) + "\n\n"
}
