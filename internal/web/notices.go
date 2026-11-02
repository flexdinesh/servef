package web

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
)

// ErrProjectUnavailable reports that no served root resolves a project.
var ErrProjectUnavailable = errors.New("project unavailable")

// noticeBuffer is how many notices one subscriber holds before the oldest
// is dropped.
const noticeBuffer = 16

// WalkthroughNotice announces a published walkthrough (decision 0066).
type WalkthroughNotice struct {
	Project, ID, Title string
	Created            bool
}

// AnnounceWalkthrough tells every notice subscriber that project's
// walkthrough id was published. It resolves project against the served roots
// (ErrProjectUnavailable) and reads the walkthrough from this App's store
// (walkthrough.ErrNotFound, wrapped).
func (a *App) AnnounceWalkthrough(project, id string, created bool) (WalkthroughNotice, error) {
	resolved, status, _ := a.project(project)
	if status != http.StatusOK {
		return WalkthroughNotice{}, fmt.Errorf("%w: %s", ErrProjectUnavailable, project)
	}
	stored, err := a.walkthroughs.Get(resolved.root, id)
	if err != nil {
		return WalkthroughNotice{}, fmt.Errorf("walkthrough %s: %w", id, err)
	}
	notice := WalkthroughNotice{Project: stored.Project, ID: stored.ID, Title: stored.Title, Created: created}
	a.notices.announce(notice)
	return notice, nil
}

// WalkthroughNotices subscribes to walkthrough announcements. Each
// subscriber buffers 16 and drops the oldest when full. The channel closes
// after cancel or when live updates stop.
func (a *App) WalkthroughNotices() (notices <-chan WalkthroughNotice, cancel func()) {
	return a.notices.subscribe()
}

// noticeHub fans announcements out to subscribers without ever blocking the
// announcer. Its zero value is ready to use.
type noticeHub struct {
	mu          sync.Mutex
	closed      bool
	subscribers map[chan WalkthroughNotice]struct{}
}

func (h *noticeHub) subscribe() (<-chan WalkthroughNotice, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	notices := make(chan WalkthroughNotice, noticeBuffer)
	if h.closed {
		close(notices)
		return notices, func() {}
	}
	if h.subscribers == nil {
		h.subscribers = map[chan WalkthroughNotice]struct{}{}
	}
	h.subscribers[notices] = struct{}{}
	return notices, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subscribers[notices]; ok {
			delete(h.subscribers, notices)
			close(notices)
		}
	}
}

// announce sends notice to every subscriber, dropping a full subscriber's
// oldest notice to make room.
func (h *noticeHub) announce(notice WalkthroughNotice) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for notices := range h.subscribers {
		for sent := false; !sent; {
			select {
			case notices <- notice:
				sent = true
			default:
				// Full: drop the oldest, unless the reader just took it.
				select {
				case <-notices:
				default:
				}
			}
		}
	}
}

// close ends every subscription; later subscriptions start closed.
func (h *noticeHub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for notices := range h.subscribers {
		close(notices)
	}
	h.subscribers = nil
}
