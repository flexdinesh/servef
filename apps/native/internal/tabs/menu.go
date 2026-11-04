package tabs

// Action is a tab menu item, as the web's TabMenuAction.
type Action string

const (
	CloseTab    Action = "close"
	CloseOthers Action = "close-others"
	CloseRight  Action = "close-right"
	CloseAll    Action = "close-all"
	Reveal      Action = "reveal"
)

// RevealLabel names Reveal.
const RevealLabel = "Reveal in File Manager"

// MenuItem is an item of the tab menu; Separator items have no action.
type MenuItem struct {
	Action    Action
	Label     string
	Enabled   bool
	Separator bool
}

// RevealablePath is a document tab's path; other tabs have nothing on disk
// to reveal.
func RevealablePath(tab Tab) (string, bool) {
	return tab.Path, tab.Kind == Document
}

// MenuItems mirror the web's tab menu: Close, Close Others, Close to the
// Right, Close All, and Reveal when the app offers it (canReveal) and the
// tab has a file on disk.
func MenuItems(tab Tab, index, count int, canReveal bool) []MenuItem {
	items := []MenuItem{
		{Action: CloseTab, Label: "Close", Enabled: true},
		{Action: CloseOthers, Label: "Close Others", Enabled: count > 1},
		{Action: CloseRight, Label: "Close to the Right", Enabled: index < count-1},
		{Separator: true},
		{Action: CloseAll, Label: "Close All", Enabled: true},
	}
	if _, ok := RevealablePath(tab); canReveal && ok {
		items = append(items, MenuItem{Separator: true}, MenuItem{Action: Reveal, Label: RevealLabel, Enabled: true})
	}
	return items
}

// Closing lists the tabs a close action on tabs[index] closes; nil for
// other actions.
func Closing(action Action, tabs []Tab, index int) []Tab {
	if index < 0 || index >= len(tabs) {
		return nil
	}
	switch action {
	case CloseTab:
		return []Tab{tabs[index]}
	case CloseOthers:
		others := make([]Tab, 0, len(tabs)-1)
		others = append(others, tabs[:index]...)
		return append(others, tabs[index+1:]...)
	case CloseRight:
		return append([]Tab(nil), tabs[index+1:]...)
	case CloseAll:
		return append([]Tab(nil), tabs...)
	}
	return nil
}
