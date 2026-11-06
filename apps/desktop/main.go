// Command srcz-desktop is the srcz desktop app (decisions 0094, 0095, 0098):
// one window per connection, whose page is the web app, served through a custom
// scheme from the local daemon or a saved remote server. It runs once per user:
// links (srcz://) and `srcz-desktop [path]` reach the first instance.
package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"github.com/flexdinesh/srcz/apps/desktop/internal/connect"
	"github.com/flexdinesh/srcz/apps/desktop/internal/link"
	"github.com/flexdinesh/srcz/apps/desktop/internal/notifier"
	"github.com/flexdinesh/srcz/apps/desktop/internal/proxy"
	"github.com/flexdinesh/srcz/pkg/client"
	"github.com/flexdinesh/srcz/pkg/connection"
	"github.com/flexdinesh/srcz/pkg/contract"
)

// Stamped by release builds with -ldflags -X.
var (
	version = "0.0.0-dev"
	commit  = ""
)

// host is the running app: the daemon, its connections, their windows, and
// what opens in them.
type host struct {
	daemon      *connect.Daemon
	connections *proxy.Connections
	store       connection.Store

	// mu guards windows, notifiers, and noticeTargets. Windows and notifiers are
	// one per connection, by its scheme host; noticeTargets is where a notice's
	// click goes, by its ID.
	mu            sync.Mutex
	windows       map[string]*mygo.Window
	notifiers     map[string]*notifier.Notifier
	noticeTargets map[string]string
	noticesDenied atomic.Bool
	// opening serializes looking for a window and creating it, so two links to
	// one connection cannot open two windows. It is separate from mu because
	// creating a window waits for the main thread, where OnClosed takes mu.
	opening sync.Mutex
	// first holds what links ask for until a window exists.
	first firstPage
	// notify shows a failure to the user.
	notify func(message, detail string)
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println(versionLine())
		return
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return // the first instance handles our arguments
	}
	resources, _ := mygo.App.Path(mygo.PathResources)
	binary, binaryErr := connect.Locate(mygo.App.IsPackaged(), resources, os.Getenv, exec.LookPath)
	app := &host{
		daemon:        connect.NewDaemon(binary, binaryErr, connect.DevFlags(os.Getenv), connect.Exec),
		windows:       map[string]*mygo.Window{},
		notifiers:     map[string]*notifier.Notifier{},
		noticeTargets: map[string]string{},
	}
	app.notify = app.showWarning
	storePath, err := connection.DefaultStorePath()
	if err != nil {
		log.Printf("srcz-desktop: %v", err)
	}
	app.store = connection.NewStore(storePath)
	app.connections = proxy.NewConnections(app.daemon, app.store)
	clickNotice = app.noticeClicked
	mygo.App.OnNotificationClick(app.noticeClicked)

	pages := proxy.New(app.connections)
	if err := mygo.Protocol.Handle(proxy.Scheme, pages); err != nil {
		log.Fatal(err)
	}
	mygo.BindAs("Bridge", &bridgeService{host: app})
	mygo.App.SetMenu(app.menu())

	workingDir, _ := os.Getwd()
	// Windows and Linux pass links as arguments, handled with the rest of
	// them; macOS sends them as Apple Events.
	if runtime.GOOS == "darwin" {
		mygo.App.OnOpenURL(func(raw string) {
			app.raise()
			go app.openLink(raw)
		})
	}
	mygo.App.OnSecondInstance(func(args []string, workingDir string) {
		app.raise()
		go app.handleArgs(args, workingDir)
	})
	mygo.App.WhenReady(func() {
		// The first page opens what the arguments ask for; ensuring the
		// daemon can take seconds, off the main thread.
		go func() {
			app.first.open(app.startURL(os.Args[1:], workingDir), app.openWindow)
		}()
	})
	mygo.App.OnQuit(pages.Close)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// raise brings a window to the front for what just asked for it.
func (h *host) raise() {
	if w := h.frontWindow(); w != nil {
		w.Restore()
		w.Focus()
	}
}

