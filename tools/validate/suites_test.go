//go:build linux || darwin

package main

import (
	"slices"
	"testing"
	"time"
)

func TestSuitesSelectIndependentGroups(t *testing.T) {
	correctness := []string{"repo", "web-static", "tooling-static", "site-static", "go", "frontend", "tooling", "docs", "browser", "desktop"}
	extended := []string{"browser-extended", "desktop-extended", "webkit"}
	for _, suite := range []string{"", "correctness", "extended", "full"} {
		want := correctness
		if suite == "extended" {
			want = extended
		} else if suite == "full" {
			want = append(slices.Clone(correctness), extended...)
		}
		if !knownSuite(suite) || !slices.Equal(chosen(options{Suite: suite}), want) {
			t.Fatalf("suite %q selected %v, want %v", suite, chosen(options{Suite: suite}), want)
		}
		for _, group := range append(slices.Clone(correctness), extended...) {
			if !knownGroup(group) || !slices.Equal(chosen(options{Suite: suite, Group: group}), []string{group}) {
				t.Fatalf("suite %q cannot target %s", suite, group)
			}
		}
	}
	if knownSuite("typo") || knownGroup("typo") {
		t.Fatal("unknown selection accepted")
	}
}

func TestSuiteCertificatesDoNotCoverOtherSuites(t *testing.T) {
	root := repository(t)
	put(t, root, "apps/web/source.ts", "source")
	commit(t, root, "initial")
	s := snap(t, root)
	a := app{Root: root, Cache: t.TempDir()}
	certify := func(suite string) {
		t.Helper()
		for _, group := range chosen(options{Suite: suite}) {
			if err := a.certify(receipt{Policy: policy, Key: key(t, s, group), Head: s.Head, Run: "pass", Finished: time.Now(), Complete: true}); err != nil {
				t.Fatal(err)
			}
		}
	}
	certify("extended")
	if err := a.status(options{Suite: "extended"}); err != nil {
		t.Fatal(err)
	}
	for _, suite := range []string{"", "correctness", "full"} {
		if err := a.status(options{Suite: suite}); err == nil {
			t.Fatalf("extended receipts covered %q", suite)
		}
	}
	certify("correctness")
	for _, suite := range []string{"", "correctness", "extended", "full"} {
		if err := a.status(options{Suite: suite}); err != nil {
			t.Fatalf("full receipts failed %q: %v", suite, err)
		}
	}
	correctnessOnly := app{Root: root, Cache: t.TempDir()}
	for _, group := range groups {
		if err := correctnessOnly.certify(receipt{Policy: policy, Key: key(t, s, group), Head: s.Head, Run: "pass", Finished: time.Now(), Complete: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := correctnessOnly.status(options{}); err != nil {
		t.Fatal(err)
	}
	for _, suite := range []string{"extended", "full"} {
		if err := correctnessOnly.status(options{Suite: suite}); err == nil {
			t.Fatalf("correctness receipts covered %s", suite)
		}
	}
}

func TestExtendedTargetedCertificatesCannotCoverGroups(t *testing.T) {
	root := repository(t)
	put(t, root, "apps/web/source.ts", "source")
	commit(t, root, "initial")
	s := snap(t, root)
	for task, group := range map[string]string{
		"_container:test:browser:extended": "browser-extended",
		"_container:test:desktop:extended": "desktop-extended",
		"_container:test:browser:webkit":   "webkit",
	} {
		t.Run(group, func(t *testing.T) {
			if !taskBelongsToGroup(task, group) || taskBelongsToGroup(task, "browser") || taskBelongsToGroup(task, "desktop") {
				t.Fatal("targeted task has incorrect ownership")
			}
			a := app{Root: root, Cache: t.TempDir()}
			for _, args := range [][]string{nil, {"filter"}} {
				_, targeted, err := manifest(s, group, task, args, nil)
				if err != nil {
					t.Fatal(err)
				}
				if targeted == key(t, s, group) {
					t.Fatal("targeted key matches full group")
				}
				if err := a.certify(receipt{Policy: policy, Key: targeted, Head: s.Head, Run: "targeted", Finished: time.Now(), Complete: false}); err != nil {
					t.Fatal(err)
				}
				if err := a.status(options{Group: group, Task: task, Args: args}); err != nil {
					t.Fatal(err)
				}
			}
			if err := a.status(options{Group: group}); err == nil {
				t.Fatal("targeted receipts covered full group")
			}
		})
	}
}

func TestConsumerInputInvalidation(t *testing.T) {
	all := chosen(options{Suite: "full"})
	cases := []struct {
		path   string
		groups []string
	}{
		{"apps/web/src/model.test.ts", []string{"web-static", "frontend"}},
		{"apps/web/src/View.test.tsx", []string{"web-static", "frontend"}},
		{"apps/srcz/internal/app/app_test.go", []string{"repo", "go", "desktop", "desktop-extended"}},
		{"apps/desktop/e2e/e2e_test.go", []string{"repo", "desktop", "desktop-extended"}},
		{"tools/validate/snapshot_test.go", []string{"repo", "go"}},
		{"tools/validate/snapshot.go", all},
		{"apps/web/test-fixtures/markdown-document.tsx", []string{"web-static", "go", "frontend", "browser", "desktop", "browser-extended", "desktop-extended", "webkit"}},
		{"tests/browser/extended/memory.spec.ts", []string{"tooling-static", "tooling", "browser", "browser-extended", "webkit"}},
		{"playwright.config.ts", []string{"tooling-static", "tooling", "browser", "browser-extended", "webkit"}},
		{"testdata/api/default/page.json", []string{"go", "frontend", "tooling", "browser", "browser-extended", "webkit"}},
		{"packages/contracts/src/index.ts", []string{"repo", "web-static", "go", "frontend", "browser", "desktop", "browser-extended", "desktop-extended", "webkit"}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			root := repository(t)
			put(t, root, tc.path, "before")
			commit(t, root, "initial")
			before := snap(t, root)
			put(t, root, tc.path, "after")
			commit(t, root, "change")
			after := snap(t, root)
			for _, group := range all {
				changed := key(t, before, group) != key(t, after, group)
				if changed != slices.Contains(tc.groups, group) {
					t.Fatalf("%s invalidates %s = %t", tc.path, group, changed)
				}
			}
		})
	}
}
