//go:build e2e

package main

import (
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
)

// Under Xvfb there is no notification service and no window manager to move
// focus. With SERVEF_E2E_NOTICES set, notices append to that file as JSON lines,
// and the test drives focus and clicks through the page's mygo.call. Release
// builds never carry it.
func init() {
	path := os.Getenv("SERVEF_E2E_NOTICES")
	if path == "" {
		return
	}
	var (
		mu      sync.Mutex
		focused atomic.Bool
	)
	displayNotice = func(id, title, body string) error {
		mu.Lock()
		defer mu.Unlock()
		line, err := json.Marshal(map[string]string{"id": id, "title": title, "body": body})
		if err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		_, err = file.Write(append(line, '\n'))
		return err
	}
	noticesSupported = func() bool { return true }
	windowFocused = func(*mygo.Window) bool { return focused.Load() }
	mygo.BindAs("E2E", &e2eService{focused: &focused})
}

type e2eService struct {
	focused *atomic.Bool
}

// SetFocused says whether windows have focus.
func (s *e2eService) SetFocused(focused bool) { s.focused.Store(focused) }

// ClickNotice does what clicking the notice with this ID does.
func (s *e2eService) ClickNotice(id string) {
	if clickNotice != nil {
		clickNotice(id)
	}
}
