package main

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"github.com/flexdinesh/srcz/apps/desktop/internal/connect"
	"github.com/flexdinesh/srcz/apps/desktop/internal/proxy"
	"github.com/flexdinesh/srcz/pkg/connection"
)

func TestWindowIdentityFollowsTheConnection(t *testing.T) {
	if got := windowStateKey(proxy.Host); got != "main" {
		t.Errorf("local state key = %q: its remembered bounds would be lost", got)
	}
	if windowStateKey("r-0123456789ab") == windowStateKey("r-000000000000") {
		t.Error("two connections share remembered bounds")
	}
	if got := windowTitle(proxy.Host, ""); got != "srcz" {
		t.Errorf("local title = %q", got)
	}
	if got := windowTitle("r-0123456789ab", "box"); got != "srcz: box" {
		t.Errorf("remote title = %q", got)
	}
}

func TestOnlyNamesConnection(t *testing.T) {
	for raw, want := range map[string]bool{
		"srcz-app://r-0123456789ab/":              true,
		"srcz-app://r-0123456789ab":               true,
		"srcz-app://r-0123456789ab/?project=%2Fa": false,
		"srcz-app://r-0123456789ab/view":          false,
		"srcz-app://r-0123456789ab/#L1":           false,
	} {
		if got := onlyNamesConnection(raw); got != want {
			t.Errorf("onlyNamesConnection(%q) = %v", raw, got)
		}
	}
}

func TestFirstPageOpensOnTheStartPage(t *testing.T) {
	var first firstPage
	got := ""
	first.open("start", func(page string) { got = page })
	if got != "start" {
		t.Fatalf("first page %q", got)
	}
}

func TestALinkBeforeTheWindowBecomesTheFirstPage(t *testing.T) {
	var first firstPage
	if !first.request("one") || !first.request("two") {
		t.Fatal("requests before the window were not kept")
	}
	got := ""
	first.open("start", func(page string) { got = page })
	if got != "two" {
		t.Fatalf("first page %q, want the latest link", got)
	}
}

func TestALinkAfterTheWindowLoadsThere(t *testing.T) {
	var first firstPage
	first.open("start", func(string) {})
	if first.request("link") {
		t.Fatal("request after the window was kept")
	}
}

func TestReloadLimiterAllowsOneReloadPerPeriod(t *testing.T) {
	var limiter reloadLimiter
	now := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	if !limiter.allow(now) {
		t.Fatal("first crash did not reload")
	}
	if limiter.allow(now.Add(crashReloadEvery - time.Second)) {
		t.Fatal("second crash inside the period reloaded")
	}
	if !limiter.allow(now.Add(crashReloadEvery)) {
		t.Fatal("crash after the period did not reload")
	}
}

// A launch that cannot do what its arguments ask still opens the default
// page, and says why.
func TestStartURLReportsWhatItCouldNotOpen(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"unknown link", []string{"srcz://nope:x/"}, "Not a srcz link."},
		{"folder", []string{"/no/such/folder"}, "Could not open folder"},
		{"connection not saved here", []string{"srcz://r-0123456789ab/"}, "This computer has no such srcz connection"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var reported []string
			daemon := connect.NewDaemon("", errors.New("no daemon"), nil, nil)
			h := &host{
				daemon:      daemon,
				connections: proxy.NewConnections(daemon, connection.NewStore(filepath.Join(t.TempDir(), "connections.json"))),
				notify:      func(message, _ string) { reported = append(reported, message) },
			}
			if got := h.startURL(c.args, "/"); got != proxy.Origin+"/" {
				t.Errorf("start page %q", got)
			}
			if len(reported) != 1 || reported[0] != c.want {
				t.Errorf("reported %v, want %q", reported, c.want)
			}
		})
	}
}

func TestWindowMenuClosesTabsInsteadOfTheWindow(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		t.Run(goos, func(t *testing.T) {
			item := (&host{}).windowMenu(goos)
			found := false
			for _, entry := range item.Submenu {
				if entry.Role == mygo.RoleClose {
					t.Fatal("native Close bypasses the workspace")
				}
				found = found || (entry.Accelerator == "CmdOrCtrl+W" && entry.Click != nil)
			}
			if !found {
				t.Fatal("no Cmd/Ctrl+W tab action")
			}
		})
	}
}
