package tabs

import (
	"encoding/json"
	"slices"
	"testing"
)

// As workspace-tabs.test.ts: documents and reviews are distinct tabs, one
// review per compare, Welcome one tab.
func TestKeysAreUniqueAcrossKinds(t *testing.T) {
	all := []Tab{DocumentTab("working"), DocumentTab("welcome"), Uncommitted(), WelcomeTab()}
	keys := map[string]bool{}
	for _, tab := range all {
		keys[tab.Key()] = true
	}
	if len(keys) != len(all) {
		t.Fatalf("keys %v", keys)
	}
	if DocumentTab("a.md") != DocumentTab("a.md") || Uncommitted() != Uncommitted() || DocumentTab("a.md") == Uncommitted() {
		t.Fatal("tab equality")
	}
	if Includes([]Tab{DocumentTab("a.md")}, Tab{}) || !Includes([]Tab{Uncommitted()}, Uncommitted()) {
		t.Fatal("includes")
	}
}

func TestNames(t *testing.T) {
	for tab, want := range map[Tab]Name{
		DocumentTab("web/src/App.tsx"): {Label: "App.tsx", Title: "web/src/App.tsx"},
		DocumentTab("README.md"):       {Label: "README.md", Title: "README.md"},
		Uncommitted():                  {Label: "Uncommitted", Title: "Uncommitted changes"},
		WelcomeTab():                   {Label: "Welcome", Title: "Welcome"},
	} {
		if got := tab.Name(); got != want {
			t.Errorf("%v: %+v, want %+v", tab, got, want)
		}
	}
}

// Stored tabs round-trip in the web's StoredTab shapes; malformed and
// unsupported entries parse as none.
func TestStoredTabsRoundTrip(t *testing.T) {
	for tab, want := range map[Tab]string{
		DocumentTab("a.md"): `"a.md"`,
		Uncommitted():       `{"review":"working"}`,
		WelcomeTab():        `{"welcome":true}`,
		{}:                  `""`,
	} {
		raw, err := json.Marshal(tab)
		if err != nil || string(raw) != want {
			t.Fatalf("%v: %s %v, want %s", tab, raw, err, want)
		}
		parsed, ok := Parse(raw)
		if ok != !tab.None() || parsed != tab && !tab.None() {
			t.Fatalf("%s: %v %v", raw, parsed, ok)
		}
	}
	for _, malformed := range []string{`""`, `3`, `null`, `["a.md"]`, `{"welcome":false}`, `{"welcome":"yes"}`,
		`{"review":"commit","commit":"x"}`, `{"review":3}`, `{"path":"a.md","at":"3577ec6"}`, `{"walkthrough":"tour"}`, `{"diff":"a.md"}`} {
		if tab, ok := Parse(json.RawMessage(malformed)); ok {
			t.Errorf("%s parsed as %v", malformed, tab)
		}
	}
}

// As use-document-workspace.ts: a closed active tab gives way to the next
// tab to its right, else to its left.
func TestCloseChoosesAReplacement(t *testing.T) {
	a, b, c, r := DocumentTab("a"), DocumentTab("b"), DocumentTab("c"), Uncommitted()
	open := []Tab{a, b, c, r}
	cases := []struct {
		closing     []Tab
		active      Tab
		tabs        []Tab
		replacement Tab
	}{
		{[]Tab{b}, b, []Tab{a, c, r}, c},
		{[]Tab{r}, r, []Tab{a, b, c}, c},
		{[]Tab{b, c}, b, []Tab{a, r}, r},
		{[]Tab{c, r}, c, []Tab{a, b}, b},
		{open, a, nil, Tab{}},
		// Closing another tab keeps the active one; the replacement is only
		// used when the active tab closes.
		{[]Tab{a}, c, []Tab{b, c, r}, r},
		{[]Tab{a}, Tab{}, []Tab{b, c, r}, b},
	}
	for _, tc := range cases {
		closed, ok := Close(open, tc.closing, tc.active)
		if !ok || !slices.Equal(closed.Tabs, tc.tabs) || closed.Replacement != tc.replacement || len(closed.Closed) != len(tc.closing) {
			t.Errorf("close %v from %v: %+v %v", tc.closing, tc.active, closed, ok)
		}
	}
	if _, ok := Close(open, []Tab{DocumentTab("gone")}, a); ok {
		t.Error("closing a tab that is not open did something")
	}
}

func TestMergePutsRestoredTabsFirst(t *testing.T) {
	got := Merge([]Tab{DocumentTab("a"), Uncommitted()}, []Tab{DocumentTab("b"), DocumentTab("a")})
	if want := []Tab{DocumentTab("a"), Uncommitted(), DocumentTab("b")}; !slices.Equal(got, want) {
		t.Fatalf("%v, want %v", got, want)
	}
}

// As tab-menu.test.ts: the menu's enabled states, and Reveal only when
// offered and for documents.
func TestMenuItems(t *testing.T) {
	ids := func(items []MenuItem) []string {
		var out []string
		for _, item := range items {
			switch {
			case item.Separator:
				out = append(out, "-")
			case !item.Enabled:
				out = append(out, string(item.Action)+" (disabled)")
			default:
				out = append(out, string(item.Action))
			}
		}
		return out
	}
	tab := DocumentTab("README.md")
	for _, tc := range []struct {
		index, count int
		want         []string
	}{
		{0, 1, []string{"close", "close-others (disabled)", "close-right (disabled)", "-", "close-all"}},
		{0, 3, []string{"close", "close-others", "close-right", "-", "close-all"}},
		{2, 3, []string{"close", "close-others", "close-right (disabled)", "-", "close-all"}},
	} {
		if got := ids(MenuItems(tab, tc.index, tc.count, false)); !slices.Equal(got, tc.want) {
			t.Errorf("%d of %d: %v", tc.index, tc.count, got)
		}
	}
	if got := ids(MenuItems(DocumentTab("src/a.ts"), 0, 1, true)); !slices.Equal(got[len(got)-2:], []string{"-", "reveal"}) {
		t.Errorf("reveal: %v", got)
	}
	for _, other := range []Tab{WelcomeTab(), Uncommitted()} {
		if slices.Contains(ids(MenuItems(other, 0, 1, true)), "reveal") {
			t.Errorf("%v offers Reveal", other)
		}
	}
	if p, ok := RevealablePath(DocumentTab("src/a.ts")); !ok || p != "src/a.ts" {
		t.Error("revealable document")
	}
}

func TestClosingActions(t *testing.T) {
	a, b, c := DocumentTab("a"), DocumentTab("b"), DocumentTab("c")
	open := []Tab{a, b, c}
	for action, want := range map[Action][]Tab{
		CloseTab: {b}, CloseOthers: {a, c}, CloseRight: {c}, CloseAll: {a, b, c}, Reveal: nil,
	} {
		if got := Closing(action, open, 1); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", action, got, want)
		}
	}
	if got := Closing(CloseRight, open, 2); len(got) != 0 {
		t.Errorf("close right of the last: %v", got)
	}
	if Closing(CloseTab, open, 3) != nil {
		t.Error("out of range")
	}
	// Closing returns new slices: closing never reorders the open tabs.
	_ = append(Closing(CloseOthers, open, 0), a)
	if !slices.Equal(open, []Tab{a, b, c}) {
		t.Errorf("open tabs changed: %v", open)
	}
}
