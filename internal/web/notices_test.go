package web

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/servef/internal/walkthrough"
)

func receiveNotice(t *testing.T, notices <-chan WalkthroughNotice) WalkthroughNotice {
	t.Helper()
	select {
	case notice, ok := <-notices:
		if !ok {
			t.Fatal("notices closed")
		}
		return notice
	case <-time.After(3 * time.Second):
		t.Fatal("no notice")
	}
	return WalkthroughNotice{}
}

func assertClosed(t *testing.T, label string, notices <-chan WalkthroughNotice) {
	t.Helper()
	for {
		select {
		case _, ok := <-notices:
			if !ok {
				return
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("%s: notices still open", label)
		}
	}
}

func publishTitled(t *testing.T, app *App, id, title string) {
	t.Helper()
	response := publish(t, app, draftJSON(t, walkthrough.Draft{Project: app.roots.List()[0], ID: id, Title: title, Steps: []walkthrough.Step{{Title: "s"}}}), nil)
	if response.Code != http.StatusCreated && response.Code != http.StatusOK {
		t.Fatalf("publish %s = %d %s", id, response.Code, response.Body.String())
	}
}

func TestPublishAnnouncesToEveryNoticeSubscriber(t *testing.T) {
	app, _ := walkthroughApp(t, t.TempDir())
	t.Cleanup(app.Close)
	first, cancelFirst := app.WalkthroughNotices()
	defer cancelFirst()
	second, cancelSecond := app.WalkthroughNotices()
	defer cancelSecond()

	publishTitled(t, app, "tour", "A tour")
	want := WalkthroughNotice{Project: app.roots.List()[0], ID: "tour", Title: "A tour", Created: true}
	for label, notices := range map[string]<-chan WalkthroughNotice{"first": first, "second": second} {
		if got := receiveNotice(t, notices); got != want {
			t.Fatalf("%s notice = %+v, want %+v", label, got, want)
		}
	}

	publishTitled(t, app, "tour", "A tour, again")
	if got := receiveNotice(t, first); got.Created || got.Title != "A tour, again" {
		t.Fatalf("republish notice = %+v, want replaced", got)
	}
}

func TestAnnounceWalkthroughReadsTheStore(t *testing.T) {
	root := t.TempDir()
	app, _ := walkthroughApp(t, root)
	t.Cleanup(app.Close)
	publishTitled(t, app, "tour", "A tour")
	notices, cancel := app.WalkthroughNotices()
	defer cancel()

	notice, err := app.AnnounceWalkthrough(app.roots.List()[0], "tour", false)
	want := WalkthroughNotice{Project: app.roots.List()[0], ID: "tour", Title: "A tour"}
	if err != nil || notice != want {
		t.Fatalf("AnnounceWalkthrough() = %+v, %v; want %+v", notice, err, want)
	}
	if got := receiveNotice(t, notices); got != want {
		t.Fatalf("notice = %+v, want %+v", got, want)
	}

	for _, project := range []string{"", filepath.Join(t.TempDir(), "elsewhere"), filepath.Join(root, "missing")} {
		if _, err := app.AnnounceWalkthrough(project, "tour", true); !errors.Is(err, ErrProjectUnavailable) {
			t.Fatalf("AnnounceWalkthrough(%q) error = %v, want ErrProjectUnavailable", project, err)
		}
	}
	if _, err := app.AnnounceWalkthrough(app.roots.List()[0], "unknown", true); !errors.Is(err, walkthrough.ErrNotFound) {
		t.Fatalf("AnnounceWalkthrough(unknown) error = %v, want walkthrough.ErrNotFound", err)
	}
	select {
	case notice := <-notices:
		t.Fatalf("failed announcement sent %+v", notice)
	default:
	}
}

func TestNoticesDropTheOldestPastSixteen(t *testing.T) {
	app, _ := walkthroughApp(t, t.TempDir())
	t.Cleanup(app.Close)
	notices, cancel := app.WalkthroughNotices()
	defer cancel()

	for i := range 18 {
		publishTitled(t, app, fmt.Sprintf("w%d", i), fmt.Sprintf("W%d", i))
	}
	for i := 2; i < 18; i++ {
		if got := receiveNotice(t, notices); got.ID != fmt.Sprintf("w%d", i) {
			t.Fatalf("notice = %+v, want w%d", got, i)
		}
	}
	select {
	case notice := <-notices:
		t.Fatalf("extra notice %+v", notice)
	default:
	}
}

func TestNoticesCloseOnCancelAndWhenLiveUpdatesStop(t *testing.T) {
	app, _ := walkthroughApp(t, t.TempDir())
	t.Cleanup(app.Close)
	cancelled, cancel := app.WalkthroughNotices()
	open, cancelOpen := app.WalkthroughNotices()
	defer cancelOpen()

	cancel()
	cancel()
	assertClosed(t, "cancelled", cancelled)
	publishTitled(t, app, "tour", "A tour")
	if got := receiveNotice(t, open); got.ID != "tour" {
		t.Fatalf("notice = %+v", got)
	}

	app.StopLiveUpdates()
	assertClosed(t, "after StopLiveUpdates", open)
	cancelOpen()
	late, cancelLate := app.WalkthroughNotices()
	defer cancelLate()
	assertClosed(t, "subscribed after stop", late)
	publishTitled(t, app, "after", "After stop")
}
