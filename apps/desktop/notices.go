package main

import (
	"errors"
	"log"
	"runtime"
	"strings"

	"github.com/egoist/mygo"

	"github.com/flexdinesh/servef/apps/desktop/internal/bridge"
	"github.com/flexdinesh/servef/apps/desktop/internal/connect"
	"github.com/flexdinesh/servef/apps/desktop/internal/notifier"
	"github.com/flexdinesh/servef/apps/desktop/internal/proxy"
)

// These reach the OS; end-to-end builds, which have no notification service,
// replace them.
var (
	displayNotice = func(id, title, body string) error {
		return mygo.NewNotification(mygo.NotificationOptions{ID: id, Title: title, Body: body}).Show()
	}
	noticesSupported = mygo.NotificationsSupported
	windowFocused    = (*mygo.Window).IsFocused
	// clickNotice lets an end-to-end build click a notice.
	clickNotice func(id string)
)

// configureNotifications applies a page's settings to its connection's notifier,
// which runs from the first page's call until the window closes.
func (h *host) configureNotifications(connectionHost string, window *mygo.Window, config bridge.Notifications) bridge.NotificationStatus {
	h.mu.Lock()
	running := h.notifiers[connectionHost]
	if running == nil {
		running = h.newNotifier(connectionHost, window)
		h.notifiers[connectionHost] = running
	}
	h.mu.Unlock()
	running.Configure(config.Project, notifier.Settings{Commits: config.Commits, Branches: config.Branches, Edits: config.Edits, Walkthroughs: config.Walkthroughs})
	return bridge.NotificationStatus{Supported: noticesSupported(), Denied: h.noticesDenied.Load()}
}

func (h *host) newNotifier(connectionHost string, window *mygo.Window) *notifier.Notifier {
	options := notifier.Options{
		Connection: connectionHost,
		Connect:    h.daemon.StreamClient,
		Focused:    func() bool { return windowFocused(window) },
		Show:       h.showNotice,
	}
	if connectionHost != proxy.Host {
		options.Label = h.connections.Label(connectionHost)
		options.Connect = connect.RemoteClients(h.store, connectionHost)
	}
	return notifier.New(options)
}

// stopNotifier ends a closed window's streams. Its server may then stop: streams
// count as requests in flight, and it stops when no UI needs it (decision 0098).
func (h *host) stopNotifier(connectionHost string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if running := h.notifiers[connectionHost]; running != nil {
		running.Stop()
		delete(h.notifiers, connectionHost)
	}
}

// showNotice shows a notice and remembers where a click goes. Off the notifier's
// goroutine: on macOS the first notice waits for the user's answer.
func (h *host) showNotice(notice notifier.Notice) {
	connectionHost, _, _ := strings.Cut(notice.ID, "/")
	target := proxy.OriginOf(connectionHost) + notice.Route
	h.mu.Lock()
	h.noticeTargets[notice.ID] = target
	h.mu.Unlock()
	go func() {
		err := displayNotice(notice.ID, notice.Title, notifier.BodyFor(runtime.GOOS, notice.Body))
		switch {
		case errors.Is(err, mygo.ErrNotificationsDenied):
			h.noticesDenied.Store(true)
		case err != nil:
			log.Printf("servef-desktop: notification: %v", err)
		default:
			h.noticesDenied.Store(false)
		}
	}()
}

// noticeClicked opens what a notice is about, in its connection's window. A
// notice from an earlier run has no target: it only brings the app forward.
func (h *host) noticeClicked(id string) {
	h.mu.Lock()
	target, ok := h.noticeTargets[id]
	h.mu.Unlock()
	if !ok {
		h.raise()
		return
	}
	go h.load(target)
}