// frontWindow is the focused window, else the local one, else any.
func (h *host) frontWindow() *mygo.Window {
	if focused := mygo.FocusedWindow(); focused != nil {
		return focused
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if local := h.windows[proxy.Host]; local != nil {
		return local
	}
	for _, window := range h.windows {
		return window
	}
	return nil
}

// openWindow opens the window of start's connection.
func (h *host) openWindow(start string) {
	connectionHost := hostOf(start)
	window := mygo.NewWindow(mygo.WindowOptions{
		Title:           windowTitle(connectionHost, h.connections.Label(connectionHost)),
		Width:           1200,
		Height:          800,
		MinWidth:        640,
		MinHeight:       400,
		StateKey:        windowStateKey(connectionHost),
		URL:             start,
		BackgroundColor: "light-dark(#d7d8dc, #101116)",
		Page:            mygo.PageOptions{PreloadScript: preloadScript},
	})
	h.mu.Lock()
	h.windows[connectionHost] = window
	h.mu.Unlock()
	window.OnClosed(func() {
		h.mu.Lock()
		current := h.windows[connectionHost] == window
		if current {
			delete(h.windows, connectionHost)
		}
		h.mu.Unlock()
		if current {
			h.stopNotifier(connectionHost)
		}
	})
	page := window.Page()
	// Pages get nothing the web app does not use.
	page.SetPermissionHandler(func(mygo.PermissionRequest) bool { return false })
	page.OnWillNavigate(func(event *mygo.NavigateEvent) {
		if !h.staysInApp(connectionHost, event.URL) {
			event.PreventDefault()
		}
	})
	page.SetWindowOpenHandler(func(request mygo.WindowOpenRequest) *mygo.WindowOptions {
		h.staysInApp(connectionHost, request.URL)
		return nil
	})
	// A crashed page comes back where it was, unless it just did that.
	var reloads reloadLimiter
	page.OnRenderProcessGone(func(string) {
		if reloads.allow(time.Now()) {
			page.Reload()
			return
		}
		if err := page.LoadURL(proxy.OriginOf(connectionHost) + proxy.CrashedPath); err != nil {
			log.Printf("srcz-desktop: %v", err)
		}
	})
}

// windowStateKey keeps "main" for the local window: its remembered bounds
// predate connections.
func windowStateKey(connectionHost string) string {
	if connectionHost == proxy.Host {
		return "main"
	}
	return "connection-" + connectionHost
}

func windowTitle(connectionHost, label string) string {
	if connectionHost == proxy.Host || label == "" {
		return "srcz"
	}
	return "srcz: " + label
}

// hostOf is a page URL's connection.
func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

// staysInApp reports navigation a connection's page may do on its own: its
// origin. The app's other links route to their own windows; http(s) and mailto
// open outside; the rest is denied (decision 0067).
func (h *host) staysInApp(connectionHost, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case proxy.Scheme:
		if u.Host == connectionHost {
			return true
		}
		if h.connections.Known(u.Host) {
			go h.load(raw)
		}
	case link.Scheme:
		go h.openLink(raw)
	case "http", "https", "mailto":
		go func() { _ = mygo.Shell.OpenExternal(raw) }()
	}
	return false
}

// startURL resolves a launch's arguments into the first page.
func (h *host) startURL(args []string, workingDir string) string {
	start := proxy.Origin + "/"
	request, err := link.FromArgs(args, workingDir)
	if err != nil {
		h.report("Not a srcz link.", err.Error())
		return start
	}
	if request.Link != nil {
		if !h.connections.Known(request.Link.Connection) {
			h.reportNotPaired()
			return start
		}
		return appURL(*request.Link)
	}
	if request.Path != "" {
		landing, err := h.openFolder(request.Path)
		if err == nil {
			return landing
		}
		h.report("Could not open folder", request.Path+": "+err.Error())
	}
	return start
}

func (h *host) handleArgs(args []string, workingDir string) {
	request, err := link.FromArgs(args, workingDir)
	if err != nil {
		h.report("Not a srcz link.", err.Error())
		return
	}
	switch {
	case request.Link != nil:
		h.loadLink(*request.Link)
	case request.Path != "":
		h.openAndLoad(request.Path)
	}
}

func (h *host) openLink(raw string) {
	target, err := link.Parse(raw)
	if err != nil {
		h.report("Not a srcz link.", raw)
		return
	}
	h.loadLink(target)
}

// loadLink opens a link in its connection's window; a connection this computer
// has not saved opens nothing.
func (h *host) loadLink(target link.Target) {
	if !h.connections.Known(target.Connection) {
		h.reportNotPaired()
		return
	}
	h.load(appURL(target))
}

func (h *host) reportNotPaired() {
	h.report("This computer has no such srcz connection", "Run `srcz connect` with the string `srcz <path> --remote` prints on the server.")
}

func (h *host) openAndLoad(folder string) {
	landing, err := h.openFolder(folder)
	if err != nil {
		h.report("Could not open folder", folder+": "+err.Error())
		return
	}
	h.load(landing)
}

// openFolder serves folder through /_daemon/open, adding a root as
// `srcz [path]` does, and returns its project's landing page.
func (h *host) openFolder(folder string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), connect.EnsureTimeout+15*time.Second)
	defer cancel()
	var opened contract.DaemonOpenResult
	err := h.daemon.Do(ctx, func(daemon client.Client) (err error) {
		opened, err = daemon.DaemonOpen(ctx, folder)
		return err
	})
	if err != nil {
		return "", err
	}
	query := url.Values{"project": {opened.Project}}
	return proxy.Origin + "/?" + query.Encode(), nil
}

// load shows target in its connection's window, opening one if none is open.
// Connecting again only brings the window forward: what it shows stays.
func (h *host) load(target string) {
	if h.first.request(target) {
		return
	}
	h.opening.Lock()
	defer h.opening.Unlock()
	h.mu.Lock()
	window := h.windows[hostOf(target)]
	h.mu.Unlock()
	if window == nil {
		h.openWindow(target)
		return
	}
	window.Restore()
	window.Focus()
	if onlyNamesConnection(target) {
		return
	}
	if err := window.Page().LoadURL(target); err != nil {
		log.Printf("srcz-desktop: %v", err)
	}
}

// onlyNamesConnection reports a link that asks for no page in particular.
func onlyNamesConnection(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Path == "" || u.Path == "/") && u.RawQuery == "" && u.Fragment == ""
}

// report tells the user something failed without blocking anything.
func (h *host) report(message, detail string) {
	log.Printf("srcz-desktop: %s: %s", message, detail)
	h.notify(message, detail)
}

func (h *host) showWarning(message, detail string) {
	go func() {
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{Parent: h.frontWindow(), Type: mygo.MessageWarning, Message: message, Detail: detail})
	}()
}

// appURL is a link's page on its connection's origin.
func appURL(target link.Target) string {
	return proxy.OriginOf(target.Connection) + target.AppPath()
}

func versionLine() string {
	return strings.TrimSpace("srcz-desktop " + version + " " + commit)
}
