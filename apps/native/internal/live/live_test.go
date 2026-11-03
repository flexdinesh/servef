package live

import (
	"testing"

	"github.com/flexdinesh/servef/pkg/contract"
)

func TestFor(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change contract.ChangeEvent
		doc    string
		want   Targets
	}{
		{"open document edited", contract.ChangeEvent{Paths: []string{"a.go"}}, "a.go", Targets{Page: true, Changes: true}},
		{"another file edited", contract.ChangeEvent{Paths: []string{"b.go"}}, "a.go", Targets{Changes: true}},
		{"tree changed", contract.ChangeEvent{Tree: true}, "", Targets{Page: true}},
		{"git only", contract.ChangeEvent{Paths: []string{}, Git: true}, "a.go", Targets{Changes: true}},
		{"overflow", Everything, "", Targets{Page: true, Changes: true}},
		{"no document", contract.ChangeEvent{Paths: []string{""}}, "", Targets{Changes: true}},
	} {
		if got := For(tc.change, tc.doc); got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestDiff(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change contract.ChangeEvent
		want   bool
	}{
		{"diffed file edited", contract.ChangeEvent{Paths: []string{"b.md"}}, true},
		{"rename source edited", contract.ChangeEvent{Paths: []string{"a.md"}}, true},
		{"another file edited", contract.ChangeEvent{Paths: []string{"c.md"}}, false},
		{"tree only", contract.ChangeEvent{Tree: true}, false},
		{"git", contract.ChangeEvent{Git: true}, true},
		{"overflow", Everything, true},
	} {
		if got := Diff(tc.change, "b.md", "a.md"); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
	if Diff(contract.ChangeEvent{Paths: []string{""}}, "b.md", "") {
		t.Error("an empty old path matched")
	}
}

func TestDescribe(t *testing.T) {
	if got := Describe(contract.StatusEvent{Mode: "watching"}); got.Label != "Live" {
		t.Fatalf("watching: %+v", got)
	}
	if got := Describe(contract.StatusEvent{Mode: "polling", Reason: "Too many directories."}); got.Label != "Polling" || got.Description != "Live updates: checking the project for changes every few seconds. Too many directories." {
		t.Fatalf("polling: %+v", got)
	}
	if got := Describe(contract.StatusEvent{Mode: "off"}); got.Label != "Off" {
		t.Fatalf("off: %+v", got)
	}
}
