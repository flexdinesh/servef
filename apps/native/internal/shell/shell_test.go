package shell

import (
	"testing"

	"github.com/flexdinesh/servef/apps/native/internal/memory"
)

// As use-sidebar.test.ts: stored views include Source Control and fall
// back to the Explorer; widths clamp as use-sidebar-resize.ts.
func TestSidebarFromMemory(t *testing.T) {
	for in, want := range map[memory.Sidebar]Sidebar{
		{View: "source-control"}:           {View: SourceControl, Width: DefaultWidth},
		{View: "search", Hidden: true}:     {View: Search, Hidden: true, Width: DefaultWidth},
		{View: "timeline", Hidden: true}:   {View: Explorer, Hidden: true, Width: DefaultWidth},
		{}:                                 {View: Explorer, Width: DefaultWidth},
		{View: "walkthroughs", Width: 1e4}: {View: Walkthroughs, Width: MaxWidth},
		{View: "explorer", Width: 10}:      {View: Explorer, Width: MinWidth},
		{View: "explorer", Width: 300}:     {View: Explorer, Width: 300},
	} {
		if got := FromMemory(in); got != want {
			t.Errorf("%+v: %+v, want %+v", in, got, want)
		}
		if got := FromMemory(want.Memory()); got != want {
			t.Errorf("round trip %+v: %+v", want, got)
		}
	}
}

// Clicking the shown view hides the sidebar, as in VS Code; another view
// or a hidden sidebar shows it.
func TestSelectTogglesTheShownView(t *testing.T) {
	s := Sidebar{View: Explorer, Width: DefaultWidth}
	if s = s.Select(Explorer); !s.Hidden || s.Shown() != "" {
		t.Fatalf("%+v", s)
	}
	if s = s.Select(Explorer); s.Hidden || s.Shown() != Explorer {
		t.Fatalf("%+v", s)
	}
	if s = s.Select(SourceControl); s.Hidden || s.View != SourceControl {
		t.Fatalf("%+v", s)
	}
	if s = s.Toggle(); !s.Hidden {
		t.Fatalf("%+v", s)
	}
	if s = s.Reveal(SourceControl); s.Hidden || s.View != SourceControl {
		t.Fatalf("%+v", s)
	}
}

func TestBadges(t *testing.T) {
	for n, want := range map[int]string{0: "", -1: "", 1: "1", 99: "99", 100: "99+", 1200: "99+"} {
		if got := Badge(n); got != want {
			t.Errorf("%d: %q", n, got)
		}
	}
	activities := Activities(false)
	if got := BadgeLabel(activities[2], 1200); got != "Source Control, 1,200 pending changes" {
		t.Error(got)
	}
	if got := BadgeLabel(activities[2], 1); got != "Source Control, 1 pending change" {
		t.Error(got)
	}
	if got := BadgeLabel(activities[3], 2); got != "Walkthroughs, 2 new walkthroughs" {
		t.Error(got)
	}
	if got := BadgeLabel(activities[0], 3); got != "Explorer" {
		t.Error(got)
	}
	if Activities(true)[0].Shortcut != "⇧⌘E" || activities[0].Shortcut != "Ctrl+Shift+E" || activities[3].Shortcut != "" {
		t.Error("shortcuts")
	}
}

var base = EmptyInput{View: Explorer, Changes: -1, Repository: true, Walkthroughs: -1}

// The tests below mirror empty-workspace.test.ts (without phones).

func TestExplorerAsksForAFile(t *testing.T) {
	if got := EmptyWorkspace(base); got != (Empty{"Choose a file", "Select a file from the folder tree.", FindFile}) {
		t.Fatalf("%+v", got)
	}
}

func TestSearchAsksForAQueryThenAMatch(t *testing.T) {
	search := base
	search.View = Search
	if got := EmptyWorkspace(search); got != (Empty{"Search file contents", "Type in the Search field, then open a match.", ""}) {
		t.Fatalf("%+v", got)
	}
	search.SearchPattern = "needle"
	if EmptyWorkspace(search).Heading != "Search file contents" {
		t.Fatal("no results")
	}
	search.SearchPattern, search.SearchFiles = "  ", 2
	if EmptyWorkspace(search).Heading != "Search file contents" {
		t.Fatal("blank pattern")
	}
	search.SearchPattern = "needle"
	if got := EmptyWorkspace(search); got != (Empty{"Open a match", "Choose a result to open it at that line.", ""}) {
		t.Fatalf("%+v", got)
	}
}

func TestSourceControlOffersTheUncommittedReview(t *testing.T) {
	sc := base
	sc.View = SourceControl
	sc.Changes = 4
	if got := EmptyWorkspace(sc); got != (Empty{"Review your changes", "4 uncommitted changes. Open a file, a commit, or a branch.", ReviewUncommitted}) {
		t.Fatalf("%+v", got)
	}
	sc.Changes = 1
	if got := EmptyWorkspace(sc).Hint; got != "1 uncommitted change. Open a file, a commit, or a branch." {
		t.Fatal(got)
	}
	sc.Changes = 1200
	if got := EmptyWorkspace(sc).Hint; got != "1,200 uncommitted changes. Open a file, a commit, or a branch." {
		t.Fatal(got)
	}
	sc.Changes = 0
	if got := EmptyWorkspace(sc); got != (Empty{"Review history", "No uncommitted changes. Open a commit or a branch.", ""}) {
		t.Fatalf("%+v", got)
	}
	sc.Changes = -1
	if EmptyWorkspace(sc).Heading != "Review history" {
		t.Fatal("unread")
	}
	sc.Changes, sc.Repository = 0, false
	if got := EmptyWorkspace(sc); got != (Empty{"Not a git repository", "Source Control needs a git repository.", ""}) {
		t.Fatalf("%+v", got)
	}
}

func TestWalkthroughsAskForOne(t *testing.T) {
	w := base
	w.View = Walkthroughs
	if EmptyWorkspace(w).Heading != "Choose a walkthrough" {
		t.Fatal("unread")
	}
	w.Walkthroughs = 3
	if got := EmptyWorkspace(w); got != (Empty{"Choose a walkthrough", "Open one from the list to start.", ""}) {
		t.Fatalf("%+v", got)
	}
	w.Walkthroughs = 0
	if got := EmptyWorkspace(w); got != (Empty{"No walkthroughs yet", "Agents publish guided tours of this project here.", ""}) {
		t.Fatalf("%+v", got)
	}
}

func TestHiddenSidebarOffersToShowIt(t *testing.T) {
	hidden := base
	hidden.View = ""
	if got := EmptyWorkspace(hidden); got != (Empty{"Nothing open", "Open a file, a review, or a walkthrough.", ShowSidebar}) {
		t.Fatalf("%+v", got)
	}
	if label, shortcut := ActionLabel(ShowSidebar, false); label != "Show sidebar" || shortcut != "Ctrl+B" {
		t.Fatal(label, shortcut)
	}
	if _, shortcut := ActionLabel(FindFile, true); shortcut != "⌘K" {
		t.Fatal(shortcut)
	}
}

func TestCount(t *testing.T) {
	for n, want := range map[int]string{0: "0", 999: "999", 1000: "1,000", 1234567: "1,234,567", -1200: "-1,200"} {
		if got := Count(n); got != want {
			t.Errorf("%d: %s", n, got)
		}
	}
}
