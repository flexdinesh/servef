package app

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/servef/internal/document"
	"github.com/flexdinesh/servef/internal/projects"
	"github.com/flexdinesh/servef/pkg/contract"
)

// walkthroughsApp serves an empty project with a private walkthrough store.
func walkthroughsApp(t *testing.T) (*Walkthroughs, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	roots, err := projects.NewRootSet([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	application, err := New(Config{Roots: roots, Depth: 5, Types: document.Builtins(), WalkthroughDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return application.Walkthroughs, root
}

func publishTitled(t *testing.T, walkthroughs *Walkthroughs, root, id, title string) contract.WalkthroughPublished {
	t.Helper()
	published, err := walkthroughs.Publish(context.Background(), contract.WalkthroughDraft{Project: root, ID: id, Title: title, Steps: []contract.WalkthroughDraftStep{{Title: "s"}}})
	if err != nil {
		t.Fatalf("Publish(%s) = %v", id, err)
	}
	return published
}

func receiveNotice(t *testing.T, notices <-chan contract.WalkthroughEvent) contract.WalkthroughEvent {
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
	return contract.WalkthroughEvent{}
}

func assertClosed(t *testing.T, label string, notices <-chan contract.WalkthroughEvent) {
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

func codeOf(err error) string {
	if err == nil {
		return ""
	}
	return AsError(err).Code
}

func TestPublishAnnouncesToEveryNoticeSubscriber(t *testing.T) {
	walkthroughs, root := walkthroughsApp(t)
	first, cancelFirst := walkthroughs.Notices()
	defer cancelFirst()
	second, cancelSecond := walkthroughs.Notices()
	defer cancelSecond()

	published := publishTitled(t, walkthroughs, root, "tour", "A tour")
	viewURL := "/view?" + url.Values{"project": {root}, "wt": {"tour"}}.Encode()
	if want := viewURL; published.ID != "tour" || !published.Created || published.URL != want {
		t.Errorf("published = %+v, want URL %s", published, want)
	}
	want := contract.WalkthroughEvent{Project: root, ID: "tour", Title: "A tour", Created: true}
	for label, notices := range map[string]<-chan contract.WalkthroughEvent{"first": first, "second": second} {
		if got := receiveNotice(t, notices); got != want {
			t.Fatalf("%s notice = %+v, want %+v", label, got, want)
		}
	}

	publishTitled(t, walkthroughs, root, "tour", "A tour, again")
	if got := receiveNotice(t, first); got.Created || got.Title != "A tour, again" {
		t.Fatalf("republish notice = %+v, want replaced", got)
	}
}

func TestAnnounceReadsTheStore(t *testing.T) {
	walkthroughs, root := walkthroughsApp(t)
	publishTitled(t, walkthroughs, root, "tour", "A tour")
	notices, cancel := walkthroughs.Notices()
	defer cancel()

	notice, err := walkthroughs.Announce(root, "tour", false)
	want := contract.WalkthroughEvent{Project: root, ID: "tour", Title: "A tour"}
	if err != nil || notice != want {
		t.Fatalf("Announce() = %+v, %v; want %+v", notice, err, want)
	}
	if got := receiveNotice(t, notices); got != want {
		t.Fatalf("notice = %+v, want %+v", got, want)
	}

	for _, project := range []string{"", filepath.Join(t.TempDir(), "elsewhere"), filepath.Join(root, "missing")} {
		if _, err := walkthroughs.Announce(project, "tour", true); codeOf(err) != contract.CodeProjectUnavailable {
			t.Fatalf("Announce(%q) error = %v, want %s", project, err, contract.CodeProjectUnavailable)
		}
	}
	if _, err := walkthroughs.Announce(root, "unknown", true); codeOf(err) != contract.CodeWalkthroughUnavailable {
		t.Fatalf("Announce(unknown) error = %v, want %s", err, contract.CodeWalkthroughUnavailable)
	}
	select {
	case notice := <-notices:
		t.Fatalf("failed announcement sent %+v", notice)
	default:
	}
}

func TestNoticesDropTheOldestPastSixteen(t *testing.T) {
	walkthroughs, root := walkthroughsApp(t)
	notices, cancel := walkthroughs.Notices()
	defer cancel()

	for i := range 18 {
		publishTitled(t, walkthroughs, root, fmt.Sprintf("w%d", i), fmt.Sprintf("W%d", i))
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

func TestNoticesCloseOnCancelAndCloseNotices(t *testing.T) {
	walkthroughs, root := walkthroughsApp(t)
	cancelled, cancel := walkthroughs.Notices()
	open, cancelOpen := walkthroughs.Notices()
	defer cancelOpen()

	cancel()
	cancel()
	assertClosed(t, "cancelled", cancelled)
	publishTitled(t, walkthroughs, root, "tour", "A tour")
	if got := receiveNotice(t, open); got.ID != "tour" {
		t.Fatalf("notice = %+v", got)
	}

	walkthroughs.CloseNotices()
	assertClosed(t, "after CloseNotices", open)
	cancelOpen()
	late, cancelLate := walkthroughs.Notices()
	defer cancelLate()
	assertClosed(t, "subscribed after close", late)
	publishTitled(t, walkthroughs, root, "after", "After close")
}

func TestWalkthroughFailuresAreTypedWithoutTransport(t *testing.T) {
	walkthroughs, root := walkthroughsApp(t)
	ctx := context.Background()
	draft := func(lines []int) contract.WalkthroughDraft {
		return contract.WalkthroughDraft{Project: root, Title: "T", Steps: []contract.WalkthroughDraftStep{{Title: "s", File: &contract.WalkthroughFileTarget{Path: "missing.md", Lines: lines}}}}
	}
	failures := []struct {
		name string
		err  error
		kind contract.ErrorKind
		code string
	}{
		{"lines not a pair", second(walkthroughs.Publish(ctx, draft([]int{1, 2, 3}))), contract.KindInvalid, contract.CodeInvalidWalkthrough},
		{"missing target", second(walkthroughs.Publish(ctx, draft([]int{1, 1}))), contract.KindInvalid, contract.CodeInvalidWalkthrough},
		{"no steps", second(walkthroughs.Publish(ctx, contract.WalkthroughDraft{Project: root, Title: "T"})), contract.KindInvalid, contract.CodeInvalidWalkthrough},
		{"get missing", second(walkthroughs.Get(root, "gone")), contract.KindNotFound, contract.CodeWalkthroughUnavailable},
		{"delete missing", walkthroughs.Delete(root, "gone"), contract.KindNotFound, contract.CodeWalkthroughUnavailable},
		{"list without a project", second(walkthroughs.List("")), contract.KindInvalid, ""},
		{"get outside the roots", second(walkthroughs.Get(filepath.Dir(root), "gone")), contract.KindInvalid, ""},
	}
	for _, failure := range failures {
		if got := AsError(failure.err); failure.err == nil || got.Kind != failure.kind || got.Code != failure.code {
			t.Errorf("%s error = %v, want %s %q", failure.name, failure.err, failure.kind, failure.code)
		}
	}
	if err := second(walkthroughs.Publish(ctx, draft([]int{1, 2, 3}))); !strings.Contains(err.Error(), "lines must be [start, end], got 3 numbers") {
		t.Errorf("lines error = %v", err)
	}
	if listed, err := walkthroughs.List(root); err != nil || listed == nil || len(listed) != 0 {
		t.Errorf("List() = %#v, %v; want an empty list", listed, err)
	}
}

func second[T any](_ T, err error) error { return err }
