package main

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/flexdinesh/servef/apps/mygo/internal/bridge"
	"github.com/flexdinesh/servef/apps/mygo/internal/proxy"
	"github.com/flexdinesh/servef/pkg/client"
	"github.com/flexdinesh/servef/pkg/contract"
)

// preloadScript is window.servefDesktop (decision 0081) over mygo's IPC, on
// the app's origin only. The UI feature-detects every method.
const preloadScript = `(() => {
  if (location.protocol !== "` + proxy.Scheme + `:" || location.host !== "` + proxy.Host + `") return
  const call = (method, ...args) => window.mygo.call("Bridge." + method, ...args)
  window.servefDesktop = Object.freeze({
    version: 2,
    openFolder() { void call("OpenFolder").catch(() => undefined) },
    showAppMenu(x, y) { void call("ShowAppMenu", x, y).catch(() => undefined) },
    revealPath(project, path) { return call("RevealPath", project, path) },
    showContextMenu(items, x, y) { return call("ShowContextMenu", items, x, y) },
  })
})()`

// errRefused answers a sender that is not the app's page or a payload that
// is invalid. Nothing happens, and no dialog says so (decision 0081).
var errRefused = errors.New("refused")

// bridgeService is what the app's page calls. mygo already rejects pages
// that are not trusted; each method checks the sender's URL itself, since
// trust also covers file: and about: pages, then validates its payload.
type bridgeService struct {
	host *host
	// menus lets one context menu or app menu open at a time.
	menus sync.Mutex
}

// callerWindow returns the sender's window, if the sender is the app's page.
func callerWindow(ctx context.Context) (*mygo.Window, bool) {
	page := mygo.CallerPage(ctx)
	if page == nil || !bridge.IsAppPage(page.URL(), proxy.Scheme, proxy.Host) {
		return nil, false
	}
	window := mygo.CallerWindow(ctx)
	return window, window != nil
}

// OpenFolder asks for a directory and opens it as a project.
func (b *bridgeService) OpenFolder(ctx context.Context) error {
	window, ok := callerWindow(ctx)
	if !ok {
		return errRefused
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Title: "Open a folder in servef", Directory: true})
	if err != nil || len(paths) == 0 {
		return err
	}
	b.host.openAndLoad(paths[0])
	return nil
}

// RevealPath shows a project file in the file manager; false when refused,
// with a report. Only real paths inside a daemon root are revealed.
func (b *bridgeService) RevealPath(ctx context.Context, project, relative any) (bool, error) {
	if _, ok := callerWindow(ctx); !ok {
		return false, errRefused
	}
	projectPath, relativePath, ok := bridge.ParseReveal(project, relative)
	if !ok {
		return false, errRefused
	}
	wait, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var status contract.DaemonStatus
	err := b.host.daemon.Do(wait, func(daemon client.Client) (err error) {
		status, err = daemon.DaemonStatus(wait)
		return err
	})
	if err != nil {
		b.host.report("Could not reveal the file", err.Error())
		return false, nil
	}
	target, err := bridge.ResolveReveal(status.Roots, projectPath, relativePath, filepath.EvalSymlinks)
	if err != nil {
		b.host.report("Could not reveal the file", err.Error())
		return false, nil
	}
	mygo.Shell.ShowItemInFolder(target)
	return true, nil
}

// ShowContextMenu pops a native menu at a point in the page and answers the
// chosen id, or null when dismissed.
func (b *bridgeService) ShowContextMenu(ctx context.Context, items, x, y any) (*string, error) {
	window, ok := callerWindow(ctx)
	if !ok {
		return nil, errRefused
	}
	width, height := window.ContentSize()
	point, okPoint := bridge.ParsePoint(x, y, width, height)
	rows, okItems := bridge.ParseItems(items)
	if !okPoint || !okItems || !b.menus.TryLock() {
		return nil, errRefused
	}
	defer b.menus.Unlock()
	var chosen *string
	entries := make([]*mygo.MenuItem, 0, len(rows))
	for _, row := range rows {
		if row.Separator {
			entries = append(entries, mygo.Separator())
			continue
		}
		id := row.ID
		entries = append(entries, &mygo.MenuItem{
			Label:    row.Label,
			Disabled: !row.Enabled,
			Click:    func(*mygo.MenuItem, *mygo.Window) { chosen = &id },
		})
	}
	mygo.NewMenu(entries).PopupAt(window, point.X, point.Y)
	return chosen, nil
}

// ShowAppMenu pops the app menu at a point in the page (decision 0075).
func (b *bridgeService) ShowAppMenu(ctx context.Context, x, y any) error {
	window, ok := callerWindow(ctx)
	if !ok {
		return errRefused
	}
	width, height := window.ContentSize()
	point, okPoint := bridge.ParsePoint(x, y, width, height)
	if !okPoint || !b.menus.TryLock() {
		return errRefused
	}
	defer b.menus.Unlock()
	b.host.menu().PopupAt(window, point.X, point.Y)
	return nil
}

// menu is the app menu: the menu bar's, and ShowAppMenu's.
func (h *host) menu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "File", Submenu: []*mygo.MenuItem{
			{Label: "Open Folder…", Accelerator: "CmdOrCtrl+O", Click: func(*mygo.MenuItem, *mygo.Window) { go h.chooseFolder() }},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleViewMenu},
		windowMenu(runtime.GOOS),
	})
}

// windowMenu is the Window menu. mygo's macOS role has no Close, so Cmd+W
// would do nothing there.
func windowMenu(goos string) *mygo.MenuItem {
	if goos != "darwin" {
		return &mygo.MenuItem{Role: mygo.RoleWindowMenu}
	}
	return &mygo.MenuItem{Role: mygo.RoleWindow, Submenu: []*mygo.MenuItem{
		{Role: mygo.RoleMinimize},
		{Role: mygo.RoleZoom},
		{Role: mygo.RoleClose},
		mygo.Separator(),
		{Role: mygo.RoleFront},
	}}
}

func (h *host) chooseFolder() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: h.window.Load(), Title: "Open a folder in servef", Directory: true})
	if err == nil && len(paths) > 0 {
		h.openAndLoad(paths[0])
	}
}
