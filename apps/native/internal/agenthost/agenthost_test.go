package agenthost_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/servef/apps/native/internal/agenthost"
	"github.com/flexdinesh/servef/packages/mygo/agent/client"
	"github.com/flexdinesh/servef/packages/mygo/agent/scenario"
)

func serve(t *testing.T, opts agenthost.Options) *client.Client {
	t.Helper()
	host, err := agenthost.Start(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.Close)
	dir, err := os.MkdirTemp("", "nag")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "a.sock")
	go func() { _ = host.Session.ListenAndServe(socket) }()
	c, err := client.DialWait(socket, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// Every scenario in scenarios/ runs headless against the shared fixtures
// and writes each of its shots.
func TestScenariosRunAgainstFixtures(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "scenarios", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("scenarios %v %v", files, err)
	}
	for _, file := range files {
		sc, err := scenario.Load(file)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(sc.Name, func(t *testing.T) {
			c := serve(t, agenthost.Options{})
			out := t.TempDir()
			result, err := scenario.Run(c, sc, scenario.Options{Out: out})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Shots) != len(sc.Shots()) {
				t.Fatalf("shots %d, want %d", len(result.Shots), len(sc.Shots()))
			}
			for _, shot := range result.Shots {
				if info, err := os.Stat(shot.Path); err != nil || info.Size() == 0 {
					t.Fatalf("%s: %v", shot.Path, err)
				}
			}
		})
	}
}

func TestStartsOnConnectPageAndTitlesWindow(t *testing.T) {
	c := serve(t, agenthost.Options{ConnectPage: true, Width: 900, Height: 600})
	if err := c.WaitText(0, "Open this computer", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := c.ClickText(0, "Open this computer"); err != nil {
		t.Fatal(err)
	}
	if err := c.WaitText(0, "/home/user/markdown", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := c.ClickText(0, "markdown"); err != nil {
		t.Fatal(err)
	}
	if err := c.WaitText(0, "guides", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	windows, err := c.Windows()
	if err != nil || len(windows) != 1 || !strings.Contains(windows[0].Title, "markdown") || windows[0].ContentBounds.Width != 900 {
		t.Fatalf("windows %+v %v", windows, err)
	}
}

func TestLinkOpensDocument(t *testing.T) {
	c := serve(t, agenthost.Options{Link: "servef-native://local/?project=/home/user/markdown&path=guides/getting-started.md"})
	if err := c.WaitText(0, "guides/getting-started.md", 5*time.Second); err != nil {
		t.Fatal(err)
	}
}
