package notifier

import (
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/srcz/pkg/contract"
)

const (
	hashA = "aaaaaaa0000000000000000000000000000000aa"
	hashB = "bbbbbbb0000000000000000000000000000000bb"
	hashC = "ccccccc0000000000000000000000000000000cc"
)

func TestHeadMessages(t *testing.T) {
	main := &contract.HeadRef{Branch: "main", Commit: hashA}
	tests := []struct {
		name   string
		last   *contract.HeadRef
		next   contract.HeadEvent
		kind   Kind
		title  string
		detail string
		silent bool
	}{
		{name: "the baseline is silent", next: contract.HeadEvent{Branch: "main", Commit: hashA}, silent: true},
		{name: "an unmoved head is silent", last: main, next: contract.HeadEvent{Branch: "main", Commit: hashA}, silent: true},
		{name: "a commit on top of the last head is new", last: main, next: contract.HeadEvent{Branch: "main", Commit: hashB, Parent: hashA, Subject: "Fix the parser"}, kind: KindCommit, title: "New commit on main", detail: "bbbbbbb Fix the parser"},
		{name: "a reset or several commits only moved", last: main, next: contract.HeadEvent{Branch: "main", Commit: hashB, Parent: hashC, Subject: "Later"}, kind: KindCommit, title: "main moved", detail: "bbbbbbb Later"},
		{name: "a server too old to send the parent only moved", last: main, next: contract.HeadEvent{Branch: "main", Commit: hashB, Subject: "Later"}, kind: KindCommit, title: "main moved", detail: "bbbbbbb Later"},
		{name: "a new branch is a switch", last: main, next: contract.HeadEvent{Branch: "feature", Commit: hashA, Subject: "Same"}, kind: KindBranch, title: "Switched to feature"},
		{name: "detaching is a switch", last: main, next: contract.HeadEvent{Commit: hashA}, kind: KindBranch, title: "Detached at aaaaaaa"},
		{name: "a commit while detached is new", last: &contract.HeadRef{Commit: hashA}, next: contract.HeadEvent{Commit: hashB, Parent: hashA, Subject: "Try"}, kind: KindCommit, title: "New commit", detail: "bbbbbbb Try"},
		{name: "a branch losing its commit is silent", last: main, next: contract.HeadEvent{Branch: "main"}, silent: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := headMessage(test.last, test.next)
			if ok == test.silent {
				t.Fatalf("headMessage = %+v, %v; want silent %v", got, ok, test.silent)
			}
			if !test.silent && (got.kind != test.kind || got.title != test.title || got.detail != test.detail) {
				t.Errorf("headMessage = %+v, want %s %q %q", got, test.kind, test.title, test.detail)
			}
		})
	}
}

func TestRepositoryEdits(t *testing.T) {
	dirty := changeSet{"a.md": contract.ChangeStatus("modified")}
	more := changeSet{"a.md": contract.ChangeStatus("modified"), "b.go": contract.ChangeStatus("added"), "c.go": contract.ChangeStatus("added"), "d.go": contract.ChangeStatus("added")}
	if got, ok := repositoryEditsMessage(dirty, dirty); ok {
		t.Errorf("an unchanged set, as after a build writing ignored files = %+v", got)
	}
	if got, ok := repositoryEditsMessage(dirty, changeSet{}); ok {
		t.Errorf("a clean tree, as after a commit = %+v", got)
	}
	got, ok := repositoryEditsMessage(dirty, more)
	if !ok || got.title != "3 files changed" || got.detail != "b.go, c.go, +1" {
		t.Errorf("three more files = %+v, %v", got, ok)
	}
	got, ok = repositoryEditsMessage(changeSet{}, dirty)
	if !ok || got.title != "1 file changed" || got.detail != "a.md" || got.query.Get("review") != "working" {
		t.Errorf("one file = %+v, %v", got, ok)
	}
}

func TestFolderEdits(t *testing.T) {
	if got, ok := folderEditsMessage(nil, false); ok {
		t.Errorf("no paths = %+v", got)
	}
	if got, ok := folderEditsMessage([]string{"docs/a.md", "b.md"}, false); !ok || got.title != "2 files changed" || got.detail != "a.md, b.md" {
		t.Errorf("two paths = %+v, %v", got, ok)
	}
	if got, ok := folderEditsMessage(nil, true); !ok || got.title != "Many files changed" {
		t.Errorf("overflow = %+v, %v", got, ok)
	}
}

