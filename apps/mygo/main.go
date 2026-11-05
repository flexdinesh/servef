// Command servef-mygo is servef's experimental mygo app (decision 0094):
// one window whose page is the web app, served from the daemon through a
// custom scheme. It runs once per user: links (servef-mygo://) and
// `servef-mygo [path]` reach the first instance.
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
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"github.com/flexdinesh/servef/apps/mygo/internal/connect"
	"github.com/flexdinesh/servef/apps/mygo/internal/link"
	"github.com/flexdinesh/servef/apps/mygo/internal/proxy"
	"github.com/flexdinesh/servef/pkg/client"
	"github.com/flexdinesh/servef/pkg/contract"
)

// Stamped by release builds with -ldflags -X.
var (
	version = "0.0.0-dev"
	commit  = ""
)

// host is the running app: the daemon, its window, and what opens in it.
type host struct {
	daemon *connect.Daemon
	window atomic.Pointer[mygo.Window]
	// first holds what links ask for until the window exists.
	first firstPage
	// reloads limits reloading a page that keeps crashing.
	reloads reloadLimiter
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
	app := &host{daemon: connect.NewDaemon(binary, binaryErr, connect.DevFlags(os.Getenv), connect.Exec)}
	app.notify = app.showWarning

	pages := proxy.New(app.daemon)
	if err := mygo.Protocol.Handle(proxy.Scheme, pages); err != nil {
		log.Fatal(err)
	}
	mygo.BindAs("Bridge", &bridgeService{host: app})
	mygo.App.SetMenu(app.menu())

	workingDir, _ := os.Getwd()
	// Windows and Linux pass links as arguments, handled with the rest of
	// them; macOS sends them as Apple Events.
	if runtime.GOOS == "darwin" {
		mygo.App.OnOpenURL(func(raw string) { go app.openLink(raw) })
	}
	mygo.App.OnSecondInstance(func(args []string, workingDir string) {
		if w := app.window.Load(); w != nil {
			w.Restore()
			w.Focus()
		}
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

func (h *host) openWindow(start string) {
	window := mygo.NewWindow(mygo.WindowOptions{
		Title:           "servef",
		Width:           1200,
		Height:          800,
		MinWidth:        640,
		MinHeight:       400,
		StateKey:        "main",
		URL:             start,
		BackgroundColor: "light-dark(#d7d8dc, #101116)",
		Page:            mygo.PageOptions{PreloadScript: preloadScript},
	})
	h.window.Store(window)
	page := window.Page()
	// Pages get nothing the web app does not use.
	page.SetPermissionHandler(func(mygo.PermissionRequest) bool { return false })
	page.OnWillNavigate(func(event *mygo.NavigateEvent) {
		if !h.staysInApp(event.URL) {
			event.PreventDefault()
		}
	})
	page.SetWindowOpenHandler(func(request mygo.WindowOpenRequest) *mygo.WindowOptions {
		h.staysInApp(request.URL)
		return nil
	})
	// A crashed page comes back where it was, unless it just did that.
	page.OnRenderProcessGone(func(string) {
		if h.reloads.allow(time.Now()) {
			page.Reload()
			return
		}
		h.load(proxy.Origin + proxy.CrashedPath)
	})
}

// staysInApp reports navigation the page may do on its own: its origin.
// Links to the app's scheme route here; http(s) and mailto open outside;
// the rest is denied (decision 0067).
func (h *host) staysInApp(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case proxy.Scheme:
		return u.Host == proxy.Host
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
		h.report("Not a servef-mygo link.", err.Error())
		return start
	}
	if request.Link != nil {
		if request.Link.Kind == link.Server {
			openServerInBrowser(*request.Link)
			return start
		}
		return localURL(*request.Link)
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
		h.report("Not a servef-mygo link.", err.Error())
		return
	}
	switch {
	case request.Link != nil && request.Link.Kind == link.Server:
		openServerInBrowser(*request.Link)
	case request.Link != nil:
		h.load(localURL(*request.Link))
	case request.Path != "":
		h.openAndLoad(request.Path)
	}
}

func (h *host) openLink(raw string) {
	target, err := link.Parse(raw)
	if err != nil {
		h.report("Not a servef-mygo link.", raw)
		return
	}
	if target.Kind == link.Server {
		openServerInBrowser(target)
		return
	}
	h.load(localURL(target))
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
// `servef [path]` does, and returns its project's landing page.
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

func (h *host) load(target string) {
	if h.first.request(target) {
		return
	}
	if w := h.window.Load(); w != nil {
		if err := w.Page().LoadURL(target); err != nil {
			log.Printf("servef-mygo: %v", err)
		}
	}
}

// report tells the user something failed without blocking anything.
func (h *host) report(message, detail string) {
	log.Printf("servef-mygo: %s: %s", message, detail)
	h.notify(message, detail)
}

func (h *host) showWarning(message, detail string) {
	go func() {
		_, _ = mygo.Dialog.Message(mygo.MessageOptions{Parent: h.window.Load(), Type: mygo.MessageWarning, Message: message, Detail: detail})
	}()
}

func localURL(target link.Target) string {
	return proxy.Origin + target.AppPath()
}

// openServerInBrowser opens an http server's link in the browser: the
// window shows the local daemon only.
func openServerInBrowser(target link.Target) {
	go func() { _ = mygo.Shell.OpenExternal(target.Origin() + target.AppPath()) }()
}

func versionLine() string {
	return strings.TrimSpace("servef-mygo " + version + " " + commit)
}