func TestWalkthroughMessages(t *testing.T) {
	list := []contract.WalkthroughSummary{
		{ID: "old", Title: "Old", UpdatedAt: "2026-01-01T00:00:00Z"},
		{ID: "new", Title: "Auth flow", UpdatedAt: "2026-02-01T00:00:00Z"},
	}
	if got, ok := walkthroughMessage(nil, list); ok {
		t.Errorf("the baseline = %+v", got)
	}
	if got, ok := walkthroughMessage(walkthroughTimes(list), list); ok {
		t.Errorf("nothing new = %+v", got)
	}
	got, ok := walkthroughMessage(map[string]string{"old": "2026-01-01T00:00:00Z"}, list)
	if !ok || got.title != "New walkthrough" || got.detail != "Auth flow" || got.query.Get("wt") != "new" {
		t.Errorf("a new one = %+v, %v", got, ok)
	}
	got, ok = walkthroughMessage(map[string]string{"old": "2025-01-01T00:00:00Z", "new": "2026-02-01T00:00:00Z"}, list)
	if !ok || got.title != "Walkthrough updated" || got.detail != "Old" {
		t.Errorf("an updated one = %+v, %v", got, ok)
	}
	// One notice covers what a reconnect finds: the newest.
	got, ok = walkthroughMessage(map[string]string{}, list)
	if !ok || got.detail != "Auth flow" {
		t.Errorf("two new ones = %+v, %v", got, ok)
	}
}

func TestAddressedNamesItsConnectionAndProject(t *testing.T) {
	m := message{kind: KindCommit, title: "New commit on main", detail: "bbbbbbb Fix", route: "/view"}
	local := addressed(m, "local", "", "/code/srcz")
	if local.Body != "srcz · bbbbbbb Fix" || local.Route != "/view?project=%2Fcode%2Fsrcz" {
		t.Errorf("local = %+v", local)
	}
	remote := addressed(m, "r-0123456789ab", "devbox", "/code/srcz")
	if remote.Body != "devbox · srcz · bbbbbbb Fix" {
		t.Errorf("remote = %+v", remote)
	}
	if local.ID == remote.ID || !strings.HasPrefix(remote.ID, "r-0123456789ab/") || strings.Contains(remote.ID, "srcz") {
		t.Errorf("IDs: local %q, remote %q", local.ID, remote.ID)
	}
	if again := addressed(message{kind: KindCommit, title: "main moved"}, "local", "", "/code/srcz"); again.ID != local.ID {
		t.Errorf("a newer notice must replace the one shown: %q, %q", again.ID, local.ID)
	}
	if other := addressed(message{kind: KindBranch}, "local", "", "/code/srcz"); other.ID == local.ID {
		t.Errorf("another kind must not replace it: %q", other.ID)
	}
}

func TestLimiter(t *testing.T) {
	var limit limiter
	start := time.Unix(1_000_000, 0)
	if !limit.allow(start, "/a", KindCommit) {
		t.Fatal("the first notice was refused")
	}
	if limit.allow(start.Add(29*time.Second), "/a", KindCommit) {
		t.Error("a second notice of a kind within 30 s was allowed")
	}
	if !limit.allow(start.Add(29*time.Second), "/a", KindBranch) {
		t.Error("another kind was refused")
	}
	if !limit.allow(start.Add(30*time.Second), "/a", KindCommit) {
		t.Error("the same kind after 30 s was refused")
	}

	// A connection gets ten a minute, however many projects it names.
	var flood limiter
	allowed := 0
	for i := range 40 {
		if flood.allow(start, "/project"+string(rune('a'+i)), KindCommit) {
			allowed++
		}
	}
	if allowed != perMinute {
		t.Errorf("allowed %d notices at once, want %d", allowed, perMinute)
	}
	if !flood.allow(start.Add(time.Minute), "/another", KindCommit) {
		t.Error("the connection's limit did not recover after a minute")
	}
}

func TestCleanKeepsNoticesPlain(t *testing.T) {
	for in, want := range map[string]string{
		"Fix the parser":             "Fix the parser",
		"two\nlines\ttabbed":         "twolinestabbed",
		"safe‮gnp.exe":               "safegnp.exe",
		"invalid\xffbyte":            "invalidbyte",
		"  padded  ":                 "padded",
		strings.Repeat("x", 300):     strings.Repeat("x", maxText) + "…",
		strings.Repeat("é", maxText): strings.Repeat("é", maxText),
	} {
		if got := clean(in, maxText); got != want {
			t.Errorf("clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBodyForEscapesMarkupOnLinuxOnly(t *testing.T) {
	body := `srcz · a1b2c3d <img src="x"> & <b>bold</b>`
	if got, want := BodyFor("linux", body), `srcz · a1b2c3d &lt;img src="x"&gt; &amp; &lt;b&gt;bold&lt;/b&gt;`; got != want {
		t.Errorf("linux = %q, want %q", got, want)
	}
	if got := BodyFor("darwin", body); got != body {
		t.Errorf("darwin = %q", got)
	}
}
